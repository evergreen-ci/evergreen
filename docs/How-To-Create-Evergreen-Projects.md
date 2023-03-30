# How to Create Evergreen Projects

This guide is meant to help users create their own projects on Evergreen without creating a ticket and waiting. 

## Authorization

To have access to create a project, the user must be either a project admins or super user.
If you do not see the `New Project` button on the project settings page, or do not have access to the project settings page, you may request admin permissions from MANA or ask a current admin to add you as one.

## Steps to Create

1. Visit the projects page in the new UI https://spruce.mongodb.com/projects/.
2. Click New Project. If you want a current project copied, click Duplicate Current Project. Otherwise, click Create New Project.
3. Enter the project name to match the GitHub repo name. Users can change this later.
4. Enter repo org ("owner") and repo name ("repo").
5. Do not set a Project ID unless you are a Server project and want to use the performance plugin.
6. If this project needs AWS credentials for S3 bucket access, click the check mark to open a JIRA ticket. (You should see the JIRA ticket under "reported by me.")
7. Click "Create New Project".


## Limitations

Because Evergreen can only support so many projects, there are limitations to the number of projects that could be enabled. 
There is a total project limit, the total number of projects that Evergreen is currently willing to support, 
and project per repo limit, a limit to the number of enabled projects that share the same GitHub owner and repo. 

If your GitHub owner and repo needs more than the alloted number of projects, please create an evergreen ticket to be able to override the project per repo limit.