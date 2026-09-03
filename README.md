<p align="center">
  <img src="frontend/public/images/favicon.png" width="96" alt="DockVMap logo" />
</p>

<h1 align="center">DockVMap</h1>

<p align="center">A self-hosted Docker/OCI registry proxy that gives you a stable tag pointing at a real image version you control.</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue.svg" alt="License: AGPL-3.0"></a>
  <img src="https://img.shields.io/badge/go-1.25-00ADD8.svg" alt="Go 1.25">
  <img src="https://img.shields.io/badge/svelte-5-FF3E00.svg" alt="Svelte 5">
</p>

---

## What it is

DockVMap sits between your Docker clients and a real registry (Docker Hub, GHCR, a private registry, or anything speaking the OCI Distribution API). You point clients at a **virtual tag**, e.g.:

```
docker pull dockvmap.local/myapp:current
```

`current` isn't a real tag on the upstream registry. It's a pointer DockVMap maintains. Behind it, you track a real upstream tag (`myapp:1.4.2`, say) per image, and change what `current` resolves to whenever *you* decide to, from a web UI. Clients never change what they pull; you change what they get.

<p align="center">
  <img src="docs/screenshots/flow.png" width="800" alt="DockVMap request flow: client pulls a virtual tag, DockVMap resolves it via the configured mapping, fetches the real image from the upstream registry, and returns it to the client">
</p>

### Why

`:latest` moves under you with no warning and no audit trail. Hardcoding a specific version tag everywhere it's referenced means every rollout is a find-and-replace across configs. DockVMap adds one layer of indirection so you get a fixed, predictable reference for clients *and* full control over what it actually resolves to, plus a record of when it changed and to what.

## Screenshots

<p align="center">
  <img src="docs/screenshots/dashboard.png" width="800" alt="DockVMap dashboard">
</p>

<p align="center">
  <img src="docs/screenshots/image-details.png" width="800" alt="Virtual image details page">
</p>

## Features

- **Virtual tag proxying**: OCI Distribution API proxy (`/v2/...`) that transparently resolves a stable tag to whatever real tag an image is currently pinned to.
- **Tag family analysis**: inspects a repository's real tags, groups them into families, and tells you when a newer tag in the same family becomes available.
- **Pinning**: park an image on its current tag from the details page. Upstream tags are still fetched and recorded, but no update is flagged and no upgrade email is sent until you unpin.
- **Tag discovery**: when adding a virtual image, scans the upstream repository in the background (result cached) so you pick from its real tags instead of typing one blind.
- **Tag history**: per-image record of every real tag the virtual tag has resolved to, and when each change happened.
- **Web UI**: Svelte SPA for managing registries, virtual images, and reviewing what changed and when.
- **Notifications**: email (SMTP) and/or generic webhooks when a tracked image's tags change. Email volume is selectable per account (every tag change / only when an upgrade becomes available / off); webhooks always fire and carry an `updateAvailable` flag.
- **Optional blob cache**: on-disk manifest/blob cache keyed by digest, to cut repeated upstream pulls.
- **Proxy authentication**: optional HTTP Basic Auth (via issued tokens) in front of the registry proxy itself.
- **Audit log**: every state-changing action recorded, with resolved client IP and actor.

## How it runs

One binary, one SQLite database, three things running inside it:

- a **proxy server** implementing the OCI Distribution API,
- a **web server** serving the REST management API and the embedded frontend,
- a **background worker** doing tag refresh, notifications, and cleanup on independent schedules.

## Quick start

The recommended way to run DockVMap is the published Docker image. It bundles the binary with the frontend embedded, and needs only a persistent `/data` volume (SQLite database, credential key, blob cache) and the two ports: `5000` for the registry proxy, `8080` for the web UI.

```bash
docker run -d --name dockvmap \
  -v dockvmap-data:/data \
  -p 5000:5000 -p 8080:8080 \
  ghcr.io/septi0/dockvmap:latest
```

Open the web UI at `http://localhost:8080`. The first visit walks you through creating the initial admin account. Point Docker clients at the proxy on `:5000`.

Pass any config as `DOCKVMAP_*` environment variables (`-e DOCKVMAP_TRUSTED_PROXIES=gateway`, etc.; see [Configuration](#configuration) for the keys). A credential encryption key is generated and persisted to the volume on first run, so registries that require auth work with no upfront setup.

Images for `linux/amd64` and `linux/arm64` are published to `ghcr.io/septi0/dockvmap` on every `v*.*.*` tag, as `latest`, `MAJOR.MINOR`, and the full version.

### Docker Compose

`compose.sample.yaml` is a documented starting point; copy it to `compose.yaml`, adjust, and `docker compose up -d`. It runs the same published image, persists `/data` to a named volume, publishes both ports, and adds a healthcheck and `restart: unless-stopped`.

### Build from source

Run without Docker. Requires Go 1.25+ and Node.js for the frontend build.

```bash
git clone https://github.com/septi0/dockvmap.git
cd dockvmap
make build          # builds the frontend, embeds it, builds bin/dockvmap
mkdir -p data
cp config.sample.yaml data/config.yaml
./bin/dockvmap -config data/config.yaml
```

Running with no config file at all is fine too; it falls back to `DOCKVMAP_*` env vars and defaults. Either way, a credential encryption key is generated and persisted at `<data-path>/credential_encryption.key` on first run; set `credential_encryption_key` yourself only if you want to manage it out of band.

> A plain `go build ./...` will fail on a fresh checkout: the web server embeds `frontend/dist` at compile time, and that directory isn't committed. `make build` (or `make build-frontend` once) handles it.

## Behind a reverse proxy

When a reverse proxy terminates TLS and forwards to the web UI on `:8080`:

- Set `trusted_proxies` (or `DOCKVMAP_TRUSTED_PROXIES`) to the proxy's address: `gateway` if it reaches DockVMap through the published port, or the proxy's own address if it shares a Docker network. See [`trusted_proxies`](#networking) for the specifics.
- Stop publishing `:8080` to the host, so the proxy is the only route in.

Skip both and anything that can reach `:8080` directly can forge `X-Forwarded-For`, defeating client-IP logging and login rate limiting.

## Configuration

Settings can come from a YAML file, from `DOCKVMAP_*` environment variables, or both; the config file is entirely optional. Precedence per setting is **env var > config file > built-in default**. Omitting `-config` runs on env vars and defaults.

Env var names mirror the YAML keys: uppercased, path segments joined with underscores, prefixed `DOCKVMAP_`. So `proxy_server_listen` becomes `DOCKVMAP_PROXY_SERVER_LISTEN`, and `smtp.host`, `blob_cache.max_size`, and `login_rate_limit.max_attempts` become `DOCKVMAP_SMTP_HOST`, `DOCKVMAP_BLOB_CACHE_MAX_SIZE`, and `DOCKVMAP_LOGIN_RATE_LIMIT_MAX_ATTEMPTS`. List values are comma-separated (`DOCKVMAP_TRUSTED_PROXIES=10.0.0.0/8,192.168.1.1`).

One setting is a CLI flag rather than a config key: `-data-path`, which sets where the database and credential key live (see [CLI](#cli)).

Every setting is optional. The Default column gives the built-in value; `none` means unset and `auto` means derived from other settings, with the Description saying how. [`config.sample.yaml`](config.sample.yaml) is a starting point; these tables are the complete list.

### Core

| Key | Default | Description |
|---|---|---|
| `virtual_tag` | `current` | The tag name clients pull instead of a real upstream tag. |
| `tags_check_interval` | `24h` | Minimum time between upstream tag-change polls. The last poll time is persisted, so restarts don't reset the interval. |
| `tag_discovery_ttl` | `1h` | How long a repository's discovered tag list (shown when adding a virtual image) is cached before a "Check repository" click refreshes it in the background. |
| `session_lifetime` | `168h` | Web UI session duration. |
| `logs_path` | none | Directory for log files. Unset logs to stdout only. |
| `log_level` | `info` | Minimum log level: `debug`, `info`, or `warn`. `debug` adds per-request proxy tracing; `warn` drops routine informational lines. |
| `tag_filters_path` | none | Path to a `filters.yaml` policy file (see [Tag filtering](#tag-filtering)). If set, the file must exist; an invalid path is a startup error, not a silent fallback. Unset uses the built-in default filters. |
| `credential_encryption_key` | none | Base64, 32-byte AES-GCM key encrypting stored registry credentials. Unset generates one and persists it at `<data-path>/credential_encryption.key` on first run. |

### Networking

Bind addresses are where DockVMap listens; `proxy_public_host` is only what the UI advertises to clients. Set `trusted_proxies` whenever a reverse proxy sits in front; otherwise client-IP logging and login rate limiting key off the proxy's address instead of the real client's.

| Key | Default | Description |
|---|---|---|
| `proxy_server_listen` | `:5000` | Registry proxy bind address. Serves HTTPS in place when `tls.enabled` is true. |
| `proxy_public_host` | none | Host clients use in `docker pull`, shown in the web UI's pull instructions. May include a port (`registry.example.com:5050`) to override `proxy_server_listen`'s port too, for when the publicly reachable port differs from the bind port. Unset lets the web UI guess from the browser's request host, which can be wrong behind a reverse proxy. |
| `proxy_access_log` | `true` | Write one structured log line per `/v2/...` request (method, path, status, bytes, duration, client address, resolved virtual image and upstream registry/repo/tag, cache hit or miss). Lines carry `component=proxy` for filtering. Set `false` to silence per-request logging without changing the global log level. |
| `web_server_http_listen` | `:8080` | Web UI/API bind address, used when `tls.enabled` is false. |
| `web_server_https_listen` | `:8443` | Web UI/API bind address, used when `tls.enabled` is true. |
| `trusted_proxies` | `[]` | CIDRs/IPs of reverse proxies trusted to report the real client IP; they must append to `X-Forwarded-For`, not pass it through. Use `gateway` to resolve the container's IPv4 default gateway at startup: the address Docker SNATs a proxy's traffic to when it reaches a published port (`trusted_proxies: ["gateway", "10.20.45.23"]`). Empty logs a warning and uses the immediate TCP peer as the client. |
| `secure_cookies` | auto | Marks the session cookie `Secure`, so browsers only send it over HTTPS. Auto-set to `true` when `tls.enabled` is true or `trusted_proxies` is non-empty, `false` otherwise. Set explicitly to override. |

### TLS (`tls`)

Terminate HTTPS in DockVMap itself instead of at a reverse proxy; the proxy and web servers switch together.

| Key | Default | Description |
|---|---|---|
| `tls.enabled` | `false` | Serve the proxy and web servers directly over HTTPS. When on, the web server binds `web_server_https_listen` instead of `web_server_http_listen`; the proxy keeps `proxy_server_listen`. |
| `tls.cert_file` | none | PEM certificate chain path. Required when `tls.enabled` is true; a blank path is a startup error. |
| `tls.key_file` | none | PEM private key path. Required when `tls.enabled` is true; a blank path is a startup error. |

### Blob cache (`blob_cache`)

On-disk cache of manifests and blobs keyed by digest, to cut repeated upstream pulls. Always stored at `<data-path>/cache`.

| Key | Default | Description |
|---|---|---|
| `blob_cache.enabled` | `true` | Turn the cache on. |
| `blob_cache.lifetime` | `24h` | How long a cached entry may go unaccessed before it's dropped and re-fetched upstream. Each hit resets the clock. |
| `blob_cache.cleanup_interval` | `1h` | How often the sweep runs to drop expired entries and enforce `max_size`. |
| `blob_cache.max_size` | `10GB` | Disk budget for the cache; accepts `B`/`KB`/`MB`/`GB`/`TB`. When exceeded, the sweep evicts least-recently-used entries down to ~90% of this. |

### Login rate limiting (`login_rate_limit`)

Per-IP lockout on the web UI login, keyed off the `trusted_proxies`-resolved client IP.

| Key | Default | Description |
|---|---|---|
| `login_rate_limit.enabled` | `true` | Turn the lockout on. |
| `login_rate_limit.max_attempts` | `5` | Failed attempts from one IP within the window before it's locked out. |
| `login_rate_limit.window` | `15m` | Time window the failed attempts are counted over. |
| `login_rate_limit.bypass_ips` | `[]` | CIDRs/IPs exempt from the lockout. |

### Notifications (`smtp`, `webhooks`)

Fire when a tracked image's tags change. Email volume is per-account (every change / only when an upgrade is available / off); webhooks always fire and carry an `updateAvailable` flag.

| Key | Default | Description |
|---|---|---|
| `smtp.enabled` | `false` | Turn on email notifications. Requires `smtp.host` and `smtp.from`. |
| `smtp.host` | none | SMTP server hostname. Blank while `smtp.enabled` is true disables SMTP with a logged warning. |
| `smtp.port` | `587` | SMTP server port. |
| `smtp.username` | none | SMTP auth username. Blank sends unauthenticated. |
| `smtp.password` | none | SMTP auth password. |
| `smtp.from` | none | From address on sent mail. Blank while `smtp.enabled` is true is a startup error. |
| `smtp.tls` | `true` | Issue STARTTLS on the SMTP connection. |
| `webhooks` | `[]` | URLs POSTed on a tag change; the payload carries an `updateAvailable` flag. |

### Proxy authentication (`proxy_auth`)

| Key | Default | Description |
|---|---|---|
| `proxy_auth.enabled` | `false` | Require HTTP Basic Auth on the registry proxy, where the password is a proxy token created in the web UI. |

## Tag filtering

Tags fetched from upstream registries can be excluded from tracking/categorization before DockVMap groups them into families. This is useful for CI-generated tags (`commit-<sha>`, `pr-<n>`, etc.) that would otherwise pollute the tag list. The rules live in their own YAML file, separate from `config.yaml`, that you point `tag_filters_path` at.

```yaml
tag_filters:
  exclude:
    - "^commit-.*"
    - "^pr-.*"
```

Each entry is a regular expression matched against the raw tag name; any match excludes the tag. A set of default patterns (CI commit/PR tags, cosign signature artifacts) ships compiled into the binary and applies whenever `tag_filters_path` is unset, so filtering works out of the box. See `internal/tagfilter/filters.yaml` for the current default list. To customize, point `tag_filters_path` at your own `filters.yaml`; the path must exist or startup fails. Your file's `exclude` list fully replaces the built-in one rather than merging with it, so re-include any defaults you want to keep. An empty `exclude` list disables filtering entirely.

## CLI

```
dockvmap -config <path>              # path to config file (optional; falls back to DOCKVMAP_* env vars and defaults if unset)
dockvmap -data-path <path>           # path to the data directory: SQLite database, blob cache, credential key (optional)
dockvmap -reset-password <username>  # generate and print a new password, invalidate their sessions, exit
dockvmap -refresh-tags               # refresh tags for all configured images from their upstream registries, exit
dockvmap -backup <path>              # write a consistent copy of the database to <path>, exit
dockvmap -version                    # print version and exit
```

### Backup and restore

`-backup` writes a consistent snapshot of the SQLite database. It runs while DockVMap is live, opens the database read-only, and writes to a path that must not already exist. It does not run migrations, so it snapshots the schema as-is; back up with your **current** binary before an upgrade to keep a pre-upgrade copy.

The snapshot is **the database only**; the blob cache is regenerable and isn't included. A full restore also needs:

- **The credential encryption key**: the file at `<data-path>/credential_encryption.key`, or the `credential_encryption_key` value if you set one in config. Without it, stored registry credentials can't be decrypted.
- **Your `config.yaml`**, which DockVMap never touches and you keep yourself.

To restore: stop DockVMap, put the snapshot at `<data-path>/dockvmap.db`, restore the credential key, and start a binary of the same version or newer (an older binary refuses to run against a newer schema).

## Development

```bash
make help    # list all targets
make dev     # backend (go run) + frontend (vite dev server) together
make dev CONFIG=path/to/config.yaml   # ... against an alternative config file
make test    # go test ./...
make check   # frontend svelte-check + tsc
make lint    # gofmt + go vet (+ golangci-lint if installed)
make verify  # lint + test + check, in one go
```

## License

AGPL-3.0 (see [LICENSE](LICENSE)). You're free to use, modify, and self-host this. If you run a modified version as a network service, you're required to make that modified source available to its users; that's the one condition of this license.
