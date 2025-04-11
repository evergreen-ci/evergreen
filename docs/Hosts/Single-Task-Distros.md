# Single Task Distros

Single task distros are distros that will only run one task before terminating. These distros should only be used by specific tasks that build binaries for public releases. 

## Using Single Task Distros

To use single task distros, the specified tasks/build variants must first be whitelisted by Evergreen admins. To do this, please create an Evergreen DEVPROD ticket on Jira requesting the use of single task distros containing the name of the project with a list of tasks/build variants that need to use these distros.

## Creating a single task distro

If a single task distro that fits your needs does not exist, create a Runtime Environments DEVPROD ticket on Jira requesting the specific image you need your distro to have. 
