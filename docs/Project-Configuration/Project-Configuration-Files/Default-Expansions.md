# Default Expansions Provided by Evergreen

Evergreen provides a set of default expansions that can be used in your project configuration files.

"execution": The execution number of the task (e.g. 0, 1, 2).
"version_id": The ID of the version this task came from.
"task_id": The ID of the task.
"task_name": The name of the task.
"build_id": The ID of the build this task came from.
"build_variant": The name of the build variant this task came from.
"revision": The commit hash this task will pull from.
"github_known_hosts": A known hosts file for GitHub.
"project" / "project_identifier": The identifier of the project.
"project_id": The ID of the project.
"distro_id": The ID of the distro.
"branch_name": The name of the branch.
"author": The author of the patch/version.
"author_email": The email of the author of the patch/version.
"create_at": The time the patch/version was created.
"requester": The type of requester that created the patch/version.

If the task is a git tag requester:
"triggered_by_git_tag": The git tag that triggered this task.

If the task is a patch (GitHub PR, GitHub merge queue, or patch) requester:
"is_patch": "true"
"revision_order_id": This template string: `${author}_${revision_order_number}`
"alias": The alias of the patch.

If the task is not a patch requester:
"is_patch": ""
"revision_order_number": The order number of the revision.

If the task is from a GitHub merge queue:
"is_commit_queue": "true"
"github_org": The organization of the GitHub merge queue.
"github_repo": The repository of the GitHub merge queue.
"github_head_branch": The head branch of the GitHub merge queue.

If the task is from a GitHub PR:
"github_pr_number": The number of the GitHub PR.
"github_org": The organization of the GitHub PR.
"github_repo": The repository of the GitHub PR.
"github_author": The author of the GitHub PR.
"github_commit": The commit of the head hash of the GitHub PR.

If the task is stepback:
"is_stepback": "true"

If the task is triggered by any upstream trigger:
"trigger_event_identifier": The identifier of the trigger event.
"trigger_event_type": The type of the trigger event.
"trigger_id": The ID of the trigger.
"trigger_repo_owner": The owner of the repository that triggered this task.
"trigger_repo_name": The name of the repository that triggered this task.
"trigger_branch": The branch of the repository that triggered this task.

If the task is triggered by an upstream project task or build trigger:
"trigger_status": The status of the task that triggered this task.
"trigger_revision": The revision of the task that triggered this task.
"trigger_version": The version ID of the task that triggered this task.

If the task is running on windows:
"host_service_password": The password for the host service user.

This is an exhaustive list from this [PopulateExpansions function](https://github.com/evergreen-ci/evergreen/blob/main/model/project.go#L1016) in the Evergreen codebase. If you need additional expansions, please file a feature request.
