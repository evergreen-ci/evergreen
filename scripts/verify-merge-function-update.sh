#!/usr/bin/env bash

set -eo pipefail

if [[ "${BRANCH_NAME}" == "" ]]; then
    BRANCH_NAME=main;
fi

merge_function_changed=false
parser_project_changed=false

# Find the common ancestor between the current set of changes and the upstream branch, then see if any source code files
# have changed in the merge functions.
common_ancestor=$(git merge-base ${BRANCH_NAME}@{upstream} HEAD);
files_changed="$(git diff --name-only "${common_ancestor}" -- 'model/project_parser_merge_functions.go')"
if [[ "${files_changed}" == "" ]]; then
    merge_function_changed=true
fi

# Find the latest commit when the ParserProject struct was updated.
project_parser_start_line_num=$(git grep -n -e "type ParserProject struct" model/project_parser.go | cut -d ':' -f 2);
project_parser_end_line_num=$(git grep -n -e "End of ParserProject struct" model/project_parser.go | cut -d ':' -f 2);
commit_project_parser_main=$(git blame -l -L ${project_parser_start_line_num},${project_parser_end_line_num} main -- model/project_parser.go | cut -d ' ' -f 1);
last_commit_project_parser_updated=$(git blame -l -L ${project_parser_start_line_num},${project_parser_end_line_num} model/project_parser.go | cut -d ' ' -f 1);

# Check if the last commit when the ParserProject struct was updated is in the list of committed changes.
if [[ "${commit_project_parser_main}" != "${last_commit_project_parser_updated}" ]]; then
    parser_project_changed=true
fi

for line in ${last_commit_project_parser_updated}
do 
    # If the ParserProject struct was updated but not committed yet, it still counts as a change.
    if [[ "${line}" == "0000000000000000000000000000000000000000" ]]; then
        parser_project_changed=true
    fi
done

if [[ "${merge_function_changed}" == "${parser_project_changed}" ]]; then
    exit 0
fi

if ${merge_function_changed}; then 
    echo "project_parser_merge_functions has been changed but ParserProject struct has not been updated. Please update project_parser_merge_functions." >&2
elif ${parser_project_changed}; then
    echo "ParserProject struct has been changed but project_parser_merge_functions has not been updated. Please update the ParserProject struct." >&2
fi
exit 1
