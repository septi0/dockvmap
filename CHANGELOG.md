# Changelog

Changes people running DockVMap will notice, newest first. Upgrade notes call out anything to do before or after upgrading.

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
