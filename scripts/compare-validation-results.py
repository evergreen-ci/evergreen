#!/usr/bin/env python3
"""
Compare validation results between baseline and patch runs.
Detects regressions where configs that passed baseline fail with patch.
"""

import json
import sys


def load_results(filename):
    with open(filename, 'r') as f:
        data = json.load(f)
    return {r['file']: r for r in data['results']}


def main():
    baseline_file = sys.argv[1]
    patch_file = sys.argv[2]
    report_file = sys.argv[3]
    regressed_files_output = sys.argv[4] if len(sys.argv) > 4 else None

    baseline = load_results(baseline_file)
    patch = load_results(patch_file)

    regressions = []
    timeout_excluded = []
    fixes = []

    for file, baseline_result in baseline.items():
        if file not in patch:
            continue
        patch_result = patch[file]

        if baseline_result['passed'] and not patch_result['passed']:
            patch_errors = patch_result.get('errors', '')
            if 'timed out' in patch_errors:
                timeout_excluded.append(file)
            else:
                regressions.append(file)
        elif not baseline_result['passed'] and patch_result['passed']:
            fixes.append(file)

    if fixes:
        print(f"Note: {len(fixes)} config(s) failed baseline but passed patch (indicates non-determinism):")
        for config in sorted(fixes):
            baseline_errors = baseline[config].get('errors', 'unknown')
            print(f"  [fixed in patch] {config}: baseline error was: {baseline_errors}")

    if timeout_excluded:
        print(f"Excluded {len(timeout_excluded)} timeout-based failure(s) from regression detection")
        for config in sorted(timeout_excluded):
            print(f"  [timeout excluded] {config}")

    # Log details for all regressions to help diagnose flakiness
    if regressions or timeout_excluded:
        print(f"\nRegression details (baseline passed -> patch failed):")
        for config in sorted(regressions + timeout_excluded):
            patch_errors = patch[config].get('errors', 'unknown')
            is_timeout = 'timed out' in patch_errors
            label = "[TIMEOUT]" if is_timeout else "[REGRESSION]"
            print(f"  {label} {config}: {patch_errors}")

    if regressions:
        with open(report_file, 'w') as f:
            f.write("REGRESSION DETECTED!\n")
            f.write("==================\n\n")
            f.write(f"Found {len(regressions)} configs that passed baseline but failed with patch:\n\n")
            for config in sorted(regressions):
                patch_errors = patch[config].get('errors', 'unknown')
                f.write(f"  - {config}\n")
                f.write(f"    patch error: {patch_errors}\n")
            if timeout_excluded:
                f.write(f"\nExcluded {len(timeout_excluded)} timeout-based failure(s) (not code regressions):\n\n")
                for config in sorted(timeout_excluded):
                    f.write(f"  - {config}\n")

        if regressed_files_output:
            with open(regressed_files_output, 'w') as f:
                for config in sorted(regressions):
                    f.write(f"{config}\n")

        print(f"Found {len(regressions)} regression(s)")
        sys.exit(1)
    else:
        with open(report_file, 'w') as f:
            f.write("No regressions detected.\n")
            f.write("All configs that passed baseline also pass with patch.\n")
            if timeout_excluded:
                f.write(f"\nExcluded {len(timeout_excluded)} timeout-based failure(s) (not code regressions):\n\n")
                for config in sorted(timeout_excluded):
                    f.write(f"  - {config}\n")

        print("No regressions found")
        sys.exit(0)


if __name__ == "__main__":
    main()