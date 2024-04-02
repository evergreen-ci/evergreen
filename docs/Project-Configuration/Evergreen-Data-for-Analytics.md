# Evergreen Data for Analytics

We aim to provide a self-service platform for users to access and analyze data from Evergreen and other Dev Prod projects. Evergreen leverages [Mongo Trino](https://docs.dataplatform.prod.corp.mongodb.com/docs/Trino/Introduction) and [Mongo Automated Reporting System](https://docs.dataplatform.prod.corp.mongodb.com/docs/MARS/Introduction) (MARS) so that users can access any quantity of data without waiting on Dev Prod teams and without impacting production Dev Prod systems.

## Getting Started

### Mongo Trino Access
All Mongo engineers are automatically granted basic read access to Trino, see the [Internal Data Platform Security](https://wiki.corp.mongodb.com/display/DW/Getting+Access+to+the+Internal+Data+Platform) documentation for more information. Access to the R&D Dev Prod data in Trino is governed through the [Internal Data Platform - Dev Prod Read](https://mana.corp.mongodbgov.com/guilds/61a43c5a210c1301d0b81297) MANA guild. Most engineering teams should have already been granted membership to this guild—if you or your team does not have access, please request membership via MANA.

### Connecting to the Database
Check out the [Mongo Trino](https://docs.dataplatform.prod.corp.mongodb.com/docs/Trino/Introduction) documentation and, more specifically, the [DBeaver Connection](https://docs.dataplatform.prod.corp.mongodb.com/docs/Trino/DBeaver%20Connection) page to get started! Users are free to choose another database tool of their liking to connect and interact with Mongo Trino. See the [Data Dictionary](#data-dictionary) section below for an exhaustive list of our data in Trino and example queries to get started!

## Data Dictionary
This section provides the most up-to-date description of our data sets in Trino, see the [Trino concepts documentation](https://trino.io/docs/current/overview/concepts.html#schema) for more information on the terminology used. Data is exposed via well-designed views, see our [Data Policies](https://github.com/10gen/dev-prod-etls/blob/main/docs/policies.md) for more information. **Only the data sets exposed via a view and documented here are considered "production-ready", all others are considered "raw" and may be accessed at the peril of the user**.

### Evergreen Task Statistics
Daily aggregated statistics for task executions run in Evergreen. Tasks are aggregated by project ID, variant, task name, and the UTC date on which the task ran and partitioned by the project ID and date (in ISO format). For example, the partition `mongodb-mongo-master/2022-12-05` would contain the daily aggregated task stats for all tasks run on `2022-12-05` in the `mongodb-mongo-master` project. When running queries against this view it is highly recommended to always filter by project ID and date.

#### Table
| Catalog        | Schema          | View                                       |
| ---------------|-----------------|--------------------------------------------|
| awsdatacatalog | dev\_prod\_live | v\_\_evergreen\_\_daily\_task\_stats\_\_v1 |

#### Columns
| Name                         | Type    | Description |
|------------------------------|---------|-------------|
| project_id                   | VARCHAR | Unique project identifier.
| variant                      | VARCHAR | Name of the build variant on which the tasks ran.
| task\_name                   | VARCHAR | Display name of the tasks.
| request\_type                | VARCHAR | Name of the trigger that requested the task executions. Will always be one of: `patch_request`, `github_pull_request`, `gitter_request` (mainline), `trigger_request`, `merge_test` (commit queue), or `ad_hoc` (periodic build).
| finish\_date                 | VARCHAR | Date, in ISO format `YYYY-MM-DD`, on which the tasks ran.
| num\_success                 | BIGINT  | Number of successful task executions in the group.
| num\_failed                  | BIGINT  | Number of failed task executions in the group.
| num\_timed\_out              | BIGINT  | Number of task executions that failed due to a time out.
| num\_test\_failed            | BIGINT  | Number of task executions that failed due to a test failure.
| num\_system\_failed          | BIGINT  | Number of task executions that failed due to a system failure.
| num\_setup\_failed           | BIGINT  | Number of task executions that failed due to a setup failure.
| total\_success\_duration\_ns | BIGINT  | Total duration, in nanoseconds, of successful task executions.

### Evergreen Test Statistics
Daily aggregated statistics for test executions run in Evergreen. Test stats are aggregated by project, variant, task name, test name, request type, and the UTC date on which the test ran and partitioned by the project and date (in ISO format). For example, the partition `mongodb-mongo-master/2022-12-05` would contain the daily aggregated test stats for all tests run on `2022-12-05` in the `mongodb-mongo-master` project. When running queries against this view it is highly recommended to always filter by project and date.

#### Table
| Catalog        | Schema          | View                                     |
| ---------------|-----------------|------------------------------------------|
| awsdatacatalog | dev\_prod\_live | v\_\_results\_\_daily\_test\_stats\_\_v1 |

#### Columns
| Name                      | Type    | Description |
|---------------------------|---------|-------------|
| project                   | VARCHAR | Unique project identifier.
| variant                   | VARCHAR | Name of the build variant on which the tests ran.
| task\_name                | VARCHAR | Name of the task that the test ran under. This is the display task name for tasks that are part of a display task.
| test\_name                | VARCHAR | Display name of the tests.
| request\_type             | VARCHAR | Name of the trigger that requested the task execution. Will always be one of: `patch_request`, `github_pull_request`, `gitter_request` (mainline), `trigger_request`, `merge_test` (commit queue), or `ad_hoc` (periodic build).
| num\_pass                 | BIGINT  | Number of passing tests.
| num\_fail                 | BIGINT  | Number of failing tests.
| total\_pass\_duration\_ns | DOUBLE  | Total duration, in nanoseconds, of passing tests.
| task\_create\_iso         | VARCHAR | Date, in ISO format `YYYY-MM-DD`, on which the tests ran.

### Evergreen Finished Tasks
Finished Evergreen tasks, partitioned by the project and date (in ISO format). For example, the partition `mongodb-mongo-master/2022-12-05` would contain all tasks that completed on `2022-12-05` in the `mongodb-mongo-master` project. When running queries against this view it is highly recommended to always filter by project and date.

#### Table
| Catalog        | Schema          | View                                    |
| ---------------|-----------------|-----------------------------------------|
| awsdatacatalog | dev\_prod\_live | v\_\_evergreen\_\_finished\_tasks\_\_v0 |

#### Columns
| Name                           | Type      | Description |
|--------------------------------|-----------|-------------|
| task\_id                       | VARCHAR   | Unique task identifier.
| execution                      | BIGINT    | Task execution number.
| display\_name                  | VARCHAR   | Display name of the task.
| version                        | VARCHAR   | ID of the version containing the task.
| build\_id                      | VARCHAR   | ID of the build containing the task.
| order                          | BIGINT    | Order number of the task's version. For patches this is the user's current patch submission count. For mainline versions this is the number of versions for that repository so far.
| gitspec                        | VARCHAR   | Git commit SHA.
| r                              | VARCHAR   | Requester type. Will always be one of: `patch_request`, `github_pull_request`, `gitter_request` (mainline), `trigger_request`, `merge_test` (commit queue), or `ad_hoc` (periodic build).
| priority                       | BIGINT    | Scheduling priority of the task.
| task\_group                    | VARCHAR   | Name of the task group that contains this task.
| task\_group_max_hosts          | BIGINT    | Maximum number of hosts that will be used to run the task group.
| task\_group_order              | BIGINT    | Position of this task in its task group.
| depends\_on                    | ROW       | depends_on row describing the task's dependencies.
| num\_dependents                | BIGINT    | The number of tasks this task depends on.
| override_dependencies          | BOOLEAN   | Whether this task will wait on its dependencies.
| is\_essential\_to\_succeed     | BOOLEAN   | Indicates that this task must finish in order for its build and version to be considered successful. For example, tasks selected by the GitHub PR alias must succeed for the GitHub PR requester before its build or version can be reported as successful, but tasks manually scheduled by the user afterwards are not required.
| display\_task\_id              | VARCHAR   | ID of the task's display task.
| display\_task\_display\_name   | VARCHAR   | Display name of the task's display task.
| parent\_patch\_id              | VARCHAR   | Patch ID of the task's parent patch, if the task is a display task.
| parent\_patch\_number          | BIGINT    | Order number of the task's parent patch, if the task is part of a child patch.
| display\_only                  | VARCHAR   | Whether this task is a display task
| execution\_tasks               | ARRAY     | IDs of the execution tasks of this task, if the task is a display task.
| activated\_by                  | VARCHAR   | User or service that activated this task.
| create\_time                   | TIMESTAMP | The creation time for the task, derived from the commit time or the patch creation time.
| ingest\_time                   | TIMESTAMP | Time the task was created.
| container\_allocated_time      | TIMESTAMP | Time a container was allocated to run the task, if the task is a containerized task.
| dispatch\_time                 | TIMESTAMP | Time the scheduler assigns the task to a host.
| scheduled\_time                | TIMESTAMP | Time the task is first scheduled.
| start\_time                    | TIMESTAMP | Time the task started running.
| finish\_time                   | TIMESTAMP | Time the task completed running.
| activated_time                 | TIMESTAMP | Time the task was marked as available to be scheduled, automatically or by a developer.
| dependencies\_met\_time        | TIMESTAMP | Time all dependencies are met, for tasks that have dependencies.
| last\_heartbeat                | TIMESTAMP | Time of the last heartbeat sent back by the agent.
| time\_taken                    | BIGINT    | Duration, in nanoseconds, the task took to execute, if it has finished, or how long the task has been running, if it has started.
| wait\_since\_dependencies\_met | BIGINT    | Duration, in nanoseconds, since the task's dependencies have been met.
| duration\_prediction           | BIGINT    | How long Evergreen estimates this task will take to run.
| distro                         | VARCHAR   | ID of the distro this task is for.
| distro\_aliases                | ARRAY     |  Optional secondary distros that can run the task when they're idle.
| build\_variant                 | VARCHAR   | Name of the task's build variant.
| build\_variant\_display\_name  | VARCHAR   | Display name of the task's build variant.
| agent\_version                 | VARCHAR   | Version of the agent that ran this task.
| host\_id                       | VARCHAR   | ID of the host that ran this task.
| host\_create\_details          | VARCHAR   | Information about why host.create failed for this task.
| pod\_id                        | VARCHAR   | ID of the pod that ran this task.
| container                      | VARCHAR   | Name of the container configuration for running a container task.
| container\_options             | ROW       | container_options row that configures the container to run the task.
| container\_allocated           | BOOLEAN   | Whether a container has been allocated for the task.
| container\_allocation_attempts | BIGINT    | Number of attempts made to allocate a container.
| results\_service               | VARCHAR   | Name of the service that stores this task's test results.
| results\_failed                | BOOLEAN   | Whether any of the task's tests have failed.
| must\_have_results             | BOOLEAN   | Whether the task will be failed on account of no test results.
| is\_github\_check              | BOOLEAN   | Whether the task should be considered for mainline github checks.
| can\_reset                     | BOOLEAN   | Whether the task has successfully archived and is in a valid state to be reset.
| reset\_when\_finished          | BOOLEAN   | Indicates that a task should be reset once it is finished running.
| reset\_failed\_when\_finished  | BOOLEAN   | That a display task only restart failed tasks when it's restarted.
| generate\_task                 | BOOLEAN   | Indicates that the task generates other tasks.
| generated\_tasks               | BOOLEAN   | Indicates that the task has already generated other tasks.
| generated\_by                  | VARCHAR   | ID of the task that generated this task.
| generated\_json                | VARCHAR   | Configuration information to update the project YAML for generate.tasks.
| generate\_error                | VARCHAR   | Error encountered while generating tasks.
| generated\_task\s_to\_stepback | ROW       | Override activation for these generated tasks, because of stepback.
| trigger\_id                    | VARCHAR   | ID of an upstream entity that triggered this task.
| trigger\_type                  | VARCHAR   | Type of an upstream build that triggered this task.
| trigger\_event                 | VARCHAR   | ID of the event that triggered this task.
| commit\_queue\_merge           | BOOLEAN   | If this task is a commit queue merge task.
| can\_sync                      | BOOLEAN   | If the task can sync when finished running.
| sync\_at\_end\_opts            | ROW       | sync_at_end_opts row configuring task sync.
| status                         | VARCHAR   | Task status.
| details                        | ROW       | Task end details row.
| oom\_killer                    | ROW       | oom_killer detector row.
| modules                        | ROW       | Modules applied for the task.
| abort                          | BOOLEAN   | If the task was aborted.
| abort\_info                    | ROW       | abort_info row about an abort.
| finish\_date                   | VARCHAR   | Date when the task finished, in ISO format.
| project\_id                    | VARCHAR   | ID of the task's project.

##### Types
###### depends\_on
| Name                   | Type    | Description |
|------------------------|---------|-------------|
| task\_id               | VARCHAR | Unique task identifier.
| status                 | VARCHAR | The status the dependency must attain to be considered satisfied.
| unattainable           | BOOLEAN | Whether the dependency has finished with a blocking status.
| finished               | BOOLEAN | If the task's dependency has finished running.
| omit\_generated\_tasks | BOOLEAN | Causes tasks that depend on a generator task to not depend on the generated tasks if this is set.


###### container\_options
| Name              | Type    | Description |
|-------------------|---------|-------------|
| cpu               | BIGINT  | Number of CPUs available to the container.
| memory\_mb        | BIGINT  | Amount of memory avialable to the container.
| working\_dir      | VARCHAR | Task working directory.
| image             | VARCHAR | Image to run in the container.
| repo\_creds\_name | VARCHAR | name of the project container secret containing the repository credentials.
| os                | VARCHAR | OS of the image.
| arch              | VARCHAR | CPU architecture necessary to run a container.
| windows\_version  | VARCHAR | Compatibility version of Windows that is required for the container to run.

###### sync\_at\_end\_opts
| Name     | Type    | Description |
|----------|---------|-------------|
| enabled  | BOOLEAN | Whether task sync is enabled.
| statuses | ARRAY   | Task statuses to run task sync for.
| timeout  | BIGINT  | Duration, in nanoseconds, to allow task sync to run before timing it out.

###### details
| Name                        | Type    | Description |
|-----------------------------|---------|-------------|
| status            | VARCHAR | Task status.
| message           | VARCHAR | Task status message.
| type              | VARCHAR | Task type.
| desc              | VARCHAR | Task status description.
| timed_out         | BOOLEAN | Whether the task timed out.
| timeout\_type     | VARCHAR | Timeout type of the task.
| timeout\_duration | BIGINT  | Duration, in nanoseconds, of how long the task ran before timing out.
| trace\_id         | VARCHAR | OTel trace ID for the task.

###### oom\_killer
| Name     | Type    | Description |
|----------|---------|-------------|
| detected | BOOLEAN | If an OOM kill was detected during the execution of the task.
| pids     | ARRAY   | pids of the processes killed by the OOM killer.


###### abort\_info
| Name         | Type    | Description |
|--------------|---------|-------------|
| user         | VARCHAR | The user who aborted the task's execution.
| task\_id     | VARCHAR | The failing task that caused this task to be automatically aborted.
| new\_version | VARCHAR | The new version that caused this task to be automatically aborted.
| pr\_closed   | BOOLEAN | If the task was aborted because the PR it was testing was closed.


### Example Queries
Query all the test statistics for a given project on a given day:
```sql
SELECT *
FROM awsdatacatalog.dev_prod_live.v__results__daily_test_stats__v1
WHERE project = '<project_id>'
AND   task_create_iso = '<YYYY-DD-MM>'
```

Aggregate test statistics for mainline commits across the past two weeks for a given project, variant, and task:
```sql
SELECT test_name                    AS "test_name",
       SUM(num_pass)                AS "num_pass",
       SUM(num_fail)                AS "num_fail",
       SUM(total_pass_duration_ns)  AS "total_pass_duration_ns"
FROM awsdatacatalog.dev_prod_live.v__results__daily_test_stats__v1
WHERE project = '<project_id>'
AND   variant = '<variant>'
AND   task_name = '<task_name>'
AND   request_type = 'gitter_request'
AND   task_create_iso BETWEEN TO_ISO8601(CURRENT_DATE - INTERVAL '15' DAY) AND TO_ISO8601(CURRENT_DATE - INTERVAL '1' DAY)
GROUP BY 1
```

Get the average statistics of mainline tests per day per variant and task for a given project over the past two weeks, sorted by average number of failing tests per day in descending order:
```sql
SELECT variant                      AS "variant",
       task_name                    AS "task_name",
       AVG(num_pass)                AS "avg_num_pass",
       AVG(num_fail)                AS "avg_num_fail",
       AVG(total_pass_duration_ns)  AS "avg_total_pass_duration_ns"
FROM awsdatacatalog.dev_prod_live.v__results__daily_test_stats__v1
WHERE project = '<project_id>'
AND   request_type = 'gitter_request'
AND   task_create_iso BETWEEN TO_ISO8601(CURRENT_DATE - INTERVAL '15' DAY) AND TO_ISO8601(CURRENT_DATE - INTERVAL '1' DAY)
GROUP BY 1
ORDER BY 3 DESC
```



