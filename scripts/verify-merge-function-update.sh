#!/usr/bin/env bash
# Script to verify that if the ParserProject struct is updated, its merge functions are also updated.

set -eo pipefail

if [[ "${BRANCH_NAME}" == "" ]]; then
    BRANCH_NAME=main;
fi

MERGE_FUNC_FILE_PATH=model/project_parser_merge_functions.go
PROJECT_PARSER_FILE_PATH=model/project_parser.go

# Find the common ancestor between the current set of changes and the upstream branch,
# then figure out which lines (if any) in the ParserProject struct have been modified since the base revision.
common_ancestor=$(git merge-base ${BRANCH_NAME}@{upstream} HEAD);
project_parser_start_line_num=$(git grep -n -e "type ParserProject struct" "${PROJECT_PARSER_FILE_PATH}" | cut -d ':' -f 2);
project_parser_end_line_num=$(git grep -n -e "End of ParserProject struct" "${PROJECT_PARSER_FILE_PATH}" | cut -d ':' -f 2);
project_parser_base_revisions=$(git blame -l -L ${project_parser_start_line_num},${project_parser_end_line_num} "${common_ancestor}" -- "${PROJECT_PARSER_FILE_PATH}" | cut -d ' ' -f 1);
project_parser_updated_revisions=$(git blame -l -L ${project_parser_start_line_num},${project_parser_end_line_num} "${PROJECT_PARSER_FILE_PATH}" | cut -d ' ' -f 1);

# Check if the line-by-line revisions in the base branch of the ParserProject struct are different 
# from the line-by-line revisions that have been committed on top of the base branch. 
# If they differ, then some commit on top of the base branch has changed the ParserProject struct.
if [[ "${project_parser_base_revisions}" == "${project_parser_updated_revisions}" ]]; then
    exit 0;
fi

# See if the project parser merge functions file has changed
files_changed="$(git diff --name-only "${common_ancestor}" -- "${MERGE_FUNC_FILE_PATH}")"
if [[ "${files_changed}" != "" ]]; then
    exit 0;
fi

echo -e "ParserProject struct has been changed but ${MERGE_FUNC_FILE_PATH} has not been updated. Please update the merge function." >&2
exit 1
