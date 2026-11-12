## Fork 1.1.5

- Codex, MiniMax and Antigravity open on the All accounts overview whenever more than one account is linked. A single account picked from the menu applies until the page is reloaded.
- Reset notifications judge timing first. A reset at or after its scheduled time is reported as a scheduled reset, even across a gap in monitoring. Only a reset that comes before its scheduled time is flagged as early.
- Resets with no recorded schedule read "New usage window" instead of "cause unknown".
- New `scripts/link-anthropic-docker.sh` links a Claude account to the container with a login that belongs to onWatch alone.
