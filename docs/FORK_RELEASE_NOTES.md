## Fork 1.1.9

- Start Claude's five-hour window automatically with a short Haiku prompt. Enable the beta starter in **Settings → Providers → Anthropic** and use **Auto** or **API Only** usage tracking.
- The starter skips accounts with exhausted weekly quota or usage credits enabled. Its cooldown survives daemon restarts.
- The starter requires Claude Code on the daemon's PATH. The fork's Alpine image also searches `/data/antigravity/.local/bin` for a persistent installation.
