#!/bin/bash

# This script prepares the swagger.json to be published. The version
# is calculated by hashing the file and comparing it to the previous
# version. If the file has changed, the version number is incremented.

# An example usage of this script is:
# SWAGGER_OLD_SUM_FILE=bin/swagger.sum OUTPUT_SUM_FILE=bin/swagger.sum sh prepare-swagger-push.sh
# Note that this isn't syncing with the deployed version of our swagger generated file. This means
# it will increment the version number if you haven't ran this script after a while and the version
# has since been incremented.

set -o errexit

# Default to local development swagger.json file.
if [[ "${SWAGGER_JSON_FILE}" == "" ]]; then
    SWAGGER_JSON_FILE="bin/swagger.json"
fi

# Check if SWAGGER_JSON_FILE exists
if [[ ! -f "${SWAGGER_JSON_FILE}" ]]; then
    echo "Error: swagger.json file '${SWAGGER_JSON_FILE}' does not exist."
    exit 1
fi

version_number=1

# Check if the old sum file exists and read values if it does.
if [[ -f "${SWAGGER_OLD_SUM_FILE}" ]]; then
    read old_sum old_version_number < "${SWAGGER_OLD_SUM_FILE}"
    version_number="${old_version_number}"
fi

# Use a temp copy to avoid modifying the original file.
temp_swagger_json=$(mktemp)
cp "${SWAGGER_JSON_FILE}" "${temp_swagger_json}"

# Replace the version placeholder with the current version number.
perl -pi -e 's/\{OPENAPI_VERSION\}/'$version_number'/' "${temp_swagger_json}"

# Generate the sum of the temporary swagger.json file.
temp_sha=$(shasum -a 256 "${temp_swagger_json}" | cut -d ' ' -f 1)

# Compare checksum and update the version number if needed.
if [[ -f "${SWAGGER_OLD_SUM_FILE}" && "${temp_sha}" == "${old_sum}" ]]; then
    echo "The swagger.json file has not changed. Using version number ${version_number}."
else
    # Increment the version number if sums don't match.
    version_number=$((version_number + 1))
    echo "The swagger.json file has changed. Incrementing version number to ${version_number}."
fi

# Replace the version placeholder with the new version number.
perl -pi -e 's/\{OPENAPI_VERSION\}/'$version_number'/' "${SWAGGER_JSON_FILE}"

# Compute a new SHA with the latest version number.
new_sha=$(shasum -a 256 "${SWAGGER_JSON_FILE}" | cut -d ' ' -f 1)

# Write the new SHA and version number to the output sum file.
echo "${new_sha} ${version_number}" > "${OUTPUT_SUM_FILE}"
echo "New SHA and version number saved to ${OUTPUT_SUM_FILE}."
