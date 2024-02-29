#!/usr/bin/env bash
# This script checks for go vulnerabilities.

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

result=$($govul -C $(pwd) ./...)
if [ $? -eq 0 ]; then
    echo "No vulnerabilities found."
    exit 0
else
    echo "Please run govulncheck to check for vulnerabilities. See below for found vulnerabilities."
    version=$($govul --version)
    echo "Currently using govulncheck version: $version"
    echo "$result"
    exit 1
fi