Evergreen REST API v1
----------

### Contents

  - [A note on authentication](#a-note-on-authentication)
  - [Retrieve a list of active projects](#retrieve-a-list-of-active-projects)
  - [Retrieve info on a particular project](#retrieve-info-on-a-particular-project)
  - [Retrieve the most recent revisions for a particular project](#retrieve-the-most-recent-revisions-for-a-particular-project)
  - [Retrieve a version with passing builds](#retrieve-a-version-with-passing-builds)
  - [Retrieve info on a particular version](#retrieve-info-on-a-particular-version)
  - [Retrieve info on a particular version by its revision](#retrieve-info-on-a-particular-version-by-its-revision)
  - [Activate a particular version](#activate-a-particular-version)
  - [Retrieve the status of a particular version](#retrieve-the-status-of-a-particular-version)
  - [Retrieve info on a particular build](#retrieve-info-on-a-particular-build)
  - [Retrieve the status of a particular build](#retrieve-the-status-of-a-particular-build)
  - [Retrieve info on a particular task](#retrieve-info-on-a-particular-task)
  - [Retrieve the status of a particular task](#retrieve-the-status-of-a-particular-task)
  - [Retrieve the most recent revisions for a particular kind of task](#retrieve-the-most-recent-revisions-for-a-particular-kind-of-task)

#### A note on authentication

Most of the these REST endpoints do not require authentication to access.
However, if the task, build, version, etc. that you are attempting to access is part of a private project, auth information is required to access.
Attempts to load private pages with a public REST call receive a 302 FOUND response.

The simplest way to do this is to use your `user` and `api_key` fields from the Settings page.
Authenticated REST access requires setting two headers, `Auth-Username` and `Api-Key`.

##### Example

```bash
    curl -H Auth-Username:my.name -H Api-Key:21312mykey12312 https://localhost:9090/rest/v1/projects/my_private_project
```
#### Retrieve a list of active project IDs

    GET /rest/v1/projects

_Note that you must use API credentials to see private projects._

##### Request

    curl https://localhost:9090/rest/v1/projects

##### Response

```json
{
  "projects": [
    "mci",
    "mongodb-mongo-master-sanitize",
    "mongo-c-driver"
  ]
}
```

#### Retrieve info on a particular project

    GET /rest/v1/projects/{project_id}

_Note that you must use API credentials to see private projects._

##### Request

    curl https://localhost:9090/rest/v1/projects/mci

##### Response

```json
{
  "owner_name": "evergreen-ci",
  "repo_name": "evergreen",
  "branch_name": "master",
  "repo_kind": "github",
  "enabled": true,
  "private": false,
  "batch_time": 1200,
  "remote_path": "self-tests.yml",
  "identifier": "mci",
  "display_name": "Evergreen Self-Tests",
  "local_config": "",
  "deactivate_previous": true,
  "tracked": true,
  "repotracker_error": null
}
```

#### Retrieve the most recent revisions for a particular project

    GET /rest/v1/projects/{project_id}/versions

##### Request

    curl https://localhost:9090/rest/v1/projects/mongodb-mongo-master/versions

##### Response

```json
{
  "project": "mongodb-mongo-master",
  "versions": [
    {
      "version_id": "mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652",
      "author": "Eric Milkie",
      "revision": "d477da53e119b207de45880434ccef1e47084652",
      "message": "SERVER-14613 corrections for gcc",
      "builds": {
        "amazon": {
          "build_id": "mongodb_mongo_master_amazon_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
          "name": "Amazon 64-bit",
          "tasks": {
            "aggregation": {
              "task_id": "mongodb_mongo_master_amazon_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09_aggregation_amazon",
              "status": "undispatched",
              "time_taken": 0
            },
            "aggregation_auth": { ... },
            ...
          }
        },
        "debian71": { ... },
      }
    },
    {
      "version_id": "mongodb_mongo_master_d30aac993ecc88052f11946e4486050ff57ba89c",
      ...
    },
    ...
  ]
}
```

#### Retrieve a version with passing builds

    GET /rest/v1/projects/{project_id}/last_green?{variants}

##### Parameters

This endpoint requires a query string listing the variants the user would like to ensure are passing.
Each variant is provided as a separate field (field values are not required: `?rhel55&osx-1010` is equivalent to `?rhel55=1&osx-1010=1`).

At least one variant is required.

##### Request

    curl https://localhost:9090/rest/v1/projects/mongodb-mongo-master/last_green?rhel55=1&rhel62=1

##### Response

The project's most recent version for which the variants provided in the query string are completely successful (i.e. "green").
The response contains the [entire version document](#retrieve-info-on-a-particular-version).




#### Retrieve info on a particular version by its revision

    GET /rest/v1/projects/{project_id}/revisions/{revision}
     
     or

    GET /rest/v1/projects/{project_id}/revisions/{revision}

_Note that the revision is equivalent to the git hash._

##### Request

    curl https://localhost:9090/rest/v1/projects/mongodb-mongo-master/revisions/d477da53e119b207de45880434ccef1e47084652

##### Response

```json
{
  "id": "mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652",
  "create_time": "2014-07-22T13:02:09.162-04:00",
  "start_time": "2014-07-22T13:03:18.151-04:00",
  "finish_time": "0001-01-01T00:00:00Z",
  "project": "mongodb-mongo-master",
  "revision": "d477da53e119b207de45880434ccef1e47084652",
  "author": "Eric Milkie",
  "author_email": "milkie@10gen.com",
  "message": "SERVER-14613 corrections for gcc",
  "status": "started",
  "activated": true,
  "builds": [
    "mongodb_mongo_master_linux_64_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
    "mongodb_mongo_master_linux_64_debug_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
    ...
  ],
  "build_variants": [
    "Linux 64-bit",
    "Linux 64-bit DEBUG",
    ...
  ],
  "order": 4205,
  "owner_name": "mongodb",
  "repo_name": "mongo",
  "branch_name": "master",
  "repo_kind": "github",
  "batch_time": 0,
  "identifier": "mongodb-mongo-master",
  "remote": false,
  "remote_path": "",
  "requester": "gitter_request"
}
```

#### Retrieve info on a particular version

    GET /rest/v1/versions/{version_id}
      
##### Request

    curl https://localhost:9090/rest/v1/versions/mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652

##### Response

```json
{
  "id": "mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652",
  "create_time": "2014-07-22T13:02:09.162-04:00",
  "start_time": "2014-07-22T13:03:18.151-04:00",
  "finish_time": "0001-01-01T00:00:00Z",
  "project": "mongodb-mongo-master",
  "revision": "d477da53e119b207de45880434ccef1e47084652",
  "author": "Eric Milkie",
  "author_email": "milkie@10gen.com",
  "message": "SERVER-14613 corrections for gcc",
  "status": "started",
  "activated": true,
  "builds": [
    "mongodb_mongo_master_linux_64_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
    "mongodb_mongo_master_linux_64_debug_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
    ...
  ],
  "build_variants": [
    "Linux 64-bit",
    "Linux 64-bit DEBUG",
    ...
  ],
  "order": 4205,
  "owner_name": "mongodb",
  "repo_name": "mongo",
  "branch_name": "master",
  "repo_kind": "github",
  "batch_time": 0,
  "identifier": "mongodb-mongo-master",
  "remote": false,
  "remote_path": "",
  "requester": "gitter_request"
}
```

#### Retrieve the YAML configuration for a specific version 

    GET /rest/v1/versions/{version_id}/config

##### Request

    curl https://localhost:9090/rest/v1/versions/mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652/config

##### Response

The contents of the YAML config for the specified version will be sent back in the body of the request, using
the header `Content-Type: application/x-yaml`.


#### Activate a particular version

    PATCH /rest/v1/versions/{version_id}

##### Input

Name      | Type | Description
--------- | ---- | -----------
activated | bool | **Optional**. Activates the version when `true`, and deactivates the version when `false`. Does nothing if the field is omitted.

##### Request

    curl -X PATCH https://localhost:9090/rest/v1/versions/mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652 -d '{"activated": false}' -H Auth-Username:my.name -H Api-Key:21312mykey12312

##### Response

```json
{
  "id": "mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652",
  "create_time": "2014-07-22T13:02:09.162-04:00",
  "start_time": "2014-07-22T13:03:18.151-04:00",
  "finish_time": "0001-01-01T00:00:00Z",
  "project": "mongodb-mongo-master",
  "revision": "d477da53e119b207de45880434ccef1e47084652",
  "author": "Eric Milkie",
  "author_email": "milkie@10gen.com",
  "message": "SERVER-14613 corrections for gcc",
  "status": "started",
  "activated": false,
  "builds": [
    "mongodb_mongo_master_linux_64_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
    "mongodb_mongo_master_linux_64_debug_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
    ...
  ],
  "build_variants": [
    "Linux 64-bit",
    "Linux 64-bit DEBUG",
    ...
  ],
  "order": 4205,
  "owner_name": "mongodb",
  "repo_name": "mongo",
  "branch_name": "master",
  "repo_kind": "github",
  "batch_time": 0,
  "identifier": "mongodb-mongo-master",
  "remote": false,
  "remote_path": "",
  "requester": "gitter_request"
}
```

#### Retrieve the status of a particular version

    GET /rest/v1/versions/{version_id}/status

##### Parameters

Name    | Type   | Default | Description
------- | ------ | ------- | -----------
groupby | string | tasks   | Determines how to key into the task status. For `tasks` use `task_name.build_variant`, and for `builds` use `build_variant.task_name`.


##### Request

    curl https://localhost:9090/rest/v1/versions/mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652/status

##### Response

```json
{
  "version_id": "mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652",
  "tasks": {
    "aggregation": {
      "amazon": {
        "task_id": "mongodb_mongo_master_amazon_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09_aggregation_amazon",
        "status": "undispatched",
        "time_taken": 0
      },
      "debian71": { ... },
      ...
    },
    "aggregation_auth": { ... },
    ...
  }
}
```

##### Request

    curl https://localhost:9090/rest/v1/versions/mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652/status?groupby=builds

##### Response

```json
{
  "version_id": "mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652",
  "builds": {
    "amazon": {
      "aggregation": {
        "task_id": "mongodb_mongo_master_amazon_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09_aggregation_amazon",
        "status": "undispatched",
        "time_taken": 0
      },
      "aggregation_auth": { ... },
      ...
    },
    "debian71": { ... },
    ...
  }
}
```

#### Retrieve info on a particular build

    GET /rest/v1/builds/{build_id}

##### Request

    curl https://localhost:9090/rest/v1/builds/mongodb_mongo_master_linux_64_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09

##### Response

```json
{
  "id": "mongodb_mongo_master_linux_64_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
  "create_time": "2014-07-22T13:02:09.162-04:00",
  "start_time": "2014-07-22T13:03:18.151-04:00",
  "finish_time": "0001-01-01T00:00:00Z",
  "push_time": "2014-07-22T13:02:09.162-04:00",
  "version": "mongodb_mongo_master_d477da53e119b207de45880434ccef1e47084652",
  "project": "mongodb-mongo-master",
  "revision": "d477da53e119b207de45880434ccef1e47084652",
  "variant": "linux-64",
  "number": "7960",
  "status": "started",
  "activated": true,
  "activated_time": "2014-07-22T13:03:07.556-04:00",
  "order": 4205,
  "tasks": {
    "aggregation": {
      "task_id": "mongodb_mongo_master_amazon_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09_aggregation_amazon",
      "status": "undispatched",
      "time_taken": 0
    },
    "aggregation_auth": { ... },
    ...
  },
  "time_taken": 0,
  "name": "Linux 64-bit",
  "requested": "gitter_request"
}
```

#### Retrieve the status of a particular build

    GET /rest/v1/builds/{build_id}/status

##### Request

    curl https://localhost:9090/rest/v1/builds/mongodb_mongo_master_linux_64_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09/status

##### Response

```json
{
    "build_id": "mongodb_mongo_master_linux_64_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09",
    "build_variant": "linux-64",
    "tasks": {
    "aggregation": {
      "task_id": "mongodb_mongo_master_amazon_d477da53e119b207de45880434ccef1e47084652_14_07_22_17_02_09_aggregation_amazon",
      "status": "undispatched",
      "time_taken": 0
    },
    "aggregation_auth": { ... },
    ...
}
}
```

#### Retrieve info on a particular task

    GET /rest/v1/tasks/{task_id}

##### Request

    curl https://localhost:9090/rest/v1/tasks/mongodb_mongo_master_linux_64_7ffac7f351b80f84589349e44693a94d5cc5e14c_14_07_22_13_27_06_aggregation_linux_64

##### Response

```json
{
  "id": "mongodb_mongo_master_linux_64_7ffac7f351b80f84589349e44693a94d5cc5e14c_14_07_22_13_27_06_aggregation_linux_64",
  "create_time": "2014-07-22T09:27:06.913-04:00",
  "scheduled_time": "2014-07-22T10:40:09.485-04:00",
  "dispatch_time": "2014-07-22T10:44:12.095-04:00",
  "start_time": "2014-07-22T10:44:15.783-04:00",
  "finish_time": "2014-07-22T10:49:02.796-04:00",
  "push_time": "2014-07-22T09:27:06.913-04:00",
  "version": "mongodb_mongo_master_7ffac7f351b80f84589349e44693a94d5cc5e14c",
  "project": "mongodb-mongo-master",
  "revision": "7ffac7f351b80f84589349e44693a94d5cc5e14c",
  "priority": 0,
  "last_heartbeat": "2014-07-22T10:48:43.761-04:00",
  "activated": true,
  "build_id": "mongodb_mongo_master_linux_64_7ffac7f351b80f84589349e44693a94d5cc5e14c_14_07_22_13_27_06",
  "distro": "rhel55-test",
  "build_variant": "linux-64",
  "depends_on": [
    "mongodb_mongo_master_linux_64_7ffac7f351b80f84589349e44693a94d5cc5e14c_14_07_22_13_27_06_compile_linux_64"
  ],
  "display_name": "aggregation",
  "host_id": "i-58e6e573",
  "restarts": 0,
  "execution": 0,
  "archived": false,
  "order": 4196,
  "requester": "gitter_request",
  "status": "success",
  "status_details": {
    "timed_out": false,
    "timeout_stage": ""
  },
  "aborted": false,
  "time_taken": 287013061125,
  "expected_duration": 0,
  "test_results": {
    "jstests/aggregation/mongos_slaveok.js": {
      "status": "pass",
      "time_taken": 25482633113,
      "logs": {
        "url": "http://buildlogs.mongodb.org/build/53ce78d7d2a60f5fac000970/test/53ce78d9d2a60f5f72000a23/"
      }
    },
    "jstests/aggregation/testSlave.js": { ... },
    ...
  },
  "min_queue_pos": 0,
  "files": []
}
```

#### Retrieve the status of a particular task

    GET /rest/v1/tasks/{task_id}/status

##### Request

    curl https://localhost:9090/rest/v1/tasks/mongodb_mongo_master_linux_64_7ffac7f351b80f84589349e44693a94d5cc5e14c_14_07_22_13_27_06_aggregation_linux_64/status

##### Response

```json
{
  "task_id": "mongodb_mongo_master_linux_64_7ffac7f351b80f84589349e44693a94d5cc5e14c_14_07_22_13_27_06_aggregation_linux_64",
  "task_name": "aggregation",
  "status": "success",
  "status_details": {
    "timed_out": false,
    "timeout_stage": ""
  },
  "tests": {
    "jstests/aggregation/mongos_slaveok.js": {
      "status": "pass",
      "time_taken": 25482633113,
      "logs": {
        "url": "http://buildlogs.mongodb.org/build/53ce78d7d2a60f5fac000970/test/53ce78d9d2a60f5f72000a23/"
      }
    },
    "jstests/aggregation/testSlave.js": { ... },
    ...
  }
}
```

#### Retrieve the most recent revisions for a particular kind of task

    GET /rest/v1/tasks/{task_name}/history

##### Parameters

Name    | Type   | Description
------- | ------ | -----------
project | string | The project name.


##### Request

    curl https://localhost:9090/rest/v1/tasks/compile/history?project=sample

##### Response

```json
{
  "Tasks": [
    {
      "_id": "3585388b1591dfca47ac26a5b9a564ec8f138a5e",
      "order": 4,
      "tasks": [
        {
          "_id": "sample_osx_108_3585388b1591dfca47ac26a5b9a564ec8f138a5e_14_08_12_21_23_53_compile_osx_108",
          "activated": true,
          "build_variant": "osx-108",
          "status": "undispatched",
          "status_details": {},
          "time_taken": 0
        },
        {
          "_id": "sample_ubuntu_3585388b1591dfca47ac26a5b9a564ec8f138a5e_14_08_12_21_23_53_compile_ubuntu",
          ...
        },
        ...
      ]
    },
    {
      "_id": "9b20d123699a8061ac9587784a6871cf58d4e78f",
      ...
    }
  ],
  "Versions": [
    {
      "id": "sample_3585388b1591dfca47ac26a5b9a564ec8f138a5e",
      "create_time": "2014-08-12T21:23:53.555-04:00",
      "start_time": "0001-01-01T00:00:00Z",
      "finish_time": "0001-01-01T00:00:00Z",
      "revision": "3585388b1591dfca47ac26a5b9a564ec8f138a5e",
      "message": "dont print for timeouts",
      "order": 4
    },
    {
      "id": "sample_9b20d123699a8061ac9587784a6871cf58d4e78f",
      ...
    },
    ...
  ],
  "FailedTests": {},
  "Exhausted": {
    "Before": true,
    "After": true
  }
}
```
