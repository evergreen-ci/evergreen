#!/usr/bin/env bash

# This script checks for swaggo and then uses the found swaggo to run swaggo fmt, failing if the command makes any changes to the files.

swaggo="swag"

# Test if swaggo is installed as a binary or a go module.
if command -v swag &> /dev/null; then
    # If the user has installed swaggo as a binary, use the binary.
    swaggo=$(command -v swag)
elif [ -n "$GOPATH" ] && [ -f "$GOPATH/bin/swag" ]; then
    swaggo="$GOPATH/bin/swag"
elif [ -n "$GOBIN" ] && [ -f "$GOBIN/swag" ]; then
    swaggo="$GOBIN/swag"
fi

# If swaggo is not installed, exit with an error.
if ! command -v "$swaggo" &> /dev/null; then
    echo "swaggo is not installed. Please install swaggo by running 'make swaggo-install'"
    exit 1
fi

# Compare the diff before and after running swaggo fmt. We only care about if the modified files has changed.
before=$(git diff --diff-filter=M)
$swaggo fmt -g service/service.go
after=$(git diff --diff-filter=M)
if [ "$before" = "$after" ]; then
    echo "No formatting errors found."
    exit 0
else
    echo "Please run 'make swaggo-format' in your local environment to fix the lint errors. If this is your local environment, please commit the changes this command made."
    version=$($swaggo --version)
    echo "Currently using swaggo version: $version"
    exit 1
fi