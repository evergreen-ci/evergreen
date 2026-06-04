# DEVPROD-25338: Migrate EC2 On-Demand Distros to CreateFleet

## What / Why
Evergreen's EC2 distros are split ~50/50 between two code paths:
- `ec2FleetManager` (uses AWS `CreateFleet` API) — the preferred path
- `ec2Manager` (uses legacy on-demand `RunInstances` API) — tech debt

The goal is to migrate ALL on-demand distros to `ec2FleetManager`, then delete `ec2Manager`
entirely. Benefit: one code path, one API call per host request (no retry chains).

## Branch State
Two commits already exist on this branch:
1. `DEVPROD-25338: Migrate ec2-ondemand distros to use ec2FleetManager (CreateFleet)` — routing change
2. `DEVPROD-25338: Populate AWS subnets in integration test for ec2FleetManager` — test fix

PR #10017 is OPEN. Reviewer (ablack12) asked for a rework:
- **DB migration job** — code routing is not enough; existing distro records in the DB still have
  `provider: ec2-ondemand`. A migration job must flip them to `ec2-fleet` so the code path change
  takes effect in production.
- **Full ec2Manager removal** — don't just route around it; delete the dead code path.
- **Defaults** — the existing commit may have changed defaults in a way the reviewer flagged.

**Before writing any new code**: read the existing diff (`git diff origin/main...HEAD`) to
understand exactly what's already in the two commits. Don't duplicate or contradict them.

## 5-Phase Plan

### Phase 1 — DB Migration Job
New file: `units/distro_ec2_migration.go` (Amboy job)

The job:
1. Queries all distros with `provider == "ec2-ondemand"`
2. For each, sets `provider = "ec2-fleet"` and records the distro ID in a migration log
   (MongoDB document) for the revert job
3. Saves the updated distro

Revert job (same file or `units/distro_ec2_migration_revert.go`):
1. Reads the migration log
2. For each logged distro ID, sets `provider = "ec2-ondemand"` back
3. Only touches distros that WERE migrated — never touches ones already on fleet

Register both jobs in `units/crons.go` or as one-shot admin jobs.

### Phase 2 — Isolate Volume Ops + Remove spawn.go Hardcoding
- `cloud/volume.go`: hardcodes on-demand — update to use fleet manager
- `units/volume_deletion_job.go`: same
- `cloud/spawn.go:188`: hardcodes on-demand conversion — remove this conversion

### Phase 3 — Remove ProviderNameEc2OnDemand from Routing
After the DB migration runs, no distro should have `provider: ec2-ondemand`. Remove:
- `globals.go`: `ProviderNameEc2OnDemand` constant (if safe to remove after migration)
- `cloud/cloud.go`: on-demand branch in `GetCloudManager` / manager selection logic
- Any `distro.go` query that filters by `ec2-ondemand`

### Phase 4 — Delete ec2Manager
Files to DELETE:
- `cloud/ec2.go`
- `cloud/ec2_test.go`
- Any client methods on the EC2 client interface that are only used by ec2Manager

Files to MODIFY:
- `cloud/cloud.go`: remove `ec2Manager` registration
- Any interface definitions that ec2Manager satisfies but ec2FleetManager already does

### Phase 5 — Final Verification
- `make lint-cloud` — confirm no orphaned references
- `make lint-units` — confirm migration jobs lint clean
- Check all callers of `GetCloudManager` — confirm none can return ec2Manager anymore
- Integration test: `SETTINGS_OVERRIDE=<file> make test-cloud`

## Key Constraints
- **Revert strategy is required**: migration job must log which distros it touched. The revert
  job only flips those IDs back. Distros already on fleet before the migration are never touched.
- **Amboy job pattern**: look at an existing job in `units/` for the standard boilerplate
  (job struct, `Run`, `ID`, registration). `units/distro_ec2_migration.go` should follow the same pattern.
- **bson.D not bson.M** in all queries — maps iterate randomly and fragment Atlas analytics.
- **No force push** — PR #10017 is open; sync with `git merge origin/main`, never rebase.

## Files Summary
| File | Action |
|---|---|
| `units/distro_ec2_migration.go` | CREATE — migration + revert Amboy jobs |
| `units/crons.go` | MODIFY — register migration job |
| `cloud/volume.go` | MODIFY — remove on-demand hardcoding |
| `units/volume_deletion_job.go` | MODIFY — remove on-demand hardcoding |
| `cloud/spawn.go` | MODIFY — remove on-demand conversion at line 188 |
| `cloud/ec2.go` | DELETE (Phase 4) |
| `cloud/ec2_test.go` | DELETE (Phase 4) |
| `cloud/cloud.go` | MODIFY — remove ec2Manager registration |
| `globals.go` | MODIFY — remove ProviderNameEc2OnDemand |
| `model/distro/distro.go` | MODIFY — remove on-demand provider branches |

## First Step at Session Start
Run `git diff origin/main...HEAD` — read the full diff of what's already committed.
Then proceed with Phase 1 (migration job) since that's the core rework the reviewer asked for.
