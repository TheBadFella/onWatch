# Subscription fees and token cost estimates

Open **Settings → General → Costs**. Add subscriptions with the provider,
account name and the monthly USD fee you actually pay. Annual subscriptions can
be entered as your annual fee divided by 12. These are full monthly fees, not
prorated invoices or amounts inferred from quota utilization.

Add model rates using the exact provider and model ID in your token telemetry.
Enter USD per million regular input, cached input and output tokens. Use your
provider's current prices or your own contracted rates. No default prices are
assumed. An explicit zero means a free rate. Save with **Save Settings**.
Changing a model rate recalculates all retained telemetry at that rate; these
estimates are not an immutable billing ledger. Taxes, discounts, batch rates,
tool fees and other charges are not calculated.

The dashboard separates monthly subscription fees from API-equivalent token
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
identify it.

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
This feature is inspired by Tokaware's distinction between seat fees and
API-equivalent token value; it does not duplicate its desktop collectors or
provider billing connectors.
