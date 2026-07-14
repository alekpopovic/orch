# 51. Security review

```text
Act as a senior application security engineer and review this repository.

Focus:
- Docker socket exposure
- agent authentication
- user authentication
- RBAC
- secrets handling
- registry credentials
- API input validation
- audit logging
- log redaction
- path traversal
- command injection
- SSRF through healthchecks
- unsafe container options
- privilege escalation
- default container security profile

Output:
1. Prioritized security findings
2. Exploit scenario for each high-risk issue
3. Minimal fix
4. Tests to prove the fix
5. Documentation updates

Then implement the critical and high-risk fixes only.

Constraints:
- Do not add large unrelated features.
- Do not weaken developer experience unnecessarily.
- Do not log secrets or tokens.
- Run tests at the end.
```
