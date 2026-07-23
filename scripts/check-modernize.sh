#!/usr/bin/env bash
set -o errexit
set -o pipefail

# Signal handlers to provide helpful messages for different scenarios
trap 'echo "Error: Script failed at line $LINENO. Check the output above for details." >&2' ERR
trap 'echo "Script interrupted by user (Ctrl+C)" >&2; exit 130' INT
trap 'echo "Script terminated by signal" >&2; exit 143' TERM

# This script guards against regressions of the Go modernization performed in DEVPROD-21825. It runs the modernize
# analyzer restricted to only the analyzer categories that were adopted in that change. Pinning the category list means a
# future x/tools/gopls release that introduces new modernizers will not fail this check until those categories are
# deliberately adopted (with the corresponding codebase changes). Findings in generated code are ignored because
# generated files are intentionally left unmodernized.

# Re-export the Go binary from GOROOT so the correct toolchain version is used (mirrors check-go-vulnerabilities.sh).
if [ -n "$GOROOT" ]; then
    export PATH="$GOROOT/bin:$PATH"
fi

# The modernize analyzer lives in the gopls module's internal packages, so it is not (and should not be) a dependency of
# evergreen's main module. Install it as a pinned versioned tool, matching how check-go-vulnerabilities.sh runs
# govulncheck. Bumping this version is a deliberate decision: a newer analyzer may report additional instances within
# these categories, which must be fixed in the same change that bumps the pin.
modernize_pkg="golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@v0.23.0"

# Analyzer categories adopted in DEVPROD-21825. Do not add a category here without first modernizing the codebase for it.
categories=(
    -any
    -forvar
    -mapsloop
    -minmax
    -plusbuild
    -rangeint
    -reflecttypefor
    -slicesbackward
    -slicescontains
    -stringsbuilder
    -stringscut
    -stringscutprefix
    -stringsseq
    -testingcontext
)

# The analyzer only sees files that build for the current platform, so files behind //go:build tags for other operating
# systems (e.g. *_linux.go, *_windows.go) are invisible unless we vary GOOS. CI achieves full coverage by running one
# task per OS in parallel, each passing its target GOOS in MODERNIZE_GOOS. A local `make modernize` leaves it unset and
# falls back to scanning only the host GOOS, which is fast; CI is the backstop that catches platform-specific
# regressions. GOARCH is left at the host default; build tags in this codebase gate on OS, not architecture.
if [ -n "$MODERNIZE_GOOS" ]; then
    platforms=("$MODERNIZE_GOOS")
else
    platforms=("$(go env GOHOSTOS)")
fi

# Generated files are intentionally left unmodernized, so drop any findings in them. This list must cover every checked-in
# generated file, because the modernize suite does not skip generated files on its own; a finding in one would otherwise
# fail CI on code that developers cannot hand-edit (it is overwritten on regeneration).
ignore_pattern='graphql/generated\.go|graphql/models_gen\.go|graphql/redacted_fields_gen\.go|rest/model/generated|thirdparty/clients/fws/'

# Install the analyzer into the local bin directory first, so its download/build output does not get mixed into the
# analyzer's findings that are captured below.
bin_dir="$(pwd)/bin"
GOBIN="$bin_dir" go install "$modernize_pkg"
modernize_bin="$bin_dir/modernize"

all_findings=""
for os in "${platforms[@]}"; do
    # Capture the analyzer's exit code so a genuine tool/build error (any code other than 0 or the "found diagnostics"
    # code 3) fails loudly instead of being misreported as a modernization finding. The `&& ... || ...` idiom keeps the
    # non-zero exit from tripping errexit or the ERR trap.
    raw=$(GOOS="$os" "$modernize_bin" "${categories[@]}" ./... 2>&1) && status=0 || status=$?

    if [ "$status" != "0" ] && [ "$status" != "3" ]; then
        echo "$raw"
        echo ""
        echo "ERROR: modernize failed for GOOS=$os (exit $status); see output above." >&2
        exit 1
    fi

    findings=$(echo "$raw" | grep -v -E "$ignore_pattern" || true)
    if [ -n "$findings" ]; then
        # Label each finding with the platform so a *_windows.go finding is not confusing when the check runs on Linux.
        all_findings+=$(echo "$findings" | sed "s/^/[GOOS=$os] /")$'\n'
    fi
done

if [ -n "$all_findings" ]; then
    printf '%s' "$all_findings"
    echo ""
    echo "FAIL: modernize found code that should use modern Go idioms (see above)."
    echo "To apply the suggested fixes automatically, run (repeat per GOOS shown above):"
    echo "  GOOS=<os> $modernize_bin ${categories[*]} -fix ./..."
    exit 1
fi

echo "PASS: no modernize regressions found."
