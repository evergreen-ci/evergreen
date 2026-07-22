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
    -stringsseq
    -testingcontext
)

# Generated files are intentionally left unmodernized, so drop any findings in them.
ignore_pattern='graphql/generated\.go|thirdparty/clients/fws/'

# Install the analyzer into the local bin directory first, so its download/build output does not get mixed into the
# analyzer's findings that are captured below.
bin_dir="$(pwd)/bin"
GOBIN="$bin_dir" go install "$modernize_pkg"
modernize_bin="$bin_dir/modernize"

# The analyzer exits non-zero when it reports findings, so guard the pipeline against errexit/pipefail.
findings=$("$modernize_bin" "${categories[@]}" ./... 2>&1 | grep -v -E "$ignore_pattern" || true)

if [ -n "$findings" ]; then
    echo "$findings"
    echo ""
    echo "FAIL: modernize found code that should use modern Go idioms (see above)."
    echo "To apply the suggested fixes automatically, run:"
    echo "  $modernize_bin ${categories[*]} -fix ./..."
    exit 1
fi

echo "PASS: no modernize regressions found."
