
# Swaggo is either installed in ../swag or as a go module. If it's installed as a go module, the swaggo command is
# available in the PATH. If it's installed in ../swag, the swaggo command is available at ../swag.

swaggo="swag"

echo $GOROOT
echo $GOBIN
echo $GOPATH

# Test if swaggo is installed as a binary or a go module.
if command -v swag &> /dev/null; then
    swaggo=$(command -v swag)
elif [ -n "$GOROOT" ] && [ -f "$GOROOT/bin/swag" ]; then
    swaggo="$GOROOT/bin/swag"
elif [ -n "$GOPATH" ] && [ -f "$GOPATH/bin/swag" ]; then
    swaggo="$GOPATH/bin/swag"
elif [ -n "$GOBIN" ] && [ -f "$GOBIN/swag" ]; then
    swaggo="$GOBIN/swag"
fi

# If swaggo is not installed, exit with an error.
if ! command -v "$swaggo" &> /dev/null; then
    echo "swaggo is not installed."
    exit 1
fi

before=$(git diff --diff-filter=M)
$swaggo fmt -g service/service.go
after=$(git diff --diff-filter=M)
if [ "$before" = "$after" ]; then
    exit 0
else
    echo "Please run 'make swaggo-format' in your local environment to fix the lint errors."
    exit 1
fi