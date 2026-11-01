## Fork 1.1.1

- Send one Codex auto-starter notification per request, listing both the 5-hour and weekly limits when applicable. The weekly-usage summary appears once.
- Keep failure deduplication per account and window, including retries after failed notification delivery.

The usage-reset notification remains separate from the subsequent auto-starter notification.
