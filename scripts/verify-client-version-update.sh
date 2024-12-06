#!/usr/bin/env bash

set -eo pipefail

if [[ "${BRANCH_NAME}" == "" ]]; then
	BRANCH_NAME=main
fi

# Find the common ancestor between the current set of changes and the upstream branch, then see if any source code files
# have changed in the operations directory.
common_ancestor=$(git merge-base ${BRANCH_NAME}@{upstream} HEAD)
# Exclude tests, agent, and smoke test files, because those do not result in user-facing CLI changes.
files_changed="$(git diff --name-only "${common_ancestor}" -- 'operations/**.go' ':!operations/**_test.go' ':!operations/*agent*.go' ':!operations/*smoke*.go' ':!operations/host_provision.go')"
if [[ "${files_changed}" == "" ]]; then
	exit 0
fi

# Get the list of commit revisions.
committed_changes=$(git log --format=%H ${common_ancestor}..)

# Find the latest commit when the client version was updated.
client_version_line_num=$(git grep -n -e "ClientVersion =" config.go | cut -d ':' -f 2)
last_commit_client_version_updated=$(git blame -l -L ${client_version_line_num},${client_version_line_num} config.go | cut -d ' ' -f 1)

# Check if the last commit when the client version was updated is in the list of committed changes.
if [[ "${committed_changes}" =~ "${last_commit_client_version_updated}" ]]; then
	exit 0
fi

# If the client version was updated but not committed yet, it still counts as a change.
if [[ "${last_commit_client_version_updated}" == "0000000000000000000000000000000000000000" ]]; then
	exit 0
fi

echo -e "Files affected by client version update:\n${files_changed}" >&2
echo "The CLI has been changed, but the ClientVersion has not been updated. Please update the ClientVersion in config.go." >&2
exit 1
