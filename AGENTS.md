# Agent Guidance

## GitHub Actions Migration

### Current Status - Run #19344006543
**Status**: Failed (43/56 jobs passed)
**Branch**: migrate-to-github-actions
**Commit**: f69cc4c60 - "Pass secrets from setup-secrets to setup-credentials action"

### Error Themes

**🔴 Theme 1: Secret Configuration Errors (PRIMARY - ~90% of failures)**
- Issue: github_app_key secret is empty/missing
- Error: expansion 'github_app_key' cannot have an empty value
- Affected: 13 test jobs (test-evergreen, test-agent-command, test-model, test-repotracker, test-units, test-rest-data, test-model-task, test-operations, test-rest-route, test-thirdparty, test-trigger, test-graphql, test-validator)
- Root cause: GITHUB_APP_KEY not properly passed from setup-secrets action to test environment

**🟠 Theme 2: Filesystem Path Bug (CRITICAL)**
- Issue: Go source file used as working directory
- Error: mkdir /opt/hostedtoolcache/go/1.24.10/x64/src/reflect/value.go: not a directory
- Affected: test-agent job
- Root cause: Code incorrectly determining working directory, possibly using reflection

**🟡 Theme 3: S3 Upload Failures**
- Issue: uploaded 0 files of 1 requested
- Affected: test-agent-command S3 tests
- Root cause: Files not found for upload (may be related to path issues or credentials)

### Priorities
1. **Immediate**: Fix github_app_key secret handling in workflow
2. **High**: Debug working directory path bug in agent code
3. **Medium**: Investigate S3 upload failures

### General Notes
- Commit frequently