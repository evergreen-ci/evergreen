# GitHub Actions Test Failure Analysis

**Date:** 2025-11-04
**Run ID:** 19085017428
**Commit:** 25282b9a46 (Add remaining 42 DB tests to GitHub Actions workflow)

---

## Executive Summary

**Test Results:** 36 out of 49 tests passed (73% success rate)
**Failed Tests:** 13 tests

Most failures are integration tests requiring external services, credentials, or specific infrastructure (EC2). The core test infrastructure is working correctly.

---

## Detailed Failure Analysis

### 1. Missing/Empty Credentials (Most Common Issue)

**Affected Tests:**
- `test-agent-command`
- `test-rest-route`
- `test-operations`
- `test-trigger`
- `test-units`
- `test-thirdparty`

**Error Pattern:**
```
Failed to store secret map field 'Settings/Expansions/github_app_key' in parameter store:
putting parameter 'Settings/Expansions/github_app_key':
invalid input: value is required
```

**Root Cause:** The `github_app_key` secret appears to be empty or not configured in the GitHub repository secrets.

**Sample Failures:**
- `test-agent-command`: TestPatchPluginAPI, TestPatchPlugin, TestGitGetProjectSuite, TestIncKey, TestPapertrailTrace, TestS3GetFetchesFiles, TestS3PutSkipExisting
- `test-rest-route`: TestAgentGetExpansionsAndVars, TestGithubWebhookRouteSuite, TestSchedulePatchRoute, TestGenerateExecuteWithLargeFileInS3
- `test-units`: TestPatchIntentUnitsSuite (22 subtests), TestPeriodicBuildsJob

**Impact:** HIGH - Affects 6 test suites

---

### 2. EC2 Metadata Service Issues

**Affected Tests:**
- `test-agent-util`

**Failed Test Cases:**
- `TestGetEC2InstanceID`
- `TestGetEC2Hostname`

**Error Pattern:**
```
Error: Received unexpected error:
server returned status 404
```

**Root Cause:** GitHub Actions runners are not EC2 instances. Tests attempting to access EC2 metadata endpoints (169.254.169.254) receive 404 responses.

**Context:** These tests are likely controlled by the `RUN_EC2_SPECIFIC_TESTS` environment variable, which is currently set to `true`.

**Impact:** LOW - Only 2 test failures, easily skippable

---

### 3. S3/Storage Integration Issues

**Affected Tests:**
- `test-model`
- `test-model-task`
- `test-rest-data`

**Failed Test Cases:**
- `test-model`: TestGetPatchedProjectAndGetPatchedProjectConfig, TestFinalizePatch, TestParserProjectStorage
- `test-model-task`: TestGeneratedJSONStorage
- `test-rest-data`: TestCreateHostsFromTask

**Root Cause:** Tests require AWS credentials and S3 buckets for:
- Parser project storage (PARSER_PROJECT_S3_PREFIX)
- Generated JSON storage (GENERATED_JSON_S3_PREFIX)
- Host creation/provisioning

**Current Configuration:**
```yaml
PARSER_PROJECT_S3_PREFIX: github-actions-testing/parser-projects
GENERATED_JSON_S3_PREFIX: github-actions-testing/generated-json
```

**Impact:** MEDIUM - Affects core model functionality tests

---

### 4. GitHub/External API Integration

**Affected Tests:**
- `test-repotracker` (8 failures)
- `test-thirdparty` (12 failures)

**test-repotracker Failed Cases:**
- TestGetRevisionsSinceWithPaging
- TestGetRevisionsSince
- TestGetRemoteConfig
- TestGetAllRevisions
- TestGetChangedFiles
- TestFetchRevisions
- TestStoreRepositoryRevisions
- TestCreateManifest

**test-thirdparty Failed Cases:**
- TestGithubSuite
- TestGetGitHubSender
- TestJiraIntegration
- TestGetImageNames (Docker)
- TestGetOSInfo
- TestGetPackages
- TestGetToolchains
- TestGetFiles
- TestGetImageDiff
- TestGetHistory
- TestGetEvents
- TestGetImageInfo

**Root Cause:** Tests require:
- Valid GitHub App credentials for repository operations
- Jira API credentials
- Docker registry access
- External service connectivity

**Impact:** HIGH - Affects 20 test cases across 2 suites

---

### 5. Configuration/Environment Issues

**Affected Tests:**
- `test-evergreen`
- `test-validator`

**Failed Test Cases:**
- `test-evergreen`: TestAdminSuite, TestEnvironmentSuite/TestLoadingConfig
- `test-validator`: TestValidateImageID

**Root Cause:** Environment configuration or validation logic issues, likely related to missing credentials or config files.

**Impact:** LOW - Only 3 test failures

---

## Recommendations

### Short-term (Quick Wins)

1. **Configure Secrets:** Add valid values for GitHub repository secrets:
   - `STAGING_GITHUB_APP_ID`
   - `STAGING_GITHUB_APP_KEY`
   - AWS credentials (if S3 tests should run)
   - Jira credentials (if Jira tests should run)

2. **Disable EC2-Specific Tests:** Set `RUN_EC2_SPECIFIC_TESTS: false` for GitHub Actions

3. **Skip External Integration Tests:** Add conditional logic to skip tests requiring external services in GHA environment

### Medium-term (Comprehensive Solution)

1. **Environment Detection:** Tests should detect they're running in GitHub Actions and adjust behavior:
   ```go
   if os.Getenv("GITHUB_ACTIONS") == "true" {
       t.Skip("Skipping EC2-specific test in GitHub Actions")
   }
   ```

2. **Mock External Services:** Use mocks for:
   - EC2 metadata service
   - S3 operations
   - GitHub API calls
   - Docker registry operations

3. **Separate Test Suites:** Create distinct test targets:
   - `test-*-unit`: No external dependencies
   - `test-*-integration`: Requires credentials/services

### Long-term

1. **Self-hosted Runners:** Use EC2-based self-hosted runners for EC2-specific tests
2. **Test S3 Buckets:** Create dedicated test S3 buckets for GitHub Actions
3. **Service Mocking:** Comprehensive mocking strategy for all external dependencies

---

## Test Success Matrix

| Test | Status | Category | Blocker |
|------|--------|----------|---------|
| test-agent | ✅ PASS | DB | - |
| test-agent-command | ❌ FAIL | DB | Missing github_app_key |
| test-agent-internal | ✅ PASS | No-DB | - |
| test-agent-internal-client | ✅ PASS | No-DB | - |
| test-agent-internal-taskoutput | ✅ PASS | No-DB | - |
| test-agent-util | ❌ FAIL | No-DB | EC2 metadata |
| test-auth | ✅ PASS | DB | - |
| test-cloud-parameterstore | ✅ PASS | DB | - |
| test-cloud-parameterstore-fakeparameter | ✅ PASS | DB | - |
| test-cloud-userdata | ✅ PASS | No-DB | - |
| test-db | ✅ PASS | DB | - |
| test-evergreen | ❌ FAIL | DB | Config/credentials |
| test-model | ❌ FAIL | DB | S3 storage |
| test-model-alertrecord | ✅ PASS | DB | - |
| test-model-annotations | ✅ PASS | DB | - |
| test-model-artifact | ✅ PASS | DB | - |
| test-model-build | ✅ PASS | DB | - |
| test-model-cache | ✅ PASS | DB | - |
| test-model-distro | ✅ PASS | DB | - |
| test-model-event | ✅ PASS | DB | - |
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
| test-operations | ❌ FAIL | DB | Missing github_app_key |
| test-plugin | ✅ PASS | DB | - |
| test-repotracker | ❌ FAIL | DB | GitHub API/credentials |
| test-rest-client | ✅ PASS | DB | - |
| test-rest-data | ❌ FAIL | DB | S3 storage |
| test-rest-model | ✅ PASS | DB | - |
| test-rest-route | ❌ FAIL | DB | Missing github_app_key |
| test-scheduler | ✅ PASS | DB | - |
| test-service | ✅ PASS | DB | - |
| test-thirdparty | ❌ FAIL | DB | GitHub/Jira/Docker APIs |
| test-thirdparty-docker | ✅ PASS | No-DB | - |
| test-trigger | ❌ FAIL | DB | Missing github_app_key |
| test-units | ❌ FAIL | DB | Missing github_app_key |
| test-util | ✅ PASS | No-DB | - |
| test-validator | ❌ FAIL | DB | Config/credentials |

---

## Next Steps

1. ✅ **Complete Phase 1C:** Migrate remaining 2 timezone tests (test-graphql, test-service-graphql)
2. ⏸️ **Defer troubleshooting:** Address test failures in a separate phase
3. 📋 **Document:** Keep this analysis for future reference

---

## Related Files

- Workflow: `.github/workflows/self-tests.yml`
- Migration Doc: `docs/github-actions-migration.md`
- Run URL: https://github.com/evergreen-ci/evergreen/actions/runs/19085017428
