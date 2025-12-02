#!/bin/bash

# Script to check GitHub Actions test results and compare with previous failures
# Usage: ./scripts/check-gha-tests.sh [run-id]
#        If no run-id provided, fetches the latest run

set -e

WORKFLOW_NAME="self-tests.yml"
BRANCH="migrate-to-github-actions"

# ANSI color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Previous known failures (from run 19107316639)
PREVIOUS_FAILURES=(
    "test-model-event"
    "test-model-task"
    "test-model"
    "test-agent-command"
    "test-rest-route"
    "test-graphql"
    "test-repotracker"
    "test-thirdparty"
    "test-operations"
    "test-trigger"
    "test-units"
    "test-validator"
)

echo -e "${BOLD}GitHub Actions Test Results Checker${NC}"
echo "========================================"
echo ""

# Get run ID
if [ -n "$1" ]; then
    RUN_ID="$1"
    echo "Using specified run ID: $RUN_ID"
else
    echo "Fetching latest run..."
    RUN_ID=$(gh run list --workflow="$WORKFLOW_NAME" --branch="$BRANCH" --limit=1 --json databaseId --jq '.[0].databaseId')
    echo "Latest run ID: $RUN_ID"
fi

echo ""
echo "Fetching run details..."
RUN_DATA=$(gh run view "$RUN_ID" 2>&1)

# Check if run is still in progress
if echo "$RUN_DATA" | grep -q "in progress"; then
    echo -e "${YELLOW}⚠ Run is still in progress. Results may be incomplete.${NC}"
    echo ""
fi

# Extract run info
RUN_CONCLUSION=$(echo "$RUN_DATA" | head -1 | awk '{print $1}')
RUN_TITLE=$(echo "$RUN_DATA" | head -1 | sed 's/^[X✓*] [^ ]* //')
RUN_URL="https://github.com/evergreen-ci/evergreen/actions/runs/$RUN_ID"

echo -e "${BOLD}Run:${NC} $RUN_TITLE"
echo -e "${BOLD}Status:${NC} $RUN_CONCLUSION"
echo -e "${BOLD}URL:${NC} $RUN_URL"
echo ""

# Parse job results
echo "Parsing test results..."
PASSED_TESTS=()
FAILED_TESTS=()
IN_PROGRESS_TESTS=()

while IFS= read -r line; do
    if echo "$line" | grep -qE "^✓ test-"; then
        test_name=$(echo "$line" | awk '{print $2}')
        PASSED_TESTS+=("$test_name")
    elif echo "$line" | grep -qE "^X test-"; then
        test_name=$(echo "$line" | awk '{print $2}')
        FAILED_TESTS+=("$test_name")
    elif echo "$line" | grep -qE "^\* test-"; then
        test_name=$(echo "$line" | awk '{print $2}')
        IN_PROGRESS_TESTS+=("$test_name")
    fi
done < <(echo "$RUN_DATA" | grep -E "^[✓X*] test-")

TOTAL_TESTS=$((${#PASSED_TESTS[@]} + ${#FAILED_TESTS[@]} + ${#IN_PROGRESS_TESTS[@]}))
PASS_COUNT=${#PASSED_TESTS[@]}
FAIL_COUNT=${#FAILED_TESTS[@]}
IN_PROGRESS_COUNT=${#IN_PROGRESS_TESTS[@]}

if [ "$TOTAL_TESTS" -eq 0 ]; then
    echo -e "${RED}Error: No test results found. Run may not have started yet.${NC}"
    exit 1
fi

# Calculate pass rate
if [ "$TOTAL_TESTS" -gt 0 ]; then
    PASS_RATE=$((PASS_COUNT * 100 / TOTAL_TESTS))
else
    PASS_RATE=0
fi

# Summary
echo ""
echo -e "${BOLD}========================================"
echo -e "SUMMARY"
echo -e "========================================${NC}"
echo -e "${GREEN}✓ Passed:${NC}      $PASS_COUNT / $TOTAL_TESTS (${PASS_RATE}%)"
echo -e "${RED}✗ Failed:${NC}      $FAIL_COUNT / $TOTAL_TESTS"
if [ "$IN_PROGRESS_COUNT" -gt 0 ]; then
    echo -e "${YELLOW}⋯ In Progress:${NC} $IN_PROGRESS_COUNT / $TOTAL_TESTS"
fi
echo ""

# Check for improvements (previously failing tests that now pass)
NEWLY_PASSING=()
for test in "${PREVIOUS_FAILURES[@]}"; do
    if printf '%s\n' "${PASSED_TESTS[@]}" | grep -qx "$test"; then
        NEWLY_PASSING+=("$test")
    fi
done

# Check for regressions (previously passing tests that now fail)
NEWLY_FAILING=()
for test in "${FAILED_TESTS[@]}"; do
    if ! printf '%s\n' "${PREVIOUS_FAILURES[@]}" | grep -qx "$test"; then
        NEWLY_FAILING+=("$test")
    fi
done

# Show improvements
if [ ${#NEWLY_PASSING[@]} -gt 0 ]; then
    echo -e "${BOLD}${GREEN}🎉 IMPROVEMENTS (Previously failing, now passing):${NC}"
    for test in "${NEWLY_PASSING[@]}"; do
        echo -e "  ${GREEN}✓${NC} $test"
    done
    echo ""
fi

# Show regressions
if [ ${#NEWLY_FAILING[@]} -gt 0 ]; then
    echo -e "${BOLD}${RED}⚠️  REGRESSIONS (Previously passing, now failing):${NC}"
    for test in "${NEWLY_FAILING[@]}"; do
        echo -e "  ${RED}✗${NC} $test"
    done
    echo ""
fi

# Still failing tests
STILL_FAILING=()
for test in "${PREVIOUS_FAILURES[@]}"; do
    if printf '%s\n' "${FAILED_TESTS[@]}" | grep -qx "$test"; then
        STILL_FAILING+=("$test")
    fi
done

if [ ${#STILL_FAILING[@]} -gt 0 ]; then
    echo -e "${BOLD}Still Failing (${#STILL_FAILING[@]} tests):${NC}"
    for test in "${STILL_FAILING[@]}"; do
        echo -e "  ${RED}✗${NC} $test"
    done
    echo ""
fi

# Detailed failure list
if [ ${#FAILED_TESTS[@]} -gt 0 ]; then
    echo -e "${BOLD}========================================"
    echo -e "FAILED TESTS DETAIL"
    echo -e "========================================${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        # Check if this was a previously known failure
        if printf '%s\n' "${PREVIOUS_FAILURES[@]}" | grep -qx "$test"; then
            echo -e "${RED}✗${NC} $test ${YELLOW}(known issue)${NC}"
        else
            echo -e "${RED}✗${NC} $test ${RED}(NEW FAILURE)${NC}"
        fi
    done
    echo ""
fi

# Output comparison summary
echo -e "${BOLD}========================================"
echo -e "COMPARISON TO PREVIOUS RUN (19107316639)"
echo -e "========================================${NC}"
echo -e "Previous: ${GREEN}39${NC} passed, ${RED}12${NC} failed (76%)"
echo -e "Current:  ${GREEN}$PASS_COUNT${NC} passed, ${RED}$FAIL_COUNT${NC} failed (${PASS_RATE}%)"

PREV_PASS=39
PREV_TOTAL=51
PASS_DIFF=$((PASS_COUNT - PREV_PASS))
RATE_DIFF=$((PASS_RATE - 76))

if [ "$PASS_DIFF" -gt 0 ]; then
    echo -e "Change:   ${GREEN}+$PASS_DIFF tests passing${NC} (+${RATE_DIFF}%)"
elif [ "$PASS_DIFF" -lt 0 ]; then
    echo -e "Change:   ${RED}$PASS_DIFF tests passing${NC} ($RATE_DIFF%)"
else
    echo -e "Change:   No change in pass count"
fi

echo ""

# Exit with appropriate code
if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${BOLD}${GREEN}All tests passing! 🎉${NC}"
    exit 0
elif [ "$PASS_DIFF" -gt 0 ]; then
    echo -e "${BOLD}${GREEN}Progress made! 📈${NC}"
    exit 0
else
    exit 1
fi
