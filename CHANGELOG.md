# Changelog

Changes people running DockVMap will notice, newest first. Upgrade notes call out anything to do before or after upgrading.

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
