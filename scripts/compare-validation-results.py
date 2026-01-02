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
            f.write(f"Found {len(regressions)} configs that passed baseline but failed with patch:\n\n")
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