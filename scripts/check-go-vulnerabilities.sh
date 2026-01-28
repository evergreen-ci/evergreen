#!/usr/bin/env bash
set -o errexit

# Signal handlers to provide helpful messages for different scenarios
trap 'echo "Error: Script failed at line $LINENO. Check the output above for details." >&2' ERR
trap 'echo "Script interrupted by user (Ctrl+C)" >&2; exit 130' INT
trap 'echo "Script terminated by signal" >&2; exit 143' TERM

# This script checks for go vulnerabilities.

# User-configurable: List of package patterns to ignore (case-insensitive substring match)
# Example: "docker" will match "github.com/docker/docker", "docker/go-connections", etc.
# For each ignored package, include the corresponding ticket number in the inline comment.
# Format: "<package-name>"    # DEVPROD-XXXXX Brief description
# TODO: Each ignored package should be removed from this list once the corresponding tracking
#       ticket has been completed and the vulnerability has been addressed.
IGNORED_PACKAGES=(
    "stdlib"        # DEVPROD-25290 Stdlib vulnerability tracking
    "rardecode"     # DEVPROD-25293 Archiver / Rardecode vulnerability tracking
    "archiver/v3"   # DEVPROD-25293 Archiver / Rardecode vulnerability tracking
    "csrf"          # DEVPROD-25429 CSRF vulnerability tracking
)

# Validate that each ignored package has a tracking ticket
script_content=$(cat "$0")
ignored_section=$(echo "$script_content" | sed -n '/^IGNORED_PACKAGES=(/,/^)/p')
package_lines=$(echo "$ignored_section" | grep '"' || true)

missing_tickets=()
if [ -n "$package_lines" ]; then
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        if ! echo "$line" | grep -q '#.*DEVPROD-[0-9]\+'; then
            package=$(echo "$line" | sed 's/.*"\(.*\)".*/\1/')
            missing_tickets+=("$package")
        fi
    done <<< "$package_lines"
fi

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

result=$($govul -json -C "$(pwd)" ./...)

# Parse vulnerabilities from govulncheck JSON output and remove duplicates
# Note: Same vulnerability can appear multiple times due to different code paths
all_vulnerabilities=$(echo "$result" | jq -s '
    [.[] | select(.finding) | {
        osv: .finding.osv,
        fixed_version: (.finding.fixed_version // "N/A"),
        module: (.finding.trace[0].module // "unknown")
    }] | unique_by(.osv)
')

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

# Section 2: Vulnerabilities with no fixes currently available (FAIL)
echo "2. VULNERABILITIES WITH NO FIXES CURRENTLY AVAILABLE ($no_fixes_available_count)"
[ "$no_fixes_available_count" -gt 0 ] && echo "   Status: FAIL - Must attempt to fix or add to ignore list with ticket" || echo "   Status: PASS"
if [ "$no_fixes_available_count" -gt 0 ]; then
    echo "   Action Required: Try updating dependencies to resolve these vulnerabilities."
    echo "   If unable to fix, add to IGNORED_PACKAGES list with tracking ticket (format: DEVPROD-XXXXX)"
    echo ""
    echo "$no_fixes_available" | jq -r 'group_by(.module) | .[] |
        "  Module: \(.[0].module)\n" +
        (map("    • \(.osv)") | join("\n")) + "\n"'
else
    echo "  None"
fi
echo "-------------------------------------------"

# Section 3: Ignored vulnerabilities (PASS - user excluded)
echo "3. IGNORED VULNERABILITIES ($ignored_vulnerabilities_count)"
echo "   Status: PASS - Temporarily excluded with tracking tickets"
if [ "$ignored_vulnerabilities_count" -gt 0 ]; then
    # Group by module and display with tickets
    modules=$(echo "$ignored_vulnerabilities" | jq -r '[.[].module] | unique | .[]')

    while IFS= read -r module; do
        [ -z "$module" ] && continue

        # Find ticket for this module from IGNORED_PACKAGES
        ticket=""
        for pkg in "${IGNORED_PACKAGES[@]}"; do
            if echo "$module" | grep -qi "$pkg"; then
                ticket=$(echo "$ignored_section" | grep -i "$pkg" | grep -o 'DEVPROD-[0-9]\+' | head -1)
                break
            fi
        done

        echo "  Module: $module${ticket:+ (Ticket: $ticket)}"
        echo "$ignored_vulnerabilities" | jq -r --arg mod "$module" '
            [.[] | select(.module == $mod)] |
            map("    • \(.osv) (Fix: \(.fixed_version))") | join("\n")'
        echo ""
    done <<< "$modules"
else
    echo "  None"
fi
echo "=========================================="

# Check for missing tickets and display error in red if found
if [ ${#missing_tickets[@]} -gt 0 ]; then
    echo ""
    echo -e "\033[0;31mERROR: Package(s) in ignore list but no ticket found. Create/add ticket number:\033[0m"
    for pkg in "${missing_tickets[@]}"; do
        echo -e "\033[0;31m  - $pkg\033[0m"
    done
    echo ""
fi

# Exit with error if there are vulnerabilities with fixes available OR no fixes available OR missing tickets
if [ "$fixes_available_count" -gt 0 ] || [ "$no_fixes_available_count" -gt 0 ] || [ ${#missing_tickets[@]} -gt 0 ]; then
    fail_reasons=()

    if [ "$fixes_available_count" -gt 0 ]; then
        fail_reasons+=("$fixes_available_count with fixes available")
    fi

    if [ "$no_fixes_available_count" -gt 0 ]; then
        fail_reasons+=("$no_fixes_available_count with no fixes currently available")
    fi

    if [ ${#missing_tickets[@]} -gt 0 ]; then
        fail_reasons+=("${#missing_tickets[@]} ignored package(s) missing tickets")
    fi

    echo "FAIL: Found issues - $(IFS=', '; echo "${fail_reasons[*]}")"
    exit 1
else
    # Build pass message
    if [ "$ignored_vulnerabilities_count" -gt 0 ]; then
        echo "PASS: No immediate fixes required ($ignored_vulnerabilities_count temporarily excluded)"
    else
        echo "PASS: No vulnerabilities found"
    fi
    exit 0
fi
