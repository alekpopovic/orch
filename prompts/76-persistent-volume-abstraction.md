# 76. Persistent volume abstraction

```text
Implement persistent volume abstraction MVP.

Context:
The orchestrator needs a storage model before stateful workloads can be safely supported.

Task:
Add storage primitives.

Models:
- Volume
- VolumeClaim
- VolumeAttachment

Requirements:
- Support local Docker volumes first.
- Add volume claim references in service/job specs.
- Scheduler must place tasks on nodes where required local volume exists or can be created.
- Agent must create/attach Docker volumes.
- Store volume attachment state.
- Prevent two tasks from writing to the same ReadWriteOnce volume unless policy allows.
- Add API:
  - POST /v1/volumes
  - GET /v1/volumes
  - GET /v1/volumes/{id}
  - DELETE /v1/volumes/{id}
  - POST /v1/volume-claims
  - GET /v1/volume-claims
- Add CLI:
  - orch volume ls
  - orch volume create
  - orch volume inspect
- Add tests:
  - create local volume
  - schedule task with volume
  - block conflicting attachment
  - cleanup after task stop
- Update docs/STORAGE.md.

Do not implement distributed storage yet.
```
