# Contributing

DockVMap is maintained by one person. Bug reports and feature ideas are welcome as [GitHub issues](https://github.com/septi0/dockvmap/issues); pull requests aren't accepted for now. To report a security problem, see [SECURITY.md](SECURITY.md).

The rest of this guide describes how the project is built, for anyone reading the code or working on a fork.

## Overview

dockvmap ("Docker Virtual Mapper") is a Docker/OCI registry proxy: clients pull a stable virtual tag (`myimage:current`) that DockVMap maps to a real upstream tag chosen per image. One binary runs the proxy (OCI Distribution API), the web server (REST API and embedded frontend) and a background worker, sharing one SQLite database and one registry HTTP client.

## Principles

A professional app that is useful, intuitive, secure and fast, without being over-engineered. These rules apply to anyone changing the project:

- **Useful, not a gimmick.** Every feature solves a real problem for someone running DockVMap. If its value isn't obvious, it doesn't ship.
- **Intuitive.** A feature is clear from the UI without the docs: plain wording, sensible defaults, errors that say what to do next.
- **Secure by default.** Least privilege, read-only wherever possible, validate every input, never send or log secrets.
- **Fast and light.** Nothing needless on hot paths; background work never touches the proxy request path. Measure changes that add load.
- **A plain registry on the proxy port.** The proxy port speaks only the OCI Distribution API, as close to a standard registry as possible, so every client works unchanged. DockVMap's own endpoints go on the web port or a port of their own.
- **Not over-engineered.** The simplest correct design. No options, layers or guards for needs or edge cases a homelab or small DevOps setup won't have.
- **Reuse first.** Find the existing helper, component, style or test fixture before writing one. One rule lives in one place.
- **Tidy and readable.** Clear names, small functions, no clever tricks, no dead code, no hacks: fix the cause, not the symptom. Remove what a change leaves unused in the same change.
- **Respect the layers.** `store` → `service` → `web` / `proxy` on the backend; `lib/api` / `lib/stores` / `lib/services` on the frontend; the agent imports only `internal/agentapi` and the standard library.
- **A live production app.** Config, API and database compatibility matter. Migrations are forward-only and come with a backup note; refactors keep behaviour identical.
- **Match the surrounding code.** Same naming, idiom and formatting. Code clear enough to need few comments; where one helps, a single short line. No em dashes in UI, code, docs or commits.
- **Verify honestly.** `make verify` plus end-to-end checks (headless browser at desktop and phone widths) on scratch data, never on real data or config. Failures are reported as they are.

## Commands

`make help` lists every target. The common ones:

- `make build`: the frontend and both binaries (`bin/dockvmap`, `bin/dockvmap-agent`).
- `make dev`: the backend and the Vite dev server together. `CONFIG=<path>` picks a config, otherwise `config/config.yaml`, seeded from `config.sample.yaml`. With `data/agent/` present it also runs an agent for this machine's Docker on `127.0.0.1:9002`.
- `make verify`: gofmt, vet, golangci-lint (if installed), the tests, svelte-check and tsc.
- `make test-integration`: replication end to end in Docker, with two DockVMap processes, a `registry:3` each and a third as upstream. Its containers are named `dvm-testlab-<pid>-*`, and it removes only those.

`frontend/dist` isn't committed and `//go:embed dist/*` needs it, so on a fresh checkout a raw `go build`, `vet` or `test` fails until `make build-frontend` has run; the Makefile targets handle it. `internal/taganalyzer` is also checked with `go run ./cmd/tagaudit -verify`.

**Releases:** a `v*.*.*` tag makes `.github/workflows/docker-image.yml` build `ghcr.io/septi0/dockvmap` and `ghcr.io/septi0/dockvmap-agent` for amd64 and arm64. `CHANGELOG.md` has one short section per version, user-visible changes and upgrade notes only; `(unreleased)` becomes the date in the release commit.

## Process model

`main()` → `run()`: shared bootstrap (config, logging, data folder, credential key), then either a one-off command or `serve()`. `serve()` is the composition root: the store, the services, then the proxy, the web server, the worker and, on a primary, the replication server, all on one `*http.Client` and one `*sql.DB`. A follower runs the sync loop and copier instead of the replication server.

Shutdown stops every server together and gives worker jobs 10s; a listener failure takes the same path and exits 1. Startup failures return instead of calling `os.Exit`, so deferred cleanup runs.

One-off commands (`cmd/dockvmap/commands.go`) build only what they need and never start a server:

- `-reset-password <username>`: a new random password, and every session of that user ended. Not on a follower, which has no accounts.
- `-refresh-tags`: one tag sweep. A failed image doesn't stop the batch, and the command exits non-zero if any failed.
- `-backup <path>`: runs before migrations, so it snapshots the schema as found, with `VACUUM INTO`. Restoring is stop, replace, start.
- `-resync [-yes]` (follower only): see [Replication](#replication).

## Architecture

Strict layering, one direction: `internal/store` (SQL) → `internal/service` (business logic) → `internal/web` / `internal/proxy` (HTTP). `internal/oci` stays independent of store and service through small interfaces, bridged in `cmd/dockvmap/adapters.go`. Constructors with several dependencies take a deps struct (`NewImages(ImagesDeps{…})`).

| Package | Purpose |
|---|---|
| `cmd/dockvmap` | entry point, `serve()`, one-off commands, worker jobs, adapters |
| `internal/config` | `Load()` merges env, file and defaults. An unknown key in the file fails startup. Settings that default on are `*bool`, so an older config still gets the default. |
| `internal/store` | SQL and numbered, checksum-verified, forward-only migrations. Refuses a schema newer than the binary, so an older build never runs against or backs up a newer database. |
| `internal/service` | business logic |
| `internal/proxy` | the OCI Distribution API: resolves virtual tags, forwards upstream, passes other names to the internal registry |
| `internal/web` | REST API and the embedded frontend; `/health` and `/health/ready` need no login |
| `internal/oci` | registry client: auth challenges, Docker Hub quirks, per-host caches. It sends only bodyless `GET`s, so retrying `429`/`503` is safe. |
| `internal/ocicopy` | copies one tag between two registries, for replication |
| `internal/taganalyzer` | pure tag-string analysis, no I/O |
| `internal/tagfilter` | exclusion patterns from `filters.yaml`, applied once where tags are fetched |
| `internal/blobcache` | optional on-disk blob cache keyed by digest, trimmed only by its cleanup job |
| `internal/httpmw` | middleware shared by web and proxy: panic recovery, access log, client IP |
| `internal/ipmatch` | IP and CIDR lists, so every address setting reads entries the same way |
| `internal/smtp`, `internal/webhook` | notification delivery |
| `internal/agentapi`, `internal/agent`, `internal/agentclient`, `cmd/dockvmap-agent` | the host agent, its wire contract and DockVMap's client |
| `internal/replicationapi`, `internal/replication` | replication wire contract and its HTTP ends |
| `cmd/tagaudit`, `cmd/taglist` | offline tools for checking tag analysis against real tags |

### Web API

- List endpoints share one pattern: `parsePagination`, a per-domain filter struct, `whereBuilder`, and `List`/`Count` returning the rows and a `total`.
- `/api/` writes need the `X-DockVMap` header, or get `403`. A browser can't add a custom header cross-origin without a CORS preflight, which DockVMap never answers, so other sites can't write with the session cookie.
- The client address is resolved once through `trusted_proxies` into `service.RequestInfo`. The audit log and the login limiter read only that, never `RemoteAddr` or `X-Forwarded-For`.
- Test connection uses a throwaway `oci.Client`, so unsaved values never reach the shared client's caches. A failed handshake is `200 {ok:false,error}`; only a malformed request is `4xx`.
- Hashed `/assets/*` are `immutable` and `index.html` is `no-cache`, so a deploy can't leave an index pointing at gone assets.
- `GET /api/proxy-metrics` has no frontend caller; keep it.

### Smaller services

- **Notifications:** `TAG_ADDED`, `TAG_REMOVED` and `UPGRADE_AVAILABLE` (never for a pinned image). Only sweeps notify; manual refreshes are silent. Email is one digest per run, webhooks get one JSON per event. Every channel is tried once: a failure is logged and recorded, and the event still counts as sent.
- **Failure log:** actionable background failures, recorded next to their log line with a `WithoutCancel` context so a timed-out check still records. Kept 30 days. `recognizeFailure` turns common causes into a plain sentence; the raw error is always shown too.
- **Proxy tokens:** stored as SHA-256 (random secrets, not passwords, so no bcrypt) and shown once. Only the Basic Auth password is checked.
- **Metrics and pull tracking:** counters and pulls are kept in memory and saved every 5 minutes and on shutdown; reads overlay unsaved pulls, so a pull shows within seconds. A manifest or blob `GET` that reached the client is a pull, a `HEAD` isn't. Blobs must count, because containerd fetches manifests with `HEAD` only. A pull time only moves forward, so follower reports and repeats are harmless.
- **Login rate limiter:** per client address, checked before any database or bcrypt work; a lockout lasts a flat window.

## Background worker

Each job runs in its own goroutine with its own timer; never share one `select` across jobs. A run's outcome goes to `worker_ticks` every time, a recovered panic included, so the schedule survives restarts and a panicking job doesn't fire again on every start. The first run waits for what's left of the interval since the last run, plus a short stagger. A job that config turns off stays listed with the reason; another role's jobs aren't listed at all. `deps.worker.Register(name)` decides which jobs System › Tasks can run on demand. `name` is the `worker_ticks` key, so keep it stable.

| Job | Interval | Runs when | Does |
|---|---|---|---|
| `session-cleanup` | 1h | always | delete expired sessions |
| `tag-discovery-cleanup` | 24h | always | drop finished tag discoveries older than 7 days |
| `background-failure-cleanup` | 24h | always | drop failures older than 30 days |
| `image-tag-refresh` | `tags_check_interval` (24h) | interval > 0 | the tag sweep |
| `blob-cache-cleanup` | `blob_cache.cleanup_interval` (1h) | cache on | drop unused blobs, then evict to 90% of `max_size` |
| `blob-cache-orphan-scan` | 24h | cache on | drop cache files with no row |
| `tag-notification` | 5m | SMTP or webhooks set | send pending notifications; skips while a sweep runs, so one sweep is one digest |
| `proxy-metrics-flush` | 5m | always | save counters and pulls, once more on shutdown |
| `proxy-metrics-cleanup` | 24h | always | drop daily metrics older than 30 days |
| `agent-poll` | `agents_poll_interval` (30s) | not a follower | ask every host's agent for changes |
| `replication-changes-cleanup` | 24h | primary | drop recorded registry changes older than 30 days |
| `replication-reconcile` | `follower.reconcile_interval` (24h) | follower with an internal registry | copy what this follower lacks, missing layers included, and count the tags the primary lacks; never delete |

## Frontend

Svelte 5, Vite and TypeScript, hash-routed, with cookie-session auth. `lib/api` (stateless fetch) → `lib/stores` → `lib/services` (the only code that changes a store), used once two or more components share data; otherwise page-local `$state` calls `lib/api`. Services are module singletons, except state that belongs to one route, which gets a factory so nothing leaks between mounts.

- Every route renders `<PageTitle>`. System is the only tabbed page: the route owns the shell, each tab loads its own data.
- Only `lib/api/client.ts` knows `ApiError`; a network failure is status `0`. Call sites use `errorMessage(err, "Failed to …")`.
- Loading, error, empty and content precedence lives once, in `lib/utils/asyncState.ts`.
- Lists: a request token drops superseded responses, a skeleton shows on first load only, and filters, sort and page live in the URL hash (`lib/utils/listQuery.ts`), never in storage.
- UI rules: a row with something to open opens on click; row actions are icons with a tooltip; badges show status only; blue text is only for links; detail pages are cards ending in a Danger zone.
- Phone breakpoint `768px`: the sidebar becomes a drawer, and tables scroll in `.table-wrap` and hide secondary columns with `.phone-hidden`.
- Shared CSS lives in `app.css`; find the existing primitive before adding one. `.table-actions` goes on a `<div>` inside the cell, never on the `<td>`.
- Polling uses `createVisibilityPoller`, which pauses in background tabs. A poll that updates a loaded object merges it field by field.
- The dashboard is one request with a `{data, error}` envelope per section, so one failed query degrades one card.
- The Updates page keeps a row after Update or Pin, offering Roll back or Unpin, so rows don't vanish under the user. A group with a flagged member is one row. Tags are marked seen on update, not on load.

Traps:

- Lucide icons: import from the icon's own subpath, never the package root, which pulls about 3800 files into svelte-check. Don't pass `class` to an icon; wrap it in `<span class="icon">`. A bare `:global(.icon)` leaks app-wide. An icon from a parent's snippet carries the parent's scope, so style it with `:global(svg)`.
- The dark theme is defined twice in `app.css` (media query and `[data-theme='dark']`), and `index.html` sets `data-theme` before first paint to avoid a flash. Keep its `localStorage` key in sync with `theme.ts`.
- `Field.svelte`'s `autofocus` calls `.focus()` on mount; the HTML attribute works only on the first page load.

## Design details

### Tag analysis

`internal/taganalyzer` runs `tokenize → classify → family → order`. It decides which tags group together, which is newest, and whether an upgrade is available. Its mistakes show as wrong recommendations, not errors, so correctness matters most. Before changing grouping, classification or ordering, run `go run ./cmd/tagaudit` and keep every invariant counter at zero, then bump `taganalyzer.AnalysisEpoch`: a refresh skips analysis for an unchanged tag set, and the epoch is part of that check.

Guarantees, holding with zero violations on hundreds of thousands of real tags: every tag is in exactly one family; family IDs are unique and stable across refreshes; a family never mixes different literal segments (`noble`/`jammy`, `jdk`/`jre`); ordering never inverts the leading version; the result is identical for any input order; it's safe to run concurrently.

- A family is a structural lineage, not a compatibility promise. Only numbers and dates vary within one; whether an upgrade is safe is the user's call.
- `blood` families share an identity pattern built from segment shapes. A bare number counts as a multi-part version only when the repository shows it as a real major (`11` next to `11.0.24`). A hash-shaped segment counts as a hash only when the repository's own tags confirm that slot (at least 5 values of one length, one with a letter), so long build counters aren't taken for hashes.
- `ancestor` merges a root into the one family it extends; ambiguity always loses. A root without hashes never absorbs a hash family, so a commit SHA is never offered as an upgrade.
- Families are ordered by relevance, not by how they were built: no singleton first, release builds before commit builds, version before date before name, then fewer segments and more tags. Retune against the corpus, never one repository.
- `1.2` doesn't flag `1.2.6` as an upgrade, since it already tracks it, but flags `1.3.0`. A family whose only varying part is a hash has no meaningful order, so it never reports an upgrade.
- Recognized beyond `MAJOR.MINOR.PATCH`: PEP 440 `aN`/`bN`, `pN` patches, Java `8u422`, Alpine `-rN` (absorbed into the version), same-day rebuilds like `20180112.1`, and ISO timestamps (folded into 14 digits first). The tokenizer splits on `-` and `_`, never `.`.

### Image tag refresh

Every trigger goes through `RefreshAvailableTags`, under a per-image lock shared with tag changes and rename. The lock is the guarantee; `refresh_status` is only a UI hint, and a `running` left by a crash is reset at startup. A manual refresh runs in the background, since the request would die at the 45s write timeout, and answers `202` if it takes longer than about 2s.

- A pinned image is still refreshed and still emits tag events, but never reports an update.
- Each check `HEAD`s the selected tag's manifest (free under Docker Hub's pull limit), so a tag republished under the same name is caught. A digest the proxy serves for the image's tag is saved too, so hosts that pulled it don't read Outdated until the next check. A stored digest counts only while it belongs to the current tag.
- A selected tag missing from the full tag list is reported as gone. `ListTags` fails rather than return a partial list, so missing really means missing.

### Image groups

Images that release together (Nextcloud AIO, Immich) always share one tag and pin. The tag stays on each image, so the proxy and the refresh don't know groups exist. Membership is manual, since a shared namespace doesn't mean a shared version.

- Enforcement lives in `Images`, not the UI: a tag or pin change applies to every member in one transaction, and `GetTags` returns only the tags every member has. Nothing is written when a member lacks the tag.
- Member locks are taken with `TryLock` and retried for 5s, then `409`, so several locks can't deadlock or outlive the write timeout.
- Group state is computed on read. Flagged with no common upgrade tag is the "blocked" state, which must stay visible: a replaced member would otherwise silence the group forever.
- A group change clears the proxy's 20s resolve cache for its names, with a generation counter so an older lookup can't cache the old tag again; otherwise a `compose pull` could mix versions.

### Tag discovery

Backs the "Add virtual image" wizard. `Check` reads one cheap page first, then scans in the background. The `tag_discoveries` row is both the cache (refreshed without losing the last result on failure) and the lock, so concurrent requests for one repository share one scan. Saving the image reuses the result instead of scanning again.

### Registry browsing

Reads `/v2/_catalog` through the normal client (logins, options, bearer flow), following pages on the same host only, up to 1000 names. Whether a registry can be browsed is asked once and cached for an hour, refusals included, network failures not. Trap: editing or deleting a registry must drop that answer and `Forget` both hosts in `oci.Client`, or the next check uses the old login.

### Hosts and the agent

Visibility only: the agent reads Docker and answers, DockVMap polls, stores and compares. DockVMap always connects to the agent, never the other way, so a host network never needs a route back, and the agent has no write path to Docker.

- Pairing is mutual TLS 1.3 with pinned self-signed identities, no tokens. The agent adopts its first caller and refuses any other. An identity change is never accepted silently: someone clicks Trust new agent, and it's audited. One connection per poll, so the pin is checked every time.
- The agent sends only Docker `GET`s with a 5s limit, below DockVMap's 10s, so a hung daemon shows as "Can't read Docker" instead of an offline host.
- An unchanged report gets `304`. A changed one replaces the host's containers in one transaction; one with a Docker error keeps the last known containers. Bump `ProtocolVersion` only for an incompatible change.
- Version status is computed on read, so a tag change shows on every host at once. A container is tracked when its image comes from `proxy_public_host`. Trap: the image ID counts as a digest, because on the containerd image store an image loses its `RepoDigests` once its tag moves.
- A container on the proxy address whose name isn't a virtual image is tracked against the internal registry. It counts in its host's rollout, never in an image's.
- Trap in the agent: its cache is fed by Docker's `/events`, and a fill that started before a reset is dropped, or it would bring back stale data.
- Only hosts polled recently with Docker readable count, and a rollout always says how many hosts couldn't be checked, so a complete number can't hide them. Crash-looping containers count, so a bad deploy shows.
- Tests use a fake Docker on a unix socket, or a real agent container against a scratch DockVMap, never a second Docker daemon on the host network.

### Internal registry

DockVMap fronts one official `registry:3` on its stack network: names that aren't virtual images pass through to it, pushes included. The registry keeps storage, uploads and garbage collection; DockVMap adds TLS, token auth, an audit entry per push and a tag check after it. DockVMap never logs in to the registry, and config is the only switch: the UI can't change which registry is internal.

- At startup one registry entry is kept marked internal: created, taken over by address, or moved to a new address, so the virtual images on it follow.
- A virtual image's name refuses writes, except a virtual image on the internal repository of the same name, which passes writes through apart from a push of the virtual tag. Any other virtual image's `tags/list` answers only its virtual tag, so skopeo, Renovate and diun follow DockVMap's digest.
- A push needs an address in `push_allowed_ips` when set and, with `proxy_auth` on, a token that can push. Either failing is `403`.
- Pass-through traps: upload `Location`s are rewritten to relative paths, since the registry's own host isn't reachable; a registry `401` becomes a `502` without `WWW-Authenticate`, or Docker loops on a login the registry can't take; the client's `Authorization` never reaches the registry.
- With the feature on, each request sets its own read deadline and moves it forward while upload data arrives, so a large layer finishes and a stalled one is cut off.
- Pass-through traffic doesn't touch proxy counters or pull tracking, so the dashboard stays about virtual images.

### Replication

A follower is a headless read replica of what the proxy needs: registries with their logins, virtual images, groups, proxy tokens and the internal registry's content. Accounts, the audit log, hosts, tag history and tag checks stay on the primary. Promoting a follower isn't supported, since it lacks the tag lists.

- **State:** SQLite triggers on every replicated table bump one version row in the same transaction, only for replicated columns and real changes, so no write path can forget it and tag checks never wake followers. `TestReplicatedTablesHaveTriggers` enforces it.
- **Sync:** a follower long-polls the version; when it moves, it gets the whole snapshot (about 100 bytes per image) and writes only the rows that differ. A snapshot from another cluster replaces the rows instead. Logins travel inside the pinned TLS connection and are re-encrypted with the follower's key.
- **Safety, like MySQL:** a primary behind the follower, or from another cluster, stops it until `-resync`; it keeps serving meanwhile and never follows a primary backwards. A follower with data of its own also waits for `-resync`. The setup check covers only settings that change what the proxy serves, and only those both sides send, so a newer primary doesn't stop older followers. Registry deletes are probed rather than configured: deleting a manifest that can't exist answers `405` with deletes off and `404` with them on. A node that starts as a follower clears its own cluster ID, so if it later becomes a primary it starts a new cluster.
- **Registry copies:** every push or delete through DockVMap bumps a per-repository counter and is recorded for 30 days, in the background. A follower replays only what changed since its counter, and compares the whole repository when the history doesn't reach back. The first copy and a resync also take every repository in the primary's catalog, for images pushed before replication was on or around DockVMap. A virtual image on the internal registry keeps its old tag until the new content has arrived. Pending copies are recomputed from the counters at every start.
- **Deletes:** registry content is deleted only by a replayed delete or a resync, never because something is missing, since a registry volume can be lost or restored apart from the database.
- **Full compares are deep:** the first copy, a resync, a repository whose history was pruned, and the daily reconcile also check every child manifest and layer of a tag the follower already has, so a layer lost to garbage collection comes back. A running `registry:3` keeps reporting a removed blob until it restarts. The reconcile counts the tags the primary lacks (`extraTags`) and never deletes them.
- **Resync** forgets the copied counters and sets `prune_requested`, which survives restarts. After the full copy succeeds, the follower deletes the tags the primary's registry lacks, unless that registry is empty, as after a lost volume. It deletes by tag, which never touches another tag; `registry:2` refuses that, so there it deletes by digest, only when every tag on the image goes. Digest isn't the default because on `registry:3` it would break a kept index whose platform manifest has its own tag.
- **Followers on the primary:** each follower is saved under an ID it makes once and keeps across resyncs (`replication_followers`), shown by its IP until it's given a name on the primary. A follower is never identified or named by its token: the token is only its credential, followers may share one, and deleting a token keeps its followers (shown as not connected) so a token can be replaced. A follower too old to send an ID is keyed by its address. Its health (ready, copies, `extraTags`, last error) rides on every state request in a header and stays in memory, since the next request resends it. A follower counts as connected only once it has contacted this primary since it started; for the first 2 minutes it's "waiting", not missing, or every restart would raise false problems.
- **Pull reports** flow back every minute. Only newer times are kept, so repeats and outages are harmless.
- **Readiness** means "can stand in for the primary": database, registry, matching setup, synced once and first copy done. Lag isn't part of it, because a dead primary makes every follower lag exactly when failover needs it. The endpoint has no auth, so it carries setting names, never values or addresses.
- **Ports:** replication has its own listener, with a self-signed identity pinned through the token (`dvmr1-<fingerprint>-<secret>`). `replication.allowed_ips` is checked on the connection's own address, since nothing proxies this port. A follower's web port serves only the health checks and a page naming the primary.

## Backend conventions

- JSON POST and PUT handlers decode with `decodeJSON[T]`.
- A new periodic job is a `scheduledJob` in `scheduledJobs`; services it needs go on `workerDeps`.
- Constraint errors are checked with `isUniqueConstraintErr` and `isForeignKeyConstraintErr`, never by error text.
- Audit-worthy actions call `recordAudit`; actor and request info come from the context.
- List filters embed `model.Pagination` in a per-domain struct. Sortable lists add `model.Sort`, `parseSort` and a store `sortSpec` that maps each field to fixed SQL, so no request value reaches the SQL; the frontend uses `createListView` with `sortFields` and `SortableHeader`.
- Date-range filters bind through `sqliteDatetime()`.
- New web dependencies are fields on `web.Dependencies`, not parameters.

## Traps

- A `ReverseProxy` `ErrorHandler` that returns without writing answers `200 OK`. When a read deadline cuts an upload, the context is cancelled first, so an early return told the client a broken upload had worked. Always write a status.
- `ReverseProxy` forwards interim `1xx` statuses to wrapped `ResponseWriter`s, and `httptest.ResponseRecorder` treats a `100` as final, so test this with a real server.
- `httpmw.Recover` must re-raise `http.ErrAbortHandler` (how `ReverseProxy` reports a client that hung up), and its writer must forward `Flush`, `ReadFrom` and `Unwrap`, or streaming breaks.
- A migration that rebuilds a table drops its triggers; recreate the replication triggers in the same migration.
- A primary's replication version can be 0, for data from before migration 037. "Synced once" is a cluster ID on the follower, never a version above 0.
- A nil service pointer in an interface field isn't nil. Set the optional `web.Dependencies` fields only when non-nil.
- `DEFAULT CURRENT_TIMESTAMP` text doesn't compare with the driver's time format; bind date filters through `sqliteDatetime()`.
- SSRF: check a user-supplied registry host against the configured registries before requesting it.
- `trusted_proxies` only protects you if the reverse proxy appends to `X-Forwarded-For` (`$proxy_add_x_forwarded_for`); `gateway` only helps if the port can be reached only through the proxy.
- Single user, no roles: there's deliberately no API to manage other accounts. Ask before adding one.
- `RegistryOptions` JSON keys are snake_case, unlike the rest of the API; don't change it in one place.
