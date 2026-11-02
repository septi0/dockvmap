# Changelog

Changes people running DockVMap will notice, newest first. Upgrade notes call out anything to do before or after upgrading.

## v1.9.0 (unreleased)

- **Hosts**: see the containers on each of your Docker hosts. Add a host with its address on the new Hosts page and run the DockVMap agent on it (`ghcr.io/septi0/dockvmap-agent`, sample in `compose.agent.sample.yaml`). DockVMap connects to the agent on port 9002 and adopts it on its first check, so hosts never need a route back to DockVMap and there are no tokens to copy. The agent only reads from Docker. Each host shows whether it's online, its containers with state, health, stack, image and published ports, and which containers pulled through DockVMap are up to date, outdated, or pulled but not recreated. DockVMap checks every host every 30 seconds (`agents_poll_interval`), and opening a host checks it right away. See "Hosts and the agent" in the README.
- **Running on**: Image details lists the containers running that image on every host, with what needs attention first, and how many are up to date.
- **Deployed column**: the Virtual Images and Hosts lists show how many containers run the current version, like "5/8", once hosts report containers pulled through DockVMap. Numbers refresh by themselves, and hosts that are offline or can't read Docker are counted separately.
- **Crashes at a glance**: the Hosts list flags containers that keep restarting or crashed in the last 7 days. A host that goes offline, or can't read Docker, keeps showing its last known containers.
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
