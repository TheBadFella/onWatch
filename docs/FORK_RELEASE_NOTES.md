## Fork 1.1.7

- The database grows far more slowly. When a poll repeats the previous reading, onWatch keeps only the first and newest snapshot of the unchanged run. Every value and the time it changed are kept, and no history is deleted by age.
- Raw provider responses are no longer stored with Codex, Antigravity, Command Code and Anthropic API snapshots. Nothing read them, and they made up most of the file.
- The first start of this version clears the old raw responses, collapses repeated snapshots already stored, and compacts the database once. On a large database this adds a few seconds to that start. The production database went from 182 MiB to 22 MiB.
- 5-hour reset and reset-soon alerts are skipped while the same account's weekly limit is at 100%, including right after a restart.
- Charts draw a long stretch with no change in usage as a solid line instead of a monitoring gap.
