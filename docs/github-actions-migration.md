# GitHub Actions Migration Plan

## Overview

This document tracks the migration of Evergreen self-tests to GitHub Actions. This is a **parallel migration** - we are NOT removing anything from `self-tests.yml`. Both systems will run simultaneously.

**Last Updated:** 2025-11-03

---

## Migration Scope

### In Scope (Phase 1)
- **Variant:** `ubuntu2204` only
- **Tasks:** All `test-*` tasks (52 total, excluding smoke tests)
- **Patterns:**
  - `run-go-test-suite` (no database)
  - `run-go-test-suite-with-mongodb` (with MongoDB)
  - `run-go-test-suite-with-mongodb-useast` (with MongoDB + timezone)

### Out of Scope (Future Phases)
- Other variants (race-detector, windows, osx, arm64, etc.)
- Smoke tests (`test-smoke-*`)
- Linter tasks
- Build tasks
- Docker-based tests (`test-cloud` uses `run-go-test-suite-with-docker`)
- `js-test` (special JS testing setup)

---

## Migration Status

**Progress:** 2/52 test tasks migrated (Phase 1B POC)

### Status Legend
- ✅ **Migrated** - Running in GitHub Actions
- 🚧 **In Progress** - Being worked on
- ⏸️ **Deferred** - Out of scope for Phase 1
- ⬜ **Not Started** - Planned but not yet implemented

---

## Task Inventory

### No-DB Tests (7 tasks)
Uses pattern: `run-go-test-suite`

| Task Name | Status | GHA Job Name | Notes |
|-----------|--------|--------------|-------|
| test-agent-internal-client | ⬜ | `test-agent-internal-client` | |
| test-agent-internal-taskoutput | ⬜ | `test-agent-internal-taskoutput` | |
| test-agent-util | ⬜ | `test-agent-util` | |
| test-cloud-userdata | ⬜ | `test-cloud-userdata` | |
| test-thirdparty-docker | ⬜ | `test-thirdparty-docker` | |
| test-util | 🚧 | `test-util` | Simple test, good starting point - POC test |

### DB Tests (43 tasks)
Uses pattern: `run-go-test-suite-with-mongodb`

| Task Name | Status | GHA Job Name | Notes |
|-----------|--------|--------------|-------|
| test-agent | ⬜ | `test-agent` | |
| test-agent-command | ⬜ | `test-agent-command` | Requires CLI binary built first |
| test-agent-internal | ⬜ | `test-agent-internal` | |
| test-auth | ⬜ | `test-auth` | |
| test-cloud-parameterstore | ⬜ | `test-cloud-parameterstore` | |
| test-cloud-parameterstore-fakeparameter | ⬜ | `test-cloud-parameterstore-fakeparameter` | |
| test-db | 🚧 | `test-db` | Simple DB test, good for validating MongoDB setup - POC test |
| test-evergreen | ⬜ | `test-evergreen` | |
| test-model | ⬜ | `test-model` | |
| test-model-alertrecord | ⬜ | `test-model-alertrecord` | |
| test-model-annotations | ⬜ | `test-model-annotations` | |
| test-model-artifact | ⬜ | `test-model-artifact` | |
| test-model-build | ⬜ | `test-model-build` | |
| test-model-cache | ⬜ | `test-model-cache` | |
| test-model-distro | ⬜ | `test-model-distro` | |
| test-model-event | ⬜ | `test-model-event` | |
| test-model-githubapp | ⬜ | `test-model-githubapp` | |
| test-model-host | ⬜ | `test-model-host` | |
| test-model-manifest | ⬜ | `test-model-manifest` | |
| test-model-notification | ⬜ | `test-model-notification` | |
| test-model-parsley | ⬜ | `test-model-parsley` | |
| test-model-patch | ⬜ | `test-model-patch` | |
| test-model-pod | ⬜ | `test-model-pod` | |
| test-model-pod-definition | ⬜ | `test-model-pod-definition` | |
| test-model-pod-dispatcher | ⬜ | `test-model-pod-dispatcher` | |
| test-model-task | ⬜ | `test-model-task` | |
| test-model-taskstats | ⬜ | `test-model-taskstats` | |
| test-model-testlog | ⬜ | `test-model-testlog` | |
| test-model-testresult | ⬜ | `test-model-testresult` | |
| test-model-user | ⬜ | `test-model-user` | |
| test-operations | ⬜ | `test-operations` | |
| test-plugin | ⬜ | `test-plugin` | |
| test-repotracker | ⬜ | `test-repotracker` | |
| test-rest-client | ⬜ | `test-rest-client` | |
| test-rest-data | ⬜ | `test-rest-data` | |
| test-rest-model | ⬜ | `test-rest-model` | |
| test-rest-route | ⬜ | `test-rest-route` | |
| test-scheduler | ⬜ | `test-scheduler` | |
| test-service | ⬜ | `test-service` | |
| test-thirdparty | ⬜ | `test-thirdparty` | |
| test-trigger | ⬜ | `test-trigger` | |
| test-units | ⬜ | `test-units` | |
| test-validator | ⬜ | `test-validator` | |

### Special Case Tests (2 tasks)
Uses pattern: `run-go-test-suite-with-mongodb-useast` (requires TZ=America/New_York)

| Task Name | Status | GHA Job Name | Notes |
|-----------|--------|--------------|-------|
| test-graphql | ⬜ | `test-graphql` | Needs `TZ=America/New_York` |
| test-service-graphql | ⬜ | `test-service-graphql` | Needs `TZ=America/New_York` |

### Deferred Tests (2 tasks)

| Task Name | Status | GHA Job Name | Notes |
|-----------|--------|--------------|-------|
| test-cloud | ⏸️ | - | Uses Docker, deferred to later phase |
| js-test | ⏸️ | - | Special JS setup, deferred to later phase |

---

## Architecture Decisions

### 1. Duplication vs Matrix Strategy
**Decision:** Use explicit job definitions (duplication) with reusable composite actions.

**Rationale:**
- Mirrors existing Evergreen structure (YAML anchors + explicit task list)
- Maximum flexibility for special cases (timezone, dependencies, Docker)
- Easier incremental migration and debugging
- Clear one-to-one mapping between Evergreen tasks and GHA jobs

### 2. Reusable Components
**Decision:** Create 4 composite actions in `.github/actions/`:
1. `setup-go-project` - Checkout code, setup Go, download modules
2. `setup-credentials` - Setup test credentials
3. `setup-mongodb` - Download and start MongoDB
4. `run-test` - Run test and upload results

**Rationale:**
- Eliminates duplication in common steps
- Easy to update common behavior across all tests
- Keeps individual job definitions concise

### 3. Parallel Execution
**Decision:** All tests run as independent jobs in parallel.

**Rationale:**
- GitHub Actions allows high concurrency
- Faster feedback than sequential execution
- Mirrors Evergreen's parallel task execution

### 4. No Changes to Evergreen Config
**Decision:** Keep `self-tests.yml` completely unchanged.

**Rationale:**
- Both systems run in parallel during migration
- Gradual validation of GitHub Actions behavior
- Easy rollback if needed
- No risk to existing CI pipeline

---

## Required Secrets & Variables

### GitHub Repository Secrets Needed

The following secrets must be added to the GitHub repository settings:

| Secret Name | Evergreen Expansion | Purpose |
|-------------|---------------------|---------|
| `STAGING_GITHUB_APP_ID` | `${staging_github_app_id}` | GitHub App authentication |
| `STAGING_GITHUB_APP_KEY` | `${staging_github_app_key}` | GitHub App private key |
| `AWS_ROLE_ARN` | `${assume_role_arn}` | AWS role for credentials |
| `JIRA_SERVER` | `${jiraserver}` | Jira integration tests |
| `JIRA_PERSONAL_ACCESS_TOKEN` | `${jira_personal_access_token}` | Jira API access |
| `CROWD_SERVER` | `${crowdserver}` | Crowd authentication |
| `PAPERTRAIL_KEY_ID` | `${papertrail_key_id}` | Papertrail logging |
| `PAPERTRAIL_SECRET_KEY` | `${papertrail_secret_key}` | Papertrail authentication |
| `RUNTIME_ENVIRONMENTS_BASE_URL` | `${staging_runtime_environments_base_url}` | Runtime environments API |
| `RUNTIME_ENVIRONMENTS_API_KEY` | `${staging_runtime_environments_api_key}` | Runtime environments auth |

### Environment Variables (Hardcoded in Workflow)

| Variable | Value | Purpose |
|----------|-------|---------|
| `GOROOT` | `/opt/hostedtoolcache/go/1.24.x/x64` | Go installation path |
| `EVERGREEN_ALL` | `"true"` | Run all tests |
| `KARMA_REPORTER` | `junit` | Test result format |
| `SETTINGS_OVERRIDE` | `creds.yml` | Credentials file |
| `RUN_EC2_SPECIFIC_TESTS` | `true` | Enable EC2 tests (ubuntu2204 variant) |

---

## Key Implementation Details

### MongoDB Setup
- **Version:** MongoDB 8.0.0 (matches Evergreen)
- **Download URL:** `https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu2204-8.0.0.tgz`
- **mongosh URL:** `https://downloads.mongodb.com/compass/mongosh-2.0.2-linux-x64.tgz`
- **Start command:** `make start-mongod` (runs in background)
- **Configure command:** `make configure-mongod`

### Test Execution
- **Pattern:** `make test-<package>` (e.g., `make test-util`)
- **Output file:** `bin/output.<package>.test`
- **Success criteria:** File contains `PASS` and does NOT contain `WARNING: DATA RACE`

### Test Results
- **Format:** Go test output + JUnit XML
- **Location:** `bin/output.*.test` and `bin/jstests/*.xml`
- **Upload:** Use `actions/upload-artifact` for test results

---

## Open Questions & Blockers

### Questions
1. ❓ **Secrets Access:** Who can add the required secrets to the GitHub repository?
2. ❓ **AWS Credentials:** How should we handle AWS role assumption in GitHub Actions? (OIDC vs access keys)
3. ❓ **Runner Resources:** Are default GitHub-hosted runners sufficient? (2-core, 7GB RAM)
4. ❓ **Merge Requirements:** Will GitHub Actions become a required check before merge?
5. ❓ **Test Dependencies:** How to handle `test-agent-command` dependency on CLI binary?

### Known Blockers
- 🚫 None currently identified

---

## Testing & Validation Strategy

### Validation Criteria
For each migrated test:
1. ✅ Test runs successfully in GitHub Actions
2. ✅ Test results match Evergreen output (pass/fail status)
3. ✅ Test artifacts are uploaded correctly
4. ✅ Execution time is comparable to Evergreen

### Comparison Approach
1. Run test in both Evergreen and GitHub Actions
2. Compare exit codes
3. Compare test output files
4. Document any differences

### Rollback Plan
If GitHub Actions migration causes issues:
1. Disable GitHub Actions workflow (delete or set to manual trigger)
2. Continue using Evergreen exclusively
3. Debug and fix issues offline
4. Re-enable when ready

---

## Implementation Timeline

### Phase 1A: Infrastructure
- [x] Create migration tracking document
- [x] Create 4 reusable composite actions
- [x] Create skeleton workflow file

### Phase 1B: Proof of Concept (Current)
- [x] Migrate 1 no-db test (`test-util`)
- [ ] Validate test execution and results
- [x] Migrate 1 db test (`test-db`)
- [ ] Validate MongoDB setup

### Phase 1C: Scale Up
- [ ] Migrate remaining 5 no-db tests
- [ ] Migrate remaining 42 db tests
- [ ] Handle special cases (timezone tests)

### Phase 1D: Validation & Documentation
- [ ] Compare all test results with Evergreen
- [ ] Document any discrepancies
- [ ] Update this document with findings

---

## Future Work (Phase 2+)

### Phase 2: Additional Variants
- Migrate `race-detector` variant
- Migrate `windows` variant
- Migrate `osx` variant
- Migrate `ubuntu2204-arm64` variant

### Phase 3: Additional Task Types
- Migrate linter tasks
- Migrate smoke tests
- Migrate build tasks
- Handle Docker-based tests

### Phase 4: Optimization
- Optimize test parallelism
- Reduce redundant work (caching)
- Implement test sharding if needed
- Consider self-hosted runners for cost/performance

### Phase 5: Transition (If Desired)
- Make GitHub Actions required for merges
- Deprecate Evergreen config
- Decommission Evergreen project

---

## Notes & Learnings

### 2025-11-03
- Initial migration plan created
- Identified 52 test tasks in scope for Phase 1
- Chose duplication approach over matrix for flexibility
- Need to determine secrets/credentials access before implementation can proceed
- Created 4 reusable composite actions (setup-go-project, setup-credentials, setup-mongodb, run-test)
- Created skeleton GitHub Actions workflow with POC tests (test-util, test-db)
- Phase 1A complete, Phase 1B in progress (validation pending)

---

## References

- **Evergreen Config:** `self-tests.yml`
- **Makefile:** Test targets defined as `test-%` pattern
- **GitHub Actions Workflow:** `.github/workflows/self-tests.yml` (to be created)
- **Composite Actions:** `.github/actions/*/action.yml` (to be created)
