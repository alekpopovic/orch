# Stateful workloads design

## Goals and non-goals

The goal is stable ordinal identity, persistent storage, conservative replacement, ordered lifecycle, and observable recovery. This design does not promise distributed storage, synchronous replication, automatic database failover, or safe fencing for arbitrary applications.

Tasks are named `<workload>-0..N`; ordinals and claims survive rescheduling. Startup defaults to ordinal order and readiness gates; shutdown reverses it. Headless discovery publishes workload and per-ordinal names. Parallel startup is opt-in.

Each claim binds once. Local volumes pin an ordinal to a node; remote drivers may advertise topology. The scheduler combines topology with resources and rejects a second `ReadWriteOnce` writer. Claims are not deleted with Tasks. Snapshot and backup hooks precede risky replacement, emit events, and have explicit timeouts.

An unreachable node is not proof that its process stopped. Automatic replacement can create split brain. Local-volume workloads remain blocked until the node returns or an operator confirms fencing and force-detach. Every forced action requires audit history and a data-loss warning.

Default rollout is ordered, highest ordinal first, with one unavailable Task, no writable-claim surge, and readiness before continuing. Partitioned rollout enables canaries. Rollback restores the spec but cannot restore storage without an application-aware snapshot.

The proposed `StatefulService` YAML adds `serviceName`, template, `volumeClaimTemplates`, ordered/parallel policy, update partition, and retention. Scheduler input gains ordinal and volume topology; agents gain idempotent attach/mount/detach plus hooks. Tests cover ordinals, ordering, RWO conflicts, agent restart, detach, migrations, Docker local volumes, fencing, partitions, backup failures, and interrupted rollout.
