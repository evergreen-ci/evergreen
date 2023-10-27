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

## Resources

The API has a series of implemented objects that it returns depending on
the queried endpoint.

### Status Message

    GET /admin/banner

    {
      "banner": "Evergreen is currently unable to pick up new commits or process pull requests due to a GitHub outage",
      "theme": "warning"
    }

### TaskStats (DEPRECATED)
**IMPORTANT: The task stats REST API has been deprecated, please use [Trino task stats](../Project-Configuration/Evergreen-Data-for-Analytics.md) instead.**

#### Endpoints

##### Fetch the Task Stats for a project

    GET /projects/<project_id>/task_stats  

Returns a paginated list of task stats associated with a specific project filtered and grouped according to the query parameters.  

| Name           | Type                                | Description                                                                                                                                                                                                                                                   |
|----------------|-------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| after_date     | string                              | The start date (included) of the targeted time interval. The format is "YYYY-MM-DD". The date is UTC.                                                                                                                                                         |
| before_date    | string                              | The end date (excluded) of the targeted time interval. The format is "YYYY-MM-DD". The date is UTC.                                                                                                                                                           |
| group_num_days | int                                 | Optional. Indicates that the statistics should be aggregated by groups of`group_num_days`days. The first group will start on`after_date`, the last group will end on the day preceding`before_date`and may have less than`group_num_days`days. Defaults to 1. |
| requesters     | []string or comma separated strings | Optional. The requesters that triggered the task execution. Accepted values are`mainline`,`patch`,`trigger`, and`adhoc`. Defaults to`mainline`.                                                                                                               |
| tasks          | []string or comma separated strings | The tasks to include in the statistics.                                                                                                                                                                                                                       |
| variants       | []string or comma separated strings | Optional. The build variants to include in the statistics.                                                                                                                                                                                                    |
| distros        | []string or comma separated strings | Optional. The distros to include in the statistics.                                                                                                                                                                                                           |
| group_by       | string                              | Optional. How to group the results. Accepted values are`task_variant`,`task`. By default the results are not grouped, i.e. are returned by combination of task + variant + distro.                                                                            |
| sort           | string                              | Optional. The order in which the results are returned. Accepted values are`earliest`and`latest`. Defaults to`earliest`.                                                                                                                                       |
| start_at       | string                              | Optional. The identifier of the task stats to start at in the pagination                                                                                                                                                                                      |
| limit          | int                                 | Optional. The number of task stats to be returned per page of pagination. Defaults to 1000.                                                                                                                                                                   |

#### TaskReliability 

Task Reliability success scores are aggregated task execution statistics for a given project. Statistics can be grouped by time period (days) and by task, variant, distro combinations.  The score is based on the lower bound value of a `Binomial proportion confidence interval <https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval>`_.  In this case, the equation is a `Wilson score interval <https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval#Wilson%20score%20interval%20with%20continuity%20correction>`_:  |Wilson score interval with continuity correction|   In statistics, a binomial proportion confidence interval is a confidence interval for the probability of success calculated from the outcome of a series of success–failure experiments (Bernoulli trials). In other words, a binomial proportion confidence interval is an interval estimate of a success probability p when only the number of experiments n and the number of successes nS are known.  The advantage of using a confidence interval of this sort is that the computed value takes the number of test into account. The lower the number of test, the greater the margin of error. This results in a lower success rate score for the cases where there are fewer test results.  During the evaluation of this algorithm, 22 consecutive test passes are required before a success  score of .85 is reached (with a significance level / α of ``0.05`).  

##### Objects

| Name                 | Type   | Description                                                                                                                                                                 |
|----------------------|--------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| task_name            | string | Name of the task the test ran under.                                                                                                                                        |
| variant              | string | Name of the build variant the task ran on. Omitted if the grouping does not include the build variant.                                                                      |
| distro               | string | Identifier of the distro that the task ran on. Omitted if the grouping does not include the distro.                                                                         |
| date                 | string | The start date ("YYYY-MM-DD" UTC day) of the period the statistics cover.                                                                                                   |
| num_success          | int    | The number of times the task was successful during the target period.                                                                                                       |
| num_failed           | int    | The number of times the task failed during the target period.                                                                                                               |
| num_total            | int    | The number of times the task ran during the target period.                                                                                                                  |
| num_timeout          | int    | The number of times the task ended on a timeout during the target period.                                                                                                   |
| num_test_failed      | int    | The number of times the task failed with a failure of type `test` during the target period.                                                                                 |
| num_system_failed    | int    | The number of times the task failed with a failure of type `system` during the target period.                                                                               |
| num_setup_failed     | int    | The number of times the task failed with a failure of type `setup` during the target period.                                                                                |
| avg_duration_success | float  | The average duration, in seconds, of the tasks that passed during the target period.                                                                                        |
| success_rate         | float  | The success rate score calculated over the time span, grouped by time period and distro, variant or task. The value ranges from 0.0 (total failure) to 1.0 (total success). |

##### Endpoints 

###### Fetch the Task Reliability score for a project

    GET /projects/<project_id>/task_reliability

Returns a paginated list of task reliability scores associated with a
specific project filtered and grouped according to the query parameters.

**Parameters**

| Name             | Type                                | Description                                                                                                                                                                                                                                                           |
|------------------|-------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `before_date`    | string                              | The end date (included) of the targeted time interval. The format is "YYYY-MM-DD". The date is UTC. Defaults to today.                                                                                                                                                |
| `after_date`     | string                              | The start date (included) of the targeted time interval. The format is "YYYY-MM-DD". The date is UTC. Defaults to `before_date`.                                                                                                                                      |
| `group_num_days` | int                                 | Optional. Indicates that the statistics should be aggregated by groups of `group_num_days` days. The first group will start on the nearest first date greater than `after_date`, the last group will start on `before_date` - `group_num_days`\` days. Defaults to 1. |
| `requesters`     | []string or comma separated strings | Optional. The requesters that triggered the task execution. Accepted values are `mainline`, `patch`, `trigger`, and `adhoc`. Defaults to `mainline`.                                                                                                                  |
| `tasks`          | []string or comma separated strings | The tasks to include in the statistics.                                                                                                                                                                                                                               |
| `variants`       | []string or comma separated strings | Optional. The build variants to include in the statistics.                                                                                                                                                                                                            |
| `distros`        | []string or comma separated strings | Optional. The distros to include in the statistics.                                                                                                                                                                                                                   |
| `group_by`       | string                              | Optional. How to group the results. Accepted values are `task`, `task_variant`, and `task_variant_distro`. By default the results are grouped by task.                                                                                                                |
| `sort`           | string                              | Optional. The order in which the results are returned. Accepted values are `earliest` and `latest`. Defaults to `latest`.                                                                                                                                             |
| `start_at`       | string                              | Optional. The identifier of the task stats to start at in the pagination                                                                                                                                                                                              |
| `limit`          | int                                 | Optional. The number of task stats to be returned per page of pagination. Defaults to 1000.                                                                                                                                                                           |


##### Examples

Get the current daily task reliability score.

    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint

Get the daily task reliability score for a specific day. ```` :

    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&before_date=2019-06-15
    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&before_date=2019-06-15&after_date=2019-06-15

Get the daily task reliability score from after date to today. ```` :

    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&after_date=2019-06-15

Get the current weekly task reliability score. ```` :

    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&group_num_days=7

Get the current monthly task reliability score. ```` :

    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&group_num_days=28

Get the task reliability score trends. ````

Project is mongodb-mongo-master, task is lint. Assuming today is
2019-08-29 then 2019-03-15 is 6 months ago. :

    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&after_date=2019-03-15
    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&after_date=2019-03-15&group_num_days=7
    GET /projects/mongodb-mongo-master/task_reliability?tasks=lint&after_date=2019-03-15&group_num_days=28

### Notifications  (DEPRECATED)

Create custom notifications for email or Slack issues. 

We are investigating moving this out of Evergreen (EVG-21065) and won't be supporting future work for this. 

#### Objects

**Email**

| Name            | Type                | Description                                                                                                       |
|-----------------|---------------------|-------------------------------------------------------------------------------------------------------------------|
| `from`          | string              | Optional. The email sender.                                                                                       |
| `recipients`    | []string            | The email recipient.                                                                                              |
| `subject`       | string              | Optional. The email subject.                                                                                      |
| `body`          | string              | Optional. The email body.                                                                                         |
| `is_plain_text` | string              | Optional. Specifies the Content-Type of the email. If true, it will be "text/plain"; otherwise it is "text/html". |
| `headers`       | map[string][]string | Optional. Email headers.                                                                                          |


**Slack**

| Name          | Type                | Description                                         |
|--------------------|---------|-----------------------------------------------------|
| `target`      | string              | Required. @name or public #channel of the recipient |
| `msg`         | string              | Required. The message for the notification.         |
| `attachments` | []SlackAttachment | Optional. Array of attachments to a message.        |


**SlackAttachment**

| Name          | Type                   | Description                                                                                                                                             |
|---------------|------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| `title`       | string                 | Optional. The attachment title.                                                                                                                         |
| `title_link`  | string                 | Optional. A URL that turns the title into a link.                                                                                                       |
| `text`        | string                 | If `fallback` is empty, this is required. The main body text of the attachment as plain text, or with markdown using `mrkdwn_in`.                       |
| `fallback`    | string                 | If `text` is empty, this is required. A plain text summary of an attachment for clients that don't show formatted text (eg. IRC, mobile notifications). |
| `mrkdwn_in`   | []string               | Optional. An array of fields that should be formatted with markdown.                                                                                    |
| `color`       | string                 | Optional. The message color. Can either be one of good (green), warning (yellow), danger (red), or any hex color code (eg. #439FE0).                    |
| `author_name` | string                 | Optional. The display name of the author.                                                                                                               |
| `author_icon` | string                 | Optional. A URL that displays the author icon. Will only work if `author_name` is present.                                                              |
| `fields`      | []SlackAttachmentField | Optional. Array of SlackAttachmentFields that get displayed in a table-like format.                                                                     |


**SlackAttachmentField**

| Name    | Type   | Description                                                                                                         |
|---------|--------|---------------------------------------------------------------------------------------------------------------------|
| `title` | string | Optional. The field title.                                                                                          |
| `value` | string | Optional. The field text. It can be formatted as plain text or with markdown by using `mrkdwn_in`.                  |
| `short` | string | Optional. Indicates whether the field object is short enough to be displayed side-by-side with other field objects. |


This corresponds with documentation for the [Slack API for
attachments](https://api.slack.com/reference/messaging/attachments).

#### Endpoints

    POST /notifications/<type>

The type can be "email" or "slack".

### Permissions

    GET /permissions

Returns a static list of project and distro permissions that can be
granted to users. The format is :

    {
       projectPermissions: [
         {
           key: "permission_key",
           name: "My Permission",
           levels: [
             {
               description: "Edit this permission",
               value: 10,
             }
           ]
         }
       ]
     distroPermissions:[
         {
           key: "permission_key",
           name: "My distro Permission",
           levels: [
             {
               description: "Edit this permission",
               value: 10,
             }
           ]
         }
       ]
    }

### Users

#### Endpoints

Give Permissions to User

    POST /users/<user_id>/permissions  Grants the user specified by user_id the permissions in the request body. 

Note that usage of this endpoint requires that the requesting user have security to modify roles. The format of the body is : 
```
{
      "resource_type": "project",
      "resources": ["project1", "project2"],
      "permissions": {
         "project_tasks: 30,
         "project_patches": 10
      }
}
```

* resource_type - the type of resources for which permission is granted. Must be one of "project", "distro", or "superuser" 
* resources - an array of strings representing what resources the access is for. For a resource_type of project, this will be a list of projects. For a resource_type of distro, this will be a list of distros. 
* permissions - an object whose keys are the permission keys returned by the /permissions endpoint above, and whose values are the levels of access to grant for that permission (also returned by the /permissions endpoint)  

Get User Permissions

    GET /users/<user_id>/permissions

**Parameters**

| Name | Type    | Description                                                      |
|------|---------|------------------------------------------------------------------|
| all  | Boolean | Optional. If included, we will not filter out basic permissions. |


Retrieves all permissions for the user (ignoring basic permissions that
are given to all users, unless all=true is included). The format of the
response is :
```
[ {
 "type": "project",
 "permissions": {
    "project1": { "project_tasks": 30, "project_patches": 10 },
    "project2": { "project_tasks": 10, "project_patches": 10 } }
 } 
 {
 "type": "distro",
 "permissions": { "distro1": {"distro_settings": 10 } } 
  } ]
```

-   type - the type of resources for which the listed permissions apply.
    Will be "project", "distro", or "superuser"
-   permissions - an object whose keys are the resources for which the
    user has permissions. Note that these objects will often have many
    keys, since logged-in users have basic permissions to every project
    and distro. The values in the keys are objects representing the
    permissions that the user has for that resource, identical to the
    format of the permissions field in the POST
    /users/\<user_id\>/permissions API.

Get All User Permissions For Resource

    GET /users/permissions  

Retrieves all users with permissions for the resource, and their highest permissions, and returns this as a mapping. This ignores basic permissions that are given to all users.  

The format of the body is: 

    {
      "resource_type": "project",
        "resources": ["project1", "project2"],
        "permissions": {
             "project_tasks: 30,
                  "project_patches": 10
                    }
                    }"
        }
    }

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
