# Controlling When a Task Runs

There are multiple ways to control the scheduling of builds/tasks on a project's waterfall page.

In short:

**Activate**: if set to false, prevents Evergreen from automatically activating a task. It can still be manually activated by a user. If set to true, it can override batchtime in the project settings.

**Cron:** activates builds/tasks on existing mainline commits based on a specified schedule.

**Batchtime:** sets an interval of time in minutes that Evergreen should wait before activating builds/tasks. It will only activate the build/tasks for latest commit.

**Periodic Builds:** creates a _new version_ with specified variants/tasks at a specified interval, regardless of commit activity.

If more than one is set, more specific details on how these features interact with each other are found
[here](Project-Configuration-Files#specific-activation-override-hierarchy).

### Activate
`activate: false` prevents a build variant or task from activating automatically. This can be specified in the
buildvariants section of the project configuration file on a build variant or a task within the build variant.

`activate: true` is a special flag that is only usable for the purpose of overriding a batchtime defined in the project
settings. Instead of using the project settings batchtime, the build variant or task will activate immediately. It does
not have any other effect.

### Cron

Cron activates build variants or tasks on existing mainline commits based on a specified schedule using UTC timezone and [cron syntax](https://crontab.guru/) or descriptors such as [@daily](https://pkg.go.dev/github.com/robfig/cron). For example, if set up to run daily, it’ll activate the most recent build variant at that time daily (it will not create any new tasks, only activate existing ones). This is ideal for activating tasks/variants based on regular intervals tied to project commit activity.

Cron can be specified in the buildvariants section in the project configuration file on a build variant or task level.

#### Example

```yaml
buildvariants:
- name: the-main-bv
  display_name: The Main BV
  cron: 0 * * * *
  run_on:
  - my-distro
  tasks:
  - name: first_test
  - name: second_test
    cron: '@daily' # overrides build variant cron
```

### Batchtime

Batchtime sets an interval of time in minutes that Evergreen should wait before activating a version/task/variant. This is ideal for delaying activation of versions/tasks/variants to batch them together, reducing the frequency of activations and managing resource usage.

A default batch time can be set on the project page [under general settings](../Project-Configuration/Project-and-Distro-Settings/#general-project-settings) for the interval of time (in minutes) that Evergreen should wait in between activating the latest version.

Batchtime can also be specified in the buildvariants section in the project configuration file on an entire build variant or for a single task in the build variant task list.

#### Example

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

### Periodic Builds

Periodic builds will create a new version (viewable on the project's waterfall page) with the tasks/variants you specify at the interval you specify, regardless of whether there are new commits. For example, if set up to run daily, a new periodic build will be created each day. This is ideal if you want to run builds on a consistent schedule, regardless of commit activity.
Periodic builds cannot be used with performance tooling, like performance monitoring charts.

Periodic builds are set up on the project settings page under the periodic builds section. For more information on how to set up periodic builds, please see [periodic builds](../Project-Configuration/Project-and-Distro-Settings#periodic-builds).
