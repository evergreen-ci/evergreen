Overview
========

This document details the decisions to be made when deciding to degrade (ie.
disable) certain functions of Evergreen. The intended audience is any superuser
of Evergreen.

In general, you should degrade a specific service of Evergreen when continuing
to run that service would do more harm than good, or would create a mess that
needs to be cleaned up afterwards. Degrading any one service has very few direct
effects on other components in the system, so you should feel confident that
choosing to degrade a service would not cause unpredictable side effects. Any
superuser of the system should feel empowered to make these changes if they
think that it is appropriate.

The interface to enable/disable each Evergreen service is available in the
Evergreen UI from the Admin Settings page of the dropdown in the top right, and
is also available from the "evergreen admin" CLI commands. Any changes to the
service flags should be communicated to other superusers. If making a change
that will be noticed by end-users (such as disabling task dispatch), the banner
message should be set as well.


Services
========

The following are considerations for turning off specific services:


Task Dispatch
-------------

Disabling task dispatch will prevent agents from getting new tasks and getting
new agent binaries.

Disable task dispatch in scenarios where there are widespread problems with the
agents such that tasks would either not execute correctly or report incorrect
results. If the problem is something that can be quickly and safely fixed,
consider instead deploying the fix and forcing agents to roll over to the new
revision.

Enable task dispatch when the problem is fixed or will be fixed by getting a new
agent revision.


Host Initialization
-------------------

Disabling hostinit will prevent dynamic hosts from starting and will prevent
started hosts from running their setup script. Static hosts and dynamic hosts
that are already running will be unaffected.

Disable hostinit if:
  - The cloud provider for dynamic hosts is experiencing a downtime
  - Requests to the provider cannot be sent correctly
  - There is a problem such that Evergreen is starting more hosts than needed,
    or more hosts than can be provisioned before the monitor kills them
  - The runner is not able to ssh into dynamic hosts

Enable hostinit when the system and the cloud provider are stable enough such
that hosts can start and be set up correctly.


Monitoring
----------

Disabling the monitor will stop it from automatically cleaning up stranded tasks
and stop it from terminating hosts.

Disable the monitor if:
  - Task heartbeats aren't being correctly tracked in the database
  - The runner is not able to ssh into dynamic hosts
  - It is too aggressively terminating hosts. Keep in mind that disabling the
    monitor entirely to try and keep hosts up longer will also disable desirable
    host termination policies, such as terminating hosts that are idle.

Enable the monitor when tasks and hosts are sufficiently stable. In general, the
monitor should stay enabled.


Notifications
-------------

Disabling notifications will stop users from receiving emails about build and
task status changes.

Disable notifications if:
  - The SMTP server is experiencing downtime
  - There is a problem in the system causing users to get an excessive amount of
    notifications

Enable notifications once emails can be delivered and are being sent in the
correct quantities.


Alerts
------

Disabling alerts will stop admins from receiving emails about potential system
issues.

Disable alerts if:
  - The SMTP server is experiencing downtime
  - There is a problem in the system causing admins to get an excessive amount
    of notifications

Enable alerts once emails can be delivered and are being sent in the correct
quantities.


Task Runner
-----------

Disabling the taskrunner will prevent new hosts from getting agents started on
them.

Disable the taskrunner if:
  - The current agent revision has a critical bug that will not be fixed soon
  - The runner is not able to ssh into dynamic hosts

Enable the taskrunner once hosts are reachable and the current agent revision is
the correct one to put on hosts.


Repo Tracker
------------

Disabling the repotracker will prevent Evergreen from getting recent commits
from Github.

Disable the repotracker if:
  - Github is experiencing downtime
  - Github APIs are not functioning correctly
  - There is a problem in the system causing incorrect versions to be created

Enable the repotracker once Github and the system are both stable and versions
should be created again.


Scheduler
---------

Disabling the scheduler will prevent new tasks from being added to the task
queues and will also prevent new hosts from starting.

Disable the scheduler if:
  - There is a problem in the system such that too many new hosts are being
  requested
  - Agents should complete the existing tasks in the queues but not receive any
    tasks not already in queues. Note that this is different from disabling task
    dispatch because agents will still get new tasks until the queues are empty.

Enable the scheduler once task queue behavior is back to normal and the correct
number of hosts are being requested.
