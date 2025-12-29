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

build_evergreen_cli() {
    print_status "Building evergreen CLI..."

    local gobin="go"
    if [ -n "$GOROOT" ]; then
        gobin="$GOROOT/bin/go"
    fi

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

    local gobin="go"
    if [ -n "$GOROOT" ]; then
        gobin="$GOROOT/bin/go"
    fi

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
    fi

    return 0
}

compare_results() {
    local baseline=$1
    local patch=$2
    local report=$3

    print_status "Comparing validation results..."

    cat > compare_results.py << 'EOF'
import json
import sys

def load_results(filename):
    with open(filename, 'r') as f:
        data = json.load(f)
    return {r['file']: r['passed'] for r in data['results']}

def main():
    baseline_file = sys.argv[1]
    patch_file = sys.argv[2]
    report_file = sys.argv[3]

    baseline = load_results(baseline_file)
    patch = load_results(patch_file)

    regressions = []

    for file, baseline_passed in baseline.items():
        if baseline_passed and file in patch and not patch[file]:
            regressions.append(file)

    if regressions:
        with open(report_file, 'w') as f:
            f.write("REGRESSION DETECTED!\n")
            f.write("==================\n\n")
            f.write(f"Found {len(regressions)} config(s) that passed baseline but failed with patch:\n\n")
            for config in sorted(regressions):
                f.write(f"  - {config}\n")

        print(f"Found {len(regressions)} regression(s)")
        sys.exit(1)
    else:
        with open(report_file, 'w') as f:
            f.write("No regressions detected.\n")
            f.write("All configs that passed baseline also pass with patch.\n")

        print("No regressions found")
        sys.exit(0)

if __name__ == "__main__":
    main()
EOF

    if python3 compare_results.py "$baseline" "$patch" "$report"; then
        print_status "No regressions found!"
        rm -f compare_results.py
        return 0
    else
        print_error "Regressions detected!"
        cat "$report"
        rm -f compare_results.py
        return 1
    fi
}

cleanup() {
    # Ensure we restore the original state if script exits
    if [ "$STASH_APPLIED" = "true" ]; then
        print_status "Restoring original code state..."
        git stash pop --quiet 2>/dev/null || true
    fi
}

main() {
    print_status "Starting config validation test"

    # Set up cleanup trap
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

    # Check if there are uncommitted changes to validator code
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

    # Build and run baseline validation
    print_status "Building baseline validator..."
    if ! build_validator; then
        if [ "$STASH_APPLIED" = "true" ]; then
            git stash pop --quiet
        fi
        print_error "Failed to build baseline validator"
        exit 1
    fi

    print_status "Running baseline validation..."
    if ! run_validation "$BASELINE_RESULTS" true; then
        if [ "$STASH_APPLIED" = "true" ]; then
            git stash pop --quiet
        fi
        print_error "Failed to run baseline validation"
        exit 1
    fi

    if [ -f "$BASELINE_RESULTS" ]; then
        local baseline_passed=$(python3 -c "import json; data=json.load(open('$BASELINE_RESULTS')); print(data['passed'])")
        local baseline_failed=$(python3 -c "import json; data=json.load(open('$BASELINE_RESULTS')); print(data['failed'])")
        print_status "Baseline: $baseline_passed passed, $baseline_failed failed"
    fi

    # Apply changes for patch validation
    if [ "$STASH_APPLIED" = "true" ]; then
        print_status "Applying validator changes for patch validation..."
        if ! git stash pop --quiet; then
            print_error "Failed to restore validator changes"
            exit 1
        fi
        STASH_APPLIED="false"  # Mark as no longer stashed since we popped it
    fi

    # Rebuild validator with changes
    print_status "Building patch validator..."
    if ! build_validator; then
        print_error "Failed to build patch validator"
        exit 1
    fi

    print_status "Running patch validation..."
    if ! run_validation "$PATCH_RESULTS" true; then
        print_error "Failed to run patch validation"
        exit 1
    fi

    if [ -f "$PATCH_RESULTS" ]; then
        local patch_passed=$(python3 -c "import json; data=json.load(open('$PATCH_RESULTS')); print(data['passed'])")
        local patch_failed=$(python3 -c "import json; data=json.load(open('$PATCH_RESULTS')); print(data['failed'])")
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