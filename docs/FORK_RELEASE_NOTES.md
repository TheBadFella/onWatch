## Fork 1.1.0

- Receive an account-specific notification when new Codex reset credits become available. Existing credits establish a quiet baseline; repeated polls and restarts do not re-announce them. Email, push, and webhook delivery reuse your notification settings.
- Track multiple Antigravity accounts through separately authenticated bridge endpoints. Select an account to see its own usage, charts, sessions, and reset history. Edit bridge connections from Settings, and use keyboard controls in the account selector.
- Verify each bridge's reported email before storing usage. A mismatched account cannot write into another account's history.
- Preserve existing single-account Antigravity data during the upgrade.
- Normalize history and cycle query bounds to UTC so servers in other timezones show the expected usage.

Additional Antigravity accounts require reachable, separately authenticated bridge endpoints. This release does not provide multiple managed `agy` logins. Refresh a bridge's CSRF token in **Settings -> Providers -> Antigravity -> Edit connection** after its server restarts.

If you have saved individual email or webhook event selections, enable the new reset-credit grant event in that channel's settings.

Reset-grant messages appear after the next detail refresh, which can take up to 30 minutes when the available-credit count stays unchanged.
