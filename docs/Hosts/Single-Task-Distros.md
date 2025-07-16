# Single Task Distros

Single task distros are distros that will only run one task before terminating. These distros should only be used by specific tasks that build binaries for public releases.

## Who Should Use Single Task Distros?

Most users should not be using these distros. They are specifically created to compile release binaries in a secure environment. If your task _does_ compile for a release, please follow the below steps.

## How to Use Single Task Distros

To use single task distros, the specified tasks/build variants must first be added to an allowlist by Evergreen admins. To do this, please create an Evergreen DEVPROD ticket on Jira requesting the use of single task distros containing the name of the project with a list of tasks/build variants that need to use these distros. All users can view which projects and tasks are allowed to run on single task distros by clicking on the "single task distros" tab on the distro settings page.

## Creating Single Task Distros

If a single task distro that fits your needs does not exist, create a Runtime Environments DEVPROD ticket on Jira requesting the specific image you need your distro to have.

## Limitations

Single host task groups will still run on the same host due to the fact that they can logically be considered one task. They will run the group setup and teardown as expected.

In addition, generate task commands will not be allowed in any host using theses distros.
