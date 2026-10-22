# Costs in the dashboard

Usage stays first. A small **Cost** section follows each account's usage.
The overall view shows the monthly total across accounts and their combined
weekly value used. Selecting an account shows only that account's costs.

Connected Codex plans fill in automatically. A Plus account uses USD20/month;
with an exchange rate of 1.4021, that is about CAD28.04. Two Plus accounts total
USD40/month, about CAD56.08. Money is rounded to cents.

## Automatic prices and CAD

Canada is the default, with CAD first and USD underneath. The fixed USD
reference prices checked on October 22, 2026 are:

| Reported Codex plan | Monthly USD reference |
| --- | --- |
| Free | 0 |
| Plus | 20 |
| Pro Lite | 100 |
| Pro | 200 |
| Pro Max | 500 |
| Business / Team | 20 per Standard seat, annual billing equivalent |

Sources: [ChatGPT pricing](https://chatgpt.com/pricing/),
[Plus](https://help.openai.com/en/articles/6950777),
[Pro tiers](https://help.openai.com/en/articles/9793128), and
[Business seats](https://help.openai.com/en/articles/8792828).
These are plan references, not invoice reconciliation. For Business, monthly
billing or a Premium seat costs more; the automatic reference assumes a
Standard seat billed annually. Unknown and Enterprise prices remain unavailable.
Claude quota responses do not identify the plan, so an optional override is
needed for its subscription price. Unpriced accounts make totals partial.

CAD uses the dated [Bank of Canada daily rate](https://www.bankofcanada.ca/rates/exchange/daily-exchange-rates/).
It is a conversion estimate rather than a Canadian checkout price. Taxes,
discounts and card charges are not inferred. Rates are reused for 24 hours and
persist across restarts. On failure, the last successful rate is retained;
without one, USD remains available. Failed lookups back off for an hour.

For a different amount, open **Settings → General → Costs → Add price override**.
Choose the linked account and enter the monthly amount. An actual CAD fee takes
priority over conversion. Removing the override returns to automatic pricing.
An optional CAD-per-USD setting overrides the daily rate. No fee entry is
required for supported Codex plans.

## Used this week

This is subscription value used, not an additional bill or API token spend.
It follows the weekly value approach found in Tokaware's published dashboard:

```text
weekly plan value = monthly fee × 12 ÷ 52
weekly value used = weekly plan value × weekly quota used ÷ 100
```

For Plus at USD20/month and 50% weekly usage, this is about USD2.31 of weekly
subscription value. At a CAD28.04 monthly reference, it is about CAD3.24.
Each account is calculated separately, then combined for the overall total.
The five-hour session is not added. When several weekly model windows overlap,
the highest valid weekly percentage is used once rather than summing windows.

Missing, expired or stale readings show as unavailable. A known zero remains
zero. The subscription fee stays visible when usage is unavailable. The weekly
value resets with the provider's weekly quota; it is not cumulative monthly
spend. Costs update after usage changes without refreshing the page.

The API also retains the earlier `windows` allocation fields for compatibility.
The dashboard uses the new `weekly` value, which is independent of month length.

## API token value

Received token telemetry is priced separately in a collapsed **API token value**
section. It is filtered to the selected account. Subscription quota percentages
do not provide token counts or a model-by-model API bill.

Use the existing [API Integrations collector](API_INTEGRATIONS_SETUP.md) to supply
request telemetry. Configure exact provider/model prices under
**Settings → General → Costs → Advanced: API-equivalent model rates**. Prices
are USD per million regular input, cached input and output tokens. Input totals
include cached tokens. Missing cache counts remain unpriced when cached and
regular rates differ; explicit zero means free. Totals cover retained events in
the current UTC month, with results above 500 account/model groups marked partial.

The [Tokaware inspection notes](FORK_COST_RESEARCH.md) record the published
artifacts examined with REA and the limits of that inspection. No Tokaware
source code or assets are included in OnWatch.
