# REST v2 API

Base URL: `http://<EVERGREEN_HOST>/rest/v2/`

## General Functionality

See [docs generated from the OpenAPI spec](./OpenAPI.md) for information on specific endpoints.

### A note on authentication

Many of the these REST endpoints do not require authentication to
access, but some do. These will return a 404 if no authentication
headers are sent, if the username is invalid, or if the API key is
incorrect. Use the `user` and `api_key` fields from the
[settings](https://spruce.mongodb.com/preferences/cli) page to set two headers,
`Api-User` and `Api-Key`.

### Content Type and Communication

The API accepts and returns all results in JSON. Some resources also
allow URL parameters to provide additional specificity to a request.

### Errors

When an error is encountered during a request, the API returns a JSON
object with the HTTP status code and a message describing the error of
the form:

    {
     "status": <http_status_code>,
     "error": <error message>
    }

### Pagination

API Routes that fetch many objects return them in a JSON array and
support paging through subsets of the total result set. When there are
additional results for the query, access to them is populated in a [Link
HTTP header](https://www.w3.org/wiki/LinkHeader). This header has the
form:

    "Link" : <http://<EVERGREEN_HOST>/rest/v2/path/to/resource?start_at=<pagination_key>&limit=<objects_per_page>; rel="next"

    <http://<EVERGREEN_HOST>/rest/v2/path/to/resource?start_at=<pagination_key>&limit=<objects_per_page>; rel="prev"

### Dates

Date fields are returned and accepted in ISO-8601 UTC extended format.
They contain 3 fractional seconds with a 'dot' separator.

### Empty Fields

A returned object will always contain its complete list of fields. Any
field that does not have an associated value will be filled with JSON's
null value.

## Deprecated endpoints

### TaskStats (DEPRECATED)
**IMPORTANT: The task stats REST API has been deprecated, please use [Trino task stats](../Project-Configuration/Evergreen-Data-for-Analytics.md) instead.**

### Notifications  (DEPRECATED)

Create custom notifications for email or Slack issues. 

We are investigating moving this out of Evergreen (EVG-21065) and won't be supporting future work for this. 

### Users

#### Endpoints

##### Delete User Permissions ``````````\`

    DELETE /users/<user_id>/permissions

Deletes all permissions of a given type for a user by deleting their
roles of that type for that resource ID. This ignores the Basic
Project/Distro Access that is given to all MongoDB employees.

Note that usage of this endpoint requires that the requesting user have
security to modify roles. The format of the body is: :

    {
      "resource_type": "project",
      "resource_id": "project_id", 
    }

-   resource_type - the type of resources for which to delete
    permissions. Must be one of "project", "distro", "superuser",
    or "all". "all" will revoke all permissions for the user.
-   resource_id - the resource ID for which to delete permissions.
    Required unless deleting all permissions.

##### Get Users for Role

    GET /roles/<role_id>/users

Gets a list of users for the specified role. The format of the response
is:
```json
{ "users": ["list", "of", "users"] }
```

##### Give Roles to User

    POST /users/<user_id>/roles

Adds the specified roles to the specified user. Attempting to add a
duplicate role will result in an error. If you're unsure of what roles
you want to add, you probably want to POST To /users/user_id/permissions
instead. Note that usage of this endpoint requires that the requesting
user have security to modify roles. The format of the body is: :

    {
      "roles": [ "role1", "role2" ],
      "create_user": true,
    }

-   roles - the list of roles to add for the user
-   create_user - if true, will also create a shell user document for
    the user. By default, specifying a user that does not exist will
    error

##### Offboard User

    POST /users/offboard_user

Marks unexpirable volumes and hosts as expirable for the user, and
removes the user as a project admin for any projects, if applicable.
This returns the IDs of the hosts/volumes that were unexpirable and
modified.

This route expects to receive the user in a json body with the following
format: :

    {
      "email": "my_user@email.com"
    }

-   email - the email of the user

The format of the response is: :

    {
      "terminated_hosts": [ "i-12345", "i-abcd" ],
      "terminated_volumes": ["volume-1"],
    }

**Query Parameters**

| Name    | Type | Description                                                                          |
|---------|------|--------------------------------------------------------------------------------------|
| dry_run | bool | If set to true, route returns the IDs of the hosts/volumes that *would* be modified. |

## REST V2 Use Case Guide

### Find all failures of a given build

#### Endpoint

`GET /builds/<build_id>/tasks`

#### Description

To better understand the state of a build, perhaps when attempting to
determine the quality of a build for a release, it is helpful to be able
to fetch information about the tasks it ran. To fetch this data, make a
call to the `GET /builds/<build_id>/tasks` endpoint. Page through the
results task data to produce meaningful statistics like the number of
task failures or percentage of failures of a given build.

## Find detailed information about the status of a particular tasks and its tests

### Endpoints

`GET /tasks/<task_id>`

`GET /tasks/<task_id>/tests`

#### Description

To better understand all aspects of a task failure, such as failure
mode, which tests failed, and how long it took, it is helpful to fetch
this information about a task. This can be accomplished using 2 API
calls. Make one call to the endpoint for a specific task
`GET /tasks/<task_id>` which returns information about the task itself.
Then, make a second cal to `GET /tasks/<task_id>/tests` which delivers
specific information about the tests that ran in a certain task.

### Get all hosts

#### Endpoint

`GET /hosts`

#### Description

Retrieving information on Evergreen's hosts can be helpful for system
monitoring. To fetch this information, make a call to `GET /hosts`,
which returns a paginated list of hosts. Page through the results to
inspect all hosts.

By default, this endpoint will only return hosts that are considered
"up" (status is equal to running, initializing, starting,
provisioning, or provision failed).

### Restart all failures for a commit

#### Endpoints

`GET /project/<project_name>/revisions/<commit_hash>/tasks`

`POST /tasks/<task_id>/restart`

#### Description

Some Evergreen projects contain flaky tests or can endure spurious
failures. To restart all of these tasks to gain better signal a user can
fetch all of the tasks for a commit. Make a request to
`GET /project/<project_name>/revisions/<commit_hash>/tasks` to fetch the
tasks that ran and then loop over all of the returned tasks, calling
`POST /tasks/<task_id>/restart` on each task which has failed.

### Modify an Existing Project

#### Endpoint

`PATCH /projects/<project_id>`

#### Description

To modify the project, make a request to the endpoint with a JSON object
as the body (using the project object descriptions on the REST V2 Usage
wiki page). The result of a successful PATCH will be a 200 status. To
see the modified project, make a request to
`GET /projects/<project_id>`.

For example, to enable the commit queue the body would be:

    { "commit_queue": 
      { "enabled": "true" } 
    }

To add and delete admins:

    { "admins": ["annie.black", "brian.samek"], // does not overwrite existing admins    
      "delete_admins": ["john.liu"] // deletes existing admin }

To add/delete variables and specify which are private:

    { "variables": 
      { "vars": { // add to existing variables
        "banana": "yellow",             
        "apple": "red", },         
      "private_vars": { "apple": "true", // this cannot be undone         
      },         
      "vars_to_delete": ["watermelon"] }}

### Copy an Existing Project

#### Endpoint

`POST /projects/<project_id>/copy`

#### Description

To copy a project to a new project, this is the route you would use. To
define the new project's name (which is required), we would include a
query parameter, for example:

    projects/my_first_project/copy?new_project=my_second_project

This route will return the new project but this will not include
variables/aliases/subscriptions; to see this, GET the new project.
