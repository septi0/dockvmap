# DockVMap improvement plan

Status: planning only. No implementation has started. This came out of the
2026-09-03 full-application audit (backend + frontend).

Effort key: `S` is under an hour, `M` is about half a day. Phases are ordering
guidance; items inside a phase are independent unless a dependency is listed.
Risk is judged against a live production deployment.

---

## Phase 1: correctness and honesty

Small, isolated, each fixes something that is currently wrong. Unblocks SYS1.

| ID | Area | Change | Why | Effort | Risk |
|----|------|--------|-----|--------|------|
| W1 | worker | On a shutdown-interrupted run (`ctx.Err() != nil`), do not record `context.Canceled` as `worker_ticks.last_error`; also write `last_count = 0`, `last_error = NULL` so the row reads clean. Minimal guard only: leave the two-write structure alone, W3 restructures it. | A false "context canceled" error shows on jobs after any restart that lands mid-run. | S | Low |
| W2 | worker | `runImageTagRefresh` returns the real `refreshed` count instead of `0`. | The one job users watch reports "ok" instead of "N refreshed". | S | Low |
| W4 | service | Drop the `DeleteExpiredSessions` call from `Sessions.Login`. First confirm `GetSessionUser` filters expired rows in SQL; if it does not, W4 is dropped. | Duplicates the hourly `session-cleanup` job; adds a DELETE to every login. | S | Low |
| P1 | proxy | Exclude upstream HTTP 404 from `metrics.upstreamFailures`. Keep counting 401, 403, 5xx, and transport errors. | The failure-rate metric is inflated by routine "tag not found" probes. | S | Low |
| L2 | config, logging | Add `log_level` (`debug` / `info` / `warn` only, default `info`, env `DOCKVMAP_LOG_LEVEL`). Wire it into the slog handler at startup. Ships with L1. | Runtime verbosity control without a recompile. | S | Low |
| L1 | proxy, logging | Demote only the routine per-request `slog.Info` chatter (`manifest request`, `manifest fetched from upstream`, `blob fetched from upstream`, `served from cache`) to `slog.Debug`. Keep `virtual image not found` and `has no upstream tag configured` at INFO, and the `httpmw.AccessLog` line at INFO. | Makes `proxy_access_log=false` actually quiet the proxy hot path. | S | Low |

## Phase 2: logging config, performance, write-amplification

| ID | Area | Change | Why | Effort | Risk | Depends |
|----|------|--------|-----|--------|------|---------|
| OPT1 | service, store | Skip the refresh body when the upstream tag set is unchanged. Hash the sorted filtered tag slice, store it in a new `images` column via a forward-only migration (next free number). On a match (any refresh path, including manual and GUI), skip `taganalyzer.Analyze`, `SetImageTags`, `DeleteImageTagsNotSeen`, and the event pass, and only bump `last_checked`. | Most 24h refreshes are no-ops but still re-analyze N tags (CPU) and rewrite N rows. Also cuts WAL growth and checkpoint pressure. | S to M | Low (a mismatch falls back to the full path) | - |
| OPT3 | blobcache | Debounce the `accessed_at` touch: skip `TouchCachedBlob` when `AccessedAt` is already within roughly `lifetime/10` of now. | Every cache hit currently costs a SELECT plus an UPDATE. This makes the common hit read-only. | S | Low | - |
| OPT5 | store | Raise `SetMaxOpenConns` from 10 to about 25 (and `SetMaxIdleConns` to match). | The dashboard fan-out (5 goroutines) plus a concurrent pull plus the worker can queue on 10. WAL readers do not block each other. | S | Low | - |
| D1 | web, service, store | Replace the 3 sequential `COUNT(*)` calls in `dashboardSummary` with one aggregate. Needs a new `Images` service method plus a new store method (web does no SQL). | 3 full-table scans per dashboard poll per client. | S to M | Low | - |
| W3 | worker, store | Collapse `worker_ticks` to a single post-run UPSERT (drop the pre-run `MarkRun`), carrying the W1 guard forward. Raise `proxy-metrics-flush` from 1m to 5m; its shutdown hook still folds the final delta. | Two statements per tick everywhere; about 3k no-op bookkeeping writes per day from the flush job alone. | S to M | Low to Med | W1 |
| O1 | oci | Give `insecureClientFor` a dedicated long or no-expiry cache keyed by registry, off the 30s `registryDataCacheTTL`. | Rebuilds a full `*http.Client` plus connection pool every 30s per self-signed registry. | S | Low | - |
| I1 | service, store | `RefreshAll`: keyset pagination on `id`. Needs an `AfterID` field on `model.ImageListFilters` plus the store query change, or a dedicated `ListImagesAfterID`. | Offset drift can skip or repeat an image if the set mutates mid-sweep. | S | Low | - |
| P2 | proxy | Wrap the upstream error body in `io.LimitReader` (a few KB) before the JSON decode in `writeUpstreamOCIError`. | Unbounded decode of a registry-controlled response body. | S | Low | - |
| OPT4 | proxy | In-memory virtual-image resolution cache, TTL-only (about 20s) plus singleflight. Store value copies, not shared pointers. No explicit invalidation: accept up to the TTL of stale resolution after a tag change, rename, or delete. Land before PXY1. | About 24 identical `SELECT ... images WHERE name = ?` per `docker pull`. The real win is connection-pool contention under concurrent pulls, not latency. | S to M | Low | - |

## Phase 3: deduplication and structure

Wider diffs. Independent of each other except where noted.

| ID | Area | Change | Why | Effort | Risk |
|----|------|--------|-----|--------|------|
| WEB1 | web, store | Extract `parseDateRange(r)` plus a generic `{items, total}` responder (web); add `whereBuilder.dateRange(col, since, until)` plus a shared `count(ctx, table, where, args)` (store). Move only `apiListFailures`, `apiListEvents`, `apiListAuditLogs` and the three store `xListWhere` plus `CountX` triples onto them. Leave the dashboard fan-out sub-sections and `apiListImages` untouched. Land before WEB2. | Near-identical handlers, filter parsers, and store list/count builders; grows with every System tab. | M | Med (tested paths, behaviour must stay identical) |
| WEB2 | web | Dedicated pass. Add `apiServiceError(rw, err)` that handles only the already-uniform cases: `ErrInvalid*` to 400 plus `err.Error()`, `ErrUpstreamUnavailable` to 502, `ErrUpstreamNotFound` / `ErrUpstreamUnauthorized` to 404, and the unknown-error 500 fallback. Each handler keeps its explicit `*NotFound`, `*AlreadyExists`, `ErrTagUnavailable`, `ErrFailedToRefreshTags`, and `ErrCredentialEncryptionNotConfigured` cases before calling it. No message or status changes to the live API. Also tidy the list-endpoint 500 defaults. | The `switch { case errors.Is(err, service.ErrX): apiError(...) }` block is copy-pasted in every mutating handler. | M | Low to Med (universal cases only, no behaviour change) |
| WRK1 | worker, service, web | Consolidate `WorkerSchedule`, `WorkerTrigger`, `WorkerActivity`, and `WorkerCatalog` into one concrete `service.Worker` type (`Ticks` / `Catalog` / `Trigger` / `Running` / `Register`), built in `cmd` and passed to both the worker and the web layer. The catalog is set after `scheduledJobs()` runs. Land before FE2. | 4 constructors, 4 web interfaces, 4 `Dependencies` fields for one subsystem. | M | Med (wide but mechanical DI churn) |
| PXY1 | proxy | Factor a shared `serveResource(w, r, name, ref, kind)` out of `handleManifest` and `handleBlob`, with a `kind` hook for virtual-tag resolution and the `IsDigest` cache guard. Preserve the OPT4 cache call, P1, and P2. Verify with throwaway tests during implementation; do not commit them. Land after OPT4, P1, P2. | Roughly 70% duplicated resolve, cache, metrics, proxy flow. | M | Med (hot path) |

## Phase 4: frontend

| ID | Area | Change | Why | Effort | Risk | Depends |
|----|------|--------|-----|--------|------|---------|
| FE1 | frontend | Replace `mergeImage` with plain per-field assignment of the fields that change (`refreshStatus`, `lastChecked`, `lastCheckError`, `updateAvailable`, `updateAvailableTag`, `tag`). Keep the `if (!image) { image = next; return }` guard for first load. | A reactivity workaround, not a pattern. | S to M | Low | - |
| FE2 | frontend, web | Move the grace window server-side: an `armedUntil` timestamp on the consolidated `Worker` (set by the trigger, cleared when the job calls `Begin`, expires on its own). The status endpoint reports `running` while armed. Then drop `tagRefreshStatus.ts`'s client-side grace window and `wasRunning` edge detector. | 116 lines of module state for a rare, short-lived event. Net removal is smaller than 116 lines because the grace concept relocates rather than disappears. | M | Med (shared by Dashboard and Images) | WRK1 |
| FE3 | frontend | Extract a shared "poll until settled" core (poll, terminal-status check, N-consecutive-error tolerance) behind `tagDiscovery.ts`, `tagRefreshStatus.ts`, and `ImageDetails.svelte`'s inline poll. Keep the three lifecycles distinct (singleton, factory, page-local). Extract only the loop. | The same roughly 40-line pattern is implemented three times, with three slightly different error behaviours. | M | Low to Med | FE2 |

## Cross-cutting

| ID | Area | Note |
|----|------|------|
| SYS1 | planning | System section ship order: Failures, then Status, then Tasks. The Tasks tab must not ship until W1 and W2 land, or it displays wrong data on day one. |

---

## Decisions locked

- `log_level` config key: in scope (L2), levels `debug` / `info` / `warn` only. Ships in Phase 1 with L1.
- L1: only the routine per-request chatter drops to `debug`; the misconfiguration lines stay at `info`.
- W1 (Phase 1 interim): an interrupted run writes a clean `worker_ticks` row (`last_count = 0`, `last_error = NULL`). W3 (Phase 2) then collapses to a single post-run UPSERT that an interrupted run skips entirely.
- `worker_ticks`: single post-run UPSERT; interrupted runs skip the write; `proxy-metrics-flush` interval 1m to 5m (W3).
- OPT1: a new `images` column plus a forward-only migration is acceptable. The hash covers the sorted filtered tag slice. All refresh paths short-circuit on a match, manual and GUI included.
- OPT4: TTL-only resolution cache, no explicit invalidation, TTL in the 15 to 30 second range.
- WEB1 scope: the three standalone list endpoints plus the store builders only. The dashboard fan-out and `apiListImages` are left alone.
- WEB2: Option 1 (scoped mapper for the already-uniform cases only), done as a dedicated pass. No live API behaviour change.
- WRK1: one concrete `service.Worker` type, not a web-side aggregating interface.
- PXY1: throwaway verification tests during implementation, not committed.
- FE2: the grace window moves server-side as an `armedUntil` on `Worker`.
- `logs_path` file output: unchanged. The Docker logging driver is the operator's responsibility.
- `maxTagListPages`: parked. The 1,000,000-tag ceiling is harmless in practice.

## Execution process

- Phase gates: complete a phase, pause for review, then start the next.
- One branch per phase (4 branches, 4 merges).
- Verification is `make verify` plus manual smoke, plus committed tests for the worker scheduling only (Option B, light):
  - In Phase 1, pull the schedule arithmetic in `cmd/dockvmap/worker.go` into pure helper functions in the same file: a `firstRunDelay` variant taking `(lastRun time.Time, ok bool, interval, offset time.Duration)`, and a `rescheduleDelay(interval, elapsed time.Duration)`. No new package.
  - Add `cmd/dockvmap/worker_test.go` (`package main`) covering: first-run-delay math (fresh, overdue, normal), reschedule-delay math, the interrupted-run clean row (W1), and count propagation (W2).
  - In Phase 2, add one case to `internal/service/images_test.go` for OPT1: unchanged tag set skips, changed tag set runs the full refresh.
  - Any real worker package split is deferred to WRK1 (Phase 3), which reworks the worker wiring anyway.
  - `internal/blobcache` (OPT3) and `internal/proxy` (OPT4) stay on inspection plus `make verify`. PXY1's throwaway tests are not committed.

## OPT1 correctness note

The skip is sound because `taganalyzer.Analyze` is a pure function of its input
tag list, and `updateAvailableFor` is a pure function of the analyzed tags plus
`image.Tag`. The pinned tag only changes through `UpdateTag`, which already
recomputes the update-available state and fires its own events. `SetImageTags`
never updates the `new` column on conflict, so an unchanged set has nothing to
re-flag. When the sorted filtered tag set is byte-identical to the last stored
hash, the analysis, the stored tag rows, the update-available flag, and the
event set are all guaranteed unchanged. Only `last_checked` needs to advance.

## Risks and open items

1. W2 semantics: with OPT1 in place, "refreshed" is "images checked without a hard error", which is roughly the total image count every sweep. If the useful number is "images whose tags changed", that is a different counter and a bigger change.
2. FE2 does not delete the grace window, it relocates it server-side. The net line reduction is smaller than the 116-line figure. Still a single source of truth instead of spread client state.
3. W3 widens the hard-crash proxy-metrics loss window from about 1 minute to about 5 minutes. Clean shutdown is unaffected (the flush hook runs).
4. OPT4: a tag change, rename, or delete takes up to the TTL (about 20s) to be visible at the proxy. Benign for normal pulls; relevant only if a tag is changed specifically to stop clients pulling a bad image. A single invalidation hook on `UpdateTag` only would close that gap while keeping create, rename, and delete on the TTL. Not currently planned.
5. OPT3: the max-size LRU eviction orders by `accessed_at`, so debouncing introduces up to `lifetime/10` of ordering fuzz under size pressure. Accuracy loss, not a correctness issue.
6. OPT5 helps reads. It slightly raises the potential for write-lock contention and `SQLITE_BUSY` retries under heavy concurrent writes. Watch for it after the change; write volume is currently low.
7. Test coverage: see the Execution process section.
8. D1 and I1 are each slightly larger than "one query": both need a new store method or a new filter field threaded through the service interface.

## Explicitly out of scope (reviewed, deliberate)

- OPT2: concurrent `RefreshAll` worker pool. Skipped for now; revisit if sweep wall-time becomes a real problem at scale.
- Passthrough `Health` and `WorkerSchedule` as a layering pattern.
- `updatesCount` and dashboard-summary fetch overlap. Dashboard-only; the badge genuinely needs an app-wide source.
- The two 2-second inline-wait loops in `api_images.go` and `discoveries.go`. Different layers.
- `refresh_status` state machine consolidation. Documented as intentional: 3 entry points, 2 consistency guarantees, judged acceptable.
- SMTP `buildMessage` verbosity (about 100 lines of hand-rolled MIME). Verbose, not complex. Only trim if the branded inline logo is dropped for a plain `multipart/alternative`.
