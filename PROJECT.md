# PROJECT.md

dockvmap ("Docker Virtual Mapper") is a Docker/OCI registry proxy: clients pull through a stable "virtual tag" (`myimage:current`) transparently mapped to a real upstream tag tracked per-image. One binary runs a proxy server (OCI Distribution API), a web server (REST management API + embedded frontend), and a background worker, sharing one SQLite DB and one registry HTTP client.

## Commands

`make help` lists all targets. Common ones: `make build` (frontend + backend, → `bin/dockvmap`), `make dev` (backend `go run` + frontend Vite dev server together, Ctrl+C stops both), `make test`, `make vet`, `make lint` (gofmt check + vet, + golangci-lint if installed), `make check` (frontend svelte-check + tsc). No CI config — the Makefile is the whole toolchain. To run the built binary directly (not via `go run`), just invoke it: `./bin/dockvmap -config data/config.yaml`.

Raw equivalents still work: `go build ./...`, `go vet ./...`, `go test ./...` (only `internal/taganalyzer` has tests — a golden-case suite over real-world tag shapes, self-contained with no corpus dependency), `go run ./cmd/dockvmap -config data/config.yaml` (`-config` has no default filename — omit it entirely and the app runs on `DOCKVMAP_*` env vars/built-in defaults only). Frontend commands run from `frontend/`: `npm run dev`, `npm run check`, `npm run build` (→ `frontend/dist`, embedded via `embed.go`).

`frontend/dist` is a gitignored build artifact (like `bin/`), not committed — `frontend/embed.go`'s `//go:embed dist/*` needs a real build there first, so plain `go build ./...` fails until `npm run build`/`make build-frontend` has run at least once. `make build` always does both in the right order.

## Architecture

- `cmd/dockvmap` — `main.go`/`server.go`/`worker.go`/`adapters.go`: wiring, HTTP server setup, the background worker, and small adapters so `internal/oci` stays decoupled from `store`/`service`. `main()` → `run() error`; startup failures return instead of `os.Exit`-ing past deferred cleanup. Listener failures go through a `serverErrs` channel into the same graceful-shutdown path as a SIGTERM, not a direct `os.Exit`. `-reset-password <username>` skips starting the servers and instead calls `Users.ResetPassword(ctx, username)` (looks the user up by username, generates a random password, invalidates all their sessions, prints it, exits) — the recovery path for a forgotten password. Takes a username (not just "the one user") so it stays correct if roles/multi-user ever get added. `-refresh-tags` similarly skips starting the servers and calls `Images.RefreshAll(ctx)` directly — the same one-shot logic the background worker runs on its ticker, exposed as a manual CLI trigger (e.g. for scripting an out-of-band refresh instead of waiting for `tags_check_interval`). Partial failures don't abort the batch; the joined per-image errors are returned (non-zero exit) after the refreshed count is printed, so one bad image doesn't hide that the rest succeeded.

- `internal/store` → `internal/service` → `internal/web` / `internal/proxy` — strict layering, SQL → business logic → HTTP.

- `internal/proxy` — OCI Distribution API (`/v2/...`); resolves virtual tag → real registry/repo/tag, forwards to upstream, optionally through `internal/blobcache`.

- `internal/web` — REST API + embedded frontend (`frontend/dist`, `handler.go`). `GET /health` is intentionally outside `/api/`'s auth middleware. `withRequestInfo` (the outermost `/api/` middleware) resolves the real client IP once via `trusted_proxies` (config CIDR/IP list) and stores it in `context.Context` (`service.RequestInfo`) — every downstream consumer (audit log, login rate limiter) reads that resolved IP, none of them touch `r.RemoteAddr`/`X-Forwarded-For` directly. `X-Forwarded-For` is only trusted when the immediate TCP peer is itself in `trusted_proxies`; otherwise it's attacker-controlled and ignored.

- `internal/ipmatch` — parses `[]string` IP-or-CIDR config lists into a matchable set; shared by `trusted_proxies` (`internal/web`) and the login rate limiter's `bypass_ips` (`internal/service`) so both interpret entries identically.

- `internal/oci` — registry HTTP client: auth challenge flow, Docker Hub compat, TTL-cached credentials/options/tokens.

- `internal/taganalyzer` — pure tag-string analysis (`tokenize → classify → family → order`), no I/O. It determines family grouping, tag ordering, family relevance, and whether an upgrade is available. See **Design Details → taganalyzer** for its guarantees and design.

- `internal/tagfilter` — loads tag exclusion regex patterns from `filters.yaml` and exposes `Filter.Apply(tags) []string`. When no custom `tag_filters_path` is configured, it uses the embedded defaults: `^commit-.*`, `^pr-.*`, and `^sha256-[0-9a-f]{64}`. Patterns are matched case-insensitively, so `^pr-.*` also covers uppercase `PR-` tags. The digest pattern intentionally has no end anchor, since a tag starting with a full SHA-256 digest is a cosign artifact regardless of any suffix such as `.sig`, `.att`, or `.sbom`.

  Tag filtering is kept separate from `internal/taganalyzer`, which remains I/O-free, and from the main runtime configuration. Tags are filtered before analysis and before image creation validates a selected tag. This ensures excluded tags are consistently hidden from discovery and rejected when explicitly selected. Tags are filtered once at the point where they are fetched and passed downstream as already-filtered data; no later layer re-applies the filter. Filtering is not performed inside `internal/oci`, since registry transport should remain independent of tracking policy.

- `internal/blobcache` — optional on-disk manifest/blob cache, keyed by digest.

- `internal/smtp` — minimal `net/smtp`-based mailer for tag-change notifications. HTML mails are sent as `multipart/related` wrapping a `multipart/alternative` (text + HTML) plus the DockVMap logo (`assets/dockvmap-logo.png`, `//go:embed`) as an inline `Content-ID: <dockvmap-logo>` part — the `tag_notification.html` template references it as `cid:dockvmap-logo`, so the branded header renders without an external fetch or a data URI (both of which major clients block).

- `internal/webhook` — minimal HTTP POST client for tag-change notifications (`webhooks` config, a list of URLs). Sends dockvmap's own fixed JSON shape (`event`/`imageName`/`tags`/`updateAvailable`/`occurredAt`) — not Slack/Discord-formatted, deliberately: no per-provider formatting is built in, since there was no expressed need for one and it'd be exactly the kind of speculative feature this project avoids elsewhere. `service.Notifications` treats email and every webhook URL as independent, best-effort, attempt-once channels per event — no per-channel delivery tracking (`tags_events.notif_sent_at` is still a single flag), so a channel that's down loses that one notification silently (logged) rather than blocking or duplicating others. The worker only runs when `smtp.enabled` or `webhooks` is non-empty — not gated on SMTP alone.

  Event types on `tags_events` are `TAG_ADDED`, `TAG_REMOVED`, and `UPGRADE_AVAILABLE` (emitted by `RefreshAvailableTags` via `Events.HandleEvent` when an image's `updateAvailable` flips true or its target advances). Webhooks fire for every event regardless. Email is gated per account by `UserPreferences.NotifyLevel` (`all` / `upgrades` / `none`, default `all`; fresh admin gets `upgrades`; legacy `notifyNewTags` true/false maps to `all`/`none` on unmarshal): `upgrades` sends only `UPGRADE_AVAILABLE`, `none` sends nothing, `all` sends everything. The gate is applied at send time in `SendPendingTagNotifications` (per recipient, via `mailTargets`); the `tags_events.notify` column still just means "not a silent refresh".

- `internal/service/failure_log.go` — `FailureLog`, a bounded (20-entry) in-memory, "since last restart" ring buffer of actionable background failures (webhook/email delivery, image tag-refresh), injected into `Images` and `Notifications` alongside their existing `slog.Error` calls (both fire — slog for the ops/log-file trail, this for the GUI's "Recent issues" Dashboard card). Same reasoning as `tags_events` existing separately from `image_tags` to drive "Recent activity": a dedicated event log decoupled from the tables that track current state, so a since-resolved transient failure (e.g. `images.last_check_error`, cleared the moment the next check succeeds) doesn't just vanish from view. `internal/web` composes the human-readable message per source (`failureMessage` in `dto.go`) from the stored raw error text — the raw text is kept verbatim, not paraphrased, since that's the actually useful troubleshooting content.

- `internal/service/discoveries.go` (table `tag_discoveries`) — backs the "Add virtual image" wizard's repository step. `Check` does a cheap single-page `oci.Client.CheckRepository` first, never the full `ListTags` pagination, so a huge repository can't make "check repository" itself slow. The real scan runs in a goroutine off the worker context (capped at 10 minutes, recovers its own panics), writing into a `tag_discoveries` row keyed `UNIQUE(registry_id, repository)` — that row is both the cache (TTL'd via `tag_discovery_ttl`, refreshed in the background without clobbering the previous result on failure) and the dedup lock (insert-or-reclaim on conflict, so concurrent requests for the same pair converge on one scan). `Check` waits inline on the row for ~2s before falling back to `status: "running"` for `GET /api/discoveries/{id}` to poll, so small/cached repos still resolve in one round trip. `RecoverFromRestart` (startup) fails any row left `running` by an unclean exit; a worker ticker prunes terminal rows older than 7 days — see `worker.go` for why shutdown doesn't wait on in-flight scans. `Images.Create` takes an optional pre-fetched `availableTags`, filled from a completed discovery's cache by `apiCreateImage`, so saving doesn't re-trigger the scan `Check` just avoided. Live progress (`tagsSeen`) lives only in an in-memory map on `Discoveries`, updated via `ListTagsWithProgress`'s per-page callback and never persisted, since the scan and the polling handlers share one process. `raw_tag_count`, captured alongside `tag_count` on completion, drives the response's `ignoredCount`.

- `internal/store/migrations` — numbered, checksum-verified schema migrations.

- `internal/service/proxy_tokens.go` (table `proxy_tokens`) — CRD-only (no update, no revoke-vs-delete distinction), issued by the single admin, not tied to a `user_id` (matches `registries`/`images`, which also carry no creator column — provenance lives only in the audit log, never duplicated onto the resource row). Token stored as a SHA-256 hash (fast, not bcrypt — high-entropy random secret, not a human password, so slow hashing buys nothing); plaintext shown exactly once on creation, never retrievable again. Enforced in `internal/proxy` via HTTP Basic Auth on every `/v2/...` request, gated by `proxy_auth.enabled` (config, default `false` — an explicit switch, not "on the moment a token exists," so creating a token to test doesn't silently start rejecting already-connected clients). Only the password field is checked against `proxy_tokens`; the username is free-form/unchecked, matching the label-not-identity design (any client presents any username, e.g. `docker login -u <label> -p <token>`).

- `cmd/dockvmap/worker.go` — independent ticker loops (session cleanup, tag discovery cleanup, tag refresh, blob cache cleanup, SMTP notifications) via `runTickerLoop`. Tag discovery goroutines spawned by `Discoveries` are separate from this: they run off the same worker context (cancelled on shutdown alongside these loops) but aren't tracked in `startWorker`'s `WaitGroup`, so shutdown doesn't wait for an in-flight scan to finish — it's just interrupted, and `Discoveries.RecoverFromRestart` cleans up the row on next startup.

- `internal/service/login_rate_limiter.go` (`login_rate_limit` config) — per-IP failed-`/login`-attempt counter, checked as the first line of `Sessions.Login` before any DB lookup or bcrypt work. State is just an attempt count in a `expirable.LRU[string, int]` keyed by resolved client IP, TTL = `window`: a failure calls `Add` (renews the TTL), a block-check only calls `Get` (never renews it) — so the lockout is a flat `window` from the failure that tripped it, not extended by repeated knocking while locked, and a quiet IP's history ages out on its own with no separate cleanup job. A successful login clears the entry. `bypass_ips` (also `internal/ipmatch`) skip the limiter entirely, still need the correct password. Defaults to enabled (`Enabled *bool`, not plain `bool`) so an existing `config.yaml` predating this feature is still protected after an upgrade, unlike the other opt-in config blocks where the zero-value default is intentionally off.

- `cmd/tagaudit` — offline auditing for `internal/taganalyzer`, run by hand, never by the app. Four modes over a corpus of cached tag lists (`-corpus`, default `sampledata/tags`): `-sample N` builds a repository list from Docker Hub's official images plus keyword search and picks the rest at random (written to `<corpus>/audit-manifest.tsv`); `-fetch` downloads the tags for anything not cached; `-verify` re-checks every invariant the package promises plus determinism and placement accuracy; `-shapes` reports the segment shapes that stop tags from grouping; `-rank` flags repositories whose leading family looks wrong. With no mode it runs all three reports. The corpus lives under the gitignored `sampledata/`, so this command — not the data — is the durable artifact: a fresh clone regenerates everything with `-sample` then `-fetch`.
  `-shapes` is the one that finds *unknown* conventions. It describes each blocking segment by character class rather than content (`gb73411456155` and `ga16392559` both become `a{2-3}d{7+}`) and ranks buckets by volume × distinct-value rate. A high distinct rate means nearly every tag has its own value there, which is what a machine-generated identifier looks like; a named axis like `alpine`/`bookworm` reuses few values many times and sinks. A shape with high volume, a high distinct rate and several unrelated repositories is a convention the analyzer does not understand yet — that is exactly the signature `git describe` output had before it was handled, and the signature Alpine's `-rN` would have had.
- `cmd/tagdebug` — prints the family breakdown for a single repository fetched live; the quick "what does this repo look like" tool, where `tagaudit` is the corpus-wide one.
- `frontend/` — Svelte 5 + Vite + TS SPA, hash-routed (`svelte-spa-router`), cookie-session auth (not JWT — no `Authorization` header anywhere).

## Design Details

### `internal/taganalyzer`

Pure tag-string analysis (`tokenize → classify → family → order`), with no I/O.

This is one of the most important components in DockVMap. It determines which tags are grouped together, which tag in a family is newest, how families are presented in the tag picker, and whether `updateAvailableFor` reports an available upgrade. Errors here usually produce a wrong recommendation rather than a visible failure, so correctness is critical.

`updateAvailableFor` (`internal/service/images.go`) skips a newer same-family candidate when the pinned tag is a strictly-less-precise prefix of it on *every* version axis — pinned `1.2`, candidate `1.2.6` is not an upgrade since `1.2` already tracks it; `1.3.0` still is; and for compound tags `17-alpine-3` does not flag `17.9-alpine-3.23` (contained on both the app and base axes) while `17.10-alpine-3.23` and `18.x-alpine-3.23` still do. The comparison runs off `image_tags.version_segments`, a per-tag JSON snapshot (`[][]int64`) of every `OrderVersion` segment's `Numbers` in order (empty for non-version axes, so date/alphabetical families are unaffected), written by `imageTagsFromAnalysis` alongside `family_id`/`tag_order`/`prerelease`. A base-image-only bump at the same app version (`17.9-alpine-3.23` → `17.9-alpine-3.24`) is not a prefix relation, so it is still reported.

Before changing grouping, classification, or ordering, re-measure with `go run ./cmd/tagaudit` — `-verify` for the invariants, accuracy and determinism, `-shapes` and `-rank` for what regressed — and keep every invariant counter at zero.

#### Guarantees

The analyzer guarantees:

- Every tag belongs to exactly one family.
- Family IDs are unique within a repository.
- A family never mixes different values of a literal/named segment.
- Ordering never inverts the leading version.
- `Analyze` is deterministic: the same tags in any input order produce a byte-identical result.
- Results are safe to compute concurrently.

These invariants have been verified against ~447k real tags across 199 repositories, and re-verified against a separate randomly sampled corpus of 398 repositories (210k tags) that the analyzer was never tuned on: zero violations in both.

A `Family` (`Kind`: `blood` / `ancestor` / `step`) represents a **structural lineage**, not runtime compatibility. It means the tags appear to be versions of the same named thing (same repository, OS base, variant, etc.). The analyzer cannot determine whether two versions are actually compatible, so it must not use compatibility assumptions to exclude a match. Whether an upgrade is safe remains the user's decision; `updateAvailableFor` only surfaces the newer tag as a suggestion.

Only version-shaped segments (numbers or dates) may vary within a family. Literal/named segments such as `noble`/`jammy` or `jdk`/`jre` must match exactly and are never treated as upgrade axes.

#### Blood families

`blood` is the strongest family relationship: every member must have an identical computed identity pattern.

The identity pattern is based on segment shapes rather than their literal values. Version segments collapse to `solo`, `multi`, or `calver`; prefixes and suffixes remain literal; date/time segments collapse to a constant; and confirmed hash segments also collapse to a constant.

A bare number such as `11` is treated as a multi-part version only when `collectKnownMajors` finds evidence that `11` is a real major version elsewhere in the same repository, such as `11.0.24`. This evidence is scoped to the preceding segment context rather than the raw segment index, preventing unrelated tag shapes from influencing one another.

Hash-shaped segments are normalized only when `collectHashLengths` confirms the slot from the repository's own tag set: at least 5 distinct values of the same length at the same position, with at least one containing `a`–`f`. This prevents long numeric build counters from being mistaken for hashes.

Hash-shaped means `^g?[0-9a-f]{7,40}$`. The optional `g` covers the abbreviated SHA produced by `git describe --tags`, such as `v3.29.6-1-gb73411456155`.

There is deliberately no additional ratio/frequency heuristic. An earlier ratio-based guard performed worse on repositories with architecture fan-out, so it was removed after testing across 200 repositories.

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

These rules were tuned against `sampledata/`. Across 200 repositories they changed the leading family in 41 cases, with all changes being improvements or neutral and no repository left leading with a singleton. Retune against the corpus rather than a single repository.

#### Family identity and ordering

`Family.ID` is an FNV-64a hash of `Family.Key`, masked to 52 bits so it remains JSON-number safe. It is not a per-run counter, so a family keeps the same ID across refreshes as long as its pattern remains unchanged.

`Family.HasOrder` is false when the only varying axis is a confirmed hash. Such tags can still be ordered for display, but the order has no semantic meaning, so `updateAvailableFor` does not report an upgrade for the family.

Anything that cannot be related to another tag becomes an isolated singleton. It retains `Kind: FamilyStep` for frontend/backward compatibility, but this no longer represents a weak grouping relationship—it simply means that no relationship was claimed.

#### Recognized version conventions

In addition to standard `MAJOR.MINOR.PATCH` versions, the parser recognizes several glued suffix conventions that affect grouping:

- PEP 440 `aN` / `bN` pre-releases
- OpenBSD/OpenSSH-style `pN` patches
- Pre-JEP 223 Java `uN` updates, such as `8u422`
- Alpine/apk `-rN` package revisions
- Debian/Ubuntu same-day rebuild suffixes such as `20180112.1`

The Alpine revision is absorbed into the preceding version, so `1.2.3-r1` stays in the same family as `1.2.4`. A leading branch number such as `r409-...` is unaffected.

Likewise, `20180112.1` is treated as the same date identity as `20180112`, with `.1` affecting ordering but not family identity.

## Conventions (follow when adding code)

- New JSON POST/PUT handler → use `internal/web/api_routes.go`'s `decodeJSON[T]\(rw, r) (T, bool)`, don't hand-roll `MaxBytesReader`/`DisallowUnknownFields`.

- New periodic background job → add it to `cmd/dockvmap/worker.go` via `runTickerLoop(ctx, interval, name, fn)`, on its own ticker — never share one `select` across jobs.

- New store constraint check → use `isUniqueConstraintErr`/`isForeignKeyConstraintErr` (typed `sqlite.Error.Code()`), never match on `err.Error()` text.

- New audit-worthy action → call `recordAudit`; actor/request info comes from `context.Context` (`service.RequestInfo`/`CurrentUser`), never pass as params.

- New list filter → embed `model.Pagination` into a per-domain filter struct (see `model.ImageListFilters`); don't add a shared generic `Filters` type.

- New date-range filter → bind through `internal/store/querybuilder.go`'s `sqliteDatetime()` — see Traps.

- New cross-cutting web-layer dependency (a service the HTTP handlers need) → add a field to `web.Dependencies` (`internal/web/handler.go`), not a new positional parameter to `web.New`/`newWebServer` — those constructors already outgrew positional args once.

- Frontend state → `lib/api` (stateless fetch) → `lib/stores` → `lib/services` (only code that mutates a store) is only worth it once 2+ components share the same reactive data; otherwise page-local `$state` calling `lib/api` directly is fine and preferred.

- Frontend list pages → guard rapid filter/pagination changes with an incrementing request token (see `Images.svelte`'s `loadToken`), so a slow superseded response can't overwrite a newer one.

- Comments: a short one-liner is fine where a WHY is genuinely non-obvious; never multi-line/paragraph comments.

## Traps

- ****`sqliteDatetime()`****: several tables set timestamps via SQLite's own `DEFAULT CURRENT_TIMESTAMP`, not Go. The driver's default `time.Time` formatting doesn't lexically compare against SQLite's stored format — always bind through `sqliteDatetime()` for a date-range filter.

- ****Lucide icons****: import from the icon's own subpath (`@lucide/svelte/icons/x`), never the package root (drags ~3800 files into `svelte-check`). Never pass `class` directly to an icon component — it's invisible to the parent's scoped CSS; wrap it in `<span class="icon">` and style that instead. A ****bare**** `:global(.icon)` leaks app-wide; only a compound form (`.parent :global(.icon)`) is safe.

- ****`//go:embed dist/*`**** needs some non-dot-prefixed file under `dist/` at build time or the build fails outright — since `dist/` isn't committed (see Commands), a bare `go build ./...` on a fresh checkout fails until the frontend has been built at least once.

- Any user-suppliable registry host must be checked against already-configured registries (`GetRegistryInfoByHost`) before the server requests it — otherwise it's SSRF (see `Discoveries.Check`).

- `trusted_proxies` only protects you if the reverse proxy in front **appends** to `X-Forwarded-For` rather than passing the client's own header through unchanged — e.g. nginx's `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for`, not `$http_x_forwarded_for`. Get that backwards and an attacker's self-supplied header is trusted verbatim, silently defeating both the login rate limiter's per-IP accounting and the audit log's IP field. This is a deployment misconfiguration, not something dockvmap's code can detect.

- Single-user model, no roles yet — there's deliberately no create/list/delete-other-account API. Don't add one without checking first; it was removed on purpose.

- `RegistryOptions`' JSON keys are snake_case while the rest of the API is camelCase (inherited from the Go struct tags) — don't "fix" it in just one place.

- `app.css` defines the dark theme tokens twice (media query + `[data-theme='dark']`), and `index.html` has a blocking inline `<script>` setting `data-theme` before first paint — both intentional (explicit choice beats system preference; avoids a theme-flash). Keep the `localStorage` key in sync between `index.html` and `theme.ts` if it changes.

- `rewriteTimestamp` (`tokenizer.go`) folds an ISO 8601 timestamp into the bare 14-digit form **before** tokenization, since the classifier only understands `20060102150405`. It covers the compact `20240101T120000`, the punctuated `2024-01-01T12-00-00`, and either with a leading label (`RELEASE.…` or `RELEASE-…`). `pretokenize` applies every rewrite in sequence rather than stopping at the first match.

- `taganalyzer`'s tokenizer only splits a tag on `-`/`_`, never `.` — deliberate, since a version's own dots (`1.2.3`) must stay inside one token. A repo that instead uses `.` to glue on a non-numeric qualifier (`confluentinc/cp-kafka`'s `7.0.10.amd64`) is handled by `splitDottedToken`, which splits at the rightmost dot where one side is a recognizable shape — so `7.0.10.amd64` tokenizes as `7.0.10` + `amd64`.