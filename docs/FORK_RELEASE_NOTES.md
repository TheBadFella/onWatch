
## Fork 1.0.0

- Starts the fork's own version series on upstream v2.14.15, including the latest chart and test fixes.
- Keeps the AMOLED dashboard, collapsible sidebar, account overview, mobile settings and provider connection tools.
- Adds configurable monthly subscription fees and API-equivalent cost estimates from received token telemetry. Unknown prices and cache counts remain explicitly unpriced.
- Includes clearer reset evidence, exact reset timestamps, and separate SMTP/webhook event controls.
- Publishes a Linux AMD64 binary and container image.

### Upgrade and setup

Existing OnWatch data and provider settings are retained. The version number
starts again at 1.0.0 for this fork; this is not a rollback to upstream v1.0.0.
Configure Costs in Settings → General. Token estimates require telemetry, not
just connected quota accounts. See [cost setup](https://github.com/TheBadFella/onWatch/blob/fork-v1.0.0/docs/FORK_COSTS.md) and the
[dashboard guide](https://github.com/TheBadFella/onWatch/blob/fork-v1.0.0/docs/FORK_UI.md). Thanks to onllm-dev and the upstream contributors;
upstream authorship and licensing are preserved.
