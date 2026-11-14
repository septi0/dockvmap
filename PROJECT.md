# PROJECT.md

dockvmap ("Docker Virtual Mapper") is a Docker/OCI registry proxy: clients pull through a stable "virtual tag" (`myimage:current`) transparently mapped to a real upstream tag tracked per-image. One binary runs a proxy server (OCI Distribution API), a web server (REST management API + embedded frontend), and a background worker, sharing one SQLite DB and one registry HTTP client.

## Principles

A professional app that is useful, intuitive, secure and fast, without being over-engineered. These rules apply to anyone changing the project, human or AI:

- **Useful, not a gimmick.** Every feature solves a real problem for someone running dockvmap. If its value isn't obvious, it doesn't ship.
- **Intuitive.** A feature is clear from the UI without reading docs: plain wording, sensible defaults, and errors that say what to do next.
- **Secure by default.** Least privilege, read-only wherever possible, validate every input, never send or log secrets.
- **Fast and light.** No needless work on hot paths; background features never touch the proxy request path. Measure before and after changes that add load.
- **A plain registry on the proxy port.** The proxy port speaks only the OCI Distribution API, as close to a standard Docker registry as possible, so every client and tool works with it unchanged. DockVMap's own endpoints (management, health, replication) go on the web port or a port of their own, never on the proxy port.
- **Not over-engineered.** The simplest design that is correct. No abstractions, options or layers for hypothetical needs, and no code, tests or guards for edge cases a real homelab or small DevOps setup won't hit.
- **Reuse first.** Before writing a helper, component, style or test fixture, find the existing one. One rule lives in one place.
- **Tidy and readable.** Clear names, small functions, no clever tricks. No dead or commented-out code, and no hacks: fix the cause, not the symptom. When a change leaves something unused (a helper, field, type, style or API field), remove it in the same change.
- **Respect the layers.** `store` → `service` → `web` / `proxy` on the backend; `lib/api` / `lib/stores` / `lib/services` on the frontend; the agent imports only `internal/agentapi` and the standard library.
- **A live production app.** Config, API and database compatibility matter. Migrations are forward-only and come with a backup note; refactors keep behaviour identical.
- **Match the surrounding code.** Same naming, idiom and formatting. A one-line comment only where the code needs it, never a comment explaining design rationale (it belongs in this file); no em dashes in UI, code, docs or commits.
- **Verify honestly.** `make verify` plus end-to-end checks (headless browser at desktop and phone widths) on scratch data, never on real data or config. Failures are reported as they are.

## Commands

`make help` lists all targets. The common ones:

- `make build`: the frontend and both binaries (`bin/dockvmap`, `bin/dockvmap-agent`); `make build-agent` builds only the static agent.
- `make dev`: the backend (`go run`) and the Vite dev server together; Ctrl+C stops both. `CONFIG=<path>` picks a config, otherwise `config/config.yaml`, seeded from `config.sample.yaml`. When `data/agent/` exists it also runs `make dev-agent`, serving this machine's Docker on `127.0.0.1:9002`; add a host `127.0.0.1` once.
- `make verify`: `lint` (gofmt, vet, golangci-lint if installed), `test` and `check` (svelte-check, tsc).
- `make test-integration`: replication end to end, with two DockVMap processes, a `registry:3` container each and a third as the upstream. Needs Docker, names its containers `dvm-testlab-<pid>-*` and removes only those. Not part of `verify`.

Targets that need the frontend rebuild `frontend/dist` every time (make can't tell whether a directory is stale, and a Vite build takes seconds); `frontend/node_modules` is reinstalled with `npm ci` only when the package files change. `internal/taganalyzer` is also verified by `cmd/tagaudit -verify` against a corpus of real tags.

**Fresh checkout:** `frontend/dist` isn't committed, and `frontend/embed.go`'s `//go:embed dist/*` needs a file there, so a raw `go build`, `go vet` or `go test` fails until `make build-frontend` has run once. Every Makefile target that compiles Go handles it.

**CI and releases:** the only CI is `.github/workflows/docker-image.yml`: on a `v*.*.*` tag it builds `ghcr.io/septi0/dockvmap` (`Dockerfile`) and `ghcr.io/septi0/dockvmap-agent` (`Dockerfile.agent`) for amd64 and arm64, tagged `{{version}}`, `{{major}}.{{minor}}` and `latest`. `CHANGELOG.md` has one short section per version, user-visible changes and upgrade notes only; `(unreleased)` becomes the date in the release commit, before tagging.

## Process model

### Runtime shape

`main()` → `run() error`. `run()` is a dispatcher: it does shared bootstrap (config, logging, data dir, credential key), then either handles a one-off CLI command or calls `serve()`.

`serve()` is the composition root: it builds the store, the services, then the long-lived components on one shared `*http.Client` and one `*sql.DB`:

- **proxy server**: OCI Distribution API (`/v2/...`).
- **web server**: REST API under `/api/` + the embedded frontend.
- **background worker**: independent scheduled jobs (see [Background worker](#background-worker)).
- **replication server** (primary only): the replication API on `replication.listen` (see [Replication](#replication)). A follower instead runs the sync loop and copier goroutines.

`awaitShutdown` takes every running server by name and shuts them down together.

### Layering

`internal/store` → `internal/service` → `internal/web` / `internal/proxy`: strict and one-directional, SQL → business logic → HTTP. `internal/oci` stays decoupled from `store`/`service` via small interfaces; `cmd/dockvmap/adapters.go` holds the adapters that bridge them.

### Startup and dispatch

`run()` order: bootstrap → `-backup` (before `store.New`, since it opens the DB read-only and must snapshot the schema as-found) → a `switch` over the remaining one-off commands vs `serve()`. Startup failures `return` rather than `os.Exit`-ing past deferred cleanup.

**One-off CLI commands** (`cmd/dockvmap/commands.go`) each build only the minimal service subset they need and exit without starting any server:

- `-reset-password <username>`: a new random password, printed, and every session of that user ended. Refuses on a follower, which has no accounts.
- `-refresh-tags`: the worker's tag sweep, once. A failed image doesn't stop the batch; it's logged and recorded in the failure log, and the sweep ends with "N of M images failed their tag check" (`ErrTagChecksFailed`, non-zero exit), the same summary System › Tasks shows.
- `-backup <path>`: opens the database with `store.OpenForBackup` (no migrations, the newer-schema guard still applies), writes it with `VACUUM INTO`, and prints what else a restore needs. Restoring is a documented stop, replace, start.
- `-resync [-yes]` (follower only): asks unless `-yes`, then sets `follower_state.resync_requested` and forgets the copied counters, so the registry is compared in full again. A running follower picks it up within seconds, a stopped one at its next start. It doubles as "copy everything again".

### Shutdown

A listener failure takes the same graceful path as a SIGTERM, through the `serverErrs` channel, and exits with 1 (a SIGTERM exits with 0). Shutdown cancels the worker context and waits for its jobs up to `workerShutdownTimeout` (10s); a hung job logs a warning instead of blocking until the runtime kills the process.

## Architecture

Package catalog. Deep dives for `taganalyzer`, image tag refresh, and tag discovery are in [Design Details](#design-details); the worker is in [Background worker](#background-worker); frontend is in [Frontend](#frontend).

- **`cmd/dockvmap`**: `main.go` (entry + flag parsing + shared bootstrap + dispatch), `serve.go` (`serve()`), `commands.go` (the one-off CLI commands), `server.go` (HTTP server construction, TLS, graceful shutdown), `worker.go` (background worker), `bootstrap.go` (`initBlobCache`/`initMailer`/`initNotifications`/`resolveCredentialEncryptionKey`), `adapters.go` (adapters keeping `internal/oci` decoupled from `store`/`service`). Lifecycle details in [Process model](#process-model).

- **`internal/config`**: `Load()` merges env > file > `default:` tag, by reflection over struct tags. The file is decoded with `KnownFields(true)`, so a misspelled key is a startup error (env vars stay permissive). `applyDerivedDefaults()` adjusts interdependent settings (like `secure_cookies` with `tls.enabled` or `trusted_proxies`) and adds a message to `Config.DerivedWarnings` for each; `main()` logs them, and System › Status shows them since they scroll away. Settings that default on (`blob_cache.enabled`, `proxy_access_log`, `login_rate_limit.enabled`) are `*bool`, so a config written before the setting existed still gets the default.

- **`internal/store`**: SQL layer. `store.New` opens and migrates; `store.OpenForBackup` only checks the schema version. The database file is `0o600` and the data folder `0o700`.

- **`internal/store/migrations`**: numbered, checksum-verified, forward-only migrations, safe on a populated database. `verifySchemaVersion` refuses a schema newer than the binary, so an older build never runs against, or backs up, a newer database.

- **`internal/service`**: business logic. Constructors with several dependencies take a struct (`NewImages(ImagesDeps{…})`).

- **`internal/proxy`**: the OCI Distribution API (`/v2/...`): resolves a virtual tag to the real registry, repository and tag, and forwards upstream, optionally through `internal/blobcache`. With `proxy_auth.enabled`, every request needs a proxy token (`proxy_tokens.go`). `httpmw.AccessLog` writes one line per request when `proxy_access_log` is on; the handler adds what it resolved through `httpmw.AccessFieldsFrom`. A successful manifest or blob `GET` is reported to `service.ProxyUsage`, and a new digest served for the image's tag to `Images.RecordServedDigest`. With `internal_registry.enabled`, other names pass through to the internal registry (see [Internal registry](#internal-registry)).

- **`internal/web`**: REST API and the embedded frontend (`handler.go`). `GET /health` is outside `/api/`'s auth and runs a real `SELECT 1`, returning `{status, version}`. `GET /health/ready` (no auth either) answers `200` or `503` with the node's role and checks. On a follower only these two and a page naming the primary are served (see [Replication](#replication)). Hashed `/assets/*` are `immutable`, `index.html` is `no-cache`, so a deploy can't leave an index pointing at gone assets. `GET /api/proxy-metrics` has no frontend caller (the dashboard reads the same data through `GET /api/dashboard`); keep it.

  List endpoints (`/api/audit-logs`, `/api/events`, `/api/failures`) share one pattern: `parsePagination`, a per-domain filter struct, `whereBuilder`, and `List`/`Count`, returning the rows and a `total`. Without a `type` filter, `/api/events` leaves out a `TAG_ADDED` row when an `UPGRADE_AVAILABLE` row for the same image covers its tags, so an upgrade shows once. The Tag activity page's `imageId` comes only from a deep link, shown as a clearable chip.

  **System › Status, Tasks, Failures, Audit** (`api_system.go`, `routes/System.svelte` over `lib/components/system/*Tab.svelte`): status is a curated set (version, database, derived warnings, virtual tag, trusted proxies, proxy auth), never the config or a secret. Tasks merges the job catalog with `worker_ticks`; `POST /api/system/tasks/{name}/run` triggers a job (`404` when it can't be triggered). Each audit entry has a one-line `summary` built in the web layer (`audit_summary.go`).

  **Test connection** (`POST /api/registries/test`, `/api/registries/{id}/test`): a live `GET /v2/` and auth handshake through `RegistryConnChecker`, a throwaway `oci.Client` so unsaved values never touch the shared client's caches. `{id}/test` uses the stored login unless the body overrides it. A failed handshake is `200 {ok:false,error}`; only a malformed request is `4xx`.

  **Write requests need `X-DockVMap`:** `requireAPIHeader` refuses `/api/` writes without it (`403`). A browser can't add a custom header cross-origin without a CORS preflight, which DockVMap never answers, so other sites can't write with the session cookie, however DockVMap is reached. Scripts add the header.

  **Client IP:** `withRequestInfo` resolves the client address once through `trusted_proxies` into `service.RequestInfo`; the audit log and the login limiter read that, never `RemoteAddr` or `X-Forwarded-For`. `httpmw.ClientIP` trusts `X-Forwarded-For` only from a trusted peer, walking it right to left past trusted hops. `gateway` is resolved once at startup from `/proc/net/route` (see [Traps](#traps) for append vs passthrough).

- **`internal/replicationapi`**: the replication wire contract both sides share, like `agentapi`: paths, the state and pull-report JSON, the `Setup` comparison, the token format (`dvmr1-<fingerprint>-<secret>`) and the errors the follower maps to its stop states.

- **`internal/replication`**: HTTP on both ends. `server.go` is the primary's replication port (state long-poll, changes, pull reports, and read-only `GET`/`HEAD` of `/v2/` on the internal registry); `client.go` is the follower's client, pinning the primary's certificate from the token. `integration_test.go` (build tag `integration`) is the Docker end-to-end suite.

- **`internal/service/replication.go`** and `replication_tokens.go` (the primary), **`follower.go`** (sync loop, stop states, readiness), `follower_copy.go` (copying, replay, reconcile) and `follower_pulls.go` (pull reports): see [Replication](#replication).

- **`internal/ocicopy`**: copies one tag between two registries: HEAD both sides, then the index, every child manifest and only the blobs the target lacks. Manifests are checked against their digest here; the target registry checks blobs on upload. Kept apart from `internal/oci`, whose client only sends retry-safe `GET`s, but reuses its media types, `NextPageURL` and `DrainAndClose`. Everything that reaches the internal registry shares `proxy.InternalRegistryTransport`, `InternalRegistry.URL` and `InternalRegistry.Ping`.

- **`internal/ipmatch`**: parses IP-or-CIDR config lists into a `Set`, shared by `trusted_proxies`, `bypass_ips`, `push_allowed_ips` and `replication.allowed_ips`, so all read entries the same way.

- **`internal/httpmw`**: middleware shared by the web and proxy servers. `Recover` turns a handler panic into a logged `500` (if nothing was written), re-raising `http.ErrAbortHandler`, which `ReverseProxy` uses for a client that hung up. Its writer forwards `Flush`/`ReadFrom`/`Unwrap` so streaming still works. Also `ClientIP` and `ParseTrustedProxies`.

- **`internal/oci`**: the registry client: auth challenges, Docker Hub quirks. Per-host caches (bearer tokens, logins, options, relaxed-TLS clients) live in `registryCache`, whose `cachedFetch[V]` does LRU, singleflight, fill, so a burst of misses makes one fetch. `do` retries `429`/`503` up to 3 times, honouring `Retry-After` up to 30s, else `1s`/`2s`/`4s` with jitter; every call is a bodyless `GET`, so retrying is safe.

- **`internal/taganalyzer`**: pure tag-string analysis, no I/O. See [Design Details](#internaltaganalyzer).

- **`internal/tagfilter`**: exclusion patterns from `filters.yaml`, matched case-insensitively, with embedded defaults (`^commit-.*`, `^pr-.*`, `^sha256-[0-9a-f]{64}`, unanchored at the end so every cosign suffix matches). Tags are filtered once, where they're fetched, never inside `internal/oci`.

- **`internal/blobcache`**: optional on-disk cache keyed by digest. `Cleanup` drops blobs unused for `lifetime`, then evicts the least recently used down to 90% of `max_size`, so it doesn't evict on every run at the limit. Enforcement is only in that job. `Usage` feeds the dashboard. It reaches `store` only through its own interface, with `model.CachedBlob` as the row type.

- **`internal/smtp`**: a small `net/smtp` mailer. HTML mails embed the logo as an inline `cid:` part, since clients block external images and data URIs.

- **Notifications** (`internal/service/notifications.go`, `notification_digest.go`, `internal/webhook`): events are `TAG_ADDED`, `TAG_REMOVED` and `UPGRADE_AVAILABLE` (when `updateAvailable` turns on or its target moves; never for a pinned image). Only sweeps create events to notify; manual refreshes are silent. Email is a digest per run: updates available to everyone with a level, tag changes to level `all` only, a group as one line. The level (`UserPreferences.NotifyLevel`: `all`, `upgrades`, `none`) is applied at send time. Webhooks get DockVMap's own JSON (`event`, `imageName`, `tags`, `updateAvailable`, `occurredAt`), once per event. Every channel is best-effort and tried once: a failure is logged and recorded in the failure log, and the event is still marked sent.

- **`internal/service/failure_log.go`** (table `background_failures`): actionable background failures (deliveries, tag checks, discovery, digest lookups), recorded next to their `slog.Error` with a `WithoutCancel` 5s context so a timed-out check still records. System › Failures lists every occurrence. The dashboard's Problems card uses `LatestDistinct` (the newest of each identical failure, with a count) and drops a tag-check failure once the image's `last_check_error` is clear. Rows are kept 30 days. The web layer builds the message (`failureMessage`); `recognizeFailure` (`failure_cause.go`) turns common causes (DNS, refused, timeout, certificate, 401/403/429, SMTP login) into a plain sentence, and the raw error is always sent alongside.

- **`internal/service/proxy_tokens.go`**: create and delete only, not tied to a user. Stored as a SHA-256 hash (a random secret, not a password, so no bcrypt), shown once. Only the Basic Auth password is checked; the username is free text. `can_push` is chosen at creation. A successful `Verify` records the token's use; the page warns after 90 days unused, never for a token with no recorded use.

- **Proxy metrics** (`proxy_metrics.go` in store, `proxy_metrics_history.go` in service, table `proxy_metrics_daily`): `proxy.Metrics` are in-memory counters; `proxy-metrics-flush` adds `Snapshot() − prev` to today's row and moves `prev` only after a successful write. `Summary` sums today, 7 and 30 days in one query. Rows are kept 30 days.

- **`internal/service/proxy_usage.go`** (`images.last_pulled_at`, `last_pulled_tag`, `proxy_tokens.last_used_at`): a manifest or blob `GET` that reached the client in full is a pull; a `HEAD` isn't, since Docker sends one just to check. Blobs must count, because Docker with the containerd store fetches the manifest with `HEAD` only. A digest reference records the image's current tag. Pulls go to an in-memory map, saved by `proxy-metrics-flush` every 5 minutes and on shutdown (merged back on failure); image, group and token reads overlay unsaved entries, so a pull shows within seconds. `RecordProxyUsage` keeps the newer time, so follower reports and repeats can't move it back. No client address is stored.

- **`internal/service/worker.go`** (table `worker_ticks`): one row per job with `last_run_at`, `last_error` and `last_count`, written after every run, a panic included, so the schedule survives restarts. It also holds the job catalog and trigger channels. `GET /api/tag-refresh-status` stays apart from the dashboard aggregate because it's polled every 2s during a sweep.

- **`internal/service/login_rate_limiter.go`**: failed logins per client address in an `expirable.LRU` with a TTL of `window`, checked before any database or bcrypt work. A failure renews the TTL, a check doesn't, so the lockout is a flat `window` and quiet addresses age out on their own. A successful login clears the entry.

- **`internal/service/discoveries.go`** (table `tag_discoveries`): backs the "Add virtual image" wizard's repository step. Deep dive: [Design Details → Tag discovery](#tag-discovery).

- **`internal/service/registry_browser.go`**: read-only browsing of a registry's repositories, and which registries can be browsed. Deep dive: [Design Details → Registry browsing](#registry-browsing).

- **`internal/service/images.go`**: virtual image CRUD, tag changes, and the `images.pinned` toggle (`SetPinned`). **`images_refresh.go`** (same package) holds the tag-refresh pipeline, the per-image refresh lock and update detection. Deep dive: [Design Details → Image tag refresh](#image-tag-refresh).

- **`internal/service/image_groups.go`** (table `image_groups`, nullable `images.group_id`): group CRUD and the shared-version rules, as more methods on `Images` so they share its store and refresh locks. Deep dive: [Design Details → Image groups](#image-groups).

- **`internal/service/hosts.go`** and **`hosts_poll.go`** (tables `hosts`, `host_containers`, migrations `032`, `034` and `036`), **`internal/agentapi`**, **`internal/agentclient`**, **`internal/agent`**, **`cmd/dockvmap-agent`**: hosts and the read-only host agent. Deep dive: [Design Details → Hosts and the agent](#hosts-and-the-agent).

- **`cmd/tagaudit`**: offline auditing for `internal/taganalyzer`, run by hand. Over a corpus of cached tag lists (`-corpus`, default the gitignored `sampledata/tags`): `-sample N` picks repositories (Docker Hub official images, keyword search, the rest at random), `-fetch` downloads missing tags, `-verify` checks every invariant plus determinism and accuracy, `-shapes` reports the segment shapes that stop tags from grouping, and `-rank` flags repositories whose leading family looks wrong; no mode runs the three reports. `-shapes` finds unknown conventions: it describes blocking segments by character class (`gb73411456155` and `ga16392559` both become `a{2-3}d{7+}`) and ranks them by volume times distinct-value rate. High volume, a high distinct rate and several unrelated repositories mean a machine-generated convention the analyzer doesn't understand yet.

- **`cmd/taglist`**: prints the family breakdown for a single repository fetched live; the quick "what does this repo look like" tool, where `tagaudit` is the corpus-wide one.

## Background worker

`cmd/dockvmap/worker.go`. `serve()` builds the jobs once with `scheduledJobs(workerDeps{…})`, hands their descriptors to `service.Worker` for System › Tasks, and runs each enabled job in **its own goroutine, with its own `time.Timer` and `select`**; never share one `select` across jobs.

### Job model

`scheduledJob{ name, description, interval, enabled, disabledReason, run, doneMsg, onShutdown, trigger }`:

- A job that config turns off stays in the list with `enabled: false` and a `disabledReason`, so System › Tasks can say why. Jobs that belong to another role are the exception and aren't listed at all: a follower has none of the primary's, and a standalone node has no replication jobs.
- `run` returns a count, logged with `doneMsg` when above 0. `executeJob` records the outcome in `worker_ticks` after every run, a recovered panic included, so a panicking job advances its schedule instead of re-firing on every restart; only a shutdown mid-run skips the write.
- First fire is `max(0, interval − since(lastRun))` plus a stagger of 10s per job (at most 30s), so a process restarted more often than `interval` still runs the job, and an overdue job runs once. Then it runs every `interval`.
- `onShutdown` runs on a clean stop with a 5s budget (`proxy-metrics-flush` saves its last delta).
- `trigger` is set by `deps.worker.Register(name)`, the one place that decides which jobs can run on demand; `POST /api/system/tasks/{name}/run` fires it.
- `name` is the `worker_ticks` key; keep it stable and unique.

Tag discovery scans and background refreshes run on the worker context but aren't waited for on shutdown; their `RecoverFromRestart` cleans up at the next start.

### The jobs

| Name | Interval | Gate | Does |
|---|---|---|---|
| `session-cleanup` | 1h | always | delete expired session rows |
| `tag-discovery-cleanup` | 24h | always | drop terminal `tag_discoveries` rows older than 7d |
| `background-failure-cleanup` | 24h | always | drop `background_failures` rows older than 30d |
| `image-tag-refresh` | `tags_check_interval` (default 24h) | interval > 0 (else disabled, with a warn) | the tag-refresh sweep; has a `trigger` for manual runs |
| `blob-cache-cleanup` | `blob_cache.cleanup_interval` (default 1h) | cache enabled | lifetime drop + max-size eviction; has a `trigger` for System > Tasks "Run now" |
| `blob-cache-orphan-scan` | 24h | cache enabled | drop cache files with no row; has a `trigger` for System > Tasks "Run now" |
| `tag-notification` | 5m | `smtp.enabled` or `webhooks` non-empty | send pending tag notifications; skips its turn while the tag sweep runs, so one sweep is one digest |
| `proxy-metrics-flush` | 5m | always | fold `Snapshot() − prev` into today's `proxy_metrics_daily` row and save pending `ProxyUsage`; `onShutdown` does both once more |
| `proxy-metrics-cleanup` | 24h | always | prune `proxy_metrics_daily` older than 30d |
| `agent-poll` | `agents_poll_interval` (default 30s) | not a follower | ask every host's agent for changes |
| `replication-changes-cleanup` | 24h | replication primary | drop recorded registry pushes and deletes older than 30d |
| `replication-reconcile` | `follower.reconcile_interval` (default 24h) | follower with an internal registry, interval > 0 | copy every tag on the primary this follower lacks, never delete |

A follower runs neither `image-tag-refresh`, `tag-notification` nor `agent-poll`: tag checks, notifications and agents belong to the primary.

## Frontend

`frontend/`: a Svelte 5, Vite and TypeScript SPA, hash-routed (`svelte-spa-router`), with cookie-session auth (no `Authorization` header anywhere). `frontend/dist` is embedded at compile time.

### State layering

`lib/api` (stateless fetch) → `lib/stores` → `lib/services` (the only code that mutates a store). Worth it only once two or more components share the data; otherwise page-local `$state` calling `lib/api` is preferred.

Services are module singletons (`auth`, `dashboard`, `tagRefreshStatus`, `theme`, `toast`, `updatesCount`), except state that belongs to one route, which gets a factory (`createTagDiscovery()`) so nothing leaks between mounts. A singleton a route drives exposes `start()` returning its cleanup, and resets its store there.

### Conventions

- Every route renders `<PageTitle title="…" />`.
- **Tabbed pages** (only System): `routes/System.svelte` owns the shell, title and tab bar for `/system/:tab`; each `lib/components/system/<Tab>.svelte` renders only its body and loads its own data.
- **Errors**: only `lib/api/client.ts` knows `ApiError`. A network failure becomes an `ApiError` with status `0`, so "unreachable" and "server said no" differ. Call sites use `errorMessage(err, "Failed to …")`, never `instanceof`.
- **Async state**: the loading, error, empty, content precedence lives once in `lib/utils/asyncState.ts`, consumed by `AsyncState.svelte` (lists and pages) and `dashboard/CardBody.svelte` (cards). Add a state to the resolver, never to one of the two.
- **List pages**: an incrementing request token (`Images.svelte`'s `loadToken`) stops a slow superseded response from overwriting a newer one. A skeleton shows on first load only (`loading && !loaded`); refetches keep the rows, dimmed (`busy`).
- **List state in the URL**: the hash query holds a list's filters, sort and page (`lib/utils/listQuery.ts`: `readListQuery`, `writeListQuery`, `pushListQuery` with `replaceState`, `watchListQuery`). A row carries the query into its detail page, which echoes it on the back link. No storage.
- **Sorting**: `createListView`'s `sortFields` (field → first-click direction) turns it on; `SortableHeader.svelte` cycles first direction, opposite, default. The sort survives "Clear filters"; changing it goes back to page 1. Only Virtual Images sorts so far; Tag doesn't, since versions don't sort as text.
- **Rows**: if a row has something to open, the whole row opens it and delete lives inside; otherwise its action sits at the end. Row actions are icons with a tooltip. Badges only show status. Blue text is only for links. Detail pages put each section in a card, ending with a Danger zone (`.danger-card`).
- **Filter bar**: each labelled control in a `.filter-field` with a `.filter-label` inside `<FilterBar>`, the search box bare. Controls are `class="input filter-control"`, with `class:is-active` when set.
- **Phone layout**: breakpoint `768px`. The sidebar becomes a drawer; tables scroll inside `.table-wrap`. A table too wide for a phone hides secondary columns with `.phone-hidden` and moves what matters under the first cell with `.phone-only`. Cells use `overflow-wrap: break-word`; a cell of long unbroken values opts into `anywhere` locally.
- **Shared CSS** (`app.css`): find the existing primitive before adding one: buttons (`.icon-button`), callouts (`.warning-box`, `.notice`), modal footers (`.modal-actions`), entry lists (`.entry-list`), rows and links (`.row-link`, `.row-name`, `.table-actions`, `.time-cell`), commands (`.command`, `.command-row` with `CopyButton.svelte`), headers (`.list-header`, `.detail-header`, `.heading-count`, `.back-link`), chips and states (`.group-label`, `.state-dot`), `.mono`. `.table-actions` goes on a `<div>` inside the last cell, never on the `<td>`, or the cell stops stretching and its border breaks. Danger and warning buttons use `--btn-danger-bg` and `--btn-warning-bg`, which stay saturated in dark mode.
- **`Field.svelte`'s `autofocus`** calls `.focus()` in `onMount`; the HTML attribute fires only on the first page load, not after client-side navigation.
- **Pollers**: `createPoller` (`lib/utils/poller.ts`) is the raw cancellable interval; `createVisibilityPoller` wraps it to pause in background tabs, and is preferred for anything that polls while a page is open. A poll that updates a loaded object merges it field by field (`mergeImage`), so a status-only tick doesn't churn child props.
- **Dashboard**: one request, `GET /api/dashboard`, with six sections, each a `{data, error}` envelope, so a failed query degrades one card. Cards are pure and take `DashboardCardProps<T>`, shaped by the `dashboardSections` store. Adding a card is a section on the server plus one line in the route.
- **Tag refresh status** (`lib/services/tagRefreshStatus.ts`) stays outside the dashboard aggregate, since it polls every 2s during a sweep. Watchers are refcounted, concurrent refreshes coalesce, and after 3 failed polls it reads Unknown. Use `onCompleted(cb)` rather than a hand-rolled running-to-idle detector.

### Updates page

`routes/Updates.svelte` lists every image with `updateAvailable`, one `UpdateReviewRow.svelte` each, with a `TagFamilyPicker` fed by `GET /api/images/{id}/tags?scope=newer`: the current tag's family from the newest down to the current tag, unfiltered, so the admin decides about prereleases. The row starts on the current tag, check-marked, and Update stays disabled until another tag is picked, as in `ChangeTagModal`. `showFirstSeen` puts a tag's first-seen date in its tooltip, only for tags newer than the image's first scan (those all share one date).

A group with a flagged member is one row (the same component with a `group` prop), listed above flagged ungrouped images on the first page only, so pagination stays per image; the row acts through its first member, since the image endpoints act on the whole group. `UpdatesBadge` and the dashboard count a group as one update.

A row stays after it's updated or pinned, offering Roll back or Unpin, so rows don't vanish under the user; a finished sweep reloads only when nothing was cleared, and `changePage` steps back by the cleared count (a few rows can repeat, none are skipped). Update, Roll back and Pin are disabled during a refresh, since `UpdateTag` waits on the same per-image lock. Tags are marked seen on update, not on load.

### User-agent parsing

`parseUserAgent` (`lib/utils/userAgent.ts`) wraps `bowser` into a device kind and a label like "Chrome 140 · Windows", for Profile's sessions and the audit log. Display only; the backend keeps the raw string.

### Traps

- **Lucide icons**: import from the icon's own subpath (`@lucide/svelte/icons/x`), never the package root (drags ~3800 files into `svelte-check`). Never pass `class` directly to an icon component; it's invisible to the parent's scoped CSS; wrap it in `<span class="icon">` and style that. A **bare** `:global(.icon)` leaks app-wide; only a compound form (`.parent :global(.icon)`) is safe. Same trap one level up: an icon arriving through a snippet from a parent (`DashboardCard`'s `icon` snippet) renders with the *parent's* scope hash, so a plain `.card-head-title svg` in the child never matches - it has to be `.card-head-title :global(svg)`.

- **`app.css` dark theme** is defined twice (media query + `[data-theme='dark']`), and `index.html` has a blocking inline `<script>` setting `data-theme` before first paint; both intentional (explicit choice beats system preference, and it avoids a theme-flash). Keep the `localStorage` key in sync between `index.html` and `theme.ts` if it changes.

## Design Details

### `internal/taganalyzer`

Pure tag-string analysis (`tokenize → classify → family → order`), with no I/O. It decides which tags group together, which is newest, how families show in the tag picker, and whether an upgrade is available. Its mistakes show up as wrong recommendations rather than errors, so correctness is critical.

`updateAvailableFor` (`internal/service/images_refresh.go`) skips a newer candidate when the selected tag's version axes are a prefix of the candidate's: `1.2` doesn't flag `1.2.6`, since `1.2` already tracks it, but flags `1.3.0`; `17-alpine-3` doesn't flag `17.9-alpine-3.23` but flags `17.10-alpine-3.23`; `10.11` doesn't flag a dated build of `10.11.11`. A base-only bump (`17.9-alpine-3.23` → `3.24`) is not a prefix, so it is reported. It compares `image_tags.version_segments`, each tag's version numbers per axis as JSON. Only the recommendation is suppressed, never the ordering.

Before changing grouping, classification or ordering, re-measure with `go run ./cmd/tagaudit` and keep every invariant counter at zero. Bump `taganalyzer.AnalysisEpoch` too: the refresh skips analysis for an unchanged tag set (`images.tag_set_hash`), and the epoch is part of that hash.

#### Guarantees

- Every tag belongs to exactly one family.
- Family IDs are unique within a repository.
- A family never mixes different values of a literal/named segment.
- Ordering never inverts the leading version.
- `Analyze` is deterministic: the same tags in any input order produce a byte-identical result.
- Results are safe to compute concurrently.

They hold with zero violations on hundreds of thousands of real tags from about 200 repositories, and on a second random corpus the analyzer was never tuned on.

A `Family` (`Kind`: `blood` / `ancestor` / `step`) is a **structural lineage**, not runtime compatibility: the tags look like versions of the same named thing. The analyzer can't know whether two versions are compatible, so it never excludes a match on that assumption; whether an upgrade is safe is the user's call.

Only version-shaped segments (numbers or dates) may vary within a family. Literal/named segments such as `noble`/`jammy` or `jdk`/`jre` must match exactly and are never treated as upgrade axes.

#### Blood families

`blood` is the strongest family relationship: every member must have an identical computed identity pattern.

The identity pattern is based on segment shapes rather than literal values. Version segments collapse to `solo`, `multi`, or `calver`; prefixes and suffixes remain literal; date/time segments collapse to a constant; confirmed hash segments also collapse to a constant.

A bare number such as `11` is treated as a multi-part version only when `collectKnownMajors` finds evidence that `11` is a real major version elsewhere in the same repository, such as `11.0.24`. This evidence is scoped to the preceding segment context rather than the raw segment index, preventing unrelated tag shapes from influencing one another.

Hash-shaped segments (`^g?[0-9a-f]{7,40}$`, the `g` from `git describe`) are normalized only when `collectHashLengths` confirms the slot from the repository's own tags: at least 5 distinct values of one length at one position, one of them with `a`–`f`, so long numeric build counters aren't taken for hashes. A ratio heuristic was tried and did worse on repositories with architecture fan-out. Normalization happens at identity time: `normalizeHashSegments` rewrites confirmed hashes as literals once repository statistics exist, so a SHA like `a953921` is never ordered as a number.

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

Retune these against the corpus, never a single repository.

#### Family identity and ordering

`Family.ID` is an FNV-64a hash of `Family.Key`, masked to 52 bits so it stays JSON-number safe. It is not a per-run counter, so a family keeps the same ID across refreshes as long as its pattern is unchanged.

`Family.HasOrder` is false when the only varying axis is a confirmed hash. Such tags can still be ordered for display, but the order has no semantic meaning, so `updateAvailableFor` does not report an upgrade for the family.

A tag that relates to nothing is a singleton with `Kind: FamilyStep`, kept for compatibility; it means no relationship was claimed.

#### Recognized version conventions

In addition to standard `MAJOR.MINOR.PATCH`, the parser recognizes several glued suffix conventions that affect grouping:

- PEP 440 `aN` / `bN` pre-releases
- OpenBSD/OpenSSH-style `pN` patches
- Pre-JEP 223 Java `uN` updates, such as `8u422`
- Alpine/apk `-rN` package revisions
- Debian/Ubuntu same-day rebuild suffixes such as `20180112.1`

The Alpine revision is absorbed into the preceding version, so `1.2.3-r1` stays in the same family as `1.2.4`. A leading branch number such as `r409-...` is unaffected. Likewise `20180112.1` is the same date identity as `20180112`, with `.1` affecting ordering but not family identity.

### Image tag refresh

Every trigger (the worker sweep, `-refresh-tags`, `Images.Create`, a manual refresh) goes through `RefreshAvailableTags` (`images_refresh.go`). It holds the per-image lock (`imageRefreshLocker`, shared with `UpdateTag` and `Rename`) for at most 10 minutes and sets `images.refresh_status` to `running`, then `idle` with a `WithoutCancel` write so a shutdown can't strand it. The lock is the real guarantee; `refresh_status` is a UI hint. `Images.RecoverFromRestart` resets a `running` left by a crash.

A manual refresh (`POST /images/{id}/refresh-tags`) claims `refresh_status` with a compare-and-set (`202 {status:"running"}` if one already runs) and runs on `Images.bgCtx`, not the request context, which dies at the 45s `WriteTimeout`. The handler waits about 2s for a quick result before answering `202`. Manual refreshes are silent: no notifications.

**Pinned images:** a pinned image is still refreshed and still emits `TAG_ADDED` / `TAG_REMOVED`, but `effectiveUpdateAvailable`, the one wrapper around `updateAvailableFor`, reports no update, so no `UPGRADE_AVAILABLE` fires. `SetPinned` recomputes through it in one transaction; unpinning re-derives the flag from the stored tags without a fetch.

**Current digest** (`images.current_digest`, `current_digest_tag`, `current_digest_since`; `images_digest.go`): each check sends one manifest `HEAD` for the selected tag, before the unchanged-tag-set shortcut, so a tag republished under the same name is caught. It accepts the same media types Docker sends, so a multi-arch tag returns the index digest, which is what hosts record in `RepoDigests`; on Docker Hub a `HEAD` doesn't count against the pull limit. Each lookup is capped at 10s. A missing digest header is stored as unknown; a failed lookup is recorded as a `digest` failure and never fails the check. `UpdateImageDigest` writes only when something changed and only while `images.tag` still equals the looked-up tag; readers trust the digest only when `current_digest_tag` equals `images.tag`. A tag change starts a background lookup, so the new digest is known within seconds.

**Served digest:** a manifest `HEAD` or `GET` through the proxy for the image's tag, from a client that accepts an index (`oci.AcceptsIndex`), whose digest differs from `Image.TagDigest()` is saved in the background (`Images.RecordServedDigest`). Without it, a tag republished between two checks would make every host that pulled it read Outdated until the next check.

**Current tag gone:** when the unfiltered tag list no longer has the selected tag, the check stores "tag X no longer exists in the registry; pick another tag" as `last_check_error` and skips the digest lookup. `ListTags` fails rather than return a partial list, so a missing tag is really missing. Changing the tag of an image with a check error starts a full background check, so the error clears within seconds.

**Frontend:** `ImageDetails.svelte` polls the image every 2s while `refreshStatus` is `running`. `ChangeTagModal` takes `refreshStatus` as a prop and reloads its picker once, keeping the selection, when it returns to `idle`.

### Image groups

A group keeps images that release together on one version (Nextcloud AIO's six images, Immich's server and machine learning). It is a strict lock: members always share `images.tag` and `images.pinned`. The tag stays on each image rather than on the group, so the proxy's resolve path and the refresh pipeline do not know groups exist. An image is in at most one group. Membership is manual because a shared namespace does not imply a shared version (Grafana, Supabase, mailcow).

**Enforcement lives in `Images`, not the UI.** `UpdateTag` and `SetPinned` go through `lockImageScope`, which resolves an image to its whole group (or to itself when ungrouped) and applies the change to every member in one transaction, so `ChangeTagModal`, tag history restore and the Updates row cannot split a group. `GetTags` returns the tags on every member for a grouped image, which is how those callers offer only common tags without knowing about groups. Matching is by exact tag string; family and order come from a lead member (`groupLead`). `GroupTagUnavailableError` wraps `ErrTagUnavailable` and names the members missing a tag, and nothing is written in that case. Each member still gets its own history row and `IMAGE_TAG_CHANGED` audit row (with a `group` field).

**Locking.** Holding several per-image locks must not deadlock or outlive the 45s `WriteTimeout`, and a refresh holds its lock through the upstream fetch. So `lockAll` takes every lock with `TryLock`, releases all on any miss, retries for `imageGroupLockWait` (5s), then returns `ErrImageGroupBusy` (`409`). A single ungrouped image still blocks on its lock as before. `lockImageScope` re-reads the scope after locking and retries if membership changed.

**State is computed on read.** `images.update_available` stays per member and refresh is untouched, so tag events stay per image. Webhooks fire per image; emails collapse a group into one digest line (see `notification_digest.go`). `applyGroupState` derives the group's state: flagged when any member is flagged, `UpdateAvailableTag` is the newest upgrade candidate present on every member, and `PartialTag` is a newer tag only some members have. Flagged with no `UpdateAvailableTag` is the blocked state. It must stay visible because a replaced member (Harbor's `redis-photon` stopped at `v2.15.1` when `valkey-photon` took over) would otherwise silence the group forever.

**Membership.** `saveGroup` validates and writes name, membership, tag and pin in one transaction, holding the locks of old and new members. Joining moves an image to the shared tag. Any pinned member pins the whole group, since a pin is an explicit decision not to move. A removed image keeps its tag and pin. Deleting a group keeps its images (`ON DELETE SET NULL`), and deleting the last member deletes the group.

**Proxy.** `resolveCache` holds an image for 20s, which would let a `compose pull` mix versions right after a group change. `Images.OnTagChanged` invalidates the changed names after commit. A generation counter stops a lookup that started before the commit from re-caching the old tag.

**Refresh.** `StartBackgroundRefresh` on a grouped image refreshes every member sequentially with the requested image last. Its `refresh_status` was already claimed as `running`, so that one status covers the whole group for the existing pollers.

**Frontend.** `ImageGroupModal.svelte` creates and manages groups; `GET /api/image-groups/common-tags?imageIds=` feeds its picker for a membership that doesn't exist yet. `routes/Groups.svelte` lists every group. `suggestImageGroups` (`lib/utils/imageGroupSuggestions.ts`) offers ungrouped images with the same registry, namespace and tag, skipping floating tags like `latest`, which say nothing about a shared version; dismissals are per browser. Virtual Images stays one row per image, with a group label, a Group filter, and search matching group names.

### Tag discovery

`internal/service/discoveries.go` (table `tag_discoveries`) backs the "Add virtual image" wizard's repository step.

`Check` does a cheap single-page `oci.Client.CheckRepository` first, never the full `ListTags` pagination, so a huge repository can't make "check repository" itself slow. The real scan runs in a goroutine off the worker context (capped at 10 minutes, recovers its own panics), writing into a `tag_discoveries` row keyed `UNIQUE(registry_id, repository)`; that row is both the cache (TTL'd via `tag_discovery_ttl`, refreshed in the background without clobbering the previous result on failure) and the dedup lock (insert-or-reclaim on conflict, so concurrent requests for the same pair converge on one scan).

`Check` waits inline on the row for ~2s before falling back to `status: "running"` for `GET /api/discoveries/{id}` to poll, so small/cached repos still resolve in one round trip. `RecoverFromRestart` (startup) fails any row left `running` by an unclean exit; the `tag-discovery-cleanup` job prunes terminal rows older than 7 days. Shutdown does not wait on in-flight scans (see [Background worker](#background-worker)).

`Images.Create` takes the tags from a completed discovery, so saving doesn't scan again. Live progress (`tagsSeen`) is only in memory. `raw_tag_count` next to `tag_count` gives the number of filtered tags (`ignoredCount`).

### Registry browsing

`internal/service/registry_browser.go`, `routes/RegistryBrowse.svelte` and `lib/components/RepositoryTagsModal.svelte`. Read-only.

**Listing.** `oci.Client.ListRepositories` reads `/v2/_catalog` through `Do`, so it gets the stored login, the registry options and the bearer flow for free. It follows `Link` on the same host only and stops at 1000 names.

**Which registries are browsable.** No list of known hosts: `Browsable` asks each registry for one catalog entry (8 at once, 5s each) and caches the answer for an hour, a refusal included (Docker Hub refuses at its token endpoint, which isn't an `*oci.Error`). A network failure isn't cached, so a registry that's down is asked again. Trap: an edit or delete must drop the answer and `Forget` both hosts in the `oci.Client` (`Registries.OnChanged`), or the next check runs with the old login from the client's 30s credential cache.

**Virtual images per registry.** The image and failed-check counts are subqueries in `registryInfoSelect`, so every registry read carries them and they can't go stale. `ErrRegistryInUse` is the real delete guard; the disabled Delete in `RegistryModal` is only a hint.

**Frontend.** The Browse icon sits in a fixed-width column, so rows don't shift when the browsable ids arrive. The tags modal and Add virtual image reuse tag discovery and `TagFamilyPicker`; `/images/new?registryId=&repository=&tag=` prefills the wizard. `registryLabel` (`lib/utils/registries.ts`) is the one "Internal registry (address)" label.

### Hosts and the agent

Visibility only: the agent reads Docker and answers, the controller (dockvmap) polls, stores and compares. The controller connects to the agent, never the other way, so a host network (a DMZ) never needs a route to the controller, and the agent has no write path to Docker to protect.

**Packages.** `internal/agentapi` is the wire contract both sides share (paths, limits, `ProtocolVersion`, `ParseImageRef`, identities). `internal/agent` imports only `agentapi` and the standard library, so the agent binary stays small and never links the controller. `internal/agentclient` is the controller's side; `cmd/dockvmap-agent` is the entry point.

**Pairing.** Mutual TLS 1.3 with pinning, no tokens. Each side has a self-signed identity, valid until 9999 since it's pinned, not chained: the agent keeps `agent.pem` and `paired.json` in its data path, the controller keeps `agent_client.pem` (covered by `-backup`). The agent pairs with its first caller (`adopt`) and refuses any other fingerprint with `403`. Adding a host with its address is the consent, so the controller pins whatever answers first. An identity change is never accepted silently: the handshake aborts (`ErrIdentityChanged`) until someone clicks Trust new agent, which is audited. One TLS connection per poll, without keep-alive, so the pin is checked every time and no pool goes stale. An agent that lost `paired.json` but kept its certificate is adopted again, since the pin still holds.

**Polling** (`hosts_poll.go`). `agent-poll` checks every host each `agents_poll_interval`, 8 at a time, 10s each, with a per-host lock against live polls. Host details polls live (3s) unless the last poll failed. An unchanged report gets `304` (`If-None-Match`). A changed report replaces the host's containers in one transaction; one with a Docker error keeps the last known ones, and one that fails validation (`snapshotFromReport`) writes nothing. Poll errors are stored on the host, not in the failure log, since the Hosts page shows them. Container state is bounded text, not an enum, so an unknown state can't reject a report. Bump `ProtocolVersion` only for an incompatible change; a refused protocol still records the agent's version, so the UI says "Update agent". Addresses are normalized and unique (`normalizeAgentAddress`).

**Agent** (`agent.go`, `server.go`, `docker.go`, `snapshot.go`). Docker `GET`s only, API 1.41 or the daemon's minimum, 5s each, below the controller's 10s, so a hung daemon shows as "Can't read Docker". A cache fed by `/events` keeps details between polls, so a steady host costs one container list. Traps: a fill carries the cache's epoch and is dropped if a reset happened meanwhile; while the event stream is down every poll reads in full; healthcheck `exec_*` events are ignored. The running image comes from inspect's `Config.Image`, since the list shows a bare `sha256:` once the tag moved.

**Version status** is computed on read (`track`), never stored, so a tag change shows on every host at once. A container is tracked when its image host is the proxy address (`proxy_public_host`). `versionStatus` compares its running digests with `Image.TagDigest()`: a match is `upToDate`, a local image with that digest is `pulledNotRecreated`, anything else is `outdated`; no digest is `unknown`, and a name that isn't a virtual image is `unknownImage` (or tracked against the internal registry, see below). Trap: `knownDigests` counts the image ID as a digest, because on the containerd image store the ID is the manifest digest and an image loses its `RepoDigests` once its tag moves; without it an old container reads `unknown` instead of `pulledNotRecreated`. A tag other than the virtual tag sets `pinnedTag`.

A host is online when a poll succeeded within 3 intervals on the current protocol, and current (`Host.Current`) when Docker was also readable; anything else is last known data, shown but never counted. A rollout counts running or restarting containers on the virtual tag with a known version, on current hosts: pinned or stopped ones would keep it from completing, while a crash-looping one must count so a bad deploy shows. `HostsOffline` and `HostsDockerError` come with every rollout, so a complete number can't hide a host nobody can check. `HostContainer.Crashed` is the one crash rule (dead, restarting, or a non-zero exit other than 137 and 143).

Redeploy commands come from `lib/utils/hosts.ts`: `redeployCommand` for one container and `redeployGroups` for the To redeploy card and Host details' banner, one command per host, compose project and folder: `docker compose pull` only when one is `outdated`, then `up -d`. They `cd` into the folder the agent reads from `com.docker.compose.project.working_dir`, so a stack started with `-p` or `-f` needs its own command. A container that needs either step says "Needs a redeploy to run <tag>".

**Frontend.** `routes/Hosts.svelte`, `routes/HostDetails.svelte`, `imageDetails/RunningOnCard.svelte`, `hosts/RolloutSummary.svelte` (the one "N of M to redeploy" wording) and the dashboard cards. Dashboard order: a summary sentence; Needs attention (To redeploy and Updates available side by side, then Problems, which lists hosts that aren't current before failures); three Overview tiles; Proxy activity and Recent tag activity. Each number appears once on the page. Version and crash rules come from the server; display rules live in `lib/utils/hosts.ts`. Hosts shows a hint when online hosts report containers but none through the proxy address, the usual sign of a wrong `proxy_public_host`.

**Tests.** `internal/agent`'s tests run the real agent against a fake Docker on a unix socket; host service tests use a fake `agentDialer`. End-to-end checks use a real agent container against a scratch controller, never a second Docker daemon on the host network.

### Internal registry

DockVMap fronts one official `registry:3` (`registry:2` works the same) on its stack network: names that aren't virtual images pass through to it, pushes included. The registry keeps storage, uploads and garbage collection; DockVMap adds TLS, proxy-token auth, an audit entry per pushed tag and a tag check after a push. DockVMap never logs in to the registry. Plain HTTP by default; `internal_registry.tls` switches to HTTPS with normal certificate checks, to take over a registry that already serves TLS. Config is the only switch: the UI can't change which registry is internal, and a bad `push_allowed_ips` entry fails startup.

**Service** (`internal/service/internal_registry.go`).
- At startup `EnsureInternal` keeps one registry entry marked internal (partial unique index, migration `035`): it creates it, takes over an entry with that address, or moves the marked entry to a new address, so virtual images, which reference it by id, follow. Startup fails on an invalid address or when another entry has the new one. `Registries` refuses to edit or delete the marked entry (`ErrRegistryManagedByConfig`, 409).
- `StartChecking` retries `GET /v2/` every 10s until the registry answers; never fatal, recorded once in the failure log.
- `Pushed` and `Deleted` (a tag `PUT` answered `201`, a manifest `DELETE` answered `202`) write the audit entry in the background right away and restart that repository's tag-check timer: 5s after the last change, at most 30s after the first, so a build pushing several tags starts one check per virtual image. `Stop` cancels pending timers (the next scheduled check covers those tags) and waits for audit writes.
- `TagDigest` returns the last known digest of `repository:tag` and refreshes it in the background when it's missing or older than 5 minutes; a push or delete marks the repository stale. It never makes a page or a poll wait.

**Proxy** (`internal/proxy`). One request path (`serve`) for the feature on and off; off, writes get `405` and the server keeps its 15s `ReadTimeout`. On:
- `parseRegistryPath` (`path.go`) classifies every path, upload paths included. Invalid names get `400 NAME_INVALID` before reaching the registry.
- A virtual image's name goes to the virtual image handlers, which refuse writes. The exception is a virtual image built on the internal registry's repository of the same name (`myapp` on `myapp`): writes and `tags/list` pass through, apart from a `PUT` of the virtual tag. Every other virtual image answers `tags/list` with its virtual tag only, from the image the request already resolved (no database read, no upstream call), so skopeo, crane, Renovate and diun work and their digest tracking follows DockVMap.
- Writes need an address in `push_allowed_ips` when it's set (the client address resolved with `trusted_proxies`, only for writes) and, with `proxy_auth` on, a token with Can push. Either failing is `403 DENIED`, so the list only narrows who can push. The "accepts pushes from anyone" warning shows only with `proxy_auth` off and an empty list.
- `passthrough.go` (`httputil.ReverseProxy`). Traps: the registry builds upload `Location`s from its own host, which clients can't reach, so they're rewritten to relative paths; a registry `401` becomes a `502` without `WWW-Authenticate`, or Docker starts a login loop the registry can't satisfy; the client's `Authorization` never reaches the registry. `ErrorHandler` always answers, naming certificate problems, an unreachable registry or an upload that ended early.
- Timeouts: with the feature on, the server's `ReadTimeout` is `0` and each request sets its own 15s read deadline, moved a minute ahead while upload data arrives, so a slow large layer finishes and a stalled one is cut off (HTTP/1.1 and HTTP/2).
- Virtual images on the internal registry skip the blob cache, since the registry is on the same machine. Pass-through requests don't touch the proxy counters or pull tracking, so the dashboard stays about virtual images.

**Hosts.** A container on the proxy address whose name isn't a virtual image is tracked against the internal registry (`trackInternal`) with `TagDigest` and the same `versionStatus`. It counts in its host's rollout, never in image rollouts. Without a tag, running digests or a known digest it stays `internalRegistry`, outside rollouts.

**Frontend.** `imageSource` (`lib/utils/registries.ts`) shows "Internal registry · team/web" wherever an image's source appears. The registry's edit window is read-only and says it's set by config. New token shows Can push, and Proxy Tokens its Access column ("Pull" / "Pull and push"), only while the feature is on; the audit tab shows the token, or Anonymous, as the actor of pushes and deletes.

### Replication

A follower is a headless read replica of what the proxy needs: registries with their logins, virtual images, groups, proxy tokens and the internal registry's content. Accounts, the audit log, hosts, tag history and tag checks stay on the primary.

**State.** The primary keeps one version row (`replication_state`: cluster ID, version, random tag). SQLite triggers on every replicated table, limited to the replicated columns and real changes (`WHEN OLD.x IS NOT NEW.x`), bump it in the same transaction, so no write path can forget it and tag checks or pull stats never wake followers. `TestReplicatedTablesHaveTriggers` fails when a replicated table lacks them. Pushes and deletes through DockVMap bump a per-repository counter (`replication_repositories`) and record the tag or reference (`replication_changes`, kept 30 days) in `InternalRegistry`'s background write, never on the request path.

**Sync.** The state is small (about 100 bytes per virtual image; 50 KB with 500), so there is no change log for the database: a follower long-polls `state?version=V&tag=T&cluster=C` and gets `304` after up to 25s, or the whole snapshot when the version moved. One shared reader checks the version row once a second, only while followers wait, which also catches changes from another process like `-refresh-tags`. `ApplyReplicationSnapshot` writes only differing rows, with the primary's row IDs (the internal registry row keeps the follower's own address), in one transaction with the applied version. A snapshot from another cluster (a first sync, or `-resync` over a node's own data) replaces the rows instead, since equal ids mean nothing across clusters. Registry logins travel inside the pinned TLS connection and are re-encrypted with the follower's key; they're compared decrypted, since every encryption uses a new nonce.

**Safety** follows MySQL's rules: a version or tag behind the follower's, or another cluster ID, is a `409`, and the follower stops, keeps serving and waits for `-resync`; it never follows a primary backwards. A follower with its own data and no history waits for `-resync` too. The setup comparison (`replicationapi.Setup`, built in `cmd/dockvmap/replication.go`) covers only settings that change what the proxy serves, and only settings both sides send, so a newer primary's extra setting doesn't stop older followers. `REGISTRY_STORAGE_DELETE_ENABLED` is probed: deleting a manifest that can't exist answers `405` with deletes off, `404` with them on.

**Registry copies.** When a repository's counter moves, the follower asks `changes?repository=R&after=N` and replays just those pushes and deletes, so a CI repository with hundreds of tags costs one copy per push. When a counter after N is no longer recorded (an outage past 30 days, a restored primary), it compares the whole repository instead. Until its first copy is done, again after `-resync`, `dueRepositories` adds the catalog's repositories with no recorded push: an upgraded primary's images and pushes made around DockVMap. Registry content is deleted only by a replayed delete, never because something is missing, since a registry volume can be lost or restored apart from the database. A virtual image on the internal registry keeps its old tag until the new content has arrived (`holdMissingContent`). Pending copies aren't stored: they're recomputed from the copied counters at every start.

**Pull reports** flow the other way: each minute a follower posts the pulls and token uses that changed since the primary last took a report (`follower_pulls.go`). `RecordProxyUsage` keeps the newer time, compared in Go since stored times don't sort as text, so repeats and outages are harmless and nothing is queued. Row IDs match only while the follower follows that primary, so a stopped follower doesn't report.

**Readiness** (`/health/ready`) means "can stand in for the primary": database, registry, matching setup, synced once and the first copy done. Lag and a stopped sync are reported, not part of readiness, because a dead primary makes every follower lag exactly when failover needs it. The endpoint has no auth, so it carries setting names, never values or follower addresses.

**Ports.** The replication port is its own listener (the proxy port stays a plain registry), with a self-signed identity from `agentapi.NewIdentity` whose fingerprint is in the token, so a follower verifies the primary without certificates or names. `replication.allowed_ips` is checked on `RemoteAddr` before the token, with no `trusted_proxies` step, since nothing proxies this pinned TLS port. A follower's web port has no `/api/` or frontend: `web.New` registers `/health`, `/health/ready` and a plain-text page naming the primary (`followerPage`); the frontend knows nothing about followers. A node that starts as a follower clears its own cluster ID, so if it later becomes a primary it gets a new cluster.

## Conventions (backend, when adding code)

- **New JSON POST/PUT handler** → use `internal/web/api_routes.go`'s `decodeJSON[T](rw, r) (T, bool)`, don't hand-roll `MaxBytesReader`/`DisallowUnknownFields`.

- **New periodic background job** → add a `scheduledJob{…}` in `scheduledJobs` (see [Job model](#job-model)); a service it needs goes on `workerDeps`.

- **New store constraint check** → use `isUniqueConstraintErr`/`isForeignKeyConstraintErr` (typed `sqlite.Error.Code()`), never match on `err.Error()` text.

- **New audit-worthy action** → call `recordAudit`; actor/request info comes from `context.Context` (`service.RequestInfo`/`CurrentUser`), never pass as params.

- **New list filter** → embed `model.Pagination` into a per-domain filter struct (see `model.ImageListFilters`); don't add a shared generic `Filters` type.

- **New sortable list** → embed `model.Sort` next to `model.Pagination` in the filter struct, parse it with `parseSort(r, fields...)` (`internal/web/sort.go`, `sort` + `order=asc|desc`, unknown values are a `400`), and give the store query a `sortSpec` (`internal/store/querybuilder.go`): each field maps to a fixed SQL expression, `nullable` sorts missing values last in both directions, `fallback` is the list's existing order (used when no sort is chosen, so the API default never changes), and `tieBreak` keeps paging stable. Field names live in `model` (see `model.ImageSortFields`) so the handler and the store share them. No request value ever reaches the SQL. Frontend: spread `NO_SORT` into the `createListView` defaults, pass `sortFields` (field → first-click direction), add `...sortParams(q)` to the fetch, and render `SortableHeader` for each sortable column.

- **New date-range filter** → bind through `internal/store/querybuilder.go`'s `sqliteDatetime()` (see [Traps](#traps)).

- **New web-layer dependency** → a field on `web.Dependencies`, not a new parameter to `web.New`; the same for the service constructors that take a deps struct.

## Traps

- **A `httputil.ReverseProxy` `ErrorHandler` that returns without writing answers `200 OK`.** When a read deadline cuts off an upload, Go cancels the request context first, so the handler sees "context canceled"; returning early there told the client a broken upload had succeeded. Always write a status. `ReverseProxy` also wraps the request body before calling the handler, so don't type-assert your own body wrapper there.

- **Interim `1xx` statuses reach wrapped `ResponseWriter`s.** `ReverseProxy` forwards `100 Continue`; a wrapper that treats the first `WriteHeader` as final logs `100` and stops a later panic from answering `500`. `httptest.ResponseRecorder` also treats a `100` as final, so test this with a real server.

- **Migrations that rebuild a table drop its triggers.** SQLite's copy, drop and rename pattern loses the replication triggers on that table; recreate them in the same migration (`TestReplicatedTablesHaveTriggers` catches a miss).

- **A primary's replication version can be 0.** Data from before migration 037 bumped nothing, so an upgraded primary stays at version 0 with an empty tag until its first change; "synced once" on a follower is a cluster ID in `follower_state`, never a version above 0.

- **A nil service pointer in an interface field isn't nil.** Optional services handed to `web.Dependencies` (`CacheUsage`, `Replication`, `Follower`) are set only when non-nil; assigning a nil `*service.X` makes `w.x != nil` true.

- **`sqliteDatetime()`**: several tables set timestamps with SQLite's `DEFAULT CURRENT_TIMESTAMP`, whose text doesn't compare with the driver's `time.Time` format, so bind date-range filters through `sqliteDatetime()`.

- **SSRF**: a registry host a user supplies must be checked against the configured registries (`GetRegistryInfoByHost`) before the server requests it (see `Discoveries.Check`).

- **`trusted_proxies` append vs passthrough**: it only protects you if the reverse proxy **appends** to `X-Forwarded-For` (nginx's `$proxy_add_x_forwarded_for`, not `$http_x_forwarded_for`). Backwards, an attacker's own header is trusted, defeating the login limiter and the audit log's address, and DockVMap can't detect it. `gateway` only helps if the published port is reachable only through the proxy; otherwise a direct hit also arrives from the gateway.

- **Single user, no roles**: there is deliberately no API to create, list or delete other accounts. Ask before adding one.

- **`RegistryOptions` JSON keys are snake_case** while the rest of the API is camelCase; don't "fix" it in one place.

- **`rewriteTimestamp`** (`tokenizer.go`) folds ISO 8601 timestamps (`20240101T120000`, `2024-01-01T12-00-00`, with or without a `RELEASE.` label) into the 14-digit form before tokenization, since the classifier only knows `20060102150405`. `pretokenize` applies every rewrite in turn.

- **The tokenizer splits on `-` and `_`, never `.`**, since a version's dots must stay in one token. A `.`-glued qualifier like `7.0.10.amd64` is split by `splitDottedToken` at the rightmost dot where one side has a known shape.
