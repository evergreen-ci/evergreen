# REST V2 Use Case Guide

## Find all failures of a given build

### Endpoint

`GET /builds/<build_id>/tasks`

### Description

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

### Description

To better understand all aspects of a task failure, such as failure
mode, which tests failed, and how long it took, it is helpful to fetch
this information about a task. This can be accomplished using 2 API
calls. Make one call to the endpoint for a specific task
`GET /tasks/<task_id>` which returns information about the task itself.
Then, make a second cal to `GET /tasks/<task_id>/tests` which delivers
specific information about the tests that ran in a certain task.

## Get all hosts

### Endpoint

`GET /hosts`

### Description

Retrieving information on Evergreen's hosts can be helpful for system
monitoring. To fetch this information, make a call to `GET /hosts`,
which returns a paginated list of hosts. Page through the results to
inspect all hosts.

By default, this endpoint will only return hosts that are considered
\"up\" (status is equal to running, initializing, starting,
provisioning, or provision failed).

## Restart all failures for a commit

### Endpoints

`GET /project/<project_name>/versions/<commit_hash>/tasks`

`POST /tasks/<task_id>/restart`

### Description

Some Evergreen projects contain flaky tests or can endure spurious
failures. To restart all of these tasks to gain better signal a user can
fetch all of the tasks for a commit. Make a request to
`GET /project/<project_name>/versions/<commit_hash>/tasks` to fetch the
tasks that ran and then loop over all of the returned tasks, calling
`POST /tasks/<task_id>/restart` on each task which has failed.

## Modify an Existing Project

### Endpoint

`PATCH /projects/<project_id>`

### Description

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

## Copy an Existing Project

### Endpoint

`POST /projects/<project_id>/copy`

### Description

To copy a project to a new project, this is the route you would use. To
define the new project's name (which is required), we would include a
query parameter, for example:

    projects/my_first_project/copy?new_project=my_second_project

This route will return the new project but this will not include
variables/aliases/subscriptions; to see this, GET the new project.
