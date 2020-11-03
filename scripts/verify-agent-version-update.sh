#!/usr/bin/env bash

set -eo pipefail

if [[ "${BRANCH_NAME}" == "" ]]; then
    BRANCH_NAME=master;
fi

# Find the common ancestor between the current set of changes and the upstream branch, then see if any source code files
# have changed in the agent or command packages.
common_ancestor=$(git merge-base ${BRANCH_NAME}@{upstream} HEAD);
if [[ "$(git diff --name-only ${common_ancestor} -- agent/**.go agent/**/*.go ':!agent/**_test.go' ':!agent/**/*_test.go' command/**.go command/**/*.go ':!command/**_test.go' ':!command/**/*_test.go' rest/client/evergreen_sender.go rest/client/logger.go rest/client/timeout_sender.go)" == "" ]]; then
    exit 0;
fi

# Get the list of commit revisions.
committed_changes=$(git log --format=%H ${common_ancestor}..);

# Find the latest commit when the agent version was updated.
agent_version_line_num=$(git grep -n -e "AgentVersion =" config.go | cut -d ':' -f 2);
last_commit_agent_version_updated=$(git blame -l -L ${agent_version_line_num},${agent_version_line_num} config.go | cut -d ' ' -f 1);

# Check if the last commit when the agent version was updated is in the list of committed changes.
if [[ "${committed_changes}" =~ "${last_commit_agent_version_updated}" ]]; then
    exit 0;
fi

# If the agent version was updated but not committed yet, it still counts as a change.
if [[ "${last_commit_agent_version_updated}" == "0000000000000000000000000000000000000000" ]]; then
    exit 0;
fi

echo "Agent has been changed but agent version has not been updated. Please update the agent version."
exit 1
