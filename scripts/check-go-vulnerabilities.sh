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

result=$($govul -json -C $(pwd) ./...)
exit_code=$?

# Split vulnerabilities into fixable and N/A lists
fixable=$(echo "$result" | jq -s '[.[] | select(.finding) | select(.finding.fixed_version != "" and .finding.fixed_version != "N/A") | {osv: .finding.osv, fixed_version: .finding.fixed_version, module: (.finding.trace[0].module // "unknown")}]')
na_fixes=$(echo "$result" | jq -s '[.[] | select(.finding) | select(.finding.fixed_version == "" or .finding.fixed_version == "N/A") | {osv: .finding.osv, fixed_version: "N/A", module: (.finding.trace[0].module // "unknown")}]')

if [ $exit_code -eq 0 ]; then
    echo "No vulnerabilities found."
    echo '{"fixable":[],"na_fixes":[]}'
    exit 0
else
    echo "Vulnerabilities with available fixes:"
    echo "$fixable" | jq -r '.[] | "  - \(.osv) in \(.module) -> Fixed in: \(.fixed_version)"'
    echo ""
    echo "Vulnerabilities with N/A fixes:"
    echo "$na_fixes" | jq -r '.[] | "  - \(.osv) in \(.module)"'
    echo ""
    echo '{"fixable":'"$fixable"',"na_fixes":'"$na_fixes"'}'

    # Exit with error only if there are fixable vulnerabilities
    if [ "$(echo "$fixable" | jq 'length')" -gt 0 ]; then
        exit 1
    else
        exit 0
    fi
fi