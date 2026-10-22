# CodexLB monitoring features for OnWatch

Assessment on October 22, 2026. This is an integration proposal, not a list of
features shipped in OnWatch. Proxying, load balancing, request forwarding,
credential pooling and automatic account routing are outside scope.

Reviewed upstream commit `72630bbfeee9e3b0d9acfb95e2b1e706b8b0188e` and the
project's published [usage reporting guide](https://soju06.github.io/codex-lb/usage-reporting/).

## Useful additions without a proxy

| Feature | Fit in OnWatch | Data needed |
| --- | --- | --- |
| Automatic model prices | Replace required manual API model rates with a validated catalog, last-good disk cache and bundled fallback. Keep optional overrides. | Public model-price catalogs; no provider credential required. |
| Token accounting | Show input/output totals, cached input and reported reasoning separately. Never add cached or reasoning subsets twice. Keep unknown distinct from zero. | Actual client usage events. |
| Account/model reports | Add daily history, date filters and CSV export to existing usage records, with the selected account applied consistently. | Retained usage events with stable account identity and model IDs. |
| Quota sorting | Sort accounts by remaining quota or reset time, with unavailable readings last. This changes presentation only. | Existing provider quota observations. |
| Health and performance | Summarize failures and latency where events report them. Keep provider polling health separate from individual request health. | Provider poll outcomes already exist; request outcomes and timings need client events. |

## Implementation path

Start with automatic API model pricing and account quota sorting. These improve
the current app without changing client request paths. Reuse Go's HTTP/JSON
support, the existing settings/store patterns and the API Integrations ingestion
path instead of importing the Python/FastAPI application or its proxy stack.

OnWatch already ingests per-request JSONL with provider/account/model, input and
output tokens, optional cost and latency, plus metadata. Its cost query reads
cached-input counts from metadata. Extend this existing contract for reported
reasoning and request outcomes before building reports around them.

For token detail without a proxy, a client-side collector must supply actual
Codex session usage events to OnWatch. The server's quota endpoint cannot supply
these counts. The collector should send usage metadata only, identify accounts
explicitly, deduplicate events and preserve the last import position. Do not
claim request-level statistics from quota percentages or scrape credentials.

Keep the subscription fee and quota-derived weekly value distinct from
API-equivalent token value, as in the account-cost correction. Automatic model
prices affect token value, not the monthly subscription fee.

## Source and license

- [Pricing math](https://github.com/Soju06/codex-lb/blob/72630bbfeee9e3b0d9acfb95e2b1e706b8b0188e/app/core/usage/pricing.py)
- [Catalog adapters and validation](https://github.com/Soju06/codex-lb/blob/72630bbfeee9e3b0d9acfb95e2b1e706b8b0188e/app/core/usage/pricing_catalog.py)
- [Report contract](https://github.com/Soju06/codex-lb/blob/72630bbfeee9e3b0d9acfb95e2b1e706b8b0188e/app/modules/reports/schemas.py)
- [MIT license](https://github.com/Soju06/codex-lb/blob/72630bbfeee9e3b0d9acfb95e2b1e706b8b0188e/LICENSE)

MIT permits adaptation with its copyright and license notice retained where
required. No CodexLB source code was copied in this assessment or in the
account-cost layout fix. A Go port would need its own tests and attribution.
