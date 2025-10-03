#!/usr/bin/env bash

# This script runs gqlgen generate and fails if the command makes any changes to the files.

# Compare the diff before and after running gqlgen generate. We only care about if the modified files has changed.
before=$(git diff --diff-filter=M)
make gqlgen
after=$(git diff --diff-filter=M)
if [ "$before" = "$after" ]; then
    echo "No changes found after running gqlgen."
    exit 0
else
    echo "Error: make gqlgen generated changes. Please run 'make gqlgen' and commit the changes."
    echo ""
    echo "Files modified:"
    git diff --name-only --diff-filter=M
    echo ""
    echo "Changes:"
    git diff --diff-filter=M
    exit 1
fi
