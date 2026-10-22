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

## Live usage updates

The fork includes upstream v2.14.14, including Codex reset-credit reporting,
Cursor usage fixes, provider polling status, and DeepSeek peak-hour information.
Auto-start badges use a compact label. Table pagination stays in one horizontal
row, scrolling within the footer when a narrow screen cannot fit every button.

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

Routine CI stays Linux-only and runs alongside image delivery. The manual
**Release** workflow accepts an existing tag, reuses the same image builder,
and publishes the Linux AMD64 binary and checksum after the image is available.
It does not wait for macOS, Windows, ARM emulation, desktop extensions, Homebrew,
or a second container build. Rerunning it updates release assets without deleting
the existing release.

To restore a previous build, select its `sha-<commit>` image tag or digest through
the normal server deployment flow. Provider data and credentials stay in the
existing data mount.
