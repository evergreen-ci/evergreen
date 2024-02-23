
#!/usr/bin/env bash

# This script checks for go vulnerabilities.

govul="govulncheck"

# Test if govulncheck is installed as a binary or a go module.
if command -v govul &> /dev/null; then
    # If the user has installed swaggo as a binary, use the binary.
    swaggo=$(command -v govul)
elif [ -n "$GOPATH" ] && [ -f "$GOPATH/bin/govulncheck" ]; then
    swaggo="$GOPATH/bin/govulncheck"
elif [ -n "$GOBIN" ] && [ -f "$GOBIN/govulncheck" ]; then
    swaggo="$GOBIN/swag"
fi

# If govulncheck is not installed, exit with an error.
if ! command -v "$govul" &> /dev/null; then
    echo "govulncheck is not installed."
    exit 1
fi

result=$($govul ./...)
if [ $? -eq 0 ]; then
    exit 0
else
    echo "Please run govulncheck to check for vulnerabilities. See below for found vulnerabilities."
    version=$($govul --version)
    echo "Currently using govulncheck version: $version"
    echo "$result"
    exit 1
fi