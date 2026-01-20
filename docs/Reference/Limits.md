# Evergreen Limits And Data Retention

Evergreen has different kinds of limits in place. The ones that users are likely
to encounter are listed here.

## Never Expiring Hosts

Evergreen limits users to two never expiring spawn hosts at a time.

### Can an exception be requested?

Exceptions can be requested on a case-by-case which will be granted based on
[our policy](https://mongodb.stackenterprise.co/questions/1122).

## Task and Version TTL

Tasks and versions expire 365 days after creation. Expired tasks will not be available through Evergreen's API or UI, but finished tasks (tasks that ran) will continue to be available in [Trino](../Project-Configuration/Evergreen-Data-for-Analytics).

### Impact on Parser Projects

Projects that serve as child patch parsers (referenced by other projects via trigger aliases) require at least one active version to function properly. If all versions of a parser project expire due to the 365-day TTL, attempts to create patches that reference this project will fail with an error indicating no valid version was found.

**Workaround:** To prevent this issue, ensure that parser projects receive at least one commit within every 365-day period. If a parser project's versions have all expired, push a new commit to the project's repository to create a fresh version and restore patch creation functionality.

## Task Artifacts Data Retention Policy

Artifacts uploaded by tasks using [s3.put](../Project-ConfigurationProject-Commands#s3put) with the default bucket (for example, the s3 bucket created for the project upon project creation) will expire based on the policy of the default bucket of that project. If artifacts were uploaded by specifying a user's S3 bucket with different retention policy to [s3.put](../Project-ConfigurationProject-Commands#s3put), it will follow the retention policy of that bucket.

## Task Output Data Retention Policy

Test results uploaded via [attach.results](../Project-ConfigurationProject-Commands#attachresults), [attach.xunit_results](../Project-ConfigurationProject-Commands#attachxunit_results), and [gotest.parse_files](../Project-ConfigurationProject-Commands#gotestparse_files) will be available via evergreen for 1 year but will continue to be available in [Trino](../Project-Configuration/Evergreen-Data-for-Analytics) for longer.

[Task traces](../Project-Configuration/Task_Traces) are available for 60 days in Honeycomb.

## Task Limits

Evergreen limits tasks per version to 50,000. This includes both selected and
unselected tasks.

## Task Scheduling Limits

Evergreen limits the number of tasks a single user can schedule in patches per
hour to 10,000. If a user has scheduled that many tasks in a given hour-long
time frame, they will either need to wait until that time frame has ended to
continue scheduling tasks, or unschedule tasks to free up space in their
scheduling quota.

## Task Timeout Max

Evergreen does not have a limit on how large task timeouts can be. For different
default timeouts that evergreen has in place, see
[pre and post](../Project-Configuration/Project-Configuration-Files/#pre-and-post)
and
[timout handler](../Project-Configuration/Project-Configuration-Files/#timeout-handler).

## Include Limits

We have a limit of 35
[included files](../Project-Configuration/Project-Configuration-Files/#Include)
per project based on this
[investigation](https://jira.mongodb.org/browse/DEVPROD-3509) about GitHub
usage.

## Config YAML Size Limits

### Current Limitations

Evergreen limits YAML project configuration sizes to 18MB. The 18MB limit is on the sum of the size
of the size of the project configuration YAML, plus all included YAML files, plus any additional
generate.tasks input files. If the YAML length >18MB after task generators have tacked on their
configuration to the project configuration YAML, the task will fail.

### CPU Degraded Mode

Running tasks with large config YAMLs is one of the most computationally intensive operations Evergreen
does. In order to facilitate incremental increases in the maximum allowed config YAML size, Evergreen has a
safeguard mechanism that will automatically detect periods of high CPU load, and reduce the maximum allowed
config YAML size to a previous limit that is known to be safe to proactively prevent performance issues.

When degraded mode is active, the maximum allowed config YAML size will decrease from 18MB to 16MB, and
a maximum number of concurrent running tasks with large config YAMLs will be enforced, potentially causing
their scheduling to slow down.

### Can an exception be requested?

This cannot be lifted because Evergreen cannot safely handle larger file sizes
from multiple users. Users are encouraged to instead try creating a new patch
with fewer variants/tasks.

## Patch size

Evergreen has a 100MB system limit on CLI patch size (the diff size for the
changes). In order to submit patches that are larger than 16MB from the CLI, the
`--large` flag needs to be used. Large patches may hit the CLI’s one minute
timeout. To work around that, users can open a PR and run the patch from there.

## What is the largest file size I can upload to Parsley?

Parsley will not accept files larger than 2.5GB. If you need to upload a file
larger than 2.5GB, we recommend downloading the raw file and splitting the file
into smaller parts, then uploading them separately.

## Task queue wait time limits

A task may wait in its distro queue for at most 7 days, at which point the task
is marked "underwater" and will automatically be unscheduled and disabled.
