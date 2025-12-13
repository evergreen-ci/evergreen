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

# Extract current versions from go.mod
current_versions=$(grep -E '^\s*github\.com|^\s*go\.|^\s*golang\.org|^\s*gopkg\.in|^\s*stdlib' go.mod | grep -v '//' | awk '{print $1 "|||" $2}' | tr '\n' '|')

# Split vulnerabilities into fixable and N/A lists, and separate stdlib vulnerabilities
fixable=$(echo "$result" | jq -s '[.[] | select(.finding) | select(has("finding") and (.finding | has("fixed_version")) and .finding.fixed_version != null and .finding.fixed_version != "" and .finding.fixed_version != "N/A") | {osv: .finding.osv, fixed_version: .finding.fixed_version, module: (.finding.trace[0].module // "unknown")}]' 2>/dev/null)
na_fixes=$(echo "$result" | jq -s '[.[] | select(.finding) | select(has("finding") and ((.finding | has("fixed_version") | not) or .finding.fixed_version == null or .finding.fixed_version == "" or .finding.fixed_version == "N/A")) | {osv: .finding.osv, fixed_version: "N/A", module: (.finding.trace[0].module // "unknown")}]' 2>/dev/null)

# Separate stdlib vulnerabilities from fixable and N/A lists
fixable_stdlib=$(echo "$fixable" | jq '[.[] | select(.module == "stdlib")]' 2>/dev/null)
fixable_non_stdlib=$(echo "$fixable" | jq '[.[] | select(.module != "stdlib")]' 2>/dev/null)
na_stdlib=$(echo "$na_fixes" | jq '[.[] | select(.module == "stdlib")]' 2>/dev/null)
na_non_stdlib=$(echo "$na_fixes" | jq '[.[] | select(.module != "stdlib")]' 2>/dev/null)

# Count findings to determine if we actually have vulnerabilities
fixable_stdlib_count=$(echo "$fixable_stdlib" | jq 'length' 2>/dev/null || echo "0")
fixable_non_stdlib_count=$(echo "$fixable_non_stdlib" | jq 'length' 2>/dev/null || echo "0")
na_stdlib_count=$(echo "$na_stdlib" | jq 'length' 2>/dev/null || echo "0")
na_non_stdlib_count=$(echo "$na_non_stdlib" | jq 'length' 2>/dev/null || echo "0")
total_vulns=$((fixable_stdlib_count + fixable_non_stdlib_count + na_stdlib_count + na_non_stdlib_count))

if [ "$total_vulns" -eq 0 ]; then
    echo "No vulnerabilities found."
    echo '{"fixable":[],"na_fixes":[],"stdlib":[]}'
    exit 0
else
    echo "Vulnerabilities with available fixes:"
    if [ "$fixable_non_stdlib_count" -gt 0 ]; then
        # Group by module, find min/max versions, and collect all OSV IDs
        echo "$fixable_non_stdlib" | jq -r --arg cv "$current_versions" '
        group_by(.module) | .[] |
        {
            module: .[0].module,
            min_version: ([.[].fixed_version] | min),
            max_version: ([.[].fixed_version] | max),
            vulnerabilities: ([.[].osv] | unique)
        } |
        . as $item |
        ($cv | split("|") | map(select(startswith($item.module + "|||"))) | .[0] // "" | split("|||")[1] // "unknown") as $current |
        if $item.min_version == $item.max_version then
            "---\n  Package: \($item.module)\n  Current: \($current)\n  Fixed in: \($item.min_version)\n  Vulnerabilities: \($item.vulnerabilities | join(", "))"
        else
            "---\n  Package: \($item.module)\n  Current: \($current)\n  Minimum fix: \($item.min_version)\n  Latest fix: \($item.max_version)\n  Vulnerabilities: \($item.vulnerabilities | join(", "))"
        end
        '
    else
        echo "  None"
    fi
    echo "----------"
    echo "Stdlib vulnerabilities:"
    echo "On hold - need to update Go Version for this"
    if [ "$fixable_stdlib_count" -gt 0 ] || [ "$na_stdlib_count" -gt 0 ]; then
        if [ "$fixable_stdlib_count" -gt 0 ]; then
            echo "  With available fixes:"
            echo "$fixable_stdlib" | jq -r --arg cv "$current_versions" '
            group_by(.module) | .[] |
            {
                module: .[0].module,
                min_version: ([.[].fixed_version] | min),
                max_version: ([.[].fixed_version] | max),
                vulnerabilities: ([.[].osv] | unique)
            } |
            . as $item |
            ($cv | split("|") | map(select(startswith($item.module + "|||"))) | .[0] // "" | split("|||")[1] // "unknown") as $current |
            if $item.min_version == $item.max_version then
                "  ---\n    Package: \($item.module)\n    Current: \($current)\n    Fixed in: \($item.min_version)\n    Vulnerabilities: \($item.vulnerabilities | join(", "))"
            else
                "  ---\n    Package: \($item.module)\n    Current: \($current)\n    Minimum fix: \($item.min_version)\n    Latest fix: \($item.max_version)\n    Vulnerabilities: \($item.vulnerabilities | join(", "))"
            end
            '
        fi
        if [ "$na_stdlib_count" -gt 0 ]; then
            echo "  With N/A fixes:"
            echo "$na_stdlib" | jq -r 'group_by(.module) | .[] | "  ---\n    Package: \(.[0].module)\n    Vulnerabilities: \([.[].osv] | unique | join(", "))"'
        fi
    else
        echo "  None"
    fi
    echo "----------"
    echo "Vulnerabilities with N/A fixes:"
    echo "On hold - create necessary tickets to address"
    if [ "$na_non_stdlib_count" -gt 0 ]; then
        # Group by module, then list unique OSV IDs
        echo "$na_non_stdlib" | jq -r 'group_by(.module) | .[] | "---\n  Package: \(.[0].module)\n  Vulnerabilities: \([.[].osv] | unique | join(", "))"'
    else
        echo "  None"
    fi

    # Exit with error only if there are non-stdlib fixable vulnerabilities
    if [ "$fixable_non_stdlib_count" -gt 0 ]; then
        exit 1
    else
        exit 0
    fi
fi
