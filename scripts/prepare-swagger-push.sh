#!/bin/bash

# This script generates a sum of the swagger.json file for the OpenAPI spec.
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
    # Use the old version number if it exists.
    version_number="${old_version_number}"
fi

# A temporary copy is used to avoid modifying the original swagger.json file.
temp_swagger_json=$(mktemp)
cp "${SWAGGER_JSON_FILE}" "${temp_swagger_json}"
# Replace the version placeholder with the current version number.
sed -i "s/{OPENAPI_VERSION}/${version_number}/g" "${temp_swagger_json}"
# Generate the sum of the temporary swagger.json file.
new_sum=$(shasum -a 256 "${temp_swagger_json}" | cut -d ' ' -f 1)

# Compare checksum and update the version number if needed.
if [[ -f "${SWAGGER_OLD_SUM_FILE}" && "${new_sum}" == "${old_sum}" ]]; then
    echo "The swagger.json file has not changed. Using version number ${version_number}."
else
    # Increment the version number if sums don't match.
    version_number=$((version_number + 1))
    echo "The swagger.json file has changed. Incrementing version number to ${version_number}."
fi

# Replace the version placeholder with the new version number.
sed -i "s/{OPENAPI_VERSION}/${version_number}/g" "${SWAGGER_JSON_FILE}"

# Write the new SHA and version number to the output sum file.
echo "${new_sum} ${version_number}" > "${OUTPUT_SUM_FILE}"
echo "New SHA and version number saved to ${OUTPUT_SUM_FILE}."
