#!/usr/bin/env bash

set -eo pipefail

if [[ "${BRANCH_NAME}" == "" ]]; then
    BRANCH_NAME=main;
fi

# Find the common ancestor between the current set of changes and the upstream branch, then see if any source code files
# have changed in the merge functions.
common_ancestor=$(git merge-base ${BRANCH_NAME}@{upstream} HEAD);
files_changed="$(git diff --name-only "${common_ancestor}" -- 'model/project_parser_merge_functions.go')"
if [[ "${files_changed}" == "" ]]; then
    exit 0;
fi

# Find the latest commit when the ParserProject struct was updated.
project_parser_start_line_num=$(git grep -n -e "type ParserProject struct" model/project_parser.go | cut -d ':' -f 2);
project_parser_end_line_num=$(git grep -n -e "End of ParserProject struct" model/project_parser.go | cut -d ':' -f 2);
commit_project_parser_main=$(git blame -l -L ${project_parser_start_line_num},${project_parser_end_line_num} main -- model/project_parser.go | cut -d ' ' -f 1);
last_commit_project_parser_updated=$(git blame -l -L ${project_parser_start_line_num},${project_parser_end_line_num} model/project_parser.go | cut -d ' ' -f 1);

# Check if the last commit when the ParserProject struct was updated is in the list of committed changes.
if [[ "${commit_project_parser_main}" == "${last_commit_project_parser_updated}" ]]; then
    exit 0;
fi

for line in ${last_commit_project_parser_updated}
do 
    # If the ParserProject struct was updated but not committed yet, it still counts as a change.
    if [[ "${line}" == "0000000000000000000000000000000000000000" ]]; then
        exit 0;
    fi
done

echo "Only one of either ParserProject struct and project_parser_merge_functions has been updated. Please update both." >&2
exit 1
