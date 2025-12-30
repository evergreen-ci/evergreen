#!/bin/bash

set -o errexit
set -o pipefail

CONFIGS_DIR="downloaded_configs"
BASELINE_RESULTS="baseline_validation.json"
PATCH_RESULTS="patch_validation.json"
REGRESSION_REPORT="regression_report.txt"
MAX_RETRIES=3

print_status() {
    echo -e "[INFO] $1"
}

print_error() {
    echo -e "[ERROR] $1"
}

get_go_binary() {
    local gobin="go"
    if [ -n "$GOROOT" ]; then
        gobin="$GOROOT/bin/go"
    fi
    echo "$gobin"
}

get_validation_stats() {
    local json_file=$1
    python3 -c "
import json
with open('$json_file', 'r') as f:
    data = json.load(f)
    print(f\"{data['passed']} {data['failed']}\")
"
}

build_evergreen_cli() {
    print_status "Building evergreen CLI..."

    local gobin=$(get_go_binary)

    if [ ! -f "bin/evergreen" ]; then
        if ! $gobin build -o bin/evergreen cmd/evergreen/evergreen.go; then
            print_error "Failed to build evergreen CLI"
            return 1
        fi
        print_status "Evergreen CLI built successfully"
    else
        print_status "Using existing evergreen CLI"
    fi

    return 0
}

setup_evergreen_config() {
    print_status "Setting up evergreen CLI configuration..."

    local config_file="$HOME/.evergreen.yml"

    if [ -z "$EVERGREEN_API_KEY" ] || [ -z "$EVERGREEN_API_USER" ] || [ -z "$EVERGREEN_API_SERVER_HOST" ]; then
        print_error "Required environment variables not set: EVERGREEN_API_KEY, EVERGREEN_API_USER, EVERGREEN_API_SERVER_HOST"
        return 1
    fi
    cat > "$config_file" << EOF
api_server_host: ${EVERGREEN_API_SERVER_HOST}
ui_server_host: ${EVERGREEN_API_SERVER_HOST}
api_key: ${EVERGREEN_API_KEY}
user: ${EVERGREEN_API_USER}
EOF

    print_status "Evergreen CLI configuration created"
    return 0
}

download_configs() {
    local attempt=1
    while [ $attempt -le $MAX_RETRIES ]; do
        print_status "Downloading all project configs (attempt $attempt/$MAX_RETRIES)..."

        rm -rf "$CONFIGS_DIR"
        mkdir -p "$CONFIGS_DIR"
        cd "$CONFIGS_DIR"

        if ../bin/evergreen admin all-configs; then
            cd ..
            print_status "Successfully downloaded configs"
            return 0
        else
            cd ..
            print_error "Failed to download configs on attempt $attempt"
            attempt=$((attempt + 1))
            if [ $attempt -le $MAX_RETRIES ]; then
                sleep 5
            fi
        fi
    done

    print_error "Failed to download configs after $MAX_RETRIES attempts"
    return 1
}

build_validator() {
    print_status "Building config validation program..."

    local gobin=$(get_go_binary)

    if ! $gobin build -o bin/validate-all-configs scripts/validate-all-configs.go; then
        print_error "Failed to build validation program"
        return 1
    fi

    print_status "Validation program built successfully"
    return 0
}

run_validation() {
    local output_file=$1

    print_status "Running validation..."

    if ! ./bin/validate-all-configs \
        --configs-dir "$CONFIGS_DIR" \
        --output "$output_file"; then
        print_error "Validation program exited with non-zero status"
        return 1
    fi

    return 0
}

compare_results() {
    local baseline=$1
    local patch=$2
    local report=$3

    print_status "Comparing validation results..."

    if python3 scripts/compare-validation-results.py "$baseline" "$patch" "$report"; then
        print_status "No regressions found!"
        return 0
    else
        print_error "Regressions detected!"
        cat "$report"
        return 1
    fi
}

cleanup() {
    if [ "$STASH_APPLIED" = "true" ]; then
        print_status "Restoring original code state..."
        git stash pop --quiet 2>/dev/null || true
    fi
}

main() {
    print_status "Starting config validation test"

    trap cleanup EXIT

    if ! build_evergreen_cli; then
        print_error "Failed to build evergreen CLI"
        exit 1
    fi

    if ! setup_evergreen_config; then
        print_error "Failed to setup evergreen CLI configuration"
        exit 1
    fi

    if ! download_configs; then
        print_error "Failed to download configs"
        exit 1
    fi

    local total_configs=$(find "$CONFIGS_DIR" -name "*.yml" -o -name "*.yaml" | wc -l)
    print_status "Found $total_configs config files to validate"

    if [ "$total_configs" -eq 0 ]; then
        print_error "No config files found to validate"
        exit 1
    fi

    STASH_APPLIED="false"
    if git diff --quiet validator/ && git diff --cached --quiet validator/; then
        print_status "No uncommitted changes to validator/, comparing against base branch"
    else
        print_status "Detected uncommitted changes to validator/"
        print_status "Stashing changes for baseline validation..."
        if ! git stash push -m "temp-stash-for-validation" validator/; then
            print_error "Failed to stash validator changes"
            exit 1
        fi
        STASH_APPLIED="true"
    fi

    print_status "Building baseline validator..."
    if ! build_validator; then
        if [ "$STASH_APPLIED" = "true" ]; then
            git stash pop --quiet
        fi
        print_error "Failed to build baseline validator"
        exit 1
    fi

    print_status "Running baseline validation..."
    if ! run_validation "$BASELINE_RESULTS"; then
        if [ "$STASH_APPLIED" = "true" ]; then
            git stash pop --quiet
        fi
        print_error "Failed to run baseline validation"
        exit 1
    fi

    if [ -f "$BASELINE_RESULTS" ]; then
        local stats=$(get_validation_stats "$BASELINE_RESULTS")
        local baseline_passed=$(echo $stats | cut -d' ' -f1)
        local baseline_failed=$(echo $stats | cut -d' ' -f2)
        print_status "Baseline: $baseline_passed passed, $baseline_failed failed"
    fi

    if [ "$STASH_APPLIED" = "true" ]; then
        print_status "Applying validator changes for patch validation..."
        if ! git stash pop --quiet; then
            print_error "Failed to restore validator changes"
            exit 1
        fi
        STASH_APPLIED="false"
    fi

    print_status "Building patch validator..."
    if ! build_validator; then
        print_error "Failed to build patch validator"
        exit 1
    fi

    print_status "Running patch validation..."
    if ! run_validation "$PATCH_RESULTS"; then
        print_error "Failed to run patch validation"
        exit 1
    fi

    if [ -f "$PATCH_RESULTS" ]; then
        local stats=$(get_validation_stats "$PATCH_RESULTS")
        local patch_passed=$(echo $stats | cut -d' ' -f1)
        local patch_failed=$(echo $stats | cut -d' ' -f2)
        print_status "Patch: $patch_passed passed, $patch_failed failed"
    fi

    if compare_results "$BASELINE_RESULTS" "$PATCH_RESULTS" "$REGRESSION_REPORT"; then
        print_status "Config validation test PASSED"

        rm -f "$BASELINE_RESULTS" "$PATCH_RESULTS" "$REGRESSION_REPORT"
        rm -rf "$CONFIGS_DIR"

        exit 0
    else
        print_error "Config validation test FAILED - regressions detected"

        if [ -s "$REGRESSION_REPORT" ]; then
            echo ""
            echo "Regression Report:"
            echo "=================="
            cat "$REGRESSION_REPORT"
        fi

        exit 1
    fi
}

main