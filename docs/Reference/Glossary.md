# Glossary

* **build**: A build is a set of tasks in a single build variant in a single version.
* **build variant**: A build variant is a distro configured in a project. It contains a distro name and some metadata.
* **command**: A tasks runs one or more commands. There are separate commands to do things like run a program or upload a file to s3.
* **display task**: A display task is a set of tasks grouped together in the UI by the project configuration. Display tasks have no set distro.
* **distro**: A distro is a set of hosts that runs tasks. A static distro is configured with a list of IP addresses, while a dynamic distro scales with demand.
* **expansions**: Expansions are Evergreen- and user-specified variables that can be used in the project configuration file. They can be set by the `expansions.update` command, in the variant, on the project page, and on the distro page.
* **function**: Commands may be grouped together in functions.
* **patch build**: A patch build is a version not triggered by a commit to a repository. It either runs tasks on a base commit plus some diff if submitted by the CLI or on a git branch if created by a GitHub pull request.
* **project configuration file**: The project configuration file is a file parsed by Evergreen that defines commands, functions, build variants, and tasks.
* **stepback** When a task fails and the offending commit is unknown, Evergreen will perform linear or bisection stepback depending on the project settings. Linear incrementally runs the same task in previous versions O(n) while bisection performs binary search O(logn).
* **task group**: A task group is a set of tasks which run with project-specified setup and teardown code without cleaning up the task directory before or after each task and without any tasks running in between tasks in the group.
* **task**: The fundamental unit of execution is the task. A task corresponds to a box on the waterfall page.
* **test**: A test is sent to Evergreen in a known format by a command during a task, parsed by Evergreen, and displayed on the task page.
* **user configuration file**: The user configuration file is a file parsed by the Evergreen CLI. It contains the user’s API key and various settings.
* **version**: A version, which corresponds to a vertical slice of tasks on the waterfall, is all tasks for a given commit or patch build.
* **working directory**: The working directory is the temporary directory which Evergreen creates to run a task. It is available as [an expansion](../Project-Configuration/Project-Configuration-Files.md#default-expansions).
