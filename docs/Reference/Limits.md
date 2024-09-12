# Evergreen Limits

Evergreen has different kinds of limits in place. The ones that users are likely
to encounter are listed here.

## Never Expiring Hosts

Evergreen limits users to two never expiring spawn hosts at a time.

#### Can an exception be requested?

Exceptions can be requested on a case-by-case which will be granted based on
[our policy](https://mongodb.stackenterprise.co/questions/1122).

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

## YAML configuration size

Large parser projects are disabled, which means that Evergreen limits YAML
project configuration sizes to 16MB. The 16 MB limit is on the sum of the size
of the generate.tasks input file and the size of the project configuration YAML.
If the YAML length > 16 MB after task generators have tacked on their
configuration to the project configuration YAML, the task will fail.

#### Can an exception be requested?

This cannot be lifted because Evergreen cannot safely handle larger file sizes
from multiple users. Users are encouraged to instead try creating a new patch
with fewer variants/tasks.

## Patch size

Evergreen has a 100MB system limit on patch size (the diff size for the
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
