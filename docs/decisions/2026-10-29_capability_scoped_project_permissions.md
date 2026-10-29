# 2026-10-29 Gate new project capabilities behind their own permission keys

* status: proposed
* date: 2026-10-29
* authors: Aaron Hammer

## Context and Problem Statement

Stakeholders asked for test quarantine to be restricted: quarantining a test causes test selection to skip it, so
quarantining the wrong test can hide real failures. Before this change, anyone with edit access to a project's tasks could
quarantine tests ([DEVPROD-44841](https://jira.mongodb.org/browse/DEVPROD-44841)).

Project permissions are stored as a map of permission key to integer level, and a check passes when the user's level is
at least the required level. Each key (e.g. `project_tasks`) is therefore a linear ladder. Every level includes all the
levels below it.

## Considered Options

1. **Add a level to an existing ladder** (e.g. `project_tasks: 25`, between edit and admin). This makes every higher
   level include the new capability implicitly. Each later capability would need yet another rung, and it adds
   capability-specific values (e.g. `EDIT_AND_QUARANTINE`) to the GraphQL `AccessLevel` enum, which every permission shares. It also breaks
   MANA, which rejects unknown values for keys it already knows about.
2. **Add a new permission key for the capability** (`project_test_quarantine`). This is the approach taken for task
   annotations (`project_task_annotations`).

## Decision Outcome

Option 2. When a new project capability needs its own access control:

* Add a new permission key with its own short ladder (usually just `None` = 0 and one granted level = 10, leaving
  room for more levels later). The capability doesn't depend on any other key, and no other key grants it.
* In GraphQL, add a value to the `ProjectPermission` enum and map the existing generic `AccessLevel` values
  (`VIEW`/`EDIT`/`ADMIN`) onto the new key's levels. Do not add capability-specific `AccessLevel` values.
* Add the key to `evergreen.ProjectPermissions` so the role manager registers it and it shows up in the permission listing APIs.
* MANA ignores keys it doesn't recognize, so Evergreen can ship the key before MANA does. MANA can't grant the key until
  Evergreen has registered it.

### Project admin roles

Project admin roles are found with `FindRoleWithPermissions`, which requires the role to have every key that's passed in.
If a new key is added to the permissions used for that lookup, admin roles created before the key existed stop matching,
and adding or removing admins fails. A new key that admins should have belongs in `adminPermissions` (used when creating roles)
and **must not** be added to `adminRoleLookupPermissions`. Existing admin roles have to be backfilled separately.

## More Information

* Evergreen PR: https://github.com/evergreen-ci/evergreen/pull/10939
* MANA PR: https://github.com/10gen/mana-evergreen-integration/pull/331
