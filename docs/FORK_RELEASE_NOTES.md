
## Fork 1.0.2

- Detect connected Codex plans automatically and calculate monthly, annualized and daily cost estimates without entering a fee.
- Default to Canada with CAD shown first. Display price ranges when billing frequency or seat tier is not reported, and mark unknown accounts as unpriced.
- Use the actual CAD amount you pay, a configured conversion rate, or a dated Bank of Canada daily rate. Automatic rates are cached across restarts.
- See the allocated subscription fee used and remaining in each current Codex or Claude quota window, without token telemetry or model prices.
- Keep optional price and billing-day overrides, including safe handling of short months. Removing a price override returns the account to automatic detection.
- Keep API-equivalent token pricing in a separate Advanced section. It remains independent of subscription charges.

Quota values are planning allocations, not actual API bills. Five-hour and
weekly windows overlap and must not be added together. Unknown or stale usage
is not treated as zero. Existing subscription settings remain compatible.

See [cost setup, formulas and currency behavior](https://github.com/TheBadFella/onWatch/blob/fork-v1.0.2/docs/FORK_COSTS.md).
Thanks to the upstream OnWatch contributors. Tokaware and CodexLB's published
cost-reporting approaches informed the separation of fees and token estimates;
no third-party source code was copied.
