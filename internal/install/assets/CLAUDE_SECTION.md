## Recon (Code Intelligence)

This project uses [recon](https://github.com/robertguss/recon) for code
intelligence. A recon orient payload is injected at session start via hook. Use
the `/recon` skill for detailed command reference.

- Use `recon find` for Go symbol lookup (gives dependency info that grep cannot)
- Use `recon recall` to check existing decisions before making architectural
  changes
- Use `recon decide` to record significant architectural discoveries or
  decisions
- Use `recon pattern` to record recurring code patterns you observe
- Use `recon sync` to re-index after major code changes
