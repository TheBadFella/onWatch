# Subscription costs and quota value

Open **Settings → General → Costs**. Add a subscription, choose a linked account,
and enter the monthly USD fee you pay. OnWatch suggests saved Codex accounts;
the single Claude connection uses provider `anthropic`, account `default`.
For an annual subscription, enter the monthly equivalent (annual fee ÷ 12).
Save with **Save Settings**.

The dashboard automatically calculates monthly and annualized fees, daily
allocations, and the estimated share of the fee used in the current 5-hour and
weekly quota windows. None of these calculations requires token telemetry or
model prices. Subscription fees for other providers can also be recorded;
quota allocations currently support Codex and the single Claude connection.

## USD and CAD

You can enter the **actual monthly CAD fee** for each subscription. For example,
USD20 and an actual CAD30 fee produce monthly totals of USD20/CAD30 and annualized
totals of USD240/CAD360. An entered CAD fee is used as-is, including any taxes,
card fees or conversion charges you include in that number.

Without an entered CAD fee, OnWatch converts USD using the latest published
[Bank of Canada daily USD/CAD rate](https://www.bankofcanada.ca/rates/exchange/daily-exchange-rates/).
The dashboard shows the source and observation date. This is a reference
conversion, not your card's settlement rate. A configured **CAD per USD** rate
can override it. For example, a planning rate of 1.5 converts USD20 to CAD30.
This configured rate is not presented as a live market rate.

The automatic rate is fetched from a fixed public Bank of Canada endpoint with
a three-second timeout. Successful rates are saved across restarts and reused
for 24 hours. Failed lookups back off for an hour and retain the last successful
rate. Rates cached for over 48 hours are labelled as older cached rates. If no
rate has ever been received, CAD conversions remain unavailable; USD values and
entered CAD fees still work. No lookup is needed when every subscription has
an entered CAD fee and no token model rates are configured.

## Billing periods and quota allocations

The billing day defaults to the first of the month and can be set from 1 to 31.
Dates are interpreted in UTC. Days absent from a month clamp to its last day:
a billing day of 31 produces January 31 to February 28, then February 28 to
March 31 in a non-leap year. The dashboard shows the exact period used.

Daily allocation is the monthly fee divided by the number of days in the
current billing period. Annualized fees are the current monthly fee × 12;
they are not a prediction of future prices or exchange rates.

Quota value uses this explicit planning assumption:

```text
window allocation = monthly fee × window duration ÷ billing-period duration
used allocation = window allocation × reported quota percentage ÷ 100
remaining allocation = window allocation - used allocation
```

For a 30-day billing period, a USD20/CAD30 subscription assigns USD4.6667/CAD7
to a seven-day window. At 50% weekly utilization, the allocated fee used is
USD2.3333/CAD3.50. A five-hour window uses the same formula with five hours.
Five-hour and weekly limits overlap, so their values must not be added.
These numbers are neither actual API spend nor extra charges, and do not mean
the provider sells quota at those rates. The subscription remains a fixed fee.

Only the current reported windows are valued. A reset changes the current
window's utilization; it does not add a subscription charge. Missing account
matches, readings older than 15 minutes, expired windows and invalid usage
remain explicitly unavailable. The original observation time is shown. Account
matching is exact; `codex` and `openai` both refer to saved Codex accounts.

## Advanced: API-equivalent token costs

The separate token-cost view remains available under an expandable Advanced
section. It estimates received telemetry at configured model prices and does
not affect subscription or quota allocation calculations.

Add model rates using the exact provider and model ID in your token telemetry.
Enter USD per million regular input, cached input and output tokens. Use your
provider's current prices or your own contracted rates. No default prices are
assumed. An explicit zero means a free rate. Save with **Save Settings**.
Changing a model rate recalculates all retained telemetry at that rate; these
estimates are not an immutable billing ledger. Taxes, discounts, batch rates,
tool fees and other charges are not calculated.

The dashboard separates subscription allocations from API-equivalent token
cost for received telemetry in the current UTC month. It shows account/model
breakdowns, unknown pricing and partial totals. API-equivalent cost is a
reference for token value, not an additional charge on top of a subscription.
Only retained, ingested events are counted. Existing reported `cost_usd` values
remain available through API Integrations and are not added to this estimate.

## Supplying usage

Quota endpoints for connected Codex and Claude accounts do not contain token
counts or per-model token spend. Connecting an account therefore does not
populate token costs. OnWatch does not read your workstation session files
through the quota connection or access Tokaware's data.

Use the existing [API Integrations collector](API_INTEGRATIONS_SETUP.md) to
supply per-request JSONL telemetry in its configured directory. In Docker the
default is `/data/api-integrations`. Your application or collector must write
these events where OnWatch can read them; do not place credentials or prompt
contents in the event stream. Example with made-up token counts:

```json
{"ts":"2026-10-22T18:00:00Z","integration":"coding","provider":"openai","account":"personal","model":"your-model-id","request_id":"unique-request-id","prompt_tokens":1000,"completion_tokens":50,"metadata":{"cached_prompt_tokens":200}}
```

`prompt_tokens` includes cached input tokens. `metadata.cached_prompt_tokens`
records how many input tokens were cache hits, including an explicit `0` when
there were none. It must be an integer between zero and `prompt_tokens`.
Missing or invalid cache counts leave the account/model group unpriced when
regular and cached input prices differ. If both rates are equal, cache counts
do not affect the total. Provider and model matching is exact and case
sensitive. Use `openai` for Codex token telemetry and `anthropic` for Claude.
Token events must contain the actual model used; a quota account alone cannot
identify it. This limitation concerns API-equivalent pricing, not the
subscription-based allocations above.

The calculation is:

```text
((input_tokens - cached_input_tokens) × regular_input_rate
 + cached_input_tokens × cached_input_rate
 + output_tokens × output_rate) / 1,000,000
```

The collector's existing validation, deduplication and retention rules still
apply. The cost dashboard refreshes once per minute while visible. It reads up
to 500 account/model groups; larger results are explicitly marked partial.
Unknown prices and missing telemetry never appear as a confirmed zero cost.
[Tokaware](https://tokaware.com/for/finance) distinguishes subscription seat fees
from token value and provider bills. [CodexLB](https://soju06.github.io/codex-lb/usage-reporting/)
calculates API-equivalent costs from observed request tokens and model prices.
Their published approaches informed this separation. No third-party source
code is copied here; OnWatch does not duplicate their desktop collectors,
proxy routing or billing connectors.
