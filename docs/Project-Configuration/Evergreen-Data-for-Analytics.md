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
| awsdatacatalog | dev\_prod\_live | v\_\_evergreen\_\_finished\_tasks\_\_v1 |

#### Columns
| Name                          | Type           | Description |
|-------------------------------|----------------|-------------|
| task\_id                      | VARCHAR        | Unique task identifier.
| execution                     | BIGINT         | Task execution number.
| display\_name                 | VARCHAR        | Display name of the task.
| version\_id                   | VARCHAR        | ID of the version containing the task.
| build\_id                     | VARCHAR        | ID of the build containing the task.
| order                         | BIGINT         | Order number of the task's version. For patches this is the user's current patch submission count. For mainline versions this is the number of versions for that repository so far.
| revision                      | VARCHAR        | Git commit SHA.
| requester                     | VARCHAR        | Requester type. Will always be one of: `patch_request`, `github_pull_request`, `gitter_request` (mainline), `trigger_request`, `merge_test` (commit queue), or `ad_hoc` (periodic build).
| tags                          | ARRAY(VARCHAR) | Array of task tags from project configuration.
| priority                      | BIGINT         | Scheduling priority of the task.
| task\_group                   | VARCHAR        | Name of the task group that contains this task.
| task\_group_max_hosts         | BIGINT         | Maximum number of hosts that will be used to run the task group.
| task\_group_order             | BIGINT         | Position of this task in its task group.
| depends\_on                   | ARRAY(ROW)     | Array of `depends_on` rows describing the task's dependencies.
| num\_dependents               | BIGINT         | The number of tasks this task depends on.
| override\_dependencies        | BOOLEAN        | Whether this task will wait on its dependencies.
| display\_task\_id             | VARCHAR        | ID of the task's display task.
| display\_task\_display\_name  | VARCHAR        | Display name of the task's display task.
| display\_only                 | VARCHAR        | Whether this task is a display task.
| execution\_tasks              | ARRAY(ROW)     | Array of `execution_task` rows, if the task is a display task.
| activated\_by                 | VARCHAR        | User or service that activated this task.
| create\_time                  | TIMESTAMP      | The creation time for the task, derived from the commit time or the patch creation time.
| ingest\_time                  | TIMESTAMP      | Time the task was created.
| dispatch\_time                | TIMESTAMP      | Time the scheduler assigns the task to a host.
| scheduled\_time               | TIMESTAMP      | Time the task is first scheduled.
| start\_time                   | TIMESTAMP      | Time the task started running.
| finish\_time                  | TIMESTAMP      | Time the task completed running.
| activated\_time               | TIMESTAMP      | Time the task was marked as available to be scheduled, automatically or by a developer.
| dependencies\_met\_time       | TIMESTAMP      | Time all dependencies are met, for tasks that have dependencies.
| time\_taken                   | BIGINT         | Duration, in nanoseconds, the task took to execute.
| distro                        | VARCHAR        | ID of the distro this task is for.
| secondary\_distros            | ARRAY(VARCHAR) | Optional secondary distros that can run the task when they're idle.
| build\_variant                | VARCHAR        | Name of the task's build variant.
| build\_variant\_display\_name | VARCHAR        | Display name of the task's build variant.
| execution\_platform           | VARCHAR        | The execution environment the task runs in. One of: `host` and `container`.
| host\_id                      | VARCHAR        | ID of the host that ran this task.
| pod\_id                       | VARCHAR        | ID of the pod that ran this task.
| generate\_task                | BOOLEAN        | Indicates that the task generates other tasks.
| generated\_by                 | VARCHAR        | ID of the task that generated this task.
| status                        | VARCHAR        | Task status.
| details                       | ROW            | `details` row.
| oom\_killer\_detected         | BOOLEAN        | Whether the oom killer was detected.
| modules                       | ROW(ARRAY)     | Row of an array of `prefix` rows applied for the task.
| abort                         | BOOLEAN        | If the task was aborted.
| finish\_date                  | VARCHAR        | Date when the task finished, in ISO format.
| project\_id                   | VARCHAR        | ID of the task's project.

##### Types
###### depends\_on
| Name                   | Type    | Description |
|------------------------|---------|-------------|
| task\_id               | VARCHAR | Unique task identifier.
| status                 | VARCHAR | The status the dependency must attain to be considered satisfied.
| unattainable           | BOOLEAN | Whether the dependency has finished with a blocking status.
| finished               | BOOLEAN | If the task's dependency has finished running.
| omit\_generated\_tasks | BOOLEAN | Causes tasks that depend on a generator task to not depend on the generated tasks if this is set.

###### execution\_tasks
| Name         | Type    | Description |
|--------------|---------|-------------|
| task\_id     | VARCHAR | Unique task identifier.
| execution    | BIGINT  | Task execution number.
| finish\_date | VARCHAR | Date when the task finished, in ISO format.

###### details
| Name              | Type    | Description |
|-------------------|---------|-------------|
| status            | VARCHAR | Task status.
| message           | VARCHAR | Task status message.
| type              | VARCHAR | Task type.
| desc              | VARCHAR | Task status description.
| timed_out         | BOOLEAN | Whether the task timed out.
| timeout\_type     | VARCHAR | Timeout type of the task.
| timeout\_duration | BIGINT  | Duration, in nanoseconds, of how long the task ran before timing out.
| trace\_id         | VARCHAR | OTel trace ID for the task.

###### prefix
| Name | Type    | Description |
|------|---------|-------------|
| k    | VARCHAR | Name of a module.
| v    | VARCHAR | Path to a module.


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



