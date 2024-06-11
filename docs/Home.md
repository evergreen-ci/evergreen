[go/evergreen-docs](http://go/evergreen-docs)

# What is Evergreen?

## Evergreen
Evergreen is a continuous integration system built by MongoDB.

It utilizes multi-platform cloud computing to run tests as quickly as possible on as many platforms as possible.

### Features
##### Elastic Computing
Evergreen was built to scale with load and provide optimal test parallelism.

When there are lots of commits coming in, Evergreen can spin up new hosts to run them; when the hosts are no longer needed, Evergreen shuts them down.

Use only what you need.

##### Multiplatform Support
Evergreen is built in Go.
Your tests can run on any machine that Evergreen can cross-compile its agent for, so it can run on all of Go's myriad supported operating systems and architectures, including
 1. Linux
 2. OSX
 3. Windows
 4. FreeBSD

on x86, amd64, and arm architectures.

##### Serial Execution
Tasks (i.e. sets of tests) run serially on hosts, meaning you can have confidence that only your tests are running on a host at any given time.

##### Patch Builds
You shouldn't have to wait until after you've committed to find out your changes break your tests.
Evergreen comes with the powerful feature of pre-commit patch builds, which allow you to submit uncommitted changes and run them against the variants and test suites you need.
These patch builds help alleviate the stress of multi-platform development and long-running tests by putting Evergreen's elastic, multi-platform computing right at your team's fingertips.

##### Easy Configuration
Evergreen is flexible.
If you can do it in a shell script, you can run it with Evergreen.
Internally, MongoDB uses Evergreen for everything from testing the main server, to compiling our documentation, creating Amazon AMIs, and even testing Evergreen itself.
Projects are defined with simple yaml config files, see [here](https://github.com/evergreen-ci/sample/blob/master/evergreen.yml) for an example.

### How It Works
Evergreen monitors a set of github repositories, waiting for new commits.
When a new commit comes in or enough time has passed, Evergreen schedules builds for different variants (different OSes, compile flags, etc).
Once builds are scheduled, an internal heuristic decides how many cloud machines to allocate in order to get tests done as quickly and cheaply as possible.

Evergreen sends an executable agent binary to each test machine, which begins running its given task.
Logs are streamed back to the main server, along with system statistics and formatted test results.

## Spruce: The New Evergreen UI

Evergreen's UI has a few different levels of information for a given patch build. The level of hierarchy determines which information gets surfaced on the page. Although this is true to some extent in both the legacy UI and the new UI, the new UI strives to keep only information relevant to the current level on the page.

The top-most level for a given patch build is the [patch details page](https://spruce.mongodb.com/version/60b68d7da4cf47179e15accf/tasks?sorts=STATUS%3AASC%3BBASE_STATUS%3ADESC). This page contains information about the patch overall, e.g., patch title, which files are changed as of the patch, and which tasks are included in the patch.

The next level is the [task details page](https://spruce.mongodb.com/task/mongodb_mongo_master_enterprise_rhel_80_64_bit_dynamic_all_feature_flags_required_jsCore_patch_0ec70f6ac70716d9296a014d52e4cc99bf4e5695_60b68d7da4cf47179e15accf_21_06_01_19_43_26/logs?execution=0), which contains information relevant to that specific task. This includes ETA if the task is running, estimated time to start and position in the task queue if the task is scheduled to run, as well as any results of the task if it has already run. Notably, the information about which files were changed in the patch are not included on this page, although they are included on the corresponding page in the legacy UI.

Here are some resources to get started with Evergreen's Project health page: Here are some resources that demonstrate the new UI: [Project Health](https://app.tango.us/app/workflow/Evergreen--Onboarding-guide-for-the-new-project-health-page--7b74b28c80f448869a01730a450bc246) (Waterfall), [Filtering by status](https://app.tango.us/app/workflow/Status-icon-behavior--1db9909b454f4800b05774fa408f2924), [Variant history page](https://app.tango.us/app/workflow/Variant-History-fa73d48662f24e48842fc315130c483f), and [Task history page](https://app.tango.us/app/workflow/Task-History--23e6b3f043234a19988d6ab0a0729598).
