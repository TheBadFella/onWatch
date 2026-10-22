# Tokaware cost presentation review

Inspected on October 22, 2026 for the OnWatch account-cost correction.

## Evidence

The signed-in Chrome connection was unavailable. No private Tokaware account
or billing data was inspected. The public dashboard HTML and its published
JavaScript chunks were downloaded and inspected as inert text with REA.

- Dashboard: <https://tokaware.com/dashboard>
- Account-card chunk: <https://tokaware.com/_next/static/chunks/app/dashboard/page-7afd7ce1bcfa6ac0.js>
- Cost/math chunk: <https://tokaware.com/_next/static/chunks/5358-25116fd5cf0a52b6.js>
- Account-card SHA256: `34702fc1bca1afcb1f9c2ee77f44f147227521d2ab9d53930ec304fd58f35788`
- Cost/math SHA256: `cac925e94a7fac9a5e3443df9c178cf3a50d7fd91abc2f0b71862d0d986c4ecb`
- REA analysis Evidence: `ev_9695eeabd0cd4876fb62c854226cbcfcc3540157f658dd111b079fc52d3be807`
- REA summary Evidence: `ev_27ef0f14f1674a34d6572584529647e1557ab5a022e47b3fc4c0711558a65a5e`
- Analyzed artifact-set SHA256: `b069bf394e3a0d80088e73f864072be6c36fd7c8c92e246a8f3de33a33a95f02`

REA recovered Webpack module identities and source ranges from three selected
chunks, with no parse failures. Its call/data-flow graph was partial. Focused
reads of the hashed chunks established the following static implementation
observations; they do not establish runtime execution in the user's session.

## Observations and adaptation

- Account headers show a small fixed monthly plan price rather than a range.
- Subscription totals are separate from token-based API value.
- The plan catalog recognizes Plus at USD20 and Pro at USD200.
- The weekly calculation divides the monthly fee by 52/12, multiplies by the
  highest non-session usage percentage, and counts each account once.
- The session classifier identifies windows of at most five hours, or the
  corresponding session window keys when duration is absent.
- Accounts with unknown plan prices are excluded from priced totals and counted
  as unpriced rather than free.

OnWatch independently implements the weekly-value calculation and concise
presentation using its existing provider observations and account IDs. It uses
current OpenAI reference prices rather than copying Tokaware's whole catalog;
for example, its Business reference is the Standard annual equivalent. It
retains explicit stale/expired checks and separate API telemetry pricing.
The user-requested layout keeps cost below usage and filters account details.
No source code or assets from the inspected bundles were copied into the app.
