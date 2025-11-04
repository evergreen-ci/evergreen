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

**Progress:** 7/52 test tasks migrated (Phase 1C - No-DB tests complete)

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
| test-agent-internal-client | ✅ | `test-agent-internal-client` | |
| test-agent-internal-taskoutput | ✅ | `test-agent-internal-taskoutput` | |
| test-agent-util | ✅ | `test-agent-util` | |
| test-cloud-userdata | ✅ | `test-cloud-userdata` | |
| test-thirdparty-docker | ✅ | `test-thirdparty-docker` | |
| test-util | ✅ | `test-util` | Simple test, good starting point - POC test |

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
| test-db | ✅ | `test-db` | Simple DB test, good for validating MongoDB setup - POC test |
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

### Phase 1B: Proof of Concept
- [x] Migrate 1 no-db test (`test-util`)
- [x] Validate test execution and results
- [x] Migrate 1 db test (`test-db`)
- [x] Validate MongoDB setup

### Phase 1C: Scale Up (Current)
- [x] Migrate remaining 5 no-db tests
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
- **GitHub Actions Workflow:** `.github/workflows/self-tests.yml`
- **Composite Actions:** `.github/actions/*/action.yml`

---

## Appendix: Architecture Walkthrough

This section provides a detailed walkthrough of how all the GitHub Actions components work together.

### Overview: Building Blocks Approach

The design uses a **building blocks** approach to eliminate duplication:

```
Workflow File (.github/workflows/self-tests.yml)
    ↓ calls
Composite Actions (.github/actions/*/action.yml)
    ↓ execute
Makefile targets (existing build system)
    ↓ run
Go tests
```

---

### The 4 Composite Actions

#### 1. setup-go-project (.github/actions/setup-go-project/action.yml)

**Purpose:** Initialize the repository and Go environment

**What it does:**
1. Checkout code using `actions/checkout@v4`
2. Setup Go 1.24 using `actions/setup-go@v5`
3. Enable automatic Go module caching based on `go.sum`

**Inputs:**
- `go-version` (optional, defaults to '1.24')

**Key feature:** The `cache: true` setting means Go modules are automatically cached between runs, speeding up subsequent builds.

---

#### 2. setup-credentials (.github/actions/setup-credentials/action.yml)

**Purpose:** Configure test credentials for services like GitHub, Jira, AWS, etc.

**What it does:**
1. Runs `scripts/setup-credentials.sh`
2. Passes 13 different environment variables
3. Creates `creds.yml` file (used by tests via `SETTINGS_OVERRIDE` env var)

**The credential flow:**
```
GitHub Secrets
    ↓
Workflow passes to action via inputs
    ↓
Action passes to script via environment variables
    ↓
Script creates creds.yml
    ↓
Tests read creds.yml
```

**Inputs (all optional, defaults to empty string):**
- `github_app_id`, `github_app_key`
- `jira_server`, `jira_token`
- `crowd_server`
- `papertrail_key_id`, `papertrail_secret_key`
- `runtime_env_base_url`, `runtime_env_api_key`
- `aws_access_key_id`, `aws_secret_access_key`, `aws_session_token`

**Hard-coded values:**
- `PARSER_PROJECT_S3_PREFIX: github-actions-testing/parser-projects`
- `GENERATED_JSON_S3_PREFIX: github-actions-testing/generated-json`

---

#### 3. setup-mongodb (.github/actions/setup-mongodb/action.yml)

**Purpose:** Download, install, and configure MongoDB 8.0 for database tests

**What it does (4 steps):**
1. Download MongoDB 8.0.0 tarball (`make get-mongodb`)
2. Download mongosh 2.0.2 shell (`make get-mongosh`)
3. Start mongod in background (`make start-mongod &`)
4. Configure replica set (`make configure-mongod`)

**Inputs (both optional with sensible defaults):**
- `mongodb_url`: Default points to ubuntu2204 MongoDB 8.0.0
- `mongosh_url`: Default points to mongosh 2.0.2

**Why the background `&`:** The mongod server needs to stay running while tests execute, so it's started in the background.

---

#### 4. run-test (.github/actions/run-test/action.yml)

**Purpose:** Execute a specific test target and upload results

**What it does:**
1. Run `make <test_name>` (e.g., `make test-util`)
   - Sets 4 environment variables
2. Upload test results as artifacts (always runs, even on failure)
   - Uploads `bin/output.*.test` files
   - Uploads `bin/jstests/*.xml` files

**Inputs:**
- `test_name` (required): e.g., "test-util", "test-db"

**Environment variables set:**
- `EVERGREEN_ALL: "true"` - Run all tests
- `KARMA_REPORTER: junit` - Output format
- `SETTINGS_OVERRIDE: creds.yml` - Point to credentials file
- `RUN_EC2_SPECIFIC_TESTS: "true"` - Enable EC2 tests (ubuntu2204 variant)

**The `if: always()`:** Ensures test results are uploaded even if tests fail, so you can debug failures.

---

### The Workflow File (.github/workflows/self-tests.yml)

#### Triggers (when it runs):
- **Push** to `main` or `migrate-to-github-actions` branches
- **Pull requests** to `main`
- **Manual trigger** via `workflow_dispatch` in GitHub UI

#### Concurrency Control:
```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

This means: "If I push again while tests are running, cancel the old run and start fresh."

#### The Two POC Jobs:

**Job 1: test-util (No-Database Test)**

Execution flow:
```
1. setup-go-project
   └─> Checkout code
   └─> Setup Go 1.24
   └─> Cache modules

2. setup-credentials
   └─> Run scripts/setup-credentials.sh
   └─> Create creds.yml with secrets

3. run-test (test_name: test-util)
   └─> make test-util
   └─> Upload test results
```

**Why no MongoDB?** The `util` package doesn't need a database, so we skip the setup-mongodb step.

**Job 2: test-db (Database Test)**

Execution flow:
```
1. setup-go-project
   └─> Checkout code
   └─> Setup Go 1.24
   └─> Cache modules

2. setup-credentials
   └─> Run scripts/setup-credentials.sh
   └─> Create creds.yml with secrets

3. setup-mongodb
   └─> Download MongoDB 8.0.0
   └─> Download mongosh 2.0.2
   └─> Start mongod in background
   └─> Configure replica set

4. run-test (test_name: test-db)
   └─> make test-db
   └─> Upload test results
```

**The key difference:** We insert the `setup-mongodb` step before running the test.

---

### How Tests Are Selected

The workflow uses the **explicit job pattern** (not matrices):

```yaml
test-util:           # Job name
  steps:
    - uses: ./.github/actions/run-test
      with:
        test_name: test-util    # This determines which make target runs
```

When you want to add `test-auth`:
```yaml
test-auth:
  steps:
    - uses: ./.github/actions/setup-go-project
    - uses: ./.github/actions/setup-credentials
    - uses: ./.github/actions/setup-mongodb    # DB test needs this
    - uses: ./.github/actions/run-test
      with:
        test_name: test-auth   # make test-auth
```

---

### Secrets Flow

Here's how secrets move through the system:

```
1. GitHub Repository Secrets (configured in repo settings)
   └─> STAGING_GITHUB_APP_ID, JIRA_SERVER, etc.

2. Workflow file references secrets
   └─> secrets.STAGING_GITHUB_APP_ID

3. Passed as inputs to composite action
   └─> github_app_id: ${{ secrets.STAGING_GITHUB_APP_ID }}

4. Composite action passes to environment variable
   └─> GITHUB_APP_ID: ${{ inputs.github_app_id }}

5. Script reads environment variable
   └─> bash scripts/setup-credentials.sh

6. Script creates creds.yml file

7. Tests read creds.yml
   └─> via SETTINGS_OVERRIDE=creds.yml env var
```

---

### The Three Test Patterns

#### Pattern 1: No-Database Tests (7 tests total)
```yaml
- setup-go-project
- setup-credentials
- run-test
```

Examples: test-util, test-agent-internal-client, test-agent-util

#### Pattern 2: Database Tests (43 tests total)
```yaml
- setup-go-project
- setup-credentials
- setup-mongodb        # ← The difference!
- run-test
```

Examples: test-db, test-auth, test-model, test-service

#### Pattern 3: Timezone Tests (2 tests total)
```yaml
env:
  TZ: America/New_York    # ← Special timezone requirement

- setup-go-project
- setup-credentials
- setup-mongodb
- run-test
```

Examples: test-graphql, test-service-graphql

---

### Execution Example: test-db End-to-End

Let's trace what happens when `test-db` runs:

1. **GitHub Actions starts ubuntu-22.04 runner**

2. **Step 1: setup-go-project**
   - Clones repo
   - Installs Go 1.24
   - Downloads/caches Go modules

3. **Step 2: setup-credentials**
   - Reads secrets from GitHub
   - Runs `bash scripts/setup-credentials.sh`
   - Creates `creds.yml` file

4. **Step 3: setup-mongodb**
   - Downloads MongoDB: `make get-mongodb`
   - Downloads mongosh: `make get-mongosh`
   - Starts mongod: `make start-mongod &`
   - Configures replica set: `make configure-mongod`

5. **Step 4: run-test**
   - Runs: `make test-db`
   - Environment vars: `EVERGREEN_ALL=true`, `SETTINGS_OVERRIDE=creds.yml`
   - Test runs and outputs to `bin/output.db.test`
   - Uploads results to GitHub artifacts

6. **Job completes** (success or failure shown in UI)

---

### Design Benefits

#### 1. No Duplication
Instead of repeating 20 lines for each test, each test only needs:
```yaml
test-foo:
  runs-on: ubuntu-22.04
  steps:
    - uses: ./.github/actions/setup-go-project
    - uses: ./.github/actions/setup-credentials
    - uses: ./.github/actions/setup-mongodb
    - uses: ./.github/actions/run-test
      with:
        test_name: test-foo
```

#### 2. Easy Updates
Want to upgrade MongoDB to 8.0.1? Change one line in `setup-mongodb/action.yml`, and all 45 database tests get it.

#### 3. Parallel Execution
All test jobs run simultaneously (GitHub provides up to 20 concurrent runners on free tier).

#### 4. One-to-One Mapping
Each Evergreen task becomes exactly one GitHub Actions job, making comparison easy.

---

### Current Status

- **Active:** 2 tests (test-util, test-db)
- **Commented out:** 50 tests waiting for Phase 1B validation
- **Ready to scale:** Just uncomment the sections in Phase 1C
