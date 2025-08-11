# Controlling When a Task Runs on the Waterfall

By default, the latest commit on the waterfall will be activated each time our activation job is run. (That means if many commits are made in a short period of time,
only the latest commit will be activated.)

By default, every task that is added to at least one build variant's tasks list will run on every commit on the waterfall. However, there are multiple ways to control the scheduling of builds/tasks on a project's _waterfall_ page. This means that tasks manually scheduled by a PR or patch will run regardless of cron/batchtime settings.

In short:

**Activate**: if set to false, this prevents Evergreen from automatically activating a task in the waterfall, which includes crons. If activate is set to false and it can still be manually activated by a user. If set to true, it can override batchtime in the project settings.

**Cron:** activates builds/tasks on existing mainline commits based on a specified schedule. If used with activate set to false, the cron setting will not activate the specified builds/tasks on the waterfall.

**Batchtime:** sets an interval of time in minutes that Evergreen should wait before activating builds/tasks on mainline commits. It will only activate the build/tasks for latest commit. If used with activate true, batchtime will be ignored and the builds/tasks will run every time.

**Periodic Builds:** creates a _new version_ with specified variants/tasks at a specified interval, regardless of commit activity. (This is not a yaml setting.)

The yaml settings **only apply to mainline commits.** If more than one is set, more specific details on how these
features interact with each other are found [here](Project-Configuration-Files#specific-activation-override-hierarchy).
Documentation on limiting when tasks runs beyond the waterfall can be found [here](Project-Configuration-Files#limiting-when-a-task-or-variant-will-run).

## Activate

`activate: false` prevents a build variant or task on a mainline commit from activating automatically. This can be specified in the
buildvariants section of the project configuration file on a build variant or a task within the build variant. If a cron job wants to activate a build/task but also has `activate` set to false, the build/task will not run.

`activate: true` is a special flag that is only usable for the purpose of overriding a batchtime defined in the project
settings. Instead of using the project settings batchtime, the build variant or task will activate immediately. It does
not have any other effect.

## Cron

Cron activates build variants or tasks on existing mainline commits based on a specified schedule using UTC timezone and [cron syntax](https://crontab.guru/) or descriptors such as [@daily](https://pkg.go.dev/github.com/robfig/cron). For example, if set up to run daily, it’ll activate the most recent build variant at that time daily (it will not create any new tasks, only activate existing ones). This is ideal for activating tasks/variants based on regular intervals tied to project commit activity. Cron will fail to activate builds/tasks if `activate` is set to false.

Cron can be specified in the buildvariants section in the project configuration file on a build variant or task level.

### Example

```yaml
buildvariants:
  - name: the-main-bv
    display_name: The Main BV
    cron: 0 12 * * * # at 12:00 every day
    run_on:
      - my-distro
    tasks:
      - name: first_test
      - name: second_test
        cron: "@daily" # overrides build variant cron
```

In the example above, when a mainline commit is triggered at 10:00, it will not initially schedule any tasks in `the-main-bv`. Let's also say that there was another mainline commit triggered at 11:00.

At 12:00, Evergreen's cron jobs will look for the latest mainline commit, which happens to be the one made at 11:00 in this example. Then, Evergreen will activate `first_test` task in the mainline commit that was created in 11:00 because the cron settings specify that the task should run at 12:00 every day.

Similarly, the `second_test` task will be scheduled at 0:00 on the latest mainline commit at the time due to the `@daily` cron.

## Batchtime

Batchtime delays a mainline task/variant activation until a specified time has passed since its last run. This is useful for projects with high commit activity, as it will prevent Evergreen from activating tasks/variants too frequently which can lead to resource contention and inefficiencies.

E.g.: Task 'A' has a batchtime of 60 minutes. The first commit of the day is at 10:00AM and the task activates immediately. A new mainline commit is made at 10:30AM. Evergreen will not activate task 'A' on the new mainline commit until 11:00AM, which is 60 minutes after the last run. If a new mainline commit is made at 10:45AM, Evergreen will still wait until 11:00AM to activate task 'A' on this newest mainline commit.

A default batch time can be set on the project page [under general settings](../Project-Configuration/Project-and-Distro-Settings/#general-project-settings) for the interval of time (in minutes) that Evergreen should wait in between activating the latest version.

Batchtime can also be specified in the buildvariants section in the project configuration file on an entire build variant or for a single task in the build variant task list.

### Example

```yaml
buildvariants:
  - name: the-main-bv
    display_name: The Main BV
    batchtime: 60
    run_on:
      - my-distro
    tasks:
      - name: first_test
      - name: second_test
        batchtime: 20 # overrides build variant batchtime of 60
```

For more on cron and batchtime, see [build variants](../Project-Configuration/Project-Configuration-Files/#build-variants).

## Periodic Builds

Periodic builds will create a new version (viewable on the project's waterfall page) with the tasks/variants you specify at the interval you specify, regardless of whether there are new commits. For example, if set up to run daily, a new periodic build will be created each day. This is ideal if you want to run builds on a consistent schedule, regardless of commit activity.
Periodic builds cannot be used with performance tooling, like performance monitoring charts.

Periodic builds are set up on the project settings page under the periodic builds section. For more information on how to set up periodic builds, please see [periodic builds](../Project-Configuration/Project-and-Distro-Settings#periodic-builds).
