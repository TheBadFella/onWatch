# Fork dashboard and image delivery

This fork keeps OnWatch's provider routes, settings, polling, and authentication
flows. Presentation changes live in `internal/web/static/fork-ui.css` and
`fork-ui.js`, with small integration points in the upstream templates and card
renderers. No frontend framework or new quota API is required.

## Dashboard

The dark palette uses a black page with neutral card surfaces. The existing
light/dark toggle and saved theme preference still work.

Provider navigation sits in a left sidebar. Use **Collapse sidebar** to switch
to the icon rail, or **Expand sidebar** to restore labels. The browser remembers
that choice. New mobile sessions start collapsed; the expanded sidebar overlays
the content and closes with the backdrop or Escape. Provider tabs support arrow,
Home, and End keys, with Enter or Space to select.

Account cards show each window's available percentage and a live **Resets in**
countdown beside its reset date. The provider header also shows the earliest
reported reset, even when its body is collapsed. Countdown updates do not make
provider requests. **Reset due** means the reported timestamp passed; the next
provider poll must confirm the new quota cycle. Unknown reset times remain
explicitly unavailable. Available capacity uses reported utilization, regardless
of the chosen usage/available display mode. No cost estimates are inferred.

Codex accounts also show a separate **Banked resets** row in both the account
overview and individual account view. It shows the reported count and next
expiry when available. **None available** means the provider reported zero;
**Not reported** means no count was supplied. Quota headings and account names
wrap so badges and reset information remain visible on narrow screens. This
display does not redeem reset credits.

Click the reset count or **View details** to expand the individual banked
resets, including their reported titles, expiry dates, and plan restrictions.
Missing expiry dates and unavailable details are labeled explicitly. If a detail
check fails, retained records are marked as last known details. The count can
remain available even when the provider's detail endpoint is unavailable.

History tables show their consumption-rate explanation below the title.
The footer contains the row count and page controls.

## Sign-in and account connections

Settings opens on **General**, followed by **Providers** and **Notifications**.
Email (SMTP), Webhook (HTTP), and Browser push have expandable sections under
Notifications. Expand a section to edit its settings, then use **Save Settings**.
Collapsing a section preserves entered values. SMTP and webhook configuration
remain accessible even when their delivery channel is off. Mobile forms stack
fields vertically, including SMTP username and password.

The Codex dashboard has a **Connect account** action. Accounts that report an
authentication pause show **Sign-in required** and a **Reconnect** action with
the existing account name filled in. **Usage available** describes the received
data, not an independent credential check. Each card shows its last data time.
Provider settings also offer **Reconnect** beside saved accounts.

In **Settings → General → Homepage prompts**, you can hide the dashboard's
**Connect account** and **Reconnect** buttons. Connection tools remain available
under **Settings → Providers**. You can also hide SMTP setup reminders there,
or check **Don't remind me about SMTP setup** on the homepage reminder itself.
These choices are saved for this OnWatch installation. SMTP setup reminders
only appear when the Email delivery channel is enabled and its destination is
incomplete. Hiding a reminder does not change notification delivery or filters.

SMTP and webhook event filters also include quota exhaustion, advance window
reset reminders, and banked reset expiry. See [notification controls and exact
times](WEBHOOK_SETUP.md#window-ending-and-exhaustion-alerts) for configuration
and polling limits.

Browser sign-in walks through naming the account, approving the device code at
OpenAI, and returning to OnWatch. Approval checks use the provider's polling
interval and do not overlap. Expired codes offer a new attempt; closing the
dialog stops checks and aborts pending browser requests. A successful connection
stays visible until **Done** reloads usage. Copy failures are reported honestly.

**Import auth.json** accepts a chosen file or pasted JSON. It checks that the
input is a JSON object before sending it to the existing OnWatch profile API.
Nothing is sent until **Connect account** is selected. Pasted contents are cleared
after saving or closing. Authentication and token storage stay in the existing
backend; these changes do not log out or reconnect an account by themselves.

The OnWatch login page explains which credentials to use, labels password
visibility, warns about Caps Lock, and shows progress while preventing duplicate
submissions. Returning with the browser's Back button restores the form controls.
It initializes without loading charts or making dashboard settings requests.
Dashboard chart libraries use the same pinned versions as upstream, served from
OnWatch's embedded assets so CDN delays cannot hold up the account controls.
Source URLs and licenses are in `internal/web/static/vendor/README.md`.

## Live usage updates

The fork includes upstream v2.14.15, including Codex reset-credit reporting,
Cursor usage fixes, provider polling status, and DeepSeek peak-hour information.
Auto-start badges use a compact label with space before it. Table pagination
stays in one horizontal row, scrolling within the footer when a narrow screen
cannot fit every button. Padding separates page buttons from the scrollbar.
Account quota labels wrap on narrow screens so their Auto-start badges remain
visible beside the usage percentage.

Visible dashboards check OnWatch's stored usage every five seconds. Provider
requests remain in the background agent; opening extra tabs does not multiply
those requests. History, insights, and visible tables refresh once per minute.
Hidden tabs pause automatic data refresh and catch up when visible again. Slow
requests do not overlap. Unchanged snapshots keep existing cards and charts;
countdowns continue locally. Account overview cards update their text, progress
bars, and status in place, preserving focus and avoiding entrance animations
when fresh snapshots arrive. Manual refresh still fetches immediately.

This server uses `ONWATCH_POLL_INTERVAL: 60` in its OnWatch Compose environment,
so enabled providers are polled about once per minute. Other deployments can set
that same non-secret option and recreate OnWatch to apply it. Existing disabled
pollers, authentication pauses, and provider retry/backoff behavior still apply.
Provider-side reporting can lag; this is near-real-time monitoring, not a stream
of individual prompts or token events.

## Fast Linux image delivery

Pushes to `main` publish the Linux AMD64 Alpine image through **Sync Upstream &
Build**. Scheduled runs still merge upstream changes first. The builder checks
out the exact pushed or synced commit, restores Go's dependency/build cache,
runs the dashboard control tests, and compiles through `app.sh --build`.
`.github/Dockerfile.fork` only packages that binary into Alpine. Its dedicated
Docker ignore file sends only the binary as the build context. The upstream
Dockerfile stays available for the original build flow.

Mutable tags remain `alpine`, `latest`, and the root `VERSION`. Every build also
publishes `sha-<full-commit>` for a fixed reference and records the source commit
in its image labels. The running binary uses `<VERSION>+fork.<short-commit>` so
embedded CSS and JavaScript URLs change with each image. Older overlapping builds
are canceled to avoid publishing an outdated mutable tag after a newer build.

UI-only pushes use the image builder's dashboard tests and Linux compilation;
they do not start a second Go CI job. Backend changes run formatting, cached Go
vet, and dashboard checks with a five-minute job limit. Test-only and CI-policy
changes do not rebuild the runtime image. The full Go race/coverage suite runs
only through the **CI** workflow's **Run workflow** action, with fail-fast enabled
and a five-minute timeout per test package. Its job has a fifteen-minute limit.
The extra CI success job has been removed; the Linux job reports its own result.

The manual
**Release** workflow accepts an existing tag, reuses the same image builder,
and publishes the Linux AMD64 binary and checksum after the image is available.
It does not wait for macOS, Windows, ARM emulation, desktop extensions, Homebrew,
or a second container build. Rerunning it updates release assets without deleting
the existing release.

To restore a previous build, select its `sha-<commit>` image tag or digest through
the normal server deployment flow. Provider data and credentials stay in the
existing data mount.

## Account navigation and notification recovery

The sidebar provider button opens that provider's account overview, including
when returning from an account's detail view. Codex and MiniMax show their account
switcher after you select an individual account. Choose **All accounts** in the
switcher, or select the provider button, to return to the overview. The switcher
has a full-width account label when expanded and an account icon when collapsed;
arrow keys, Enter and Escape work in its menu.

Notification settings must finish loading before they can be saved. If the load
fails, reload the settings page; blank fields cannot overwrite the saved setup.
The dashboard shows when an enabled email or webhook channel needs a destination.
This status describes saved configuration, not confirmed delivery.

For missing webhook alerts, open **Settings > Notifications**, enable the webhook
channel, restore its destination and save. Select the events to deliver and check
the global event switches and thresholds too. The test button uses the saved
configuration and bypasses event filters. A successful test alone does not prove
that a usage threshold has fired. With repeats off, threshold alerts fire once
per account, quota and cycle. Codex reset callbacks now preserve the account ID,
so resetting one account clears its own alert history and leaves others alone.
No existing alert history is deleted during the upgrade.

## Reset reporting and readable alerts

The Codex dashboard includes **Recent resets** below the account cards. The
all-accounts view groups limits observed resetting together under each account.
Selecting an account limits the history to that account. Expand an entry to see
recorded usage before and after, the previous due date, the next due date, and
the evidence used for classification. Dates follow your dashboard timezone.
Existing cycle and usage records supply history after upgrading; no historical
credit redemption is invented when older data lacks that evidence.

- **Scheduled reset:** the observation matches or follows the previous due time,
  allowing two minutes for polling and clock differences. This is a timing-based
  classification, not an assertion about who triggered it.
- **Banked reset used:** a previously available or redeeming credit was reported
  as redeemed between the two usage samples. That confirms a redemption was
  observed in the same interval; it does not identify the person who used it.
- **Early reset, cause unconfirmed:** usage reset before the previous due time
  without a confirmed credit redemption in that interval. The provider response
  does not distinguish a provider-issued reset from another manual action.
- **Cause unknown:** the previous deadline or surrounding usage samples were not
  recorded, or a monitoring gap of more than five minutes prevents a reliable
  comparison. A credit change across that gap does not establish this reset
  used the credit.

A falling credit count or a credit disappearing from the response is not enough
to claim redemption. A credit already marked redeemed on first discovery has no
known redemption time. Credit details are refreshed before reset classification
when the normal refresh is due or the usage response reports a count change;
this does not add a new polling loop or perform a reset.

Discord alerts use account names, readable limit labels, grouped details, and
Discord timestamps that adapt to the reader's timezone. Reset alerts include
before/after usage and both deadlines. Window-start alerts explicitly describe
the starter request as separate from resetting usage. Generic webhook event
names and account IDs remain available; optional `account_name` and `reset`
fields carry the extra context. Email uses the same readable reset explanation.
Unknown usage is omitted rather than presented as a measured zero.

The upgrade adds a nullable redemption-observation timestamp to stored credits.
It keeps existing cycles and notification deduplication intact. Rolling back the
image leaves that extra column in place; older builds ignore it. New formatting
applies to future alerts and does not resend old Discord messages.

## Fork versions and history

The fork's own release numbering starts at 1.0.0 on upstream commit
`396b93a` (v2.14.15 plus its subsequent test fixes). Its initial main branch
contains one consolidated fork commit above that upstream base. Later fork
changes can use normal commits and increment the fork version.

Fork releases use `fork-v<version>` tags to avoid replacing inherited upstream
tags. The updater checks TheBadFella/onWatch releases and downloads matching
fork-tagged assets. The inherited upstream `v1.0.0` tag remains unchanged. The app's VERSION
and image version tag use `1.0.0`. Future upstream merges must preserve the
fork's VERSION and release links. Existing binaries and data remain compatible;
this version reset does not downgrade or reset stored usage.

See [cost tracking](FORK_COSTS.md) for monthly subscriptions and API-equivalent
estimates. Quota percentages cannot establish token spend.

## Account costs

From fork 1.0.3, usage stays first. A compact Cost section sits below each
account's usage, with one monthly total after the account cards. Selecting an
account scopes both subscription and token value to that account. Plus uses
USD20/month, about CAD28 at the current conversion rate. Weekly value follows
the used share of the plan's weekly fee; it is not an extra bill. Details and
optional overrides are in the [cost guide](FORK_COSTS.md).
