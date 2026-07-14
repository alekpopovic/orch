# 71. Drift detection between GitOps desired state and cluster state

```text
Implement GitOps drift detection.

Context:
If GitOps is enabled, users may manually change services through the API or CLI. The system should detect and optionally correct drift.

Task:
Add drift detection for GitOps-managed services.

Requirements:
- Mark services managed by a GitOps source.
- Store source commit SHA and file path for each applied service.
- Compare live service spec with Git desired spec.
- Add status:
  - in_sync
  - drifted
  - unknown
  - sync_error
- Add CLI:
  - orch gitops status
  - orch gitops diff <service>
- Support policy:
  - warn only
  - auto-revert drift
- Emit events for drift.
- Add tests:
  - manual scale creates drift
  - manual rollout creates drift
  - auto-revert restores desired state
  - deleted file with prune enabled deletes service
- Update docs/GITOPS.md.
```
