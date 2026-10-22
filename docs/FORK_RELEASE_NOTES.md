
## Fork 1.0.1

- Enter a subscription fee once to calculate monthly, annualized and daily totals in USD and CAD.
- Use the actual CAD amount you pay, a configured conversion rate, or a dated Bank of Canada daily rate. Automatic rates are cached across restarts.
- See the allocated subscription fee used and remaining in each current Codex or Claude quota window, without token telemetry or model prices.
- Link saved accounts automatically and choose the billing day, including safe handling of short months.
- Keep API-equivalent token pricing in a separate Advanced section. It remains independent of subscription charges.

Quota values are planning allocations, not actual API bills. Five-hour and
weekly windows overlap and must not be added together. Unknown or stale usage
is not treated as zero. Existing subscription settings remain compatible.

See [cost setup, formulas and currency behavior](https://github.com/TheBadFella/onWatch/blob/fork-v1.0.1/docs/FORK_COSTS.md).
Thanks to the upstream OnWatch contributors. Tokaware and CodexLB's published
cost-reporting approaches informed the separation of fees and token estimates;
no third-party source code was copied.
