# onWatch - personal fork

This is my fork of [onllm-dev/onWatch](https://github.com/onllm-dev/onWatch), maintained for my own multi-account monitoring setup.
The changes focus on the dashboard, account connections, subscription costs, reset visibility, and notifications.

The original application, provider integrations, polling, storage, and core functionality come from upstream.
This README covers the additions and changes in this fork.

## Dashboard and account navigation

- A collapsible left sidebar with a compact icon view, mobile navigation, and keyboard controls.
- A revised dark theme with neutral card surfaces and clearer spacing.
- Account overviews and individual account views, with the selected Codex account name shown above its details.
- Available quota percentages and live reset countdowns on account cards and provider headings.
- Usage updates that preserve existing cards, charts, expanded details, and keyboard focus instead of rebuilding the page on each poll.
- Consumption-rate explanations beside table headings, with pagination kept separate.
- Locally served chart assets so account controls do not depend on a CDN loading first.

## Account connections and settings

- Visible **Connect account** and **Reconnect** actions for Codex accounts.
- A guided device-login flow with account naming, expiry handling, cancellation, and a clear completion step.
- File or pasted `auth.json` import through the existing account API.
- Clearer sign-in, stale-data, and last-updated information on account cards.
- Reorganized notification settings, collapsible delivery sections, and improved mobile forms.
- Optional homepage connection prompts and dismissible SMTP setup reminders.

## Subscription costs

- Account-specific cost sections below usage, with combined totals in the overall view.
- Automatic subscription price references for recognized Codex plans, plus manual account price overrides.
- CAD-first display with USD reference values and a cached Bank of Canada exchange rate.
- Weekly subscription value used, calculated from each account's monthly fee and reported weekly utilization.
- Separate API-equivalent token pricing when request telemetry is available.

These figures are estimates, not invoice reconciliation.
Subscription quota percentages are not treated as token counts or additional API charges.
See [cost tracking](docs/FORK_COSTS.md) for the calculation, configuration, and handling of unavailable data.

## Reset visibility

- Expandable banked-reset details showing reported titles, expiry dates, and plan restrictions.
- Explicit labels for unavailable details, unknown expiry dates, and retained data from an earlier successful check.
- Recent reset history grouped by account, including usage before and after, previous and next reset times, and the evidence used to classify the event.
- Separate labels for scheduled resets, observed banked-credit redemption, and early resets whose cause cannot be confirmed.

The dashboard does not redeem banked reset credits.
A reset countdown reaching zero is not presented as confirmation that an account's quota has already refreshed.

## Notifications and quota-starter behavior

- More readable Discord embeds and email messages with account names, quota-window labels, reset details, and timestamps.
- Additional SMTP and webhook event controls for exhausted quotas, approaching resets, and banked-credit expiry.
- Weekly usage, remaining percentage, and reset time alongside Codex and Claude quota alerts.
- Weekly quota context in Codex auto-starter notifications, including an explicit weekly-exhaustion message when recent account data confirms it.
- Starter requests are skipped while the latest poll reports an exhausted weekly quota.
- Repeated starter failures are suppressed per account and window after a notification is delivered; a successful start or observed reset clears that record.
- Failed notification delivery remains retryable, and unknown usage is not displayed as a measured zero.

See [webhook and notification setup](docs/WEBHOOK_SETUP.md) for event filters and delivery configuration.

## Antigravity and fork builds

- The fork's Alpine image includes the `agy` CLI so OnWatch can use the managed CLI source inside the container.
- An explicit `ANTIGRAVITY_SOURCE=cli` selection takes priority over a configured IDE bridge URL.
- Builds and releases focus on Linux AMD64, with cached compilation and a separate runtime-image step.
- Images are published with moving tags and commit-specific tags for deployment and rollback.

Antigravity still requires a valid sign-in.
See [Antigravity setup](docs/ANTIGRAVITY_SETUP.md) for source selection and authentication.

## Quick start

The fork's moving Docker image tag is:

```text
ghcr.io/thebadfella/onwatch:alpine
```

Use `sha-<full-commit>` to pin a specific build.
A pinned commit tag will not report an update when a newer commit is published.
The image runs as a non-root user and stores persistent data under `/data`.

For provider configuration and installation details, use the [upstream README](https://github.com/onllm-dev/onWatch#quick-start) and this repository's [setup guides](docs/).
The upstream installers and packages install upstream builds; use this fork's image or [fork releases](https://github.com/TheBadFella/onWatch/releases) for these changes.

## Documentation

- [Fork dashboard and delivery guide](docs/FORK_UI.md)
- [Subscription costs and API token value](docs/FORK_COSTS.md)
- [Codex account setup](docs/CODEX_SETUP.md)
- [Antigravity setup](docs/ANTIGRAVITY_SETUP.md)
- [Notification and webhook setup](docs/WEBHOOK_SETUP.md)

## Upstream credit and license

[onWatch](https://github.com/onllm-dev/onWatch) is developed by [onllm.dev](https://onllm.dev) and its contributors.
Their work provides the foundation for this fork.
Please use the upstream project for its original documentation, supported distribution channels, and contribution guidelines.

This fork retains the [GNU General Public License v3.0](LICENSE).
Existing contributor credits and third-party license notices remain in the repository.
