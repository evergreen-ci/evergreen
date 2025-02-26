#!/usr/bin/env bash

set -eo pipefail

# Check if the correct number of arguments is provided
if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <config_file> <output_directory> <openapi_generator_script>"
    exit 1
fi

# Assign command-line arguments to variables
OPENAPI_HARDCODED_CONFIG="$1"
OPENAPI_OUTPUT_DIR="$2"
OPENAPI_GENERATOR="$3"

# Create the bin directory if it doesn't exist
mkdir -p bin
cd bin

# Check if Maven is downloaded; download if it doesn't exist
MAVEN_DIR="apache-maven-3.9.9"
if [ ! -d "$MAVEN_DIR" ]; then
    echo "Downloading Maven..."
    curl -O https://dlcdn.apache.org/maven/maven-3/3.9.9/binaries/apache-maven-3.9.9-bin.tar.gz
    tar xvzf apache-maven-3.9.9-bin.tar.gz
else
    echo "Maven already downloaded."
fi

# Check if OpenAPI generator CLI script is downloaded; download if it doesn't exist
OPENAPI_CLI="openapi-generator-cli.sh"
if [ ! -f "$OPENAPI_CLI" ]; then
    echo "Downloading OpenAPI generator CLI..."
    curl -O https://raw.githubusercontent.com/OpenAPITools/openapi-generator/master/bin/utils/openapi-generator-cli.sh
    chmod +x ./"$OPENAPI_CLI"
else
    echo "OpenAPI generator CLI already downloaded."
fi

cd ..

# Update the PATH to include Maven and Java
export PATH="${PWD}/bin/${MAVEN_DIR}/bin:${PATH}"
export PATH="/opt/bin/java/jdk21/bin:${PATH}"

# Generate the OpenAPI client
"$OPENAPI_GENERATOR" generate -i "$OPENAPI_HARDCODED_CONFIG" -g java -o "$OPENAPI_OUTPUT_DIR"
