#!/usr/bin/env bash
# This script checks for go vulnerabilities.

# User-configurable: List of package patterns to ignore (case-insensitive substring match)
# Example: "docker" will match "github.com/docker/docker", "docker/go-connections", etc.
IGNORED_PACKAGES=(
    # Add package patterns here, one per line
    # Example: "docker"
    "docker"
    "stdlib"
)

govul="govulncheck"


# Test if govulncheck is installed as a binary or a go module.
if command -v govulncheck &> /dev/null; then
    # If the user has installed govulncheck as a binary, use the binary.
    govul=$(command -v govulncheck)
elif [ -n "$GOPATH" ] && [ -f "$GOPATH/bin/govulncheck" ]; then
    govul="$GOPATH/bin/govulncheck"
elif [ -n "$GOBIN" ] && [ -f "$GOBIN/govulncheck" ]; then
    govul="$GOBIN/govulncheck"
fi

# If govulncheck is not installed, exit with an error.
if ! command -v "$govul" &> /dev/null; then
    echo "govulncheck is not installed. Please install govulncheck by running 'make govul-install'"
    exit 1
fi

# We re-export the go binary in the front to make sure agents pick up the latest version. Without this,
# the agent picks up an older version that isn't compatible with govulncheck.
export PATH="$GOROOT/bin:$PATH"

result=$($govul -json -C $(pwd) ./... 2>&1)
exit_code=$?

# Parse vulnerabilities from govulncheck JSON output and remove duplicates
# Note: Same vulnerability can appear multiple times due to different code paths
all_vulnerabilities=$(echo "$result" | jq -s '
    [.[] | select(.finding) | {
        osv: .finding.osv,
        fixed_version: (.finding.fixed_version // "N/A"),
        module: (.finding.trace[0].module // "unknown")
    }] | unique_by(.osv)
' 2>/dev/null)

# Build case-insensitive regex pattern from ignored packages
if [ ${#IGNORED_PACKAGES[@]} -gt 0 ]; then
    ignored_pattern=$(printf "%s\n" "${IGNORED_PACKAGES[@]}" | awk '{print tolower($0)}' | paste -sd '|' -)
else
    ignored_pattern=""
fi

# Split vulnerabilities: fixes available, no fixes available, and ignored
if [ -n "$ignored_pattern" ]; then
    fixes_available=$(echo "$all_vulnerabilities" | jq --arg ignored "$ignored_pattern" '
        [.[] | select(.fixed_version != "N/A") |
         select((.module | ascii_downcase | test($ignored)) | not)]
    ')
    no_fixes_available=$(echo "$all_vulnerabilities" | jq --arg ignored "$ignored_pattern" '
        [.[] | select(.fixed_version == "N/A") |
         select((.module | ascii_downcase | test($ignored)) | not)]
    ')
    ignored_vulnerabilities=$(echo "$all_vulnerabilities" | jq --arg ignored "$ignored_pattern" '
        [.[] | select(.module | ascii_downcase | test($ignored))]
    ')
else
    fixes_available=$(echo "$all_vulnerabilities" | jq '[.[] | select(.fixed_version != "N/A")]')
    no_fixes_available=$(echo "$all_vulnerabilities" | jq '[.[] | select(.fixed_version == "N/A")]')
    ignored_vulnerabilities='[]'
fi

# Count vulnerabilities in each category
fixes_available_count=$(echo "$fixes_available" | jq 'length')
no_fixes_available_count=$(echo "$no_fixes_available" | jq 'length')
ignored_vulnerabilities_count=$(echo "$ignored_vulnerabilities" | jq 'length')

# Display results
echo ""
echo "=========================================="
echo "VULNERABILITY SCAN RESULTS"
echo "=========================================="
echo ""

# Section 1: Vulnerabilities with fixes available (FAIL if found)
echo "1. VULNERABILITIES WITH FIXES AVAILABLE ($fixes_available_count)"
[ "$fixes_available_count" -gt 0 ] && echo "   Status: FAIL - MUST FIX BELOW VULNERABILITIES" || echo "   Status: PASS"
if [ "$fixes_available_count" -gt 0 ]; then
    echo "$fixes_available" | jq -r 'group_by(.module) | .[] |
        "  Module: \(.[0].module)\n" +
        (map("    • \(.osv) (Fix: \(.fixed_version))") | join("\n")) + "\n"'
else
    echo "  None"
fi
echo "-------------------------------------------"

# Section 2: Vulnerabilities with no fixes currently available (PASS)
echo "2. VULNERABILITIES WITH NO FIXES CURRENTLY AVAILABLE ($no_fixes_available_count)"
echo "   Status: PASS - No fix released yet, create tickets to track"
if [ "$no_fixes_available_count" -gt 0 ]; then
    echo "$no_fixes_available" | jq -r 'group_by(.module) | .[] |
        "  Module: \(.[0].module)\n" +
        (map("    • \(.osv)") | join("\n")) + "\n"'
else
    echo "  None"
fi
echo "-------------------------------------------"

# Section 3: Ignored vulnerabilities (PASS - user excluded)
echo "3. IGNORED VULNERABILITIES ($ignored_vulnerabilities_count)"
echo "   Status: PASS - Temporarily excluded, create tickets to track"
if [ "$ignored_vulnerabilities_count" -gt 0 ]; then
    echo "$ignored_vulnerabilities" | jq -r 'group_by(.module) | .[] |
        "  Module: \(.[0].module)\n" +
        (map("    • \(.osv) (Fix: \(.fixed_version))") | join("\n")) + "\n"'
else
    echo "  None"
fi
echo "=========================================="

# Exit with error ONLY if there are vulnerabilities with fixes available
if [ "$fixes_available_count" -gt 0 ]; then
    echo "FAIL: Found $fixes_available_count vulnerabilities with fixes available"
    exit 1
else
    # Build conditional pass message
    pass_msg="PASS: No immediate fixes required"
    conditions=()

    if [ "$no_fixes_available_count" -gt 0 ]; then
        conditions+=("$no_fixes_available_count awaiting upstream fixes")
    fi

    if [ "$ignored_vulnerabilities_count" -gt 0 ]; then
        conditions+=("$ignored_vulnerabilities_count temporarily excluded")
    fi

    # Add conditions to message if any exist
    if [ ${#conditions[@]} -gt 0 ]; then
        pass_msg="$pass_msg (but $(IFS=', '; echo "${conditions[*]}"))"
    else
        pass_msg="PASS: No vulnerabilities found"
    fi

    echo "$pass_msg"
    exit 0
fi
