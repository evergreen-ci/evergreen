# How to Create Evergreen Projects

This guide is meant to help users create their own projects on Evergreen without creating a ticket and waiting. 

## Authorization

To have access to create a project you must be either a super user or the admin of an existing project.
If you do not see the `New Project` button on the [project settings page](https://spruce.mongodb.com/project/YourProject/settings/general), or do not have access to the project settings page, you may follow the steps under 'For Evergreen Users' in the wiki below.

Note that that projects can only be created in spruce as it has been deprecated from the legacy UI.

## Steps to Create

Follow the steps on the [wiki](https://wiki.corp.mongodb.com/display/BUILD/How+to+Create+a+New+Evergreen+Project)

## Limitations

Because Evergreen can only support so many projects, there are limitations to the number of projects that could be enabled. 
There is a total project limit, the total number of projects that Evergreen is currently willing to support, 
and project per repo limit, a limit to the number of enabled projects that share the same GitHub owner and repo. 

If your GitHub owner and repo needs more than the allotted number of projects, please create an Evergreen ticket to be able to override the project per repo limit.