# Backfill repo permission scopes

One-time repair after deployment of #10976 and #10985.

The script repairs existing `admin_repo_<repo ID>` and `unrestricted_branches_<repo ID>` scopes from `repo_ref` and `project_ref`. Repo admin scopes contain the repo and all attached projects. Unrestricted scopes contain attached projects whose effective restriction is false. An explicit branch flag overrides the repo flag; an absent or null branch flag inherits it. Hidden and disabled attached projects are included, matching Evergreen's scope initialization logic.

Loading the script defines its functions. Calling `backfillRepoPermissionScopes(db)` previews changes by default. The output includes each change and the original and proposed resource arrays. Scope writes preserve the rest of the scope document. Updating scope resources changes effective access for every role that references the scope, including legacy project grants that reused repo scopes. Review both additions and removals before applying. User and role documents are preserved.

The script requires the two existing scopes for each repo to have type `project` and no parent scope, matching their creation in Evergreen. Missing scopes, unexpected parents, or invalid source records stop planning before any writes. Scopes with parents must be reconciled through the role manager to propagate membership changes correctly.

## Preview

From the Evergreen checkout:

```sh
python3 ~/Development/kernel-tools/evergreen/db_access.py evergreen \
  --eval 'load("scripts/backfill-repo-permission-scopes.js"); void backfillRepoPermissionScopes(db)' \
  > /tmp/repo-scope-backfill-preview.json
```

This uses kernel-tools' `EvergreenAtlasRO` profile. Keep the preview output with the rollout record. If kernel-tools is installed elsewhere, adjust its path.

## Apply after review

Coordinate with MANA before rollout. Pause project attachment, detachment, replacement/movement, and branch/repo restriction changes for the duration of apply and verification. Pausing MANA reconciliation alone does not stop all of these writers. `writesPaused: true` records the operator's assertion; it does not install a lock or pause the service.

Connect interactively with the write profile:

```sh
python3 ~/Development/kernel-tools/evergreen/db_access.py evergreen --admin
```

Kernel-tools intentionally disallows `--eval` with `--admin`. In the connected mongosh session, load the script, preview on the primary connection, save the output, then apply:

```js
load("scripts/backfill-repo-permission-scopes.js");
backfillRepoPermissionScopes(db);
backfillRepoPermissionScopes(db, { apply: true, writesPaused: true });
```

Apply reads from primary, writes with majority acknowledgement, and uses the original resource array in each update filter. A concurrent scope change stops execution instead of being overwritten. Source changes still require the coordinated pause because Evergreen can update scopes before persisting project attachments. Updates are per scope, so an interrupted run may have applied some changes; save the output, review a fresh preview, and rerun under the same pause. A successful rerun is a no-op.

The script rechecks all scopes after applying and requires zero remaining drift before completing. Resume the paused operations afterward and run the audit again. Keep the printed before/after arrays and progress log with the rollout record.

## Local validation

The tests use synthetic data and no database connection:

```sh
node --check scripts/backfill-repo-permission-scopes.js
node scripts/backfill-repo-permission-scopes.test.js
```
