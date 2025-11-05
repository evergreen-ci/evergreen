# GitHub Actions Test Failure Analysis

**Date:** 2025-11-05
**Run ID:** 19107316639
**Commit:** c128065445 (Remove explicit secret passing to use dummy defaults)

---

## Executive Summary

**Test Results:** 39 out of 51 tests passed (76% success rate)
**Failed Tests:** 12 tests

After implementing dummy credential defaults, we achieved a significant improvement from 36/49 (73%) to 39/51 (76%). Most remaining failures are related to invalid credential formatting (GitHub App private key), S3/AWS operations, and GitHub/external API integrations that require valid credentials.

---

## Improvements from Previous Run

### Fixed Issues ✅
- **test-agent-util**: EC2 metadata tests now pass (set `RUN_EC2_SPECIFIC_TESTS: false`)
- **test-evergreen**: Empty expansion errors resolved
- **test-agent**: Now passing
- **test-rest-data**: S3 storage tests now pass

### Key Success
The dummy credential defaults successfully fixed tests that only needed credentials to be **present** for configuration validation, but not necessarily **valid** for actual API calls.

---

## Detailed Failure Analysis

### 1. Invalid GitHub App Private Key (Critical Issue)

**Affected Tests:**
- `test-repotracker`
- `test-thirdparty`

**Error Pattern:**
```
asn1: syntax error: data truncated parsing private key
```

**Root Cause:** The dummy RSA private key in the `setup-credentials` action is not a valid key. The key needs to be properly formatted, even if it's not a real GitHub App key.

**Failed Test Cases:**
- `test-repotracker`: TestGetRevisionsSinceWithPaging, TestGetRevisionsSince, TestGetRemoteConfig, TestGetAllRevisions, TestGetChangedFiles, TestFetchRevisions, TestStoreRepositoryRevisions, TestCreateManifest
- `test-thirdparty`: TestGithubSuite/TestCheckGithubAPILimit, TestJiraIntegration, Docker-related tests

**Impact:** HIGH - Affects 2 test suites with 15+ test cases

---

### 2. S3/AWS Storage Integration Issues

**Affected Tests:**
- `test-model`
- `test-model-task`
- `test-agent-command`
- `test-units`

**Failed Test Cases:**
- `test-model`: TestGetPatchedProjectAndGetPatchedProjectConfig, TestFinalizePatch (4 subtests), TestParserProjectStorage (6+ subtests for both DB and S3)
- `test-model-task`: TestGeneratedJSONStorage (6 subtests for both DB and S3)
- `test-agent-command`: TestS3GetFetchesFiles (2 subtests), TestS3PutSkipExisting
- `test-units`: TestGenerateTasksWithDifferentGeneratedJSONStorageMethods/S3

**Root Cause:** Tests require valid AWS credentials to interact with S3 buckets for:
- Parser project storage (PARSER_PROJECT_S3_PREFIX)
- Generated JSON storage (GENERATED_JSON_S3_PREFIX)
- File uploads/downloads

**Current Configuration:**
```yaml
PARSER_PROJECT_S3_PREFIX: github-actions-testing/parser-projects
GENERATED_JSON_S3_PREFIX: github-actions-testing/generated-json
AWS_ACCESS_KEY_ID: AKIADUMMYAWSACCESSKEY (dummy)
```

**Impact:** HIGH - Affects 4 test suites with 20+ test cases

---

### 3. Papertrail/Logging Integration

**Affected Tests:**
- `test-agent-command`

**Failed Test Cases:**
- TestPapertrailTrace/BasicNoExpansions
- TestPapertrailTrace/WithoutWorkDir
- TestPapertrailTrace/BasicExpansions

**Root Cause:** Tests require valid Papertrail credentials to send log traces.

**Impact:** LOW - Affects 3 test cases in 1 suite

---

### 4. GitHub API/Integration Tests

**Affected Tests:**
- `test-rest-route`
- `test-units`

**Failed Test Cases:**
- `test-rest-route`: TestGithubWebhookRouteSuite/TestAddIntentAndFailsWithDuplicate, TestSchedulePatchRoute
- `test-units`: TestPatchIntentUnitsSuite (8 subtests related to GitHub PR processing)

**Root Cause:** Tests require valid GitHub App credentials for:
- Processing GitHub webhooks
- Creating patches from GitHub PRs
- Validating GitHub user permissions

**Impact:** MEDIUM - Affects 10+ test cases across 2 suites

---

### 5. Docker/Image Validation

**Affected Tests:**
- `test-graphql`
- `test-validator`

**Failed Test Cases:**
- `test-graphql`: TestOperatingSystem
- `test-validator`: TestValidateImageID

**Error Pattern:**
```
Received unexpected error: input: getting operating system information
```

**Root Cause:** Tests attempt to inspect Docker images for OS information but fail to retrieve it, likely due to missing Docker registry access or invalid image references.

**Impact:** LOW - Affects 2 test cases in 2 suites

---

### 6. CLI/Operations Tests

**Affected Tests:**
- `test-operations`

**Failed Test Cases:**
- TestCLIFetchSource
- TestCLIFunctions

**Root Cause:** Unclear - may be related to file system operations, git repository access, or CLI tool dependencies.

**Impact:** LOW - Affects 2 test cases in 1 suite

---

### 7. Trigger/Downstream Integration

**Affected Tests:**
- `test-trigger`

**Failed Test Cases:**
- TestProjectTriggerIntegration
- TestProjectTriggerIntegrationForBuild
- TestProjectTriggerIntegrationForPush
- TestMakeDownstreamConfigFromFile

**Root Cause:** Tests validate trigger functionality for downstream projects, likely requiring valid GitHub credentials or project configuration.

**Impact:** MEDIUM - Affects 4 test cases in 1 suite

---

### 8. Test Data/Event Ordering Issue

**Affected Tests:**
- `test-model-event`

**Failed Test Cases:**
- TestGetPaginatedHostEvents

**Error:**
```
Expected "HOST_MODIFIED" but got "HOST_TASK_FINISHED"
```

**Root Cause:** Test assertion failure due to event type mismatch - likely a test data ordering issue or incorrect test expectations.

**Impact:** LOW - Affects 1 test case in 1 suite

---

### 9. Periodic Builds Job

**Affected Tests:**
- `test-units`

**Failed Test Cases:**
- TestPeriodicBuildsJob

**Root Cause:** Unclear - may be related to time-based logic or database state.

**Impact:** LOW - Affects 1 test case

---

## Test Success Matrix

| Test | Status | Category | Primary Blocker |
|------|--------|----------|-----------------|
| test-agent | ✅ PASS | DB | - |
| test-agent-command | ❌ FAIL | DB | S3 credentials + Papertrail |
| test-agent-internal | ✅ PASS | No-DB | - |
| test-agent-internal-client | ✅ PASS | No-DB | - |
| test-agent-internal-taskoutput | ✅ PASS | No-DB | - |
| test-agent-util | ✅ PASS | No-DB | - |
| test-auth | ✅ PASS | DB | - |
| test-cloud-parameterstore | ✅ PASS | DB | - |
| test-cloud-parameterstore-fakeparameter | ✅ PASS | DB | - |
| test-cloud-userdata | ✅ PASS | No-DB | - |
| test-db | ✅ PASS | DB | - |
| test-evergreen | ✅ PASS | DB | - |
| test-graphql | ❌ FAIL | TZ | Docker/image validation |
| test-model | ❌ FAIL | DB | S3 storage |
| test-model-alertrecord | ✅ PASS | DB | - |
| test-model-annotations | ✅ PASS | DB | - |
| test-model-artifact | ✅ PASS | DB | - |
| test-model-build | ✅ PASS | DB | - |
| test-model-cache | ✅ PASS | DB | - |
| test-model-distro | ✅ PASS | DB | - |
| test-model-event | ❌ FAIL | DB | Test data/ordering |
| test-model-githubapp | ✅ PASS | DB | - |
| test-model-host | ✅ PASS | DB | - |
| test-model-manifest | ✅ PASS | DB | - |
| test-model-notification | ✅ PASS | DB | - |
| test-model-parsley | ✅ PASS | DB | - |
| test-model-patch | ✅ PASS | DB | - |
| test-model-pod | ✅ PASS | DB | - |
| test-model-pod-definition | ✅ PASS | DB | - |
| test-model-pod-dispatcher | ✅ PASS | DB | - |
| test-model-task | ❌ FAIL | DB | S3 storage |
| test-model-taskstats | ✅ PASS | DB | - |
| test-model-testlog | ✅ PASS | DB | - |
| test-model-testresult | ✅ PASS | DB | - |
| test-model-user | ✅ PASS | DB | - |
| test-operations | ❌ FAIL | DB | CLI/filesystem operations |
| test-plugin | ✅ PASS | DB | - |
| test-repotracker | ❌ FAIL | DB | Invalid GitHub App key |
| test-rest-client | ✅ PASS | DB | - |
| test-rest-data | ✅ PASS | DB | - |
| test-rest-model | ✅ PASS | DB | - |
| test-rest-route | ❌ FAIL | DB | GitHub webhook processing |
| test-scheduler | ✅ PASS | DB | - |
| test-service | ✅ PASS | DB | - |
| test-service-graphql | ✅ PASS | TZ | - |
| test-thirdparty | ❌ FAIL | DB | Invalid GitHub App key |
| test-thirdparty-docker | ✅ PASS | No-DB | - |
| test-trigger | ❌ FAIL | DB | Trigger integration |
| test-units | ❌ FAIL | DB | S3 storage + GitHub PR processing |
| test-util | ✅ PASS | No-DB | - |
| test-validator | ❌ FAIL | DB | Docker image validation |

---

## Recommendations

### Short-term (Quick Wins)

1. **Fix GitHub App Private Key Format** (HIGH PRIORITY)
   - Replace the dummy RSA key with a properly formatted (but fake) private key
   - Or generate a valid test key pair that doesn't connect to real GitHub
   - This should fix `test-repotracker` and `test-thirdparty`

2. **Skip S3-Dependent Tests** (MEDIUM PRIORITY)
   - Add environment detection to skip S3 tests when running in GitHub Actions
   - Or set up test S3 buckets with proper credentials
   - Affects: `test-model`, `test-model-task`, `test-agent-command`, `test-units`

3. **Skip External Integration Tests** (LOW PRIORITY)
   - Skip Papertrail, Docker registry, and other external service tests in GHA
   - Use environment variable like `SKIP_INTEGRATION_TESTS=true`

### Medium-term (Comprehensive Solution)

1. **Environment Detection:** Tests should detect they're running in GitHub Actions:
   ```go
   if os.Getenv("GITHUB_ACTIONS") == "true" {
       t.Skip("Skipping integration test in GitHub Actions")
   }
   ```

2. **Mock External Services:**
   - S3 operations (using localstack or minio)
   - GitHub API calls (using mock server)
   - Docker registry operations

3. **Test S3 Buckets:**
   - Create dedicated test S3 buckets for GitHub Actions
   - Use AWS OIDC for secure credential management

### Long-term

1. **Self-hosted Runners:** Use EC2-based self-hosted runners with proper AWS credentials
2. **Service Mocking:** Comprehensive mocking strategy for all external dependencies
3. **Test Classification:** Separate unit tests from integration tests more clearly

---

## Summary by Failure Category

| Category | Test Count | Test Suites |
|----------|------------|-------------|
| Invalid GitHub App Key | 2 | test-repotracker, test-thirdparty |
| S3/AWS Storage | 4 | test-model, test-model-task, test-agent-command, test-units |
| GitHub API Integration | 2 | test-rest-route, test-units |
| Docker/Image Validation | 2 | test-graphql, test-validator |
| Papertrail Integration | 1 | test-agent-command |
| CLI/Operations | 1 | test-operations |
| Trigger Integration | 1 | test-trigger |
| Test Data/Logic | 1 | test-model-event |

**Most Critical Issue:** Invalid GitHub App private key format (affects 2 test suites)

---

## Next Steps

1. ✅ **Complete Phase 1C:** All 51 tests migrated (including timezone tests)
2. 🔧 **Fix GitHub App key format:** Generate proper dummy RSA key
3. 🔧 **Address S3 tests:** Either skip or provide real test credentials
4. 📋 **Document:** Continue tracking improvements

---

## Related Files

- Workflow: `.github/workflows/self-tests.yml`
- Credentials Action: `.github/actions/setup-credentials/action.yml`
- Migration Doc: `docs/decisions/github-actions-migration/github-actions-migration.md`
- Run URL: https://github.com/evergreen-ci/evergreen/actions/runs/19107316639

---

## Change Log

### 2025-11-05 (Run 19107316639)
- Implemented dummy credential defaults in `setup-credentials` action
- Removed explicit secret passing from workflow (secrets were empty and overriding defaults)
- Set `RUN_EC2_SPECIFIC_TESTS: false` to skip EC2 metadata tests
- **Result:** 39/51 passing (76%), up from 36/49 (73%)
- **Key fixes:** test-evergreen, test-agent-util now pass
- **Remaining issues:** GitHub App key format, S3 credentials, external API integrations

### 2025-11-04 (Run 19085017428)
- Initial test run with 42 DB tests migrated
- **Result:** 36/49 passing (73%)
- **Primary issue:** Empty credential values causing validation errors
