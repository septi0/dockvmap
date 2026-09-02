# PROJECT.md

dockvmap ("Docker Virtual Mapper") is a Docker/OCI registry proxy: clients pull through a stable "virtual tag" (`myimage:current`) transparently mapped to a real upstream tag tracked per-image. One binary runs a proxy server (OCI Distribution API), a web server (REST management API + embedded frontend), and a background worker, sharing one SQLite DB and one registry HTTP client.

## Commands

`make help` lists all targets. Common ones: `make build` (frontend + backend, → `bin/dockvmap`), `make dev` (backend `go run` + frontend Vite dev server together, Ctrl+C stops both; `CONFIG=<path>` runs against an alternative config file, otherwise `config/config.yaml` is used and seeded from `config.sample.yaml` on first run), `make test`, `make vet`, `make lint` (gofmt check + vet, + golangci-lint if installed), `make check` (frontend svelte-check + tsc), `make verify` (lint + test + check - what CI should run). Any target that needs the frontend rebuilds `frontend/dist` unconditionally (make can't see inside a directory to know whether it is stale, and a Vite build is a couple of seconds); only `frontend/node_modules` is cached, reinstalled via `npm ci` when `package.json`/`package-lock.json` change. The Makefile is the whole local toolchain; the only CI is `.github/workflows/docker-image.yml`, which on a `v*.*.*` tag builds the multi-arch image (amd64 + arm64) and pushes it to `ghcr.io/septi0/dockvmap` (`{{version}}` / `{{major}}.{{minor}}` / `latest`). To run the built binary directly (not via `go run`), just invoke it: `./bin/dockvmap -config data/config.yaml`.

Raw equivalents still work: `go build ./...`, `go vet ./...`, `go test ./...` (unit tests live in `internal/oci`, `internal/service`, `internal/taganalyzer` and `internal/web`; `internal/taganalyzer` is additionally verified out-of-band by `cmd/tagaudit -verify` against a real-tag corpus), `go run ./cmd/dockvmap -config data/config.yaml` (`-config` has no default filename; omit it entirely and the app runs on `DOCKVMAP_*` env vars/built-in defaults only). Frontend commands run from `frontend/`: `npm run dev`, `npm run check`, `npm run build` (→ `frontend/dist`, embedded via `embed.go`).

**Fresh-checkout build:** `frontend/dist` is a gitignored build artifact (like `bin/`), not committed. `frontend/embed.go`'s `//go:embed dist/*` needs some non-dot-prefixed file present under `dist/` at build time, so a plain `go build ./...` - and equally `go vet ./...` / `go test ./...` - fails on a fresh clone until the frontend has been built once. Every Makefile target that compiles Go (`build`, `test`, `vet`, `lint`, `verify`, `dev-backend`, `run`) depends on the dist stamp, so they all handle it in the right order; only the raw `go` commands need the manual `make build-frontend` first. (This is the canonical explanation; other mentions cross-reference here.)

## Process model

### Runtime shape

`main()` → `run() error`. `run()` is a dispatcher: it does shared bootstrap (config, logging, data dir, credential key), then either handles a one-off CLI command or calls `serve()`.

`serve()` is the full service composition root and lifecycle. It builds the store, the service layer, then three long-lived components off one shared `*http.Client` and one `*sql.DB`:

- **proxy server**: OCI Distribution API (`/v2/...`).
- **web server**: REST API under `/api/` + the embedded frontend.
- **background worker**: independent scheduled jobs (see [Background worker](#background-worker)).

### Layering

`internal/store` → `internal/service` → `internal/web` / `internal/proxy`: strict and one-directional, SQL → business logic → HTTP. `internal/oci` stays decoupled from `store`/`service` via small interfaces; `cmd/dockvmap/adapters.go` holds the adapters that bridge them.

### Startup and dispatch

`run()` order: bootstrap → `-backup` (before `store.New`, since it opens the DB read-only and must snapshot the schema as-found) → a `switch` over the remaining one-off commands vs `serve()`. Startup failures `return` rather than `os.Exit`-ing past deferred cleanup.

**One-off CLI commands** (`cmd/dockvmap/commands.go`) each build only the minimal service subset they need and exit without starting any server:

- `-reset-password <username>` runs `Users.ResetPassword(ctx, username)`: look the user up by username, generate a random password, invalidate all their sessions, print it. Takes a username (not "the one user") so it stays correct if multi-user is ever added.
- `-refresh-tags` runs `Images.RefreshAll(ctx)`, the same one-shot sweep the worker runs on its ticker, exposed for scripting an out-of-band refresh. Partial failures don't abort the batch; joined per-image errors are returned (non-zero exit) after the refreshed count is printed.
- `-backup <path>`: opens the DB via `store.OpenForBackup` (no migrations; the newer-schema guard still applies), calls `Store.Backup` (`VACUUM INTO`, consistent while running, refuses if the target exists), and prints what else a restore needs (the credential key file/value, and `config.yaml`). No `-restore`; that's a documented stop/replace/start procedure.

### Shutdown

Listener failures go through a `serverErrs` channel into the same graceful-shutdown path as a SIGTERM, never a direct `os.Exit`. On shutdown, `awaitShutdown` cancels the worker context and waits for the worker goroutines, but only up to `workerShutdownTimeout` (10s): a hung job logs a warning and shutdown proceeds rather than blocking until the runtime SIGKILLs.

## Architecture

Package catalog. Deep dives for `taganalyzer`, image tag refresh, and tag discovery are in [Design Details](#design-details); the worker is in [Background worker](#background-worker); frontend is in [Frontend](#frontend).

- **`cmd/dockvmap`**: `main.go` (entry + flag parsing + shared bootstrap + dispatch), `serve.go` (`serve()`), `commands.go` (the one-off CLI commands), `server.go` (HTTP server construction, TLS, graceful shutdown), `worker.go` (background worker), `bootstrap.go` (`initBlobCache`/`initMailer`/`initNotifications`/`resolveCredentialEncryptionKey`), `adapters.go` (adapters keeping `internal/oci` decoupled from `store`/`service`). Lifecycle details in [Process model](#process-model).

- **`internal/config`**: `Load()` merges env > file > `default:` tag (env/defaults applied by reflection over struct tags). The file is decoded with `KnownFields(true)`, so an unknown or misspelled key is a startup error, not a silent no-op (env vars stay permissive). `applyDerivedDefaults()` normalizes interdependent settings (e.g. `tls.enabled` with a blank cert/key path flips off; `secure_cookies` defaults on when `tls.enabled` or `trusted_proxies` is set) and appends a human message to `Config.DerivedWarnings` for each adjustment, plus one when `trusted_proxies` is empty (client IP will be the immediate TCP peer). `main()` drains and `slog.Warn`s the warnings once logging is configured. The web server binds `web_server_https_listen` when `tls.enabled`, else `web_server_http_listen`; the proxy always binds `proxy_server_listen`, switching only its scheme.

- **`internal/store`**: SQL layer. `store.New` = `open` + `migrate`; `store.OpenForBackup` = `open` + `verifySchemaVersion` only. `open` also `chmod`s the DB file to `0o600` (it holds password hashes, session tokens, encrypted creds); `main()` creates the data dir `0o700`.

- **`internal/store/migrations`**: numbered, checksum-verified schema migrations. `verifySchemaVersion` (run by both `migrate()` and `store.OpenForBackup`) refuses to proceed when `MAX(schema_migrations.version)` exceeds the highest bundled migration, so an older binary won't silently run against, or back up, a schema a newer build created. Migrations are forward-only and safe on a populated DB.

- **`internal/service`**: business logic. Multi-dependency constructors take a config struct, not positional args: `NewImages(ImagesDeps{…})`, `NewDiscoveries(DiscoveriesDeps{…})`.

- **`internal/proxy`**: OCI Distribution API (`/v2/...`); resolves virtual tag → real registry/repo/tag, forwards to upstream, optionally through `internal/blobcache`. Basic-Auth gate on every request when `proxy_auth.enabled` (see `proxy_tokens.go`).

- **`internal/web`**: REST API + embedded frontend (`handler.go`). `GET /health` is intentionally outside `/api/`'s auth middleware; it runs a real `SELECT 1` (`Store.Ping`, not the driver's connection-only `PingContext`) and returns `{status, version}` so deploy checks can read the running version without authenticating. The embedded frontend is served with `Cache-Control: immutable` for content-hashed `/assets/*` and `no-cache` for `index.html` / the SPA fallback, so a deploy can't leave a stale index pointing at gone asset hashes. `GET /api/events`, `/api/recent-failures` and `/api/proxy-metrics` are deliberately kept with no frontend caller - the dashboard reads all three through `GET /api/dashboard` - so don't prune them as dead code.

  **Client IP resolution:** `withRequestInfo` (the outermost `/api/` middleware) resolves the real client IP once via `trusted_proxies` and stores it in `context.Context` (`service.RequestInfo`); every downstream consumer (audit log, login rate limiter) reads that, never `r.RemoteAddr`/`X-Forwarded-For` directly. `resolveClientIP` trusts `X-Forwarded-For` only when the immediate TCP peer is itself in `trusted_proxies`, then walks the header right-to-left skipping trusted hops. `trusted_proxies` entries are IPs/CIDRs, plus the literal token `gateway`, resolved once at startup by `expandGatewayProxies` to the container's IPv4 default gateway (parsed from `/proc/net/route`), the address a proxy's traffic is SNAT'd to under Docker's default bridge + published port. Mixed lists are fine (`["gateway", "10.20.45.23"]`). The deployment gotcha around append-vs-passthrough `X-Forwarded-For` is in [Traps](#traps).

- **`internal/ipmatch`**: parses `[]string` IP-or-CIDR config lists into a matchable `Set`; shared by `trusted_proxies` (`internal/web`) and the login rate limiter's `bypass_ips` (`internal/service`) so both interpret entries identically. The `gateway` token is expanded before `ipmatch.Parse` sees it.

- **`internal/httpmw`**: HTTP middleware shared by the web and proxy servers. `Recover` wraps both handlers in `server.go`: a handler panic is logged via `slog` with method/path/remote/stack and turned into a `500` if nothing was written yet, instead of a dropped connection. Its `recoverWriter` forwards `Flush`/`ReadFrom`/`Unwrap` so proxy streaming and `http.ResponseController` still work.

- **`internal/oci`**: registry HTTP client: auth challenge flow, Docker Hub compat. `Client` holds the HTTP client + the two provider interfaces; all per-registry-host caching (bearer tokens, resolved credentials/options, cert-relaxed HTTP clients) lives in `registryCache`, whose `cachedFetch[V]` helper does LRU-check → singleflight → fill so a burst of misses triggers one fetch. `do` retries `429`/`503` up to 3 times (4 attempts total) with a context-aware wait: `Retry-After` when present and capped at 30s, otherwise `1s`/`2s`/`4s` backoff plus jitter. An exhausted retry returns the last response as-is, still surfaced through `registryStatusError` → `*oci.Error`. All registry calls are `GET` with no body, so re-sending is safe.

- **`internal/taganalyzer`**: pure tag-string analysis (`tokenize → classify → family → order`), no I/O. Determines family grouping, tag ordering, family relevance, and whether an upgrade is available. Deep dive: [Design Details → taganalyzer](#internaltaganalyzer).

- **`internal/tagfilter`**: loads tag exclusion regex patterns from `filters.yaml` and exposes `Filter.Apply(tags) []string`. With no configured `tag_filters_path` it uses embedded defaults: `^commit-.*`, `^pr-.*`, `^sha256-[0-9a-f]{64}`. Patterns match case-insensitively (so `^pr-.*` also covers `PR-`). The digest pattern intentionally has no end anchor, since a tag starting with a full SHA-256 digest is a cosign artifact regardless of any `.sig`/`.att`/`.sbom` suffix. Tags are filtered once, at the point they're fetched, then passed downstream as already-filtered data; no later layer re-applies the filter, and filtering never happens inside `internal/oci` (transport stays independent of tracking policy). Kept separate from `internal/taganalyzer` (which stays I/O-free) and from runtime config (this is policy, in its own file).

- **`internal/blobcache`**: optional on-disk manifest/blob cache, keyed by digest. `Cleanup` (worker job) first drops blobs untouched for `blob_cache.lifetime`, then, if `blob_cache.max_size` is set, evicts least-recently-accessed blobs (file + row) down to 90% of the cap, a low-water mark so it doesn't evict on every run at the limit. Enforcement is async-only (bounded by the cleanup interval); the running byte total is decremented locally during eviction since `Cleanup` holds `c.mu` and `writer.commit` takes the same lock. `Usage(ctx)` (used/max bytes) feeds the dashboard. `parseBytes` in `internal/config` is 1024-based; `""`/`0` = unlimited. The stored row type is `model.CachedBlob` so `internal/store` doesn't import `internal/blobcache` back; `blobcache` depends on `store` only through its own `Store` interface.

- **`internal/smtp`**: minimal `net/smtp`-based mailer for tag-change notifications. HTML mails are `multipart/related` wrapping a `multipart/alternative` (text + HTML) plus the DockVMap logo (`assets/dockvmap-logo.png`, `//go:embed`) as an inline `Content-ID: <dockvmap-logo>` part; `tag_notification.html` references it as `cid:dockvmap-logo`, so the branded header renders without an external fetch or a data URI (both blocked by major clients).

- **`internal/webhook`**: minimal HTTP POST client for tag-change notifications (`webhooks` config, a list of URLs). Sends dockvmap's own fixed JSON shape (`event`/`imageName`/`tags`/`updateAvailable`/`occurredAt`), deliberately not Slack/Discord-formatted; no per-provider formatting is built in, since there was no expressed need and it'd be the kind of speculative feature this project avoids. `service.Notifications` treats email and every webhook URL as independent, best-effort, attempt-once channels per event, with no per-channel delivery tracking (`tags_events.notif_sent_at` is a single flag), so a down channel loses that one notification silently (logged) rather than blocking or duplicating others. The worker runs when `smtp.enabled` **or** `webhooks` is non-empty.

  Event types on `tags_events` are `TAG_ADDED`, `TAG_REMOVED`, `UPGRADE_AVAILABLE` (emitted by `RefreshAvailableTags` via `Events.HandleEvent` when an image's `updateAvailable` flips true or its target advances). Webhooks fire for every event. Email is gated per account by `UserPreferences.NotifyLevel` (`all` / `upgrades` / `none`, default `all`; fresh admin gets `upgrades`; legacy `notifyNewTags` true/false maps to `all`/`none` on unmarshal): `upgrades` sends only `UPGRADE_AVAILABLE`, `none` nothing, `all` everything. The gate is applied at send time in `SendPendingTagNotifications` (per recipient, via `mailTargets`); `tags_events.notify` still just means "not a silent refresh".

- **`internal/service/failure_log.go`** (table `background_failures`): `FailureLog`, a persistent log of actionable background failures (webhook/email delivery, image tag-refresh, discovery, event registration), injected into `Images`, `Notifications`, `Discoveries` alongside their existing `slog.Error` calls (both fire: slog for the ops trail, this for the GUI's "Recent issues" card). `Record(ctx, …)` inserts on a `context.WithoutCancel` + 5s timeout so a timed-out refresh still logs its failure; a failed insert is swallowed with a `slog.Error`. `Recent(ctx)` returns the last 50 by `occurred_at DESC`; a 24h `background-failure-cleanup` job drops rows older than 30 days (`occurred_at` is `DEFAULT CURRENT_TIMESTAMP`, so the prune binds via `sqliteDatetime()`). Same reasoning as `tags_events` existing separately from `image_tags`: a dedicated event log decoupled from the current-state tables, so a since-resolved transient failure (e.g. `images.last_check_error`) doesn't just vanish from view, and now survives a restart. `internal/web` composes the human-readable message per source (`failureMessage` in `dto.go`) from the stored raw error text, kept verbatim, since that's the useful troubleshooting content.

- **`internal/service/proxy_tokens.go`** (table `proxy_tokens`): CRD-only (no update, no revoke-vs-delete distinction), issued by the single admin, not tied to a `user_id` (matches `registries`/`images`, which also carry no creator column; provenance lives only in the audit log). Token stored as a SHA-256 hash (fast, not bcrypt: a high-entropy random secret, not a human password); plaintext shown once on creation, never retrievable again. Enforced in `internal/proxy` via HTTP Basic Auth on every `/v2/...` request, gated by `proxy_auth.enabled` (config, default `false`: an explicit switch, not "on the moment a token exists"). Only the password field is checked; the username is free-form/unchecked (label-not-identity: any client presents any username, e.g. `docker login -u <label> -p <token>`).

- **`internal/store/proxy_metrics.go`** (table `proxy_metrics_daily`, `day` TEXT PK in UTC) + **`internal/service/proxy_metrics_history.go`**: `proxy.Metrics` stays an in-memory atomic counter set; the `proxy-metrics-flush` job folds `Snapshot() − prev` into today's row (`RecordDelta` → `INSERT … ON CONFLICT DO UPDATE col = col + excluded.col`), and its `onShutdown` does a final fold. `prev` advances only on a successful write, so a failed flush retries the whole delta. `ProxyMetricsHistory.Summary` reads the last 30 rows and sums them into `today` / `last7d` / `last30d` windows (server-side, one query) for `GET /api/proxy-metrics` and for the `metrics` section of `GET /api/dashboard`, both of which also carry `cache.{usedBytes,maxBytes}` from `blobcache.Cache.Usage` (omitted when the cache is off). The dashboard reads the aggregate; `/api/proxy-metrics` is kept but has no frontend caller. `proxy-metrics-cleanup` prunes rows older than 30 days.

- **`internal/store/worker_ticks.go`** (table `worker_ticks`) + **`internal/service/worker_schedule.go`**: one `last_run_at` row per `scheduledJob.name`, so worker scheduling survives restarts. Separate from `images.last_checked` by design: it records when the tag-refresh job last swept, not per-image freshness. `WorkerSchedule` is a pass-through service (like `Health`), keeping cmd → service → store layering. The migration doesn't seed rows; on first startup each job has none and runs once (staggered), then falls into its normal `interval`. `GET /api/tag-refresh-status` reads the `image-tag-refresh` row (`service.WorkerJobTagRefresh`, the one job name shared across packages) plus `tags_check_interval`, feeding the dashboard's "Tag checks" tile and the sweep button on `Images.svelte`. It stays a separate endpoint rather than folding into `GET /api/dashboard`, because the frontend polls it every 2s while a sweep runs (see [Frontend](#frontend)).

- **`internal/service/login_rate_limiter.go`** (`login_rate_limit` config): per-IP failed-`/login`-attempt counter, checked as the first line of `Sessions.Login` before any DB lookup or bcrypt work. State is an attempt count in an `expirable.LRU[string, int]` keyed by resolved client IP, TTL = `window`: a failure calls `Add` (renews the TTL), a block-check only `Get` (never renews), so the lockout is a flat `window` from the failure that tripped it, not extended by knocking while locked, and a quiet IP ages out on its own with no cleanup job. A successful login clears the entry. `bypass_ips` (also `internal/ipmatch`) skip the limiter entirely, still need the correct password. Defaults to enabled (`Enabled *bool`, not plain `bool`) so a `config.yaml` predating this feature is still protected after an upgrade, unlike the other opt-in blocks where the zero-value default is intentionally off.

- **`internal/service/discoveries.go`** (table `tag_discoveries`): backs the "Add virtual image" wizard's repository step. Deep dive: [Design Details → Tag discovery](#tag-discovery).

- **`internal/service/images.go`**: virtual image CRUD + the tag-refresh pipeline. Deep dive: [Design Details → Image tag refresh](#image-tag-refresh).

- **`cmd/tagaudit`**: offline auditing for `internal/taganalyzer`, run by hand, never by the app. Four modes over a corpus of cached tag lists (`-corpus`, default `sampledata/tags`): `-sample N` builds a repository list from Docker Hub's official images plus keyword search and picks the rest at random (written to `<corpus>/audit-manifest.tsv`); `-fetch` downloads tags for anything not cached; `-verify` re-checks every invariant the package promises plus determinism and placement accuracy; `-shapes` reports the segment shapes that stop tags from grouping; `-rank` flags repositories whose leading family looks wrong. No mode = all three reports. The corpus lives under gitignored `sampledata/`, so this command, not the data, is the durable artifact: a fresh clone regenerates everything with `-sample` then `-fetch`. `-shapes` is the one that finds *unknown* conventions: it describes each blocking segment by character class rather than content (`gb73411456155` and `ga16392559` both become `a{2-3}d{7+}`) and ranks buckets by volume × distinct-value rate. A high distinct rate means nearly every tag has its own value there, which is what a machine-generated identifier looks like; a named axis like `alpine`/`bookworm` reuses few values many times and sinks. High volume + high distinct rate + several unrelated repositories = a convention the analyzer doesn't understand yet (the signature `git describe` output had before it was handled, and Alpine's `-rN` would have had).

- **`cmd/taglist`**: prints the family breakdown for a single repository fetched live; the quick "what does this repo look like" tool, where `tagaudit` is the corpus-wide one.

## Background worker

`cmd/dockvmap/worker.go`. `startWorker` takes a `workerDeps` struct (not positional args, same reasoning as `web.Dependencies`), builds a config-filtered `[]scheduledJob`, and spawns each via `runScheduledJob`: **one goroutine, one `time.Timer`, one `select` per job**; never share one `select` across jobs.

### Job model

`scheduledJob{ name, interval, run func(ctx) (int, error), doneMsg, onShutdown, trigger }`:

- `run` is an inline closure calling the service method (the `counted` helper adapts `int64` counts). `executeJob` logs `doneMsg` with the returned count when it's > 0. Only `image-tag-refresh` keeps a named function (`runImageTagRefresh`), for its start/elapsed logging.
- `executeJob` writes the tick (`MarkRun`, via `context.WithoutCancel` + 5s) **before** calling `run`, so a job that always fails still advances instead of retrying every wake. It also `recover()`s so one job's panic can't take down the worker.
- First fire = `max(0, interval - since(lastRun)) + stagger`, where `lastRun` is the persisted `worker_ticks` row (`service.WorkerSchedule`), so a process restarted more often than its `interval` still runs the job instead of resetting the clock. An overdue job runs once, not once per missed interval. Steady state is a fixed `interval` period, like `time.Ticker`. First-fire stagger is `10s × job index` capped at `30s`, first fire only.
- `onShutdown` (called on `ctx.Done()` with a `context.WithoutCancel` + 5s budget before the goroutine exits), used by `proxy-metrics-flush` to persist its last delta on a clean stop.
- `trigger` (`<-chan struct{}`, nil unless the job can run early on demand), used by `image-tag-refresh` for the manual "refresh all tags" GUI button, via `service.WorkerTrigger`.
- `name` is the `worker_ticks` key; keep it stable and unique.

Tag discovery goroutines spawned by `Discoveries` are **not** tracked here: they run off the same worker context (cancelled on shutdown) but aren't in `startWorker`'s `WaitGroup`, so shutdown interrupts an in-flight scan rather than waiting; `Discoveries.RecoverFromRestart` cleans up the row on next startup.

### The jobs

| Name | Interval | Gate | Does |
|---|---|---|---|
| `session-cleanup` | 1h | always | delete expired session rows |
| `tag-discovery-cleanup` | 24h | always | drop terminal `tag_discoveries` rows older than 7d |
| `background-failure-cleanup` | 24h | always | drop `background_failures` rows older than 30d |
| `image-tag-refresh` | `tags_check_interval` (default 24h) | interval > 0 (else disabled, with a warn) | the tag-refresh sweep; has a `trigger` for manual runs |
| `blob-cache-cleanup` | `blob_cache.cleanup_interval` (default 1h) | cache enabled | lifetime drop + max-size eviction |
| `blob-cache-orphan-scan` | 24h | cache enabled | drop cache files with no row |
| `tag-notification` | 5m | `smtp.enabled` or `webhooks` non-empty | send pending tag notifications |
| `proxy-metrics-flush` | 1m | always | fold `Snapshot() − prev` into today's `proxy_metrics_daily` row; `onShutdown` folds once more |
| `proxy-metrics-cleanup` | 24h | always | prune `proxy_metrics_daily` older than 30d |

## Frontend

`frontend/` - Svelte 5 + Vite + TS SPA, hash-routed (`svelte-spa-router`), cookie-session auth (not JWT, no `Authorization` header anywhere). `frontend/dist` is built and embedded at compile time (see [Commands](#commands)).

### State layering

`lib/api` (stateless fetch) → `lib/stores` → `lib/services` (only code that mutates a store). Only worth it once 2+ components share the same reactive data; otherwise page-local `$state` calling `lib/api` directly is fine and preferred.

Services are module singletons (`auth`, `dashboard`, `tagRefreshStatus`, `theme`, `toast`, `updatesCount`) **except** where the state belongs to one route, which gets a factory instead - `createTagDiscovery()` in `lib/services/tagDiscovery.ts` - so nothing leaks between mounts of that route. A singleton a route drives must expose a `start()` returning its cleanup, and reset its store there (see `dashboard.start()`).

### Conventions

- Every route component renders `<PageTitle title="…" />` (`lib/components/PageTitle.svelte`, a `<svelte:head><title>` wrapper appending `· DockVMap`). `ImageDetails` passes the loaded image name reactively. List/empty views can pass an `emptyAction` snippet to `AsyncState` to put the primary CTA inside the empty state.

- **Errors**: `lib/api/client.ts` is the only module that knows `ApiError` exists. `request()` catches the `fetch` rejection and rethrows a transport failure as `ApiError` (status `0`, "Could not reach the server…"), so "unreachable" and "server said no" are distinguishable instead of both landing in a generic fallback. Call sites never test `instanceof`: use `errorMessage(err, "Failed to …")`, whose fallback now only covers non-`ApiError` bugs.

- **Async state**: the loading → error → empty → content precedence lives once, in `lib/utils/asyncState.ts` (`resolveAsyncState({ loading, hasError, empty, busy })` → `{ kind, busy }`; `busy` is only ever true for `content`). Two presentations consume it: `AsyncState.svelte` (list/page - `TableSkeleton` or spinner, inline `.error`, `.empty` card) and `dashboard/CardBody.svelte` (cards - centred `CardState` blocks through `errorState`/`emptyState` snippets, `BusyOverlay` corner spinner). Both keep their own snippet-availability guards, so a missing snippet falls through to content. Add a state to the resolver, never to one of the two.

- **List pages**: guard rapid filter/pagination changes with an incrementing request token (see `Images.svelte`'s `loadToken`) so a slow superseded response can't overwrite a newer one. Track a `loaded` flag alongside `loading` and pass `AsyncState loading={loading && !loaded} busy={loading && loaded}`; skeleton on first load only; refetches keep current rows visible, dimmed with a corner spinner.

- **List-view persistence**: the hash query string is the single source of truth for a list page's filters + pagination. `lib/utils/listQuery.ts`: `readListQuery(defaults)` parses the flat filter object out of `location.hash`, coercing each key by its default's type; `writeListQuery(filters)` serializes back to a `?…` string (dropping empty / `false` / `0`); `pushListQuery(routePath, filters)` does a `history.replaceState` (no `hashchange`, so no reload loop); `watchListQuery(sync)` runs `sync` once and on every `hashchange`, returning a cleanup. A list page (`Images.svelte` / `AuditLog.svelte`) wires a local `syncFromUrl` (via `readListQuery`) into `onMount(() => watchListQuery(syncFromUrl))`, calls a local `syncToUrl` (via `pushListQuery`) on every filter change, and, where it has a detail route, appends `writeListQuery(...)` when navigating into a row. The detail page reads its own carried query once at init and echoes it onto the "← back" link (`backHref`). No storage, no sidebar coupling.

- **Filter bar**: wrap each labelled control in `<div class="filter-field"><span class="filter-label">…</span> …</div>` inside `<FilterBar>`; the search box goes in bare (no label). `FilterBar.svelte` owns the bar layout (compound `:global` rules keyed off `.filter-bar`). Every control carries `class="input filter-control"` (`.filter-control` in `app.css` is the compact toolbar treatment: 13px, `--radius-sm`, softer than a form `.input`). Add `class:is-active={<value set>}` so an engaged filter gets the accent border + tint. Selects render a custom chevron via the `--select-chevron` token; `select.filter-control` is auto-width with a `min-width` floor.

- **Shared CSS primitives** (`app.css`): `.icon-button` (ghost square button; `.bordered` for the command-row copy variant), `.command` / `.command-row` (mono command/token box + copy row), `.list-header` (title-left / action-right row), `.table-wrap` (alongside `.card` on a list table's wrapper; `overflow-x: auto` so the table scrolls instead of clipping inside `.card`'s `overflow: hidden`). `.table td` is vertically centred globally; `.input:disabled` dims + `not-allowed`. Keyboard focus: a baseline `button/a/[tabindex]:focus-visible` outline rule covers everything; components only need their own `:focus-visible` for a non-default look. Destructive/warning CTAs use `--btn-danger-bg` / `--btn-warning-bg` (fixed across themes) rather than `--color-danger` / `--color-warning`, which desaturate in dark mode. `index.html` carries a pre-bundle `#app-boot` spinner (literal hex mirroring the theme tokens) that `main.ts` clears before `mount`.

- **`Field.svelte`** (shared input) takes an optional `autofocus` prop that calls `.focus()` on the element in `onMount`, not the HTML `autofocus` attribute, which only fires on initial document parse, not on client-side navigation into a route (login reached after logout / session expiry / auth redirect). `Login.svelte` sets it on the username field.

- **Pollers**: `lib/utils/poller.ts` (`createPoller`) is the raw cancellable-interval primitive - a tick returning `false` stops the loop. `ImageDetails.svelte` uses it directly to poll `GET /api/images/{id}` every 2s while `refreshStatus === "running"` (merging each poll into `image` field-by-field via `mergeImage`, not reassigning, so a status-only tick doesn't churn child props); `lib/services/tagDiscovery.ts` and `lib/services/tagRefreshStatus.ts` own their own loops on it. `lib/utils/visibilityPoller.ts` (`createVisibilityPoller`) wraps it for anything that polls *while a page is open*: immediate first tick, `poller.stop()` on `visibilitychange` to hidden, resume with an optional `refetchOnResume` staleness predicate, and `start()` returns the stop function. `dashboard` and `updatesCount` use it. Prefer it over a bare `createPoller` for interval polling, so background tabs stay quiet.

- **Dashboard**: one request. `GET /api/dashboard` returns five sections (`summary`, `updates`, `issues`, `activity`, `metrics`), each a `{data, error}` envelope, so one failed query degrades one card instead of the page (`internal/web/api_dashboard.go` fans out with a `sync.WaitGroup`, each goroutine writing its own field). `Dashboard.svelte` is the only thing that fetches: `dashboard.start()` on mount, 60s visibility-gated poll, store reset on unmount. Cards are pure - they take `DashboardCardProps<T>` (`lib/components/dashboard/types.ts`: `{data, error, loading, busy, onRetry}`), and the `dashboardSections` derived store in `lib/stores/dashboard.ts` shapes each section into that contract, so the markup is `<RecentIssuesCard {...sections.issues} {onRetry} />`. Adding a card is a section server-side plus one line in the route; `hasErrors` iterates `Object.values(sections)` and needs no edit.

- **Tag refresh status** (`lib/services/tagRefreshStatus.ts`) is shared by `Dashboard.svelte` and `Images.svelte`, and deliberately stays *outside* the dashboard aggregate: it polls every 2s while a sweep runs, which a 60s aggregate must not. `watch()` refcounts watchers and returns an unwatch; concurrent `refresh()` calls coalesce onto one in-flight request. Poll failures are tolerated up to `MAX_CONSECUTIVE_POLL_ERRORS` (3), after which polling stops and `unavailable` is set - the "Tag checks" tile then reads *Unknown* rather than a stale *Running*, and no completion fires. Consumers must not hand-roll a `wasRunning` edge detector: `onCompleted(cb)` fires once when a sweep goes running → idle (`Dashboard` refreshes, `Images` reloads its table).

### User-agent parsing

`lib/utils/userAgent.ts`: `parseUserAgent(ua?)` wraps the `bowser` dependency and returns `{ browser?, os?, device: "desktop"|"mobile"|"tablet"|"unknown", label, raw }`. `browser` is name + major version ("Chrome 140"); `label` joins the resolved parts ("Chrome 140 · Windows"), or is the raw string when Bowser can't identify the browser, or "Unknown device" when the UA is empty. `lib/components/DeviceIcon.svelte` maps `device` to a Lucide icon. Used by `Profile.svelte` (active sessions: icon + label, raw UA in `title=`) and `AuditLog.svelte` (audit detail: icon + label, raw UA shown beneath in muted text). Display-only; the backend stores and returns the raw UA untouched.

### Traps

- **Lucide icons**: import from the icon's own subpath (`@lucide/svelte/icons/x`), never the package root (drags ~3800 files into `svelte-check`). Never pass `class` directly to an icon component; it's invisible to the parent's scoped CSS; wrap it in `<span class="icon">` and style that. A **bare** `:global(.icon)` leaks app-wide; only a compound form (`.parent :global(.icon)`) is safe. Same trap one level up: an icon arriving through a snippet from a parent (`DashboardCard`'s `icon` snippet) renders with the *parent's* scope hash, so a plain `.card-head-title svg` in the child never matches - it has to be `.card-head-title :global(svg)`.

- **`app.css` dark theme** is defined twice (media query + `[data-theme='dark']`), and `index.html` has a blocking inline `<script>` setting `data-theme` before first paint; both intentional (explicit choice beats system preference, and it avoids a theme-flash). Keep the `localStorage` key in sync between `index.html` and `theme.ts` if it changes.

## Design Details

### `internal/taganalyzer`

Pure tag-string analysis (`tokenize → classify → family → order`), with no I/O.

This is one of the most important components in DockVMap. It determines which tags are grouped together, which tag in a family is newest, how families are presented in the tag picker, and whether `updateAvailableFor` reports an available upgrade. Errors here usually produce a wrong recommendation rather than a visible failure, so correctness is critical.

`updateAvailableFor` (`internal/service/images.go`) skips a newer same-family candidate when the pinned tag's version axes are a prefix of the candidate's: every axis of the pinned tag is a numeric prefix of the candidate's corresponding axis, and the candidate is more specific somewhere (an axis is longer, or the candidate has extra trailing version axes the pinned tag lacks). So pinned `1.2`, candidate `1.2.6` is not an upgrade since `1.2` already tracks it; `1.3.0` still is; `17-alpine-3` does not flag `17.9-alpine-3.23` (prefix on both the app and base axes) while `17.10-alpine-3.23` / `18.x-alpine-3.23` still do; and pinned `10.11` does not flag `10.11.11.20260606-153911` (a dated build - same `10.11` prefix plus an extra time axis), the same way it already does not flag clean `10.11.11`. The comparison runs off `image_tags.version_segments`, a per-tag JSON snapshot (`[][]int64`) of every `OrderVersion` segment's `Numbers` in order (empty for non-version axes, so date/alphabetical families are unaffected), written by `imageTagsFromAnalysis` alongside `family_id`/`tag_order`/`prerelease`. A base-image-only bump at the same app version (`17.9-alpine-3.23` → `17.9-alpine-3.24`) is not a prefix relation, so it is still reported. This suppresses the recommendation, not the ordering: when a real version gap exists and the newest tag is a dated build (`10.11.5` → a dated build of `10.11.11`), an upgrade is still reported, just pointed at the snapshot rather than the clean tag.

Before changing grouping, classification, or ordering, re-measure with `go run ./cmd/tagaudit` (`-verify` for the invariants, accuracy and determinism, `-shapes` and `-rank` for what regressed), and keep every invariant counter at zero.

#### Guarantees

The analyzer guarantees:

- Every tag belongs to exactly one family.
- Family IDs are unique within a repository.
- A family never mixes different values of a literal/named segment.
- Ordering never inverts the leading version.
- `Analyze` is deterministic: the same tags in any input order produce a byte-identical result.
- Results are safe to compute concurrently.

These invariants have been verified against hundreds of thousands of real tags across ~200 repositories, and re-verified against a separate randomly sampled corpus of similar size that the analyzer was never tuned on: zero violations in both. Re-verify with `cmd/tagaudit -verify` after any change.

A `Family` (`Kind`: `blood` / `ancestor` / `step`) represents a **structural lineage**, not runtime compatibility. It means the tags appear to be versions of the same named thing (same repository, OS base, variant, etc.). The analyzer cannot determine whether two versions are actually compatible, so it must not use compatibility assumptions to exclude a match. Whether an upgrade is safe remains the user's decision; `updateAvailableFor` only surfaces the newer tag as a suggestion.

Only version-shaped segments (numbers or dates) may vary within a family. Literal/named segments such as `noble`/`jammy` or `jdk`/`jre` must match exactly and are never treated as upgrade axes.

#### Blood families

`blood` is the strongest family relationship: every member must have an identical computed identity pattern.

The identity pattern is based on segment shapes rather than literal values. Version segments collapse to `solo`, `multi`, or `calver`; prefixes and suffixes remain literal; date/time segments collapse to a constant; confirmed hash segments also collapse to a constant.

A bare number such as `11` is treated as a multi-part version only when `collectKnownMajors` finds evidence that `11` is a real major version elsewhere in the same repository, such as `11.0.24`. This evidence is scoped to the preceding segment context rather than the raw segment index, preventing unrelated tag shapes from influencing one another.

Hash-shaped segments are normalized only when `collectHashLengths` confirms the slot from the repository's own tag set: at least 5 distinct values of the same length at the same position, with at least one containing `a`–`f`. This prevents long numeric build counters from being mistaken for hashes.

Hash-shaped means `^g?[0-9a-f]{7,40}$`. The optional `g` covers the abbreviated SHA produced by `git describe --tags`, such as `v3.29.6-1-gb73411456155`.

There is deliberately no additional ratio/frequency heuristic. An earlier ratio-based guard performed worse on repositories with architecture fan-out, so it was removed after testing across the corpus.

Hash normalization happens at identity time rather than classification time. `normalizeSegments` initially parses numeric tokens as versions; once repository statistics are available, `normalizeHashSegments` rewrites confirmed hashes as literals. This also prevents commit SHAs such as `a953921` from being ordered numerically as if they were versions.

#### Ancestor families

`ancestor` connects a root family to another family when the root's pattern unambiguously extends into exactly one target via `isSkeletonExtension`. Only variable segments with no prefix/suffix or statistically confirmed hashes may be skipped.

A hash-free root never merges into a family whose varying axis is a hash. Otherwise release families can absorb commit builds and expose a SHA as an upgrade.

A version segment is also never skipped when the root's next segment is itself a version. This prevents a real version component such as a JDK major from being confused with a later build number.

Root candidates are tried longest-pattern-first so more specific families claim targets before shorter patterns. Merged families use their most general member as `rootRep`, and `attachAncestors` repeats the process until no further merges are possible.

Ambiguity always loses: zero or multiple matching targets leave the root unchanged. A later pass may resolve the relationship after competing families have themselves merged.

#### Family relevance

Families are ordered by `familyRelevance` rather than by `Kind`. How a family was constructed does not determine how useful it is to a user.

Relevance is evaluated from the family's root, using these criteria in order:

1. A singleton never leads.
2. A root without hash literals beats a commit-built root.
3. Version-led beats date-led, which beats alphabetical-led.
4. Fewer root segments beat more.
5. A multi-part version with a plain or `v` prefix beats a named version such as `alpine3.24`, which beats a bare integer/build counter.
6. More tags beat fewer.
7. The family key breaks remaining ties.

These rules were tuned against `sampledata/`. Across the sample corpus they changed the leading family in a few dozen repositories, all changes improvements or neutral, with no repository left leading with a singleton. Retune against the corpus rather than a single repository.

#### Family identity and ordering

`Family.ID` is an FNV-64a hash of `Family.Key`, masked to 52 bits so it stays JSON-number safe. It is not a per-run counter, so a family keeps the same ID across refreshes as long as its pattern is unchanged.

`Family.HasOrder` is false when the only varying axis is a confirmed hash. Such tags can still be ordered for display, but the order has no semantic meaning, so `updateAvailableFor` does not report an upgrade for the family.

Anything that cannot be related to another tag becomes an isolated singleton. It retains `Kind: FamilyStep` for frontend/backward compatibility, but this no longer represents a weak grouping relationship; it simply means no relationship was claimed.

#### Recognized version conventions

In addition to standard `MAJOR.MINOR.PATCH`, the parser recognizes several glued suffix conventions that affect grouping:

- PEP 440 `aN` / `bN` pre-releases
- OpenBSD/OpenSSH-style `pN` patches
- Pre-JEP 223 Java `uN` updates, such as `8u422`
- Alpine/apk `-rN` package revisions
- Debian/Ubuntu same-day rebuild suffixes such as `20180112.1`

The Alpine revision is absorbed into the preceding version, so `1.2.3-r1` stays in the same family as `1.2.4`. A leading branch number such as `r409-...` is unaffected. Likewise `20180112.1` is the same date identity as `20180112`, with `.1` affecting ordering but not family identity.

### Image tag refresh

Every trigger (worker `RefreshAll`, `-refresh-tags`, `Images.Create`, the GUI's manual refresh) goes through `RefreshAvailableTags` (`internal/service/images.go`). It takes the per-image lock (`imageRefreshLocker`, shared with `UpdateTag`/`Rename`), wraps its body in a 10-minute `context.WithTimeout`, and sets `images.refresh_status` to `running` on entry / `idle` on exit, the exit write via `context.WithoutCancel` + 5s so a shutdown mid-refresh can't strand the row at `running`.

`imageRefreshLocker` is the real serialization guarantee; `refresh_status` is only a best-effort UI hint (the entry/exit writes aren't CAS, so a concurrent worker refresh finishing first can clear a GUI-initiated claim, which is harmless, at worst a redundant refresh).

The GUI's `POST /images/{id}/refresh-tags` calls `Images.StartBackgroundRefresh`: `TryStartImageRefresh` CAS-gates `refresh_status` `idle`→`running` (0 rows ⇒ a worker or manual refresh is already running ⇒ `202 {status:"running"}`), then the refresh runs on `Images.bgCtx`, not `r.Context()` (which dies at the web server's 45s `WriteTimeout` and on client disconnect), untracked by `startWorker`'s WaitGroup like discovery scans. The handler polls `refresh_status` for ~2s (150ms) before falling back to `202`; a finished refresh returns `200 {status:"refreshed"}` or `{status:"error", error}` (from `last_check_error`). Manual refresh uses `RegisterEvent: EventSilent` / `FlagAsNew: true`, so no notifications, unlike the worker's `EventNormal`.

`Images.RecoverFromRestart` (startup, beside `Discoveries.RecoverFromRestart`) is the only recovery path, resetting any `running` left by a hard crash or clean shutdown mid-refresh back to `idle`.

**Frontend:** `ImageDetails.svelte` polls `GET /api/images/{id}` every 2s while `refreshStatus === "running"`, via `createPoller`. It merges each poll into `image` field-by-field (`mergeImage`) instead of reassigning, so a `refreshStatus`-only tick doesn't churn child props. The change-tag modal fires the action (toast, no picker reload on `202`) and takes `refreshStatus` as a prop, swapping its Refresh button for `<RefreshingIndicator>` while running, then reloading its picker once (keeping the selection) when the prop returns to `idle`; the last-check-error row's refresh button is hidden the same way. (`@keyframes spin` / `.spin` are global in `app.css`.)

### Tag discovery

`internal/service/discoveries.go` (table `tag_discoveries`) backs the "Add virtual image" wizard's repository step.

`Check` does a cheap single-page `oci.Client.CheckRepository` first, never the full `ListTags` pagination, so a huge repository can't make "check repository" itself slow. The real scan runs in a goroutine off the worker context (capped at 10 minutes, recovers its own panics), writing into a `tag_discoveries` row keyed `UNIQUE(registry_id, repository)`; that row is both the cache (TTL'd via `tag_discovery_ttl`, refreshed in the background without clobbering the previous result on failure) and the dedup lock (insert-or-reclaim on conflict, so concurrent requests for the same pair converge on one scan).

`Check` waits inline on the row for ~2s before falling back to `status: "running"` for `GET /api/discoveries/{id}` to poll, so small/cached repos still resolve in one round trip. `RecoverFromRestart` (startup) fails any row left `running` by an unclean exit; the `tag-discovery-cleanup` job prunes terminal rows older than 7 days. Shutdown does not wait on in-flight scans (see [Background worker](#background-worker)).

`Images.Create` takes an optional pre-fetched `availableTags`, filled from a completed discovery's cache by `apiCreateImage`, so saving doesn't re-trigger the scan `Check` just avoided. Live progress (`tagsSeen`) lives only in an in-memory map on `Discoveries`, updated via `ListTagsWithProgress`'s per-page callback and never persisted, since the scan and the polling handlers share one process. `raw_tag_count`, captured alongside `tag_count` on completion, drives the response's `ignoredCount`.

Any user-suppliable registry host must be checked against already-configured registries (`GetRegistryInfoByHost`) before the server requests it (see [Traps](#traps)).

## Conventions (backend, when adding code)

- **New JSON POST/PUT handler** → use `internal/web/api_routes.go`'s `decodeJSON[T](rw, r) (T, bool)`, don't hand-roll `MaxBytesReader`/`DisallowUnknownFields`.

- **New periodic background job** → append a `scheduledJob{…}` to the slice in `startWorker` (`cmd/dockvmap/worker.go`); `run` is `func(ctx) (int, error)` returning the count to log against `doneMsg`. Each gets its own goroutine, `time.Timer` and `select`; never share one `select`. Set `onShutdown` if the job must do a final action on a clean stop; set `trigger` if it can be run early on demand. The `name` is the `worker_ticks` key; keep it stable and unique. A new service the job needs goes on the `workerDeps` struct, not a new positional arg.

- **New store constraint check** → use `isUniqueConstraintErr`/`isForeignKeyConstraintErr` (typed `sqlite.Error.Code()`), never match on `err.Error()` text.

- **New audit-worthy action** → call `recordAudit`; actor/request info comes from `context.Context` (`service.RequestInfo`/`CurrentUser`), never pass as params.

- **New list filter** → embed `model.Pagination` into a per-domain filter struct (see `model.ImageListFilters`); don't add a shared generic `Filters` type.

- **New date-range filter** → bind through `internal/store/querybuilder.go`'s `sqliteDatetime()` (see [Traps](#traps)).

- **New cross-cutting web-layer dependency** (a service the HTTP handlers need) → add a field to `web.Dependencies` (`internal/web/handler.go`), not a new positional parameter to `web.New`/`newWebServer`. Same rule for the multi-dependency service constructors that already use a config struct: `service.NewImages(ImagesDeps{…})`, `service.NewDiscoveries(DiscoveriesDeps{…})`, `startWorker(workerDeps{…})`.

- **Comments**: a short one-liner is fine where a WHY is genuinely non-obvious; never multi-line/paragraph comments.

## Traps

- **`sqliteDatetime()`**: several tables set timestamps via SQLite's own `DEFAULT CURRENT_TIMESTAMP`, not Go. The driver's default `time.Time` formatting doesn't lexically compare against SQLite's stored format, so always bind through `sqliteDatetime()` for a date-range filter. (Canonical explanation; the Conventions entry points here.)

- **`//go:embed dist/*`** needs a non-dot-prefixed file under `dist/` at build time or the build fails outright; a bare `go build ./...` on a fresh checkout fails until the frontend has been built once. See [Commands](#commands).

- **SSRF**: any user-suppliable registry host must be checked against already-configured registries (`GetRegistryInfoByHost`) before the server requests it (see `Discoveries.Check`).

- **`trusted_proxies` append vs passthrough**: it only protects you if the reverse proxy in front **appends** to `X-Forwarded-For` rather than passing the client's own header through unchanged, e.g. nginx's `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for`, not `$http_x_forwarded_for`. Get that backwards and an attacker's self-supplied header is trusted verbatim, silently defeating both the login rate limiter's per-IP accounting and the audit log's IP field. This is a deployment misconfiguration, not something dockvmap can detect. The `gateway` token (see `internal/web`) only helps if the published port is reachable *only* through the proxy; otherwise a direct hit is also SNAT'd to the gateway and its forged header is trusted.

- **Single-user model, no roles yet**: there's deliberately no create/list/delete-other-account API. Don't add one without checking first; it was removed on purpose.

- **`RegistryOptions` JSON keys are snake_case** while the rest of the API is camelCase (inherited from the Go struct tags); don't "fix" it in just one place.

- **`rewriteTimestamp`** (`tokenizer.go`) folds an ISO 8601 timestamp into the bare 14-digit form **before** tokenization, since the classifier only understands `20060102150405`. It covers the compact `20240101T120000`, the punctuated `2024-01-01T12-00-00`, and either with a leading label (`RELEASE.…` or `RELEASE-…`). `pretokenize` applies every rewrite in sequence rather than stopping at the first match.

- **`taganalyzer`'s tokenizer only splits on `-`/`_`, never `.`**: deliberate, since a version's own dots (`1.2.3`) must stay inside one token. A repo that uses `.` to glue on a non-numeric qualifier (`confluentinc/cp-kafka`'s `7.0.10.amd64`) is handled by `splitDottedToken`, which splits at the rightmost dot where one side is a recognizable shape - so `7.0.10.amd64` tokenizes as `7.0.10` + `amd64`.
