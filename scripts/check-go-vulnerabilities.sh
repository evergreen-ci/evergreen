#!/usr/bin/env bash
# This script checks for go vulnerabilities.

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

# Split vulnerabilities into fixable and N/A lists
fixable=$(echo "$result" | jq -s '[.[] | select(.finding) | select(.finding.fixed_version != null and .finding.fixed_version != "" and .finding.fixed_version != "N/A") | {osv: .finding.osv, fixed_version: .finding.fixed_version, module: (.finding.trace[0].module // "unknown")}]' 2>/dev/null)
na_fixes=$(echo "$result" | jq -s '[.[] | select(.finding) | select(.finding.fixed_version == null or .finding.fixed_version == "" or .finding.fixed_version == "N/A") | {osv: .finding.osv, fixed_version: "N/A", module: (.finding.trace[0].module // "unknown")}]' 2>/dev/null)

# Count findings to determine if we actually have vulnerabilities
fixable_count=$(echo "$fixable" | jq 'length' 2>/dev/null || echo "0")
na_count=$(echo "$na_fixes" | jq 'length' 2>/dev/null || echo "0")
total_vulns=$((fixable_count + na_count))

if [ "$total_vulns" -eq 0 ]; then
    echo "No vulnerabilities found."
    echo '{"fixable":[],"na_fixes":[]}'
    exit 0
else
    echo "Vulnerabilities with available fixes:"
    if [ "$fixable_count" -gt 0 ]; then
        # Group by module and fixed_version, then list OSV IDs
        echo "$fixable" | jq -r 'group_by(.module, .fixed_version) | .[] | "\n  Package: \(.[0].module)\n  Fixed in: \(.[0].fixed_version)\n  Vulnerabilities: \([.[].osv] | join(", "))"'
    else
        echo "  None"
    fi
    echo ""
    echo "Vulnerabilities with N/A fixes:"
    if [ "$na_count" -gt 0 ]; then
        # Group by module, then list OSV IDs
        echo "$na_fixes" | jq -r 'group_by(.module) | .[] | "\n  Package: \(.[0].module)\n  Vulnerabilities: \([.[].osv] | join(", "))"'
    else
        echo "  None"
    fi

    # Exit with error only if there are fixable vulnerabilities
    if [ "$(echo "$fixable" | jq 'length')" -gt 0 ]; then
        exit 1
    else
        exit 0
    fi
fi