# 44. Config management hardening

```text
Harden configuration management for orch-server, orch-agent, and orch CLI.

Task:
Implement consistent config loading.

Sources:
1. command flags
2. environment variables
3. config file
4. defaults

Requirements:
- Use one config package.
- Support YAML config files.
- Add config validation.
- Print effective config with secrets redacted:
  - orch-server config print
  - orch-agent config print
- Add docs/CONFIGURATION.md.
- Add .env.example updates.
- Add tests for precedence:
  - flags override env
  - env overrides config file
  - config file overrides defaults
- Ensure no secret is printed in logs or config output.
```
