# Notifications

These are for setting up personal user notifications. For project-level notifications, see [project settings](Project-and-Distro-Settings#project-level-notifications).

Evergreen has the ability to issue notifications based on events that happen in the system.

## Notifications Setup
Evergreen can notify you based on the outcome of your patches. To enable these, go to your [user profile](https://evergreen.mongodb.com/settings) and:
1. Enter your GitHub Username (so we can identify which commits you make)
2. Optionally, enter your Slack username (if you want to be notified on Slack). Your Slack username is available from https://YOUR_ORGANIZATION.slack.com/account/settings#username (This is NOT your slack display name.)
3. Under Notification Settings, select the Slack or Email next to the notifications you'd like.
4. Click save!

## Options for Notifications

### Patch Finish
For all new patches you create in the future (including GitHub Pull Requests), you'll receive an email or slack message when the patch has completed.

### Tasks
For new tasks that fit the desired requester and finish type, you'll receive a notification. Note that for system unresponsive tasks, we only send a notification on the last execution, since we auto-retry these.

### Spawn Host Outcome
For your spawn hosts, you will receive notifications when a host is started, stopped, modified, or terminated. You will also receive notifications when your project-specific host setup script succeeds or fails to run on the spawn host.

### Spawn Host Expiration
Receive notifications that your spawn host is going to expire soon, so you can update expiration accordingly if you don't want to lose the host.

### Build Break Notifications
Project Admins may enable this at the project level.

When a task that was previously green (or is a new), becomes red or purple, a build-break notification will be issued. If the original committer can be identified, Evergreen will lookup their build-break notification settings, and follow that. 

If we can't identify the original committer, Evergreen will notify project admins.

### Filtering Emails and Webhooks
Evergreen sets a handful of headers which can be used to filter emails or webhook posts.

|          Header             | Meaning |
| --------------------------- | --- |
| `X-Evergreen-id`              | a unique ID for every notification. It is safe to use this value for deduplication. |
| `X-Evergreen-object`          | The object that generated this notification. Valid values are: `patch`, `version`, `build`, `task`, `host` |
| `X-Evergreen-project`         | The Evergreen project that created this notification. For example, a notification created by MongoDB's master branch would have the value of `mongodb-mongo-master` |
| `X-Evergreen-owner`           | The id of the Evergreen user that created the object. For events created by repotracker, if the object can be attributed to an Evergreen user, the Owner will be that user. |

### Warning to GMail Users
If you're using GMail through the browser UI, you won't be able to filter notifications because GMail does not support filtering on custom headers. Instead, we inject the custom Evergreen headers into the body of the email and hide it from view. You can create a filter in GMail using the "Has the words" field.

For example, if you want to create a filter that captures all email notifications in the `mci` project, add `X-Evergreen-project:mci` into this field. Do not put spaces before or after the colon. 

To add more than one filter, separate the terms with a space, e.g: `X-Evergreen-project:mci X-Evergreen-object:patch` would capture patch notifications in the "mci" project. Order does not matter.

