
# Swaggo is either installed in ../swag or as a go module. If it's installed as a go module, the swaggo command is
# available in the PATH. If it's installed in ../swag, the swaggo command is available at ../swag.

swaggo="../swag"

# Test if swaggo is installed as a go module.
if command -v swag &> /dev/null; then
    swaggo=$(command -v swag)
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