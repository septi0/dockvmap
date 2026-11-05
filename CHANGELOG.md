# Changelog

Changes people running DockVMap will notice, newest first. Upgrade notes call out anything to do before or after upgrading.

## v1.11.0 (2026-11-05)

- **Browse a registry**: a Browse icon on Registries opens the repositories and tags of any registry that lists them, shows which repositories have no virtual image yet, and starts Add virtual image with the repository and tag filled in. DockVMap checks which registries list their repositories, so Docker Hub and GHCR simply have no Browse icon. See "Browse a registry" in the README.
- **Virtual images per registry**: Registries shows how many virtual images use each registry, and the number opens Virtual Images filtered by that registry. Virtual Images has a Registry filter. A registry still in use links to its virtual images and can't be deleted until they're gone.
- **Failing checks per registry**: a registry whose virtual images fail their tag checks gets the same failed-check icon on Registries as on Virtual Images and Groups, with the count in its tooltip, so an expired login points at the registry instead of only at each image.
- **Tidier Registries list**: the internal registry comes first, then the rest by name. Authentication shows the username, or None, in place of the separate Username column and badge.
- **Repository suggestions**: New virtual image now suggests repositories for every registry that lists them, not only the internal registry.
- **Registries**: the "anyone can push" warning no longer shows on the Registries list; it stays in the log and on Proxy Tokens.
- **Tag list for virtual images**: `GET /v2/<virtual image>/tags/list` answers the virtual tag instead of `404`, so skopeo, crane, Renovate and diun work with virtual images, and Renovate or diun digest tracking of `myapp:current` follows DockVMap moving the tag. A virtual image built on its own repository in the internal registry (`myapp` on `myapp`) lists the registry's real tags.
- **Clearer Virtual Images columns**: Update is now **Newer tag** ("1.43.7 available", or Latest) and Deployed shows "2 behind", or Current, so a row no longer says "update" twice with two meanings. Hosts and Running on use the same "behind" and "current" wording.
- **Outdated for re-pushed tags**: a container running a real name from the internal registry, like `…:5000/team/web:latest`, now shows Up to date, **Outdated** once a new image is pushed to that tag, or Pulled, not recreated, and counts in the Hosts page and dashboard "behind" numbers. A push through DockVMap shows within one host check.
- **Behind on hosts**: a new dashboard card under Needs attention lists the containers that need a redeploy, with the host, the stack and the command to copy, so the dashboard no longer says everything is up to date while hosts are behind. The Updates card's empty state now says "No newer tags", and the Hosts tile says "4 behind".
- **Redeploy command**: an Outdated or Pulled, not recreated container gets a copy icon with the command that fixes it, in Running on, Host details and the dashboard, like `cd /srv/shop && docker compose pull web && docker compose up -d web`. The agent now also reports the compose folder of each container (its `com.docker.compose.project.working_dir` label); agents older than v1.11.0 keep working, without the icon.
- **Push IP allowlist**: `internal_registry.push_allowed_ips` limits pushes and deletes to your build hosts, without tokens on every host. Other addresses get `403` naming the refused address; pulls aren't affected. With `proxy_auth` on, a push needs an allowed address and a token with Can push. System → Status shows the list, and the "anyone can push" warning goes once it's set.
- **Proxy**: with the internal registry off, a pull of a malformed repository name (uppercase letters, for example) now gets `400 NAME_INVALID` instead of `404`, the same answer as with it on. Valid pulls are unchanged.
- **Fixed**: an active filter (Status, Group, Registry) keeps its dropdown arrow.
- **Fixed**: `trusted_proxies` and `login_rate_limit.bypass_ips` now match an IPv4 address that a reverse proxy writes in IPv6 form (`::ffff:10.0.0.5`).
- **Internal registry docs**: the sample and README use `registry:3`, with its garbage-collect path and `--delete-untagged` (checked safe for multi-arch images on 3.1.2), and explain how to keep DockVMap's own image in the internal registry without DockVMap needing itself to start.
- **Host details**: each container pulled through DockVMap shows its version (Up to date, Outdated, Pulled, not recreated) under the image instead of a warning icon you had to hover, so it reads on phones too. The Show filter's Outdated is now **Behind**, since it also lists pulled, not recreated containers.
- **Clearer labels**: the header and the Updates page show "No newer tags" instead of "All caught up", which also read as "nothing to do" while hosts were behind. Proxy Tokens has an Access column ("Pull" or "Pull and push") while the internal registry is on, instead of a Can push badge, and the internal registry is marked "Internal registry" under its name instead of a badge. The current tag on Image details is no longer blue, since blue means a link. Clicking a Running on row opens its host, as on the dashboard. Browsing a registry that doesn't answer now says to check that it's running and its login.
- **Audit log**: entries made by DockVMap itself, like the internal registry changes at startup, show System in the User column instead of a dash, as in the entry's details.

**Upgrade note:** this release migrates the database (migration 036). Run `dockvmap -backup <path>` before upgrading. Update the agents to get the redeploy command.

## v1.10.0 (2026-11-05)

- **Internal registry**: run the official `registry:2` next to DockVMap, with only DockVMap exposed, and push and pull every image through one address. Names that aren't virtual images pass straight through to the registry; virtual tags work as before. Turn it on with `internal_registry.enabled`; `compose.registry.sample.yaml` is a ready stack. At startup DockVMap adds the registry on the Registries page (marked Internal), or takes over an existing entry with that address, so virtual images on it keep working. A new tag pushed to a repository shows as an update within seconds, and every pushed tag and deleted manifest is in the audit log with the token that made the change. A virtual image built on a repository of the same name (`myapp` on `myapp`) still takes pushes to that repository. See "Internal registry" in the README.
- **Internal registry over TLS**: `internal_registry.tls` puts DockVMap in front of a registry that already serves TLS, so you can take over an existing registry without changing it or the clients that use it directly. See "Keeping an existing registry as it is" in the README.
- **Can push on proxy tokens**: with `proxy_auth` on, pushes need a token with Can push, chosen when you create it (the option shows while the internal registry is on). Hosts keep pull-only tokens. With `proxy_auth` off, pushes are open, and DockVMap warns about it in the log, on Proxy Tokens and on Registries.
- **Real client address in the proxy**: `trusted_proxies` now also applies to the registry proxy, so the access log (`client_ip`) and the push audit show the real client behind a reverse proxy.
- **Repository suggestions**: New virtual image suggests the repositories already in the internal registry.
- **Clearer errors**: a virtual image whose tag was removed from its registry shows "tag v1 no longer exists in the registry; pick another tag" on the image and in Recent issues, instead of a failed digest lookup. A pull through a registry that doesn't answer gets an error naming that registry instead of a plain "bad gateway".
- **Fixed**: a container pulled after its tag was republished upstream, for example a rebuilt `v1.9.0`, no longer shows as Outdated until the next tag check. When a pull through DockVMap gets a newer digest for an image's tag, DockVMap now uses that digest right away.
- **Fixed**: when DockVMap can't start, for example because a port is already in use, it exits with an error code, so `restart: on-failure` and systemd restart it, and the log shows only the cause instead of an error for every background job.

**Upgrade note:** this release migrates the database (migration 035). Run `dockvmap -backup <path>` before upgrading. Existing proxy tokens can't push; create a token with Can push for anything that pushes through DockVMap. Nothing changes until you turn on `internal_registry.enabled`.

## v1.9.0 (2026-11-04)

- **Hosts**: see the containers on each of your Docker hosts. Add a host with its address on the new Hosts page and run the DockVMap agent on it (`ghcr.io/septi0/dockvmap-agent`, sample in `compose.agent.sample.yaml`). DockVMap connects to the agent on port 9002 and adopts it on its first check, so hosts never need a route back to DockVMap and there are no tokens to copy. The agent only reads from Docker. Each host shows whether it's online, its containers with state, health, stack, image and published ports, and which containers pulled through DockVMap are up to date, outdated, or pulled but not recreated. DockVMap checks every host every 30 seconds (`agents_poll_interval`), and opening a host checks it right away. See "Hosts and the agent" in the README.
- **Running on**: Image details lists the containers running that image on every host, with what needs attention first, and how many are up to date.
- **Deployed column**: the Virtual Images and Hosts lists show how many containers run the current version, like "5/8", once hosts report containers pulled through DockVMap. Numbers refresh by themselves, and hosts that are offline or can't read Docker are counted separately.
- **Crashed containers**: the Hosts list flags containers that keep restarting or crashed in the last 7 days. A host that goes offline, or can't read Docker, keeps showing its last known containers.
- **Lighter Image details**: the pull command and the Tag history and Upstream tag activity buttons sit under the image name, the rest is split into Version and Activity, repeated rows are gone, and the Danger zone is more compact. With `proxy_public_host` on port 443, pull commands leave out `:443`.
- **Hosts on the dashboard**: a tile with how many hosts are online, how many need attention, and how many containers are outdated. Every dashboard tile now opens the matching page (Image pulls opens Virtual Images sorted by last pull, Tag checks opens System > Tasks), and the tiles sit 3 per row on smaller screens.
- **One way to work with lists**: rows with something to open are clickable, and row actions are icons with a tooltip. Registries rows open the edit window, with Delete inside it. Sessions and System > Tasks use an icon too, and the Groups review link is an up arrow. On Groups, the failed check icon sits next to the name and the copy button is gone. Sortable column headers show their arrows on hover or when sorted. Blue text is now only for links: tags and group member names in lists are plain text, and status badges are never links.
- **Small UI fixes**: the Update badge on Virtual Images shows the tag as it is ("v1.2.0", not "V1.2.0"), and image paths wrap at `/` and `.` instead of mid-word. Host details shows proxied images as `name:tag`, with the full reference on hover. Proxy Tokens shows when a token was created like "10d ago". Tag history no longer offers Roll back to the tag you're already on.
- **Tables on phones**: words in table cells no longer break in the middle ("Usernam/e", "v3.2./3"). Registries hides Username and Options on phones, both still in the edit dialog, and long registry hosts and failure messages wrap where needed.
- **Recent issues without repeats**: the dashboard shows each failure once, with how many times it happened, like "5h ago · 6 times", so a check that fails every day no longer pushes other problems off the card. System > Failures still lists every occurrence.
- **Roll back**: Tag history and the notice after a tag change both say Roll back, instead of Restore and Revert.
- **Fixed**: Last pulled now updates for Docker hosts that use the containerd image store (the default on new installs). They fetch everything through blob requests and never asked for a manifest, so their pulls were not recorded. A pull that downloads nothing new is still not counted.
- **Removed**: Image details no longer shows when a client last checked for a newer version.
- **Task times on one line**: System > Tasks shows "Last run" like "52m ago" on one line on wider screens.

**Upgrade note:** this release migrates the database and can't be rolled back without a restore. Run `dockvmap -backup <path>` before upgrading. Set `proxy_public_host` if you haven't, so containers are matched to virtual images by the address clients pull from. DockVMap now also keeps `agent_client.pem` in its data folder; back it up with the database. If you ran `v1.9.0-rc.1`, its hosts are removed: add them again with their address and replace the agents with the new compose file.

## v1.8.0 (2026-11-01)

- **Digest tracking**: every tag check now also looks up the digest behind each image's current tag, with one lightweight request per image that doesn't count against Docker Hub's pull limit. A tag change looks up the new digest within seconds. Image details shows the digest and when it was first seen, so a tag republished under the same name is visible. Lookup failures appear in System > Failures as "Digest lookup" and never fail the tag check.
- **Fixed**: on the Registries and Proxy Tokens tables, the line under each row no longer breaks at the edit and delete buttons.

**Upgrade note:** this release migrates the database and can't be rolled back without a restore. Run `dockvmap -backup <path>` before upgrading. Digests show as Unknown until the first tag check after the upgrade, or until you refresh an image.

## v1.7.1 (2026-11-01)

- **Stale proxy tokens**: on the Proxy Tokens page, a token not used in over 90 days gets a warning under its Last used time, so forgotten tokens are easy to spot and delete.

## v1.7.0 (2026-10-31)

- **Sorting on Virtual Images**: click the Name, Last checked, Last pulled or Update column header to sort; click again to reverse, and a third time to go back to the usual order. The sort is kept when you open an image and come back, and in shared links. Without a chosen sort, the list keeps its usual order.
- **Group pull status**: the Groups page and an image's Group row warn when some members haven't been pulled on the group's current tag yet, for example "4 of 6 pulled on v2.7", and name the missing images.

## v1.6.1 (2026-10-31)

- **Tidier Virtual Images list**: the registry and repository now show as one line under the image name instead of two columns, and Last checked shows a relative time like Last pulled, with the exact date on hover.
- **Up to date is plain text**: on Virtual Images and Groups, only states that need attention (an update, Pinned, a blocked group) keep a colored label, so they stand out.
- **Quieter copy buttons**: on Virtual Images and Groups, the copy button is dimmed until you hover or focus its row. Touch screens always show it in full.

## v1.6.0 (2026-10-31)

- **Last pulled**: Image details and the Virtual Images list show when each image was last pulled through the proxy, and which real tag clients got. After a tag change, Image details warns until a client pulls the new one, and notes when a client last checked without downloading.
- **Proxy token last used**: the Proxy Tokens page shows when each token last authenticated a pull, while proxy authentication is enabled.

**Upgrade note:** this release migrates the database and can't be rolled back without a restore. Run `dockvmap -backup <path>` before upgrading. Pulls are tracked from the upgrade on, so images and tokens show nothing until their first pull after it.

## v1.5.0 (2026-10-31)

- **Digest emails**: each scheduled tag check now sends at most two emails: one listing available updates, and one listing new and removed tags. A group appears as one line, and emails go out once the check has finished.
- **Upstream page link** on Image details and Updates, for Docker Hub, GHCR and Quay images.
- **Copy from the lists**: copy a pull command from Virtual Images, or all of a group's image references from Groups.
- **Revert on Image details**: undo a tag change right after making it. On a grouped image this reverts the whole group.
- **Refreshing an image no longer asks for confirmation.** "Refresh all tags" still does.
- **Copy buttons now work over plain HTTP**, not just HTTPS.

**Upgrade note:** email subjects have changed, for example from `dockvmap: Upgrade available for nginx` to `dockvmap: 3 updates available`. Mail filters matching the old subjects need updating. Webhooks are unchanged and still fire once per image.

## v1.4.0 (2026-10-31)

- **Image groups**: keep images that release together on one version. Changing the tag of one member changes all of them, or none. New Groups page, with group labels and filters on Virtual Images.
- **Tag search** in the Change tag modal, the create wizard and the group modal, for lists of 12 or more tags.
- **Tag age**: see when a tag was first seen, on Updates and in the Change tag modal.
- **Create and add another** in the create wizard, keeping the registry and namespace.
- **Pre-releases are greyed out** in the Change tag modal.
- **Phone layout**: slide-out menu, and no sideways scrolling.

**Upgrade note:** this release migrates the database and can't be rolled back without a restore. Run `dockvmap -backup <path>` before upgrading.
