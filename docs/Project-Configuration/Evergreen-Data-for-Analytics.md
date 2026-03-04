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

| Catalog        | Schema        | View                                     |
| -------------- | ------------- | ---------------------------------------- |
| awsdatacatalog | dev_prod_live | v\_\_evergreen\_\_daily_task_stats\_\_v1 |

#### Columns

| Name                      | Type    | Description                                                                                                                        |
| ------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| project_id                | VARCHAR | Unique project identifier.                                                                                                         |
| variant                   | VARCHAR | Name of the build variant on which the tasks ran.                                                                                  |
| task_name                 | VARCHAR | Display name of the tasks.                                                                                                         |
| request_type              | VARCHAR | Name of the trigger that requested the task executions. Will always be one of [these values](../Reference/Glossary.md#requesters). |
| finish_date               | VARCHAR | Date, in ISO format `YYYY-MM-DD`, on which the tasks ran.                                                                          |
| num_success               | BIGINT  | Number of successful task executions in the group.                                                                                 |
| num_failed                | BIGINT  | Number of failed task executions in the group.                                                                                     |
| num_timed_out             | BIGINT  | Number of task executions that failed due to a time out.                                                                           |
| num_test_failed           | BIGINT  | Number of task executions that failed due to a test failure.                                                                       |
| num_system_failed         | BIGINT  | Number of task executions that failed due to a system failure.                                                                     |
| num_setup_failed          | BIGINT  | Number of task executions that failed due to a setup failure.                                                                      |
| num_status_swaps          | BIGINT  | Number of task status changes between subsequent runs.                                                                             |
| total_success_duration_ns | BIGINT  | Total duration, in nanoseconds, of successful task executions.                                                                     |

### Evergreen Test Statistics

Daily aggregated statistics for test executions run in Evergreen. Test stats are aggregated by project, variant, task name, test name, request type, and the UTC date on which the test ran and partitioned by the project and date (in ISO format). For example, the partition `mongodb-mongo-master/2022-12-05` would contain the daily aggregated test stats for all tests run on `2022-12-05` in the `mongodb-mongo-master` project. When running queries against this view it is highly recommended to always filter by project and date.

#### Table

| Catalog        | Schema        | View                                   |
| -------------- | ------------- | -------------------------------------- |
| awsdatacatalog | dev_prod_live | v\_\_results\_\_daily_test_stats\_\_v1 |

#### Columns

| Name                   | Type    | Description                                                                                                                       |
| ---------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------- |
| project                | VARCHAR | Unique project identifier.                                                                                                        |
| variant                | VARCHAR | Name of the build variant on which the tests ran.                                                                                 |
| task_name              | VARCHAR | Name of the task that the test ran under. This is the display task name for tasks that are part of a display task.                |
| test_name              | VARCHAR | Display name of the tests.                                                                                                        |
| request_type           | VARCHAR | Name of the trigger that requested the task execution. Will always be one of [these values](../Reference/Glossary.md#requesters). |
| num_pass               | BIGINT  | Number of passing tests.                                                                                                          |
| num_fail               | BIGINT  | Number of failing tests.                                                                                                          |
| num_status_swaps       | BIGINT  | Number of test status changes between subsequent runs. Only available for tests that ran after 2024-09-15.                        |
| total_pass_duration_ns | DOUBLE  | Total duration, in nanoseconds, of passing tests.                                                                                 |
| task_create_iso        | VARCHAR | Date, in ISO format `YYYY-MM-DD`, on which the tests ran.                                                                         |

### Evergreen Finished Tasks

Finished Evergreen tasks, partitioned by the project and date (in ISO format). For example, the partition `mongodb-mongo-master/2022-12-05` would contain all tasks that completed on `2022-12-05` in the `mongodb-mongo-master` project. When running queries against this view it is highly recommended to always filter by project and date.

#### Table

| Catalog        | Schema        | View                                   |
| -------------- | ------------- | -------------------------------------- |
| awsdatacatalog | dev_prod_live | v\_\_evergreen\_\_finished_tasks\_\_v1 |

#### Columns

| Name                       | Type           | Description                                                                                                                                                                         |
| -------------------------- | -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| task_id                    | VARCHAR        | Unique task identifier.                                                                                                                                                             |
| execution                  | BIGINT         | Task execution number.                                                                                                                                                              |
| display_name               | VARCHAR        | Display name of the task.                                                                                                                                                           |
| version_id                 | VARCHAR        | ID of the version containing the task.                                                                                                                                              |
| build_id                   | VARCHAR        | ID of the build containing the task.                                                                                                                                                |
| order                      | BIGINT         | Order number of the task's version. For patches this is the user's current patch submission count. For mainline versions this is the number of versions for that repository so far. |
| revision                   | VARCHAR        | Git commit SHA.                                                                                                                                                                     |
| requester                  | VARCHAR        | Name of the trigger that requested the task execution. Will always be one of [these values](../Reference/Glossary.md#requesters).                                                   |
| tags                       | ARRAY(VARCHAR) | Array of task tags from project configuration.                                                                                                                                      |
| priority                   | BIGINT         | Scheduling priority of the task.                                                                                                                                                    |
| task_group                 | VARCHAR        | Name of the task group that contains this task.                                                                                                                                     |
| task_group_max_hosts       | BIGINT         | Maximum number of hosts that will be used to run the task group.                                                                                                                    |
| task_group_order           | BIGINT         | Position of this task in its task group.                                                                                                                                            |
| depends_on                 | ARRAY(ROW)     | Array of `depends_on` rows describing the task's dependencies.                                                                                                                      |
| num_dependents             | BIGINT         | The number of tasks this task depends on.                                                                                                                                           |
| override_dependencies      | BOOLEAN        | Whether this task will wait on its dependencies.                                                                                                                                    |
| display_task_id            | VARCHAR        | ID of the task's display task.                                                                                                                                                      |
| display_task_display_name  | VARCHAR        | Display name of the task's display task.                                                                                                                                            |
| display_only               | VARCHAR        | Whether this task is a display task.                                                                                                                                                |
| execution_tasks            | ARRAY(ROW)     | Array of `execution_task` rows, if the task is a display task.                                                                                                                      |
| activated_by               | VARCHAR        | User or service that activated this task.                                                                                                                                           |
| create_time                | TIMESTAMP      | The creation time for the task, derived from the commit time or the patch creation time.                                                                                            |
| ingest_time                | TIMESTAMP      | Time the task was created.                                                                                                                                                          |
| dispatch_time              | TIMESTAMP      | Time the scheduler assigns the task to a host.                                                                                                                                      |
| scheduled_time             | TIMESTAMP      | Time the task is first scheduled.                                                                                                                                                   |
| start_time                 | TIMESTAMP      | Time the task started running.                                                                                                                                                      |
| finish_time                | TIMESTAMP      | Time the task completed running.                                                                                                                                                    |
| activated_time             | TIMESTAMP      | Time the task was marked as available to be scheduled, automatically or by a developer.                                                                                             |
| dependencies_met_time      | TIMESTAMP      | Time dependencies are confirmed as fully met by the task scheduler, for tasks that have dependencies. This will always be some time after the last dependency task finishes.        |
| time_taken                 | BIGINT         | Duration, in nanoseconds, the task took to execute.                                                                                                                                 |
| distro                     | VARCHAR        | ID of the distro this task is for.                                                                                                                                                  |
| secondary_distros          | ARRAY(VARCHAR) | Optional secondary distros that can run the task when they're idle.                                                                                                                 |
| build_variant              | VARCHAR        | Name of the task's build variant.                                                                                                                                                   |
| build_variant_display_name | VARCHAR        | Display name of the task's build variant.                                                                                                                                           |
| execution_platform         | VARCHAR        | The execution environment the task runs in. One of: `host` and `container`.                                                                                                         |
| host_id                    | VARCHAR        | ID of the host that ran this task.                                                                                                                                                  |
| generate_task              | BOOLEAN        | Indicates that the task generates other tasks.                                                                                                                                      |
| generated_by               | VARCHAR        | ID of the task that generated this task.                                                                                                                                            |
| status                     | VARCHAR        | Task status.                                                                                                                                                                        |
| details                    | ROW            | `details` row.                                                                                                                                                                      |
| oom_killer_detected        | BOOLEAN        | Whether the oom killer was detected.                                                                                                                                                |
| modules                    | ROW(ARRAY)     | Row of an array of `prefix` rows applied for the task.                                                                                                                              |
| abort                      | BOOLEAN        | If the task was aborted.                                                                                                                                                            |
| finish_date                | VARCHAR        | Date when the task finished, in ISO format.                                                                                                                                         |
| project_id                 | VARCHAR        | ID of the task's project.                                                                                                                                                           |

##### Types

###### depends_on

| Name                 | Type    | Description                                                                                       |
| -------------------- | ------- | ------------------------------------------------------------------------------------------------- |
| task_id              | VARCHAR | Unique task identifier.                                                                           |
| status               | VARCHAR | The status the dependency must attain to be considered satisfied.                                 |
| unattainable         | BOOLEAN | Whether the dependency has finished with a blocking status.                                       |
| finished             | BOOLEAN | If the task's dependency has finished running.                                                    |
| omit_generated_tasks | BOOLEAN | Causes tasks that depend on a generator task to not depend on the generated tasks if this is set. |

###### execution_tasks

| Name        | Type    | Description                                 |
| ----------- | ------- | ------------------------------------------- |
| task_id     | VARCHAR | Unique task identifier.                     |
| execution   | BIGINT  | Task execution number.                      |
| finish_date | VARCHAR | Date when the task finished, in ISO format. |

###### details

| Name             | Type    | Description                                                           |
| ---------------- | ------- | --------------------------------------------------------------------- |
| status           | VARCHAR | Task status.                                                          |
| message          | VARCHAR | Task status message.                                                  |
| type             | VARCHAR | Task type.                                                            |
| desc             | VARCHAR | Task status description.                                              |
| timed_out        | BOOLEAN | Whether the task timed out.                                           |
| timeout_type     | VARCHAR | Timeout type of the task.                                             |
| timeout_duration | BIGINT  | Duration, in nanoseconds, of how long the task ran before timing out. |
| trace_id         | VARCHAR | OTel trace ID for the task.                                           |

###### prefix

| Name | Type    | Description       |
| ---- | ------- | ----------------- |
| k    | VARCHAR | Name of a module. |
| v    | VARCHAR | Path to a module. |

### Evergreen AWS Task Costs

Documentation and management for this data can be found in the data/product analytics team documentation. See [their
documentation](https://wiki.corp.mongodb.com/spaces/ADS/pages/385851483/Pipelines+and+Processes#PipelinesandProcesses-Evergreen)
for more info.
