# Analyze Data with Materialized Intelligence

Materialized Intelligence is R&D Developer Productivity's data offering for downstream analytics and business intelligence. We aim to provide a self-service platform for users to access and analyze data from Evergreen and other Dev Prod projects. This platform leverages [Mongo Trino](https://docs.dataplatform.prod.corp.mongodb.com/docs/Trino/Introduction) and [Mongo Automated Reporting System](https://docs.dataplatform.prod.corp.mongodb.com/docs/MARS/Introduction) (MARS) so that users can access any quantity of data without waiting on Dev Prod teams and without impacting production Dev Prod systems.

## Getting Started

### Mongo Trino Access
All Mongo engineers are automatically granted basic read access to Trino, see the [Internal Data Platform Security](https://wiki.corp.mongodb.com/display/DW/Internal+Data+Platform+Security) documentation for more information. Access to the R&D Dev Prod data in Trino is governed through the [Internal Data Platform - Dev Prod Read](https://mana.corp.mongodbgov.com/guilds/61a43c5a210c1301d0b81297) MANA guild. Most engineering teams should have already been granted membership to this guild—if you or your team does not have access, please request membership via MANA.

### Connecting to the Database
Check out the [Mongo Trino](https://docs.dataplatform.prod.corp.mongodb.com/docs/Trino/Introduction) documentation and, more specifically, the [DBeaver Connection](https://docs.dataplatform.prod.corp.mongodb.com/docs/Trino/DBeaver%20Connection) page to get started! Users are free to choose another database tool of their liking to connect and interact with Mongo Trino. See the [Data Dictionary](https://docs.google.com/document/d/1rSB9Xc7c7yrF7q4V_kWrJLw-8N4X5BNlT6S0xf-NdsY/edit?pli=1#heading=h.meqwpfqi1sn4) section for an exhaustive list of our data in Trino and example queries to get started!

## Data Dictionary
This section provides the most up-to-date description of our data sets in Trino, see the [Trino concepts documentation](https://trino.io/docs/current/overview/concepts.html#schema) for more information on the terminology used. Data is exposed via well-designed views, see our [Data Policies](https://github.com/10gen/dev-prod-etls/blob/main/docs/policies.md) for more information. **Only the data sets exposed via a view and documented here are considered "production-ready", all others are considered "raw" and may be accessed at the peril of the user**.

### Evergreen Test Statistics
Daily aggregated statistics for test executions run in Evergreen. Test stats are aggregated by project, variant, task name, test name, request type, and the UTC date on which the test ran and partitioned by the project and date (in ISO format). For example, the partition `mongodb-mongo-master/2022-12-05` would contain the daily aggregated test stats for all tests run on `2022-12-05` in the `mongodb-mongo-master` project. When running queries against this view, it is highly recommended to always filter by project and date. Stats are aggregated by project, variant, task name, test name, request type, and the UTC date the test ran.

#### Table
| Catalog        | Schema          | View                                     |
| ---------------|-----------------|------------------------------------------|
| awsdatacatalog | dev\_prod\_live | v\_\_results\_\_daily\_test\_stats\_\_v1 |

#### Columns
| Name                      | Type    | Description |
|---------------------------|---------|-------------|
| project                   | VARCHAR | Unique project identifier.
| variant                   | VARCHAR | Name of the build variant the test ran on.
| task\_name                | VARCHAR | Name of the task that the test ran under. This is the display task name for tasks that are part of a display task.
| test\_name                | VARCHAR | Name of the test.
| request\_type             | VARCHAR | Name of the trigger that requested the test execution. Will always be one of: `patch_request`, `github_pull_request`, `gitter_request` (mainline), `trigger_request`, `merge_test` (commit queue), or `ad_hoc` (periodic build).
| num\_pass                 | BIGINT  | The number of times the test grouping passed that day.
| num\_fail                 | BIGINT  | The number of times the test grouping failed that day.
| total\_pass\_duration\_ns | DOUBLE  | The average duration, in nanoseconds, of tests that passed that day.
| task\_create\_iso         | VARCHAR | The date, in ISO format `YYYY-MM-DD`, the test ran.


