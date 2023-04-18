# REST API v2

## General Functionality

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

### Task

The task is a basic unit of work understood by Evergreen. They usually
comprise a suite of tests or generation of a set of artifacts.

#### Objects

**Task**

| Name                   | Type          | Description                                                                                                                                                                                                                                             |
|------------------------|---------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `task_id`              | string        | Unique identifier of this task                                                                                                                                                                                                                          |
| `create_time`          | time          | Time that this task was first created                                                                                                                                                                                                                   |
| `dispatch_time`        | time          | Time that this time was dispatched                                                                                                                                                                                                                      |
| `scheduled_time`       | time          | Time that this task is scheduled to begin                                                                                                                                                                                                               |
| `start_time`           | time          | Time that this task began execution                                                                                                                                                                                                                     |
| `finish_time`          | time          | Time that this task finished execution                                                                                                                                                                                                                  |
| `version_id`           | string        | An identifier of this task by its project and commit hash                                                                                                                                                                                               |
| `branch`               | string        | The version control branch that this task is associated with                                                                                                                                                                                            |
| `revision`             | string        | The version control identifier associated with this task                                                                                                                                                                                                |
| `requester`            | string        | Version created by one of patch_request", "github_pull_request", "gitter_request" (caused by git commit, aka the repotracker requester), "trigger_request" (Project Trigger versions) , "merge_test" (commit queue patches), "ad_hoc" (periodic builds) |
| `priority`             | int           | The priority of this task to be run                                                                                                                                                                                                                     |
| `activated`            | boolean       | Whether the task is currently active                                                                                                                                                                                                                    |
| `activated_by`         | string        | Identifier of the process or user that activated this task                                                                                                                                                                                              |
| `build_id`             | string        | Identifier of the build that this task is part of                                                                                                                                                                                                       |
| `distro_id`            | string        | Identifier of the distro that this task runs on                                                                                                                                                                                                         |
| `build_variant`        | string        | Name of the buildvariant that this task runs on                                                                                                                                                                                                         |
| `depends_on`           | array         | List of task_ids of task that this task depends on before beginning                                                                                                                                                                                     |
| `display_name`         | string        | Name of this task displayed in the UI                                                                                                                                                                                                                   |
| `host_id`              | string        | The ID of the host this task ran or is running on                                                                                                                                                                                                       |
| `tags`                 | []string      | List of tags defined for the task, if any                                                                                                                                                                                                               |
| `execution`            | int           | The number of the execution of this particular task                                                                                                                                                                                                     |
| `order`                | int           | For mainline commits, represents the position in the commit history of commit this task is associated with. For patches, this represents the number of total patches submitted by the user.                                                             |
| `status`               | string        | The current status of this task                                                                                                                                                                                                                         |
| `display_status`       | string        | The status of this task that is displayed in the UI                                                                                                                                                                                                     |
| `status_details`       | status_object | Object containing additional information about the status                                                                                                                                                                                               |
| `logs`                 | logs_object   | Object containing raw and event logs for this task                                                                                                                                                                                                      |
| `parsley_logs`         | logs_object   | Object containing parsley logs for this task                                                                                                                                                                                                            |
| `time_taken_ms`        | int           | Number of milliseconds this task took during execution                                                                                                                                                                                                  |
| `expected_duration_ms` | int           | Number of milliseconds expected for this task to execute                                                                                                                                                                                                |
| `previous_executions`  | []Task        | Contains previous executions of the task if they were requested, and available. May be empty.                                                                                                                                                           |
| `parent_task_id`       | string        | The ID of the task's parent display task, if requested and available                                                                                                                                                                                    |
| `artifacts`            | []File        | The list of artifacts associated with the task.                                                                                                                                                                                                         |

**Logs**

| Name       | Type   | Description                                           |
|--------------------|---------|----------------------------------------------|
| agent_log  | string | Link to logs created by the agent process             |
| task_log   | string | Link to logs created by the task execution            |
| system_log | string | Link to logs created by the machine running the task  |
| all_log    | string | Link to logs containing merged copy of all other logs |

**Status**

| Name      | Type    | Description                                  |
|-----------|---------|----------------------------------------------|
| status    | string  | The status of the completed task             |
| type      | string  | The method by which the task failed          |
| desc      | string  | Description of the final status of this task |
| timed_out | boolean | Whether this task ended in a timeout         |

**File**
| Name             | Type    | Description                                               |
|------------------|---------|-----------------------------------------------------------|
| name             | string  | Human-readable name of the file                           |
| link             | string  | Link to the file                                          |
| visibility       | string  | Determines who can see the file in the UI                 |
| ignore_for_fetch | boolean | When true, these artifacts are excluded from reproduction |

#### Endpoints

##### List Tasks By Build

    GET /builds/<build_id>/tasks

List all tasks within a specific build.

| Name                 | Type    | Description                                                                          |
|----------------------|---------|--------------------------------------------------------------------------------------|
| start_at             | string  | Optional. The identifier of the task to start at in the pagination                   |
| limit                | int     | Optional. The number of tasks to be returned per page of pagination. Defaults to 100 |
| fetch_all_executions | boolean | Optional. Fetches previous executions of tasks if they are available                 |
| fetch_parent_ids     | boolean | Optional. Fetches the parent display task ID for each returned execution task        |

##### List Tasks By Project And Commit

    GET /projects/<project_name>/revisions/<commit_hash>/tasks

List all tasks within a mainline commit of a given project (excludes
patch tasks)

| Name          | Type   | Description                                                                          |
|---------------|--------|--------------------------------------------------------------------------------------|
| start_at      | string | Optional. The identifier of the task to start at in the pagination                   |
| limit         | int    | Optional. The number of tasks to be returned per page of pagination. Defaults to 100 |
| variant       | string | Optional. Only return tasks within this variant                                      |
| variant_regex | string | Optional. Only return tasks within variants that match this regex                    |
| task_name     | string | Optional. Only return tasks with this display name                                   |
| status        | string | Optional. Only return tasks with this status                                         |

##### Get A Single Task

    GET /tasks/<task_id>

Fetch a single task using its ID

| Name                 | Type | Description                                                             |
|----------------------|------|-------------------------------------------------------------------------|
| fetch_all_executions | any  | Optional. Fetches previous executions of the task if they are available |

##### Restart A Task

    POST /tasks/<task_id>/restart

Restarts the task of the given ID. Can only be performed if the task is
finished.

| Name        | Type    | Description                                                                                                                          |
|-------------|---------|--------------------------------------------------------------------------------------------------------------------------------------|
| failed_only | boolean | Optional. For a display task, restarts only failed execution tasks. When used with a non-display task, this parameter has no effect. |

##### Abort A Task

    POST /tasks/<task_id>/abort

Abort the task of the given ID. Can only be performed if the task is in
progress.

##### Change A Task's Execution Status

    PATCH /tasks/<task_id> 

    Change the current execution status of a task. Accepts a JSON body with the new task status to be set.

**Accepted Parameters**

| Name      | Type    | Description                                                              |
|-----------|---------|--------------------------------------------------------------------------|
| activated | boolean | The activation status of the task to be set to                           |
| priority  | int     | The priority of this task's execution. Limited to 100 for non-superusers |

For example, to set activate the task and set its status priority to
100, add the following JSON to the request body:

    {
      "activated": true,
      "priority": 100
    }

### Task Annotations

Task Annotations give users more context about task failures.

#### Objects

**Annotation**

| Name             | Type                   | Description                                                                                                      |
|------------------|------------------------|------------------------------------------------------------------------------------------------------------------|
| task_id          | string                 | Identifier of the task that this annotation is for                                                               |
| task_execution   | int                    | The number of the execution of the task that the annotation is for                                               |
| metadata         | map[string]interface{} | Structured data about the task. Since this is user-given json data, the structure can differ between annotations |
| note             | note_object            | Comment about the task failure                                                                                   |
| issues           | []issue_link           | Links to tickets definitely related                                                                              |
| suspected_issues | []issue_link           | Links to tickets possibly related                                                                                |
| metadata_links   | []metadata_link        | List of links associated with a task, to be displayed in the task metadata sidebar, currently limited to 1       |


**Note**

| Name    | Type          | Description                    |
|---------|---------------|--------------------------------|
| message | string        | Comment about the task failure |
| source  | source_object | The source of the note         |

**Source**

| Name      | Type   | Description                           |
|-----------|--------|---------------------------------------|
| author    | string | The author of the edit                |
| time      | time   | The time of the edit                  |
| requester | string | The source of the request (api or ui) |

**Issue Link**

| Name             | Type          | Description                       |
|------------------|---------------|-----------------------------------|
| url              | string        | The url of the ticket             |
| issue_key        | string        | Text to be displayed              |
| source           | source_object | The source of the edit            |
| confidence_score | float32       | The confidence score of the issue |

**Metadata Link**

| Name             | Type          | Description            |
|------------------|---------------|------------------------|
| url              | string        | The url of the link    |
| text             | string        | Text to be displayed   |
| source           | source_object | The source of the edit |

#### Endpoints

##### Fetch Task Annotations
    GET /tasks/<task_id>/annotations  

    Returns a list containing the latest annotation for the given task, or null if there are no annotations.  

| Name                 | Type    | Description                                                                                                                                  |
|----------------------|---------|----------------------------------------------------------------------------------------------------------------------------------------------|
| fetch_all_executions | boolean | Optional. Fetches annotations for all executions of the task if they are available                                                           |
| execution            | int     | Optional. The 0-based number corresponding to the execution of the task the annotation is associated with. Defaults to the latest execution. |

Create or Update a New Task Annotation

    PUT tasks/{task_id}/annotation

Creates a task annotation, or updates an existing task annotation,
overwriting any existing fields that are included in the update. The
annotation is created based on the annotation specified in the request
body. Task execution must be provided for this endpoint, either in the
request body or set as a url parameter. If no task_execution is
specified in the request body or in the url, a bad status error will be
returned. Note that usage of this endpoint requires that the requesting
user have security to modify task annotations. The user does not need to
specify the source, it will be added automatically. Example request
body:

**Parameters**

| Name      | Type | Description                                                                    |
|-----------|------|--------------------------------------------------------------------------------|
| execution | int  | Optional. Can be set in lieu of specifying task_execution in the request body. |

    {
      "task_id": "my_task_id",
      "task_execution": 4321,
      "note": {
         "message": "this is a note about my_task_id's failure",
      },
      "issues":[
          {
             "url": "https://link.com",
             "issue_key": "link-1234"
          },
      ]
    }

Create or Update a New Task Annotation By Appending

    PATCH tasks/{task_id}/annotation  

Creates a task annotation, or updates an existing task annotation, appending issues and suspected issues that are included in the update. A new annotation is created based if the annotation exists and if upsert is true. Task execution must be provided for this endpoint, either in the request body or set as a url parameter.  If no task_execution is specified in the request body or in the url, a bad status error will be returned. Note that usage of this endpoint requires that the requesting user have security to modify task annotations. The user does not need to specify the source, it will be added automatically. Example request body:    

| Name      | Type | Description                                                                               |
|-----------|------|-------------------------------------------------------------------------------------------|
| execution | int  | Optional. Can be set in lieu of specifying task_execution in the request body.            |
| upsert    | bool | Optional. Will create a new annotation if task annotation isn't found and upsert is true. |

    {
      "task_id": "my_task_id",
        "task_execution": 4321,
          "upsert": false,
          "issues":[
          {
                   "url": "https://link.com",
                            "issue_key": "link-1234"
                                  
          }
          ]
    }


Bulk Create or Update Many Task Annotations

    PATCH tasks/annotations

Creates many new task annotations, or updates the annotation if it
already exists. A list of updates to a task annotation is provided in
the request body, where each list item specifies a set of task id /
execution pairs, and an annotation update to apply to all tasks matching
that criteria. Note that usage of this endpoint requires that the
requesting user have security to modify task annotations. Example
request body:

    {
      "tasks_updates": [
       {
         "task_data": [{"task_id": "t1", "execution":3}],
         "annotation": {
           "note": {
             "message": "this is a note about my_task_id's failure"
           },
           "issues":[
            {
              "url": "https://link.com",
              "issue_key": "link-1234"
            }
           ]
         }
       },
       {
         "task_data": [{"task_id": "t2", "execution":0}, {"task_id": "t2", "execution":1}],
         "annotation": {
           "note": {
             "message": "this is a note about my_task_id's failure"
           },
           "issues":[
            {
              "url": "https://other-link.com",
              "issue_key": "link-4567"
            }
           ]
         }
       }]
    }

List Task Annotations By Build 

    GET /builds/<build_id>/annotations

Fetches the annotations for all the tasks in a build.

**Parameters**

| Name                 | Type    | Description                                                                        |
|----------------------|---------|------------------------------------------------------------------------------------|
| fetch_all_executions | boolean | Optional. Fetches annotations for all executions of the task if they are available |


List Task Annotations By Version 

    GET /versions/<version_id>/annotations

Fetches the annotations for all the tasks in a version.

**Parameters**

| Name                 | Type    | Description                                                                        |
|----------------------|---------|------------------------------------------------------------------------------------|
| fetch_all_executions | boolean | Optional. Fetches annotations for all executions of the task if they are available |


Send a Newly Created Ticket For a Task 

    PUT /tasks/<task_id>/created_ticket 

If a [file ticket webhook](../../03-Apply and Analyze Evergreen Data/01-Webhooks.md#task-annotations-file-ticket-webhook)
is configured for a project, this endpoint should be used to let
evergreen know when a ticket was filed for a task so that it can be
stored and displayed to the user. The request body should include the
ticket url and issue_key. Note that usage of this endpoint requires that
the requesting user have security to modify task annotations. The user
does not need to specify the source of the ticket, it will be added
automatically. Example request body:

    {
        "url": "https://link.com",
        "issue_key": "link-1234"
     }

### Test

A test is a sub-operation of a task performed by Evergreen.

#### Objects

**Test**

| Name       | Type     | Description                                                |
|------------|----------|------------------------------------------------------------|
| task_id    | string   | Identifier of the task this test is a part of              |
| Status     | string   | Execution status of the test                               |
| test_file  | string   | Name of the test file that this test was run in            |
| logs       | test_log | Object containing information about the logs for this test |
| exit_code  | int      | The exit code of the process that ran this test            |
| start_time | time     | Time that this test began execution                        |
| end_time   | time     | Time that this test stopped execution                      |

**Test Logs**

| Name     | Type   | Description                                                              |
|----------|--------|--------------------------------------------------------------------------|
| url      | string | URL where the log can be fetched                                         |
| line_num | int    | Line number in the log file corresponding to information about this test |
| url_raw  | string | URL of the unprocessed version of the logs file for this test            |
| log_id   | string | Identifier of the logs corresponding to this test                        |

#### Endpoints

##### Get Tests From A Task

    GET /tasks/<task_id>/tests

Fetches a paginated list of tests that ran as part of the given task. To
filter the tasks, add the following parameters into the query string
(reference [Pagination](../05-Use the API/01-REST-V2-Usage.md#pagination)
to see this format).

**Parameters**

| Name      | Type   | Description                                                                                                                      |
|-----------|--------|----------------------------------------------------------------------------------------------------------------------------------|
| start_at  | string | Optional. The identifier of the test to start at in the pagination                                                               |
| limit     | int    | Optional. The number of tests to be returned per page of pagination. Defaults to 100                                             |
| status    | string | Optional. A status of test to limit the results to.                                                                              |
| execution | int    | Optional. The 0-based number corresponding to the execution of the task. Defaults to 0, meaning the first time the task was run. |
| test_name | string | Optional. Only return the test matching the name.                                                                                |
| latest    | bool   | Optional. Return tests from the latest execution. Cannot be used with execution.                                                 |


##### Get The Test Count From A Task

    GET /tasks/<task_id>/tests/count

Returns an integer representing the number of tests that ran as part of
the given task.

**Parameters**

| Name      | Type | Description                                                                                                                      |
|-----------|------|----------------------------------------------------------------------------------------------------------------------------------|
| execution | int  | Optional. The 0-based number corresponding to the execution of the task. Defaults to 0, meaning the first time the task was run. |


### Manifest

A manifest is a representation of the modules associated with a version.

#### Objects

**Manifest**

| Name     | Type                | Description                                                      |
|----------|---------------------|------------------------------------------------------------------|
| \_id     | string              | Identifier for the version.                                      |
| revision | string              | The revision of the version.                                     |
| project  | string              | The project identifier for the version.                          |
| branch   | string              | The branch of the repository.                                    |
| modules  | map[string]\*Module | Map from the Github repository name to the module's information. |
| is_base  | bool                | True if the version is a mainline build.                         |

**Module**

| Name     | Type   | Description                                             |
|----------|--------|---------------------------------------------------------|
| repo     | string | The name of the repository.                             |
| branch   | string | The branch of the repository.                           |
| revision | string | The revision of the head of the branch.                 |
| owner    | string | The owner of the repository.                            |
| url      | string | The url to the GitHub API call to that specific commit. |


#### Endpoints

##### Get Manifest for Task

    GET /tasks/<task_id>/manifest

Fetch the manifest for a task using the task ID.

### Host

The hosts resource defines a running machine instance in Evergreen.

#### Objects

**Host**

| Name         | Type        | Description                                                                          |
|--------------|-------------|--------------------------------------------------------------------------------------|
| host_id      | string      | Unique identifier of a specific host                                                 |
| distro       | distro_info | Object containing information about the distro type of this host                     |
| started_by   | string      | Name of the process or user that started this host                                   |
| host_type    | string      | The instance type requested for the provider, primarily used for ec2 dynamic hosts   |
| user         | string      | The user associated with this host. Set if this host was spawned for a specific user |
| status       | string      | The current state of the host                                                        |
| running_task | task_info   | Object containing information about the task the host is currently running           |

**Distro Info**

| Name      | Type   | Description                                                                               |
|-----------|--------|-------------------------------------------------------------------------------------------|
| distro_id | string | Unique Identifier of this distro. Can be used to fetch more informaiton about this distro |
| provider  | string | The service which provides this type of machine                                           |

**Task Info**

| Name          | Type   | Description                                                                           |
|---------------|--------|---------------------------------------------------------------------------------------|
| task_id       | string | Unique Identifier of this task. Can be used to fetch more informaiton about this task |
| name          | string | The name of this task                                                                 |
| dispatch_time | time   | Time that this task was dispatched to this host                                       |
| version_id    | string | Unique identifier for the version of the project that this task is run as part of     |
| build_id      | string | Unique identifier for the build of the project that this task is run as part of       |


#### Endpoints

##### Fetch All Hosts

    GET /hosts

Returns a paginated list of all hosts in Evergreen

**Parameters**

| Name     | Type   | Description                                                                          |
|----------|--------|--------------------------------------------------------------------------------------|
| start_at | string | Optional. The identifier of the host to start at in the pagination                   |
| limit    | int    | Optional. The number of hosts to be returned per page of pagination. Defaults to 100 |
| status   | string | Optional. A status of host to limit the results to                                   |


##### Fetch Hosts Spawned By User

    GET /users/<user_id>/hosts

Returns a list of hosts spawned by the given user.

**Parameters**

| Name     | Type   | Description                                                                          |
|----------|--------|--------------------------------------------------------------------------------------|
| start_at | string | Optional. The identifier of the host to start at in the pagination                   |
| limit    | int    | Optional. The number of hosts to be returned per page of pagination. Defaults to 100 |
| status   | string | Optional. A status of host to limit the results to                                   |


##### Fetch Host By ID

    GET /hosts/<host_id>

Fetches a single host using its ID

##### Spawn a Host

    POST /hosts

Spawns a host. The host must be of a distro which is spawnable by users
(see [Distro](#distro)).

**Parameters**

| Name      | Type   | Description                     |
|-----------|--------|---------------------------------|
| `distro`  | string | [Distro](#distro) name to spawn |
| `keyname` | string | [Key](#key) name to use         |


##### Terminate Host with Given Host ID

    POST /hosts/<host_id>/terminate

Immediately terminate a single host with given ID. Users may only
terminate hosts which were created by them, unless the user is a
super-user.

Hosts which have not been initialised yet will be marked as Terminated.

Trying to terminate a host which has already been terminated will result
in an error.

All other host statuses will result in an attempt to terminate using the
provider's API

A response code of 200 OK indicates that the host was successfully
terminated

All other response codes indicate errors; the response body can be
parsed as a rest.APIError

##### Change RDP Password of Host with Given Host ID

    POST /hosts/<host_id>/change_password

Immediately changes the RDP password of a Windows host with a given ID.
Users may only change passwords for hosts which were created by them,
unless the user is a super-user.

A response code of 200 OK indicates that the host's password was
successfully terminated

Attempting to set the RDP password of a host that is not a Windows host
or host that is not running will result in an error.

All other response codes indicate errors; the response body can be
parsed as a rest.APIError

**Change Password**

| Name    | Type   | Description                                                                                                                                                                                  |
|---------|--------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| rdp_pwd | string | New RDP password; must meet RDP password criteria as provided by Microsoft at: <https://technet.microsoft.com/en-us/library/cc786468(v=ws.10>).aspx and be between 6 and 255 characters long |

##### Extend the Expiration of Host with Given Host ID

    POST /hosts/<host_id>/extend_expiration

Extend the expiration time of a host with a given ID. Users may only
extend expirations for hosts which were created by them, unless the user
is a super-user

The expiration date of a host may not be more than 1 week in the future.

A response code of 200 OK indicates that the host's expiration was
successfully extended.

Attempt to extend the expiration time of a terminated host will result
in an error

All other response codes indicate errors; the response body can be
parsed as a rest.APIError

**Extend Expiration**

| Name      | Type | Description                                             |
|-----------|------|---------------------------------------------------------|
| add_hours | int  | Number of hours to extend expiration; not to exceed 168 |


### Patch

A patch is a manually initiated version submitted to test local changes.

#### Objects

**Patch**

| Name                  | Type           | Description                                                                                                                          |
|-----------------------|----------------|--------------------------------------------------------------------------------------------------------------------------------------|
| patch_id              | string         | Unique identifier of a specific patch                                                                                                |
| description           | string         | Description of the patch                                                                                                             |
| project_id            | string         | Name of the project                                                                                                                  |
| branch                | string         | The branch on which the patch was initiated                                                                                          |
| git_hash              | string         | Hash of commit off which the patch was initiated                                                                                     |
| patch_number          | int            | Incrementing counter of user's patches                                                                                               |
| author                | string         | Author of the patch                                                                                                                  |
| status                | string         | Status of patch                                                                                                                      |
| commit_queue_position | int            | Only populated for commit queue patches: returns the 0-indexed position of the patch on the queue, or -1 if not on the queue anymore |
| create_time           | time           | Time patch was created                                                                                                               |
| start_time            | time           | Time patch started to run                                                                                                            |
| finish_time           | time           | Time at patch completion                                                                                                             |
| build_variants        | string[]       | List of identifiers of builds to run for this patch                                                                                  |
| tasks                 | string[]       | List of identifiers of tasks used in this patch                                                                                      |
| variants_tasks        | variant_task[] | List of documents of available tasks and associated build variant                                                                    |
| activated             | bool           | Whether the patch has been finalized and activated                                                                                   |

**Variant Task**

| Name  | Type       | Description                                      |
|-------|------------|--------------------------------------------------|
| name  | string     | Name of build variant                            |
| tasks | string[] | All tasks available to run on this build variant |


#### Endpoints

##### Fetch Patches By Project

    GET /projects/<project_id>/patches

Returns a paginated list of all patches associated with a specific
project

**Parameters**

| Name     | Type   | Description                                                                            |
|----------|--------|----------------------------------------------------------------------------------------|
| start_at | string | Optional. The create_time of the patch to start at in the pagination. Defaults to now  |
| limit    | int    | Optional. The number of patches to be returned per page of pagination. Defaults to 100 |


##### Fetch Patches By User

    GET /users/<user_id>/patches

Returns a paginated list of all patches associated with a specific user

**Parameters**

| Name     | Type   | Description                                                                            |
|----------|--------|----------------------------------------------------------------------------------------|
| start_at | string | Optional. The create_time of the patch to start at in the pagination. Defaults to now  |
| limit    | int    | Optional. The number of patches to be returned per page of pagination. Defaults to 100 |


##### Fetch Patch By Id

    GET /projects/<project_id>/patches

Fetch a single patch using its ID

##### Get Patch Diff

    GET /patches/<patch_id>/raw

Fetch the raw diff for a patch

**Parameters**

| Name   | Type   | Description                                                                                           |
|--------|--------|-------------------------------------------------------------------------------------------------------|
| module | string | Optional. A module to get the diff for. Returns the empty string when no patch exists for the module. |

##### Abort a Patch

    POST /patches/<patch_id>/abort

Aborts a single patch using its ID and returns the patch

##### Configure/Schedule a Patch

    POST /patches/<patch_id>/configure  

Update the list of tasks that the specified patch will run. This works both for initially specifying a patch's tasks, as well as for adding additional tasks to an already-scheduled patch.  The request body should be in the following format: 

    {
      "description": "this is my patch",
          "variants": [
          {
                 "id": "variant-1",
                        "tasks": ["task1", task2"]
                            },
          {
                 "id": "variant-2",
                        "tasks": ["task2", task3"]
                            }
                              ]
                              }"]
          }"]
          }
          ]
    }

| Name        | Type                     | Description                                                           |
|-------------|--------------------------|-----------------------------------------------------------------------|
| description | string                   | Optional, if sent will update the patch's description                 |
| variants    | array of variant objects | Required, these are the variants and tasks that the patch should run. |

Each variant object is of the format { "variant": "\<variant name\>", "tasks": ["task name"] }. This field is analogous in syntax and usage to the "buildvariants" field in the project's evergreen.yml file. Names of display tasks can be specified in the tasks array and will work as one would expect. For an already-scheduled patch, any new tasks in this array will be created, and any existing tasks not in this array will be unscheduled.  

##### Restart a Patch

    POST /patches/<patch_id>/restart

Restarts a single patch using its ID then returns the patch

##### Change Patch Status

    PATCH /patches/<patch_id>

Sets the priority and activation status of a single patch to the input
values

**Parameters**

| Name      | Type | Description                                         |
|-----------|------|-----------------------------------------------------|
| priority  | int  | Optional. The priority to set the patch to          |
| activated | bool | Optional. The activation status to set the patch to |


### Build

The build resource represents the combination of a version and a
buildvariant.

#### Objects

**Build**

| Name                    | Type     | Description                                                                                                                                                                                                                                                                      |
|-------------------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `project_id`            | string   | The identifier of the project this build represents                                                                                                                                                                                                                              |
| `create_time`           | time     | Time at which build was created                                                                                                                                                                                                                                                  |
| `start_time`            | time     | Time at which build started running tasks                                                                                                                                                                                                                                        |
| `finish_time`           | time     | Time at which build finished running all tasks                                                                                                                                                                                                                                   |
| `version`               | string   | The version this build is running tasks for                                                                                                                                                                                                                                      |
| `branch`                | string   | The branch of project the build is running                                                                                                                                                                                                                                       |
| `gitspec`               | string   | Hash of the revision on which this build is running                                                                                                                                                                                                                              |
| `build_variant`         | string   | Build distro and architecture information                                                                                                                                                                                                                                        |
| `status`                | string   | The status of the build                                                                                                                                                                                                                                                          |
| `tags`                  | []string | List of tags defined for the build variant, if any                                                                                                                                                                                                                               |
| `activated`             | bool     | Whether this build was manually initiated                                                                                                                                                                                                                                        |
| `activated_by`          | string   | Who initiated the build                                                                                                                                                                                                                                                          |
| `activated_time`        | time     | When the build was initiated                                                                                                                                                                                                                                                     |
| `order`                 | int      | Incrementing counter of project's builds                                                                                                                                                                                                                                         |
| `tasks`                 | []string | The tasks to be run on this build                                                                                                                                                                                                                                                |
| `time_taken_ms`         | int      | How long the build took to complete all tasks                                                                                                                                                                                                                                    |
| `display_name`          | string   | Displayed title of the build showing version and variant running                                                                                                                                                                                                                 |
| `predicted_makespan_ms` | int      | Predicted makespan by the scheduler prior to execution                                                                                                                                                                                                                           |
| `actual_makespan_ms`    | int      | Actual makespan measured during execution                                                                                                                                                                                                                                        |
| `origin`                | string   | The source of the patch, a commit or a patch                                                                                                                                                                                                                                     |
| `status_counts`         | Object   | Contains aggregated data about the statuses of tasks in this build. The keys of this object are statuses and the values are the number of tasks within this build in that status. Note that this field provides data that you can get yourself by querying tasks for this build. |
| `task_cache`            | Object   | Contains a subset of information about tasks for the build; this is not provided/accurate for most routes ([get versions for project](../05-Use the API/01-REST-V2-Usage.md#get-versions-for-a-project) is an exception).                                 |
| `definition_info`       | Object   | Some routes will return information about the variant as defined in the project. Does not expand expansions; they will be returned as written in the project yaml (i.e. `${syntax}`)                                                                                             |


**Definition Info**

| Name      | Type   | Description                                                                                                                                                      |
|-----------|--------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| cron      | string | The cron defined for the variant, if provided, as defined [here](../01-Configure a Project/01-Project-Configuration-Files.md#build-variants)      |
| batchtime | int    | The batchtime defined for the variant, if provided, as defined [here](../01-Configure a Project/01-Project-Configuration-Files.md#build-variants) |


#### Endpoints

##### Fetch Build By Id

    GET /builds/<build_id>

Fetches a single build using its ID

##### Abort a Build

    POST /builds/<build_id>/abort

Aborts a single build using its ID then returns the build

##### Restart a Build

    POST /builds/<build_id>/restart

Restarts a single build using its ID then returns the build

##### Change Build Status

    PATCH /builds/<build_id>

Sets the priority and activation status of a single build to the input
values

**Parameters**

| Name      | Type | Description                                                           |
|-----------|------|-----------------------------------------------------------------------|
| priority  | int  | Optional. The priority to set the build to                            |
| activated | bool | Optional. Set to true to activate, and false to deactivate the build. |


### Version

A version is a commit in a project.

#### Objects

**Version**

| Name                    | Type            | Description                                                                                                                                                                                                                                              |
|-------------------------|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `create_time`           | time            | Time that the version was first created                                                                                                                                                                                                                  |
| `start_time`            | time            | Time at which tasks associated with this version started running                                                                                                                                                                                         |
| `finish_time`           | time            | Time at which tasks associated with this version finished running                                                                                                                                                                                        |
| `revision`              | string          | The version control identifier                                                                                                                                                                                                                           |
| `author`                | string          | Author of the version                                                                                                                                                                                                                                    |
| `author_email`          | string          | Email of the author of the version                                                                                                                                                                                                                       |
| `message`               | string          | Message left with the commit                                                                                                                                                                                                                             |
| `status`                | string          | The status of the version                                                                                                                                                                                                                                |
| `repo`                  | string          | The github repository where the commit was made                                                                                                                                                                                                          |
| `branch`                | string          | The version control branch where the commit was made                                                                                                                                                                                                     |
| `build_variants_status` | []buildDetail   | List of documents of the associated build variant and the build id                                                                                                                                                                                       |
| `requester`             | string          | Version created by one of "patch_request", "github_pull_request", "gitter_request" (caused by git commit, aka the repotracker requester), "trigger_request" (Project Trigger versions) , "merge_test" (commit queue patches), "ad_hoc" (periodic builds) |
| `activated`             | boolean or null | Will be null for versions created before this field was added.                                                                                                                                                                                           |


#### Endpoints

##### Fetch Version By Id

    GET /versions/<version_id>

Fetches a single version using its ID

##### Abort a Version

    POST /versions/<version_id>/abort

Aborts a single version using its ID then returns the version

##### Restart a Version

    POST /versions/<version_id>/restart

Restarts a single version using its ID then returns the version

##### Activate or Deactivate a Version

    PATCH /versions/<version_id>

Activate or deactivates a given version. Does not return the version.

**Parameters**

| Name      | Type | Description                                                          |
|--------------------|---------|-------------------------------------------|
| activated | bool | Required. Will activate the version if true and deactivate if false. |


##### Get Builds From A Version

    GET /versions/<version_id>/builds

Fetches a list of builds associated with a version

**Parameters**

| Name    | Type   | Description                                                                  |
|---------|--------|------------------------------------------------------------------------------|
| variant | string | Optional. Only return the build with this variant (using Distro identifier). |


Returns a list of
[Builds](../05-Use the API/01-REST-V2-Usage.md#build).

##### Create a New Version

    PUT /versions

Creates a version and optionally runs it, conceptually similar to a
patch. The main difference is that the config yml file is provided in
the request, rather than retrieved from the repo.

**Parameters**

| Name       | Type         | Description                                                                                                                                                                         |
|------------|--------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| project_id | string       | Required. This is the project with which the version will be associated, and the code to test will be checked out from the project's branch.                                        |
| message    | string       | Optional. A description of the version which will be displayed in the UI                                                                                                            |
| activate   | boolean      | Optional. If true, the defined tasks will run immediately. Otherwise, the version will be created and can be activated in the UI                                                    |
| is_adhoc   | boolean      | Optional. If true, the version will be indicated as coming from an ad hoc source and will not display as if it were a patch or commit. If false, it will be assumed to be a commit. |
| config     | string (yml) | Required. This is the yml config that will be used for defining tasks, variants, and functions.                                                                                     |


Returns the version object that was created

### Project

A project corresponds to a single repository.
Most of these project fields are accessible to all users via the /projects route, with the
exception of project variables, task annotation settings, workstation settings, and container secrets.

#### Objects

**Project**

| Name                 | Type                | Description                                                                                                        |
|----------------------|---------------------|--------------------------------------------------------------------------------------------------------------------|
| admins               | []string or null    | Usernames of project admins. Can be null for some projects ([EVG-6598](https://jira.mongodb.org/browse/EVG-6598)). |
| delete_admins        | []string            | Usernames of project admins to remove                                                                              |
| batch_time           | int                 | Unique identifier of a specific patch                                                                              |
| branch_name          | string              | Name of branch                                                                                                     |
| commit_queue         | CommitQueueParams   | Options for commit queue                                                                                           |
| deactivate_previous  | bool                | List of identifiers of tasks used in this patch                                                                    |
| display_name         | string              | Project name displayed to users                                                                                    |
| enabled              | bool                | Whether evergreen is enabled for this project                                                                      |
| identifier           | string              | Internal evergreen identifier for project                                                                          |
| notify_on_failure    | bool                | Notify original committer (or admins) when build fails                                                             |
| owner_name           | string              | Owner of project repository                                                                                        |
| patching_disabled    | bool                | Disable patching                                                                                                   |
| pr_testing_enabled   | bool                | Enable github pull request testing                                                                                 |
| private              | bool                | A user must be logged in to view private projects                                                                  |
| remote_path          | string              | Path to config file in repo                                                                                        |
| repo_name            | string              | Repository name                                                                                                    |
| tracks_push_events   | bool                | If true, repotracker is run on github push events. If false, repotracker is run periodically every few minutes.    |
| revision             | string              | Only used when modifying projects to change the base revision and run the repotracker.                             |
| triggers             | []TriggerDefinition | a list of triggers for the project                                                                                 |
| aliases              | []APIProjectAlias   | a list of aliases for the project                                                                                  |
| variables            | ProjectVars         | project variables information                                                                                      |
| subscriptions        | []Subscription      | a list of subscriptions for the project                                                                            |
| delete_subscriptions | []string            | subscription IDs. Will delete these subscriptions when given.                                                      |


**CommitQueueParams**

| Name         | Type   | Description                               |
|--------------|--------|-------------------------------------------|
| enabled      | bool   | Enable/disable the commit queue           |
| merge_method | string | method of merging (squash, merge, rebase) |
| patch_type   | string | type of patch (PR, CLI)                   |


**TriggersDefinition**

| Name          | Type   | Description                               |
|---------------|--------|-------------------------------------------|
| definition_id | string | unique ID                                 |
| project       | string | project ID                                |
| level         | string | build or task                             |
| variant_regex | string | matching variants will trigger a build    |
| task_regex    | string | matching tasks will trigger a build       |
| status        | string | status to trigger for (or "\*")           |
| config_file   | string | definition file                           |
| command       | string | shell command that creates task json file |


**ProjectAlias**

| Name    | Type     | Description                                                                              |
|---------|----------|------------------------------------------------------------------------------------------|
| \_id    | string   | The id for the alias. If the alias should be deleted, this must be given.                |
| alias   | string   | Required. Alias to use with the CLI. May be specified multiple times.                    |
| variant | string   | Required. Variant regex for alias.                                                       |
| task    | string   | Task regex for alias. Will use the union of task and tags. Either task or tags required. |
| tags    | []string | Tags for alias. Will use the union of task and tags. Either task or tags required.       |
| delete  | bool     | If the given alias for the project should be deleted, set this to true.                  |


**ProjectVars**

| Name            | Type              | Description                                                                                                                                               |
|-----------------|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| vars            | map[string]string | Map of the variable to its value (if private, value is not shown)                                                                                         |
| private_vars    | map[string]bool   | Indicates whether that variable should be private, i.e. the value will not be shown (NOTE: once a variable has been set to private this cannot be undone) |
| restricted_vars | map[string]bool   | Indicates whether that variable should be restricted, i.e. only used by commands that are guaranteed to not leak the values (currently s3.put and s3.get) |
| vars_to_delete  | []string          | Only used to remove existing variables.                                                                                                                   |


**Subscription**

| Name           | Type              | Description                                 |
|----------------|-------------------|---------------------------------------------|
| id             | string            |                                             |
| resource_type  | string            |                                             |
| trigger        | string            |                                             |
| selectors      | []Selector        |                                             |
| regex_selector | []Selector        |                                             |
| subscriber     | Subscriber        |                                             |
| owner_type     | string            | For projects, this will always be "project" |
| owner          | string            | The project ID                              |
| trigger_data   | map[string]string |                                             |


**Selector**

| Name | Type   |
|------|--------|
| type | string |
| data | string |


**Subscriber**

| Name   | Type        |
|--------|-------------|
| type   | string      |
| target | interface{} |


#### Endpoints

##### Fetch all Projects

    GET /projects

Returns a paginated list of all projects. Any authenticated user can
access this endpoint, so potentially sensitive information (variables, task 
annotation settings, workstation settings, and container secrets) is omitted.

**Parameters**

| Name     | Type   | Description                                                                             |
|----------|--------|-----------------------------------------------------------------------------------------|
| start_at | string | Optional. The id of the project to start at in the pagination. Defaults to empty string |
| limit    | int    | Optional. The number of projects to be returned per page of pagination. Defaults to 100 |


##### Get A Project

    GET /projects/<project_id>

Returns the project (restricted to project admins). Includes public
variables, aliases, and subscriptions. Note that private variables are
*always redacted.* If you want to use this to copy project variables,
see instead the "Copy Project Variables" route.

**Parameters**

| Name                 | Type | Description                                                                                                                                 |
|----------------------|------|---------------------------------------------------------------------------------------------------------------------------------------------|
| includeRepo          | bool | Optional. Setting to true will return the merged result of project and repo level settings. Defaults to false                               |
| includeProjectConfig | bool | Optional. Setting to true will return the merged result of the project and the config properties set in the project YAML. Defaults to false |


##### Modify A Project

    PATCH /projects/<project_id>

Modify an existing project (restricted to project admins). Will enable webhooks 
if an enabled project, and enable PR testing and the commit queue if specified. 

For lists, if there is a complementary
"delete" field, then the former field indicates items to be added,
while the "delete" field indicates items to be deleted. Otherwise, the
given list will overwrite the original list (the only exception is for project
variables -- we will ignore any empty project variables to avoid accidentally 
overwriting private variables).

##### Copy a Project 

    POST /projects/<project_id>/copy?new_project=<new_project_id>

Restricted to admins of the original project. Create a new project that
is identical to indicated project\--this project is initially disabled
(PR testing and CommitQueue also initially disabled). The unique
identifier is passed to the query parameter `new_project` and is
required.

Project variables, aliases, and subscriptions also copied. Returns the
new project (but not variables/aliases/subscriptions).

##### Copy Variables to an Existing Project

    POST /projects/<source_project>/copy/variables

Restricted to admins of the source project/repo and the destination
project/repo. Copies variables from projectA to projectB.

**CopyVariablesOptions**

| Name            | Type   | Description                                                                                                                       |
|-----------------|--------|-----------------------------------------------------------------------------------------------------------------------------------|
| copy_to         | string | Required. ProjectID to copy `source_project` variables to.                                                                        |
| include_private | bool   | If set to true, private variables will also be copied.                                                                            |
| overwrite       | bool   | If set to true, will remove variables from the `copy_to` project that are not in `source_project`.                                |
| dry_run         | bool   | If set to true, route returns the variables from `source_project` that will be copied. (If private, the values will be redacted.) |


If `dry_run` is set, then the route does not complete the copy, but
returns OK if no project variables in the source project will be
overwritten (this concerns [all]{.title-ref} variables in the
destination project, but only redacted variables in the source project).
Otherwise, an error is given which includes the project variable keys
that overlap.

if `dry_run` is not set, the copy is completed, and variables could be
overwritten.

##### Get Versions For A Project

    GET /projects/<project_id>/versions

Returns a paginated list of recent versions for a project. Parameters
should be passed into the JSON body (the route still accepts limit and
start as query parameters to support legacy behavior).

**Parameters**

| Name             | Type   | Description                                                                                                                                                                                                                                                                                                  |
|------------------|--------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| skip             | int    | Optional. Number of versions to skip.                                                                                                                                                                                                                                                                        |
| limit            | int    | Optional. The number of versions to be returned per page of pagination. Defaults to 20.                                                                                                                                                                                                                      |
| revision_start   | int    | Optional. The version order number to start at, for pagination.                                                                                                                                                                                                                                              |
| revision_end     | int    | Optional. The version order number to end at, for pagination.                                                                                                                                                                                                                                                |
| start_time_str   | string | Optional. Timestamp to start looking for applicable versions.                                                                                                                                                                                                                                                |
| end_time_str     | string | Optional. Timestamp to stop looking for applicable versions.                                                                                                                                                                                                                                                 |
| requester        | string | Returns versions for this requester only. Defaults to `gitter_request` (caused by git commit, aka the repotracker requester). Can also be set to `patch_request`, `github_pull_request`, `trigger_request` (Project Trigger versions) , `merge_test` (commit queue patches), and `ad_hoc` (periodic builds). |
| include_builds   | bool   | If set, will return some information for each build in the version.                                                                                                                                                                                                                                          |
| by_build_variant | string | If set, will only include information for this build, and only return versions with this build activated. Must have `include_builds` set.                                                                                                                                                                    |
| include_tasks    | bool   | If set, will return some information for each task in the included builds. This is only allowed if `include_builds` is set.                                                                                                                                                                                  |
| by_task          | string | If set, will only include information for this task, and will only return versions with this task activated. Must have `include_tasks` set.                                                                                                                                                                  |


**Response**

| Name                    | Type          | Description                                                                                                                          |
|-------------------------|---------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `create_time`           | time          | Time that the version was first created                                                                                              |
| `start_time`            | time          | Time at which tasks associated with this version started running                                                                     |
| `finish_time`           | time          | Time at which tasks associated with this version finished running                                                                    |
| `revision`              | string        | The version control identifier                                                                                                       |
| `author`                | string        | Author of the version                                                                                                                |
| `message`               | string        | Message left with the commit                                                                                                         |
| `status`                | string        | The status of the version                                                                                                            |
| `errors`                | []string      | List of errors creating the version                                                                                                  |
| `build_variants_status` | []buildDetail | List of documents of the associated build variant and the build id (this won't be populated if include_builds is set)                |
| `builds`                | []APIBuild    | List of builds for the version (only populated if include_builds is set). If include_tasks is set, then the task_cache is populated. |


##### Modify Versions For A Project

    PATCH /projects/<project_id>/versions

Modifies a group of versions for a project. Parameters
should be passed into the JSON body. Currently supports
setting priority for all versions that the given options apply to.
This route is restricted to project admins.

**Parameters**

| Name             | Type   | Description                                                                                                                                                                                                                                                                                                  |
|------------------|--------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| start_time_str   | string | Timestamp to start looking for applicable versions.                                                                                                                                                                                                                                                          |
| end_time_str     | string | Optional. Timestamp to stop looking for applicable versions.                                                                                                                                                                                                                                                 |
| revision_start   | int    | Optional. The version order number to start at.                                                                                                                                                                                                                                                              |
| revision_end     | int    | Optional. The version order number to end at.                                                                                                                                                                                                                                                                |
| priority         | int    | Priority to set for all tasks within applicable versions.                                                                                                                                                                                                                                                    |
| limit            | int    | Optional. The number of versions to be returned per page of pagination. Defaults to 20.                                                                                                                                                                                                                      |
| requester        | string | Returns versions for this requester only. Defaults to `gitter_request` (caused by git commit, aka the repotracker requester). Can also be set to `patch_request`, `github_pull_request`, `trigger_request` (Project Trigger versions) , `merge_test` (commit queue patches), and `ad_hoc` (periodic builds). |
| by_build_variant | string | If set, will only include information for this build, and only return versions with this build activated. Must have `include_builds` set.                                                                                                                                                                    |
| by_task          | string | If set, will only include information for this task, and will only return versions with this task activated. Must have `include_tasks` set.                                                                                                                                                                  |
| skip             | int    | Optional. Number of versions to skip.                                                                                                                                                                                                                                                                        |


##### Get Tasks For A Project

    GET /projects/<project_id>/tasks/<task_name>

Returns the last set number of completed tasks that exist for a given
project. Parameters should be passed into the JSON body. Ensure that a
task name rather than a task ID is passed into the URL.

**Parameters**

| Name          | Type   | Description                                                             |
|---------------|--------|-------------------------------------------------------------------------|
| num_versions  | int    | Optional. The number of latest versions to be searched. Defaults to 20. |
| start_at      | int    | Optional. The version order number to start returning results after.    |
| build_variant | string | If set, will only include tasks that have run on this build variant.    |

##### Get Execution Info for a Task

    GET /projects/<project_id>/task_executions

Right now, this returns the number of times the given task has executed (i.e. succeeded or failed). 
Parameters should be passed into the JSON body. 

**Parameters**

| Name          | Type     | Description                                                                                                                                                                                                                                        |
|---------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| task_name     | string   | Required. The task to return execution info for.                                                                                                                                                                                                   |
| build_variant | string   | Required. The build variant to return task execution info for.                                                                                                                                                                                     |
| start_time    | Time     | Required. Will only return execution info after this time. Format should be 2022-12-01T12:30:00.000Z                                                                                                                                               |
| end_time      | Time.    | Optional. If not provided, will default to the current time.                                                                                                                                                                                       |
| requesters    | []string | Optional. If not provided, will default to `gitter_request` (versions created by git commit). Can also be `github_pull_request`, `trigger_request` (Project Trigger versions) , `merge_test` (commit queue patches), or `ad_hoc` (periodic builds) |

**Response**

| Name          | Type | Description                                                                             |
|---------------|------|-----------------------------------------------------------------------------------------|
| num_completed | int  | The number of completed executions for the task/variant pair within the given interval. |


##### Rotate Variables

    PUT /projects/variables/rotate

Restricted to superusers due to the fact it modifies ALL projects.

**RotateVariablesOptions**

| Name        | Type   | Description                                                       |
|-------------|--------|-------------------------------------------------------------------|
| to_replace  | string | Required. Variable value to search and replace.                   |
| replacement | string | Required. Value to replace the variables that match `to_replace`. |
| dry_run     | bool   | If set to true, we don't complete the update                      |


If `dry_run` is set, the route doesn't update but returns a map of
`projectId` to a list of keys that will be replaced.

if `dry_run` is not set, route returns a map of `projectId` to a list of
keys that were replaced.

Get Recent Versions For A Project (legacy) 

    GET /projects/<project_id>/recent_versions

Returns a paginated list of recent versions for a project. NOTE that
this route is legacy, and is no longer supported.

**Parameters**

| Name   | Type | Description                                                                            |
|--------|------|----------------------------------------------------------------------------------------|
| offset | int  | Optional. Zero-based offset to return results from.                                    |
| limit  | int  | Optional. The number of versions to be returned per page of pagination. Defaults to 10 |


**Response**

| Name           | Type                         | Description                                                                                                                                                                                                  |
|----------------|------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| rows           | object                       | The keys of this object are build variant identifiers. The values are BuildList objects from below. These are the builds contained in the recent versions, but grouped by build variant rather than version. |
| versions       | Array of APIVersions objects | This array contains the recent versions for the requested project, in reverse chronological order.                                                                                                           |
| build_variants | Array of strings             | The deduplicated display names for all the build variants in the rows parameter                                                                                                                              |


#### Objects

**BuildList**

| Name                 | Type      | Description                                                                                                                                       |
| -------------------- | --------- | -------------------------------------------                                                                                                       |
| `build_variant`      | string    | the identifier of each of the build variant objects below (all are the same variant)                                                              |
| `builds`             | object    | The keys of this object are build IDs. The values are the [full build objects](../05-Use the API/01-REST-V2-Usage.md#id12) |


**APIVersions**

| Name        | Type                     | Description                                                                                                                                                                       |
|-------------|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `rolled_up` | boolean                  | if true, these are inactive versions                                                                                                                                              |
| `versions`  | Array of Version objects | If rolled_up is true, this will contain multiple version objects, none of which ran any tasks. Otherwise, this will contain a single version object, of which at least 1 task ran |


Get Current Parameters For a Project 

    GET /projects/<project_id>/parameters

Returns a list of parameters for the project.

**Parameter**

| Name  | Type   | Description                         |
|-------|--------|-------------------------------------|
| key   | string | The name of the parameter.          |
| value | string | The default value of the parameter. |


##### Put A Project

    PUT /projects/<project_id>

Create a new project with the given project ID. Restricted to super
users.

##### Check Project Alias Results

    GET /projects/test_alias?version=<version_id>&alias=<alias>&include_deps=<true|false>  

Checks a specified project alias in a specified project against an Evergreen configuration, returning the tasks and variants that alias would select. Currently only supports passing in the configuration via an already-created version. 

**Parameters**

| Name         | Type    | Description
|--------------|---------|
| version      | string  | Required. The ID of the version (commit or patch) from which to retrieve the configuration as well as the project ID
| alias        | string  | Required. The name of the alias to test against the configuration. The special aliases \__commit_queue, \__github, and \__git_tag can be used here
| include_deps | boolean | Optional. If true, will also select the tasks that are dependencies of the selected tasks, even if they do not match the alias definition. Defaults to false.


#### Distro 

A distro is an Evergreen host type. This isn't necessarily a Linux distribution - Mac and Windows host types are other possibilities.  

##### Objects

    GET /distros

Fetches distros defined in the system.

### Key

#### Objects

**Key**

| Name | Type   | Description                             |
|------|--------|-----------------------------------------|
| name | string | The unique name of the public key       |
| key  | string | The public key, (e.g: 'ssh-rsa ...')    |


#### Endpoints

##### Fetch Current User's SSH Public Keys

    GET /keys

Fetch the SSH public keys of the current user (as determined by the
Api-User and Api-Key headers) as an array of Key objects.

If the user has no public keys, expect: []

##### Add a Public Key to the Current User

    POST /keys

Add a single public key to the current user (as determined by the
Api-User and Api-Key headers) as a Key object. If you attempt to insert
a key with a duplicate name, it will fail

Both name and key must not be empty strings, nor strings consisting
entirely of whitespace

If the key was successfully inserted, the server will return HTTP status
code 200 OK

If the a key with the supplied name already exists, the key will not be
added, and the route will return status code 400 Bad Request.

Any other status code indicates that the key was not successfully added.

##### Delete A Specified Public Key from the Current User

    DELETE /keys/{key_name}

Delete the SSH public key with name `{key_name}` from the current user
(as determined by the Api-User and Api-Key headers).

If a public key with name `{key_name}` was successfully deleted, HTTP
status code 200 OK will be returned.

If a public key with name `{key_name}` does not exist, HTTP status
code 400 Bad Request will be returned.

Any other code indicates that the public key was not deleted

### Status

Status

#### Objects

**APICLIUpdate**

| Name            | Type            | Description                                                                                            |
|-----------------|-----------------|--------------------------------------------------------------------------------------------------------|
| `client_config` | APIClientConfig | Client version/URLs                                                                                    |
| `ignore_update` | bool            | If true, degraded mode for clients is enabled, and the client should treat their version as up-to date |


**APIClientConfig**

| Name            | Type              | Description                              |
|-----------------|-------------------|------------------------------------------|
| latest_revision | string            | a string representing the client version |
| client_binaries | []APIClientBinary | Array of APIClientBinary objects         |


**APIClientBinary**

| Name | Type   | Description                                        |
|------|--------|----------------------------------------------------|
| arch | string | architecture of the binary; must be a valid GOARCH |
| os   | string | OS of the binary; must be a valid GOOS             |
| url  | string | URL where the binary can be fetched                |


#### Endpoints

##### Fetch CLI Client Version

    GET /status/cli_version

Fetch the CLI update manifest from the server

If you cannot find an endpoint, it may not be documented here. Check the
defined endpoints in evergreen source:
<https://github.com/evergreen-ci/evergreen/blob/master/rest/route/service.go>

### Status Message

    GET /admin/banner

    {
      "banner": "Evergreen is currently unable to pick up new commits or process pull requests due to a GitHub outage",
      "theme": "warning"
    }

### TaskStats

Task stats are aggregated task execution statistics for a given project.
The statistics can be grouped by time period and by task, variant,
distro combinations.

#### Objects

**TaskStats**

| Name                   | Type   | Description                                                                                               |
|------------------------|--------|-----------------------------------------------------------------------------------------------------------|
| `task_name`            | string | Name of the task the test ran under.                                                                      |
| `variant`              | string | Name of the build variant the task ran on. Omitted if the grouping does not include the build variant.    |
| `distro`               | string | Identifier of the distro that the task ran on. Omitted if the grouping does not include the distro.       |
| `date`                 | string | The start date ("YYYY-MM-DD" UTC day) of the period the statistics cover.                                 |
| `num_success`          | int    | The number of times the task was successful during the target period.                                     |
| `num_failed`           | int    | The number of times the task failed during the target period.                                             |
| `num_total`            | int    | The number of times the task ran during the target period.                                                |
| `num_timeout`          | int    | The number of times the task ended on a timeout during the target period.                                 |
| `num_test_failed`      | int    | The number of times the task failed with a failure of type [test]{.title-ref} during the target period.   |
| `num_system_failed`    | int    | The number of times the task failed with a failure of type [system]{.title-ref} during the target period. |
| `num_setup_failed`     | int    | The number of times the task failed with a failure of type [setup]{.title-ref} during the target period.  |
| `avg_duration_success` | float  | The average duration, in seconds, of the tasks that passed during the target period.                      |


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

### Notifications

Create custom notifications for email, slack, JIRA comments, and JIRA
issues.

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

| Name          | Type                | Description                                  |
|--------------------|---------|-------------------------------------------|
| `target`      | string              | Required. The name of the recipient.         |
| `msg`         | string              | Required. The message for the notification.  |
| `attachments` | []SlackAttachment | Optional. Array of attachments to a message. |


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

**JIRA Issue**

| Name          | Type                   | Description                                     |
|---------------|------------------------|-------------------------------------------------|
| `issue_key`   | string                 | Optional.                                       |
| `project`     | string                 | Optional. The project name.                     |
| `summary`     | string                 | Optional. The summary text.                     |
| `description` | string                 | Optional. The issue description.                |
| `reporter`    | string                 | Optional. The issue reporter.                   |
| `assignee`    | string                 | Optional. The issue assignee.                   |
| `type`        | string                 | Optional. The issue type.                       |
| `components`  | string                 | Optional. The project components.               |
| `labels`      | string                 | Optional. The issue labels.                     |
| `fields`      | map[string]interface{} | Optional. Arbitrary map of custom field values. |


This corresponds with the documentation in the [JIRA API for creating
issues](https://docs.atlassian.com/software/jira/docs/api/REST/7.6.1/#api/2/issue-createIssue).

**JIRA Comment**

| Name       | Type   | Description                                                       |
|------------|--------|-------------------------------------------------------------------|
| `issue_id` | string | Optional. The ID of the issue where the comment should be posted. |
| `body`     | string | Optional. The comment text.                                       |


This corresponds with the documentation in the [JIRA API for adding
comments](https://docs.atlassian.com/software/jira/docs/api/REST/7.6.1/#api/2/issue-addComment).

#### Endpoints

    POST /notifications/<type>

The type can be "email", "slack", "jira_issue", or
"jira_comment".

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

`GET /project/<project_name>/versions/<commit_hash>/tasks`

`POST /tasks/<task_id>/restart`

#### Description

Some Evergreen projects contain flaky tests or can endure spurious
failures. To restart all of these tasks to gain better signal a user can
fetch all of the tasks for a commit. Make a request to
`GET /project/<project_name>/versions/<commit_hash>/tasks` to fetch the
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
