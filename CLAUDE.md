# Claude Code Project Documentation

This file contains information about scripts, workflows, and conventions for this project that Claude Code should be aware of.

---

## GitHub Actions Migration

### Test Results Checker Script

**Location:** `scripts/check-gha-tests.sh`

**Purpose:** Check GitHub Actions test results and track progress on the GitHub Actions migration.

**Usage:**
```bash
# Check the latest run
./scripts/check-gha-tests.sh

# Check a specific run by ID
./scripts/check-gha-tests.sh 19112883183
```

**Features:**
- Fetches GHA run results using `gh` CLI
- Shows pass/fail summary with percentages
- Highlights improvements (tests that were failing but now pass)
- Highlights regressions (tests that were passing but now fail)
- Compares to baseline run 19107316639 (39/51 passing, 76%)
- Color-coded output for easy scanning

**Example Output:**
```
GitHub Actions Test Results Checker
========================================

Latest run ID: 19112883183

Run: Replace dummy RSA key with properly generated key
Status: failure
URL: https://github.com/evergreen-ci/evergreen/actions/runs/19112883183

========================================
SUMMARY
========================================
✓ Passed:      40 / 51 (78%)
✗ Failed:      10 / 51

🎉 IMPROVEMENTS (Previously failing, now passing):
  ✓ test-model-event
  ✓ test-agent-command

Still Failing (10 tests):
  ✗ test-model-task
  ✗ test-model
  ...

========================================
COMPARISON TO PREVIOUS RUN (19107316639)
========================================
Previous: 39 passed, 12 failed (76%)
Current:  40 passed, 10 failed (78%)
Change:   +1 tests passing (+2%)

Progress made! 📈
```

**When to Use:**
- After pushing changes that might affect test results
- To quickly check if your changes improved the pass rate
- To identify which specific tests were fixed or broke

---

## Related Documentation

- **Migration Plan:** `docs/decisions/github-actions-migration/github-actions-migration.md`
- **Test Failures Analysis:** `docs/decisions/github-actions-migration/github-actions-test-failures.md`
- **GitHub Actions Workflow:** `.github/workflows/self-tests.yml`
- **Composite Actions:** `.github/actions/*/action.yml`

---

## Key Contexts

### GitHub Actions Self-Tests Migration
This is a **parallel migration** - both Evergreen and GitHub Actions run simultaneously. The goal is to migrate 51 test tasks from the `ubuntu2204` variant to GitHub Actions as a proof of concept.

**Current Status:** 40/51 tests passing (78% as of 2025-11-05)

**Remaining Issues:**
- S3/AWS storage integration (4 test suites)
- Invalid GitHub API credentials (2 test suites)
- Docker/image validation (2 test suites)
- Other integration issues (2 test suites)
