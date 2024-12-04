# CLI

How to set up and use the command-line tool
==

Downloading the Command Line Tool
--

Go to your [evergreen user settings page](https://spruce.mongodb.com/preferences) and follow the steps there.
Copy and paste the text in the configuration panel on the settings page into a file in your *home directory* called `.evergreen.yml`, which will contain the authentication information needed for the client to access the server.

On macOS, the evergreen binary is currently not notarized. To allow running it, go to System Preferences, then Security and Privacy. You should be able to make an exception for it in the "General" tab.

Basic Patch Usage
--

`evergreen patch` allows you to submit patches to test your local changes. It will also check your project YAML any for validation errors before submission. If you want to view warnings, look at the [validate](#validating-changes-to-config-files) command.

To submit a patch, run this from your local copy of the mongodb/mongo repo:
```bash
evergreen patch -p <project-id>
```
    
Variants and tasks for a patch can be specified with the `-v` and `-t`:
```bash
evergreen patch -v enterprise-suse11-64 -t compile
```

Multiple tasks and variants are specified by passing the flags multiple times:
```bash
evergreen patch -v enterprise-suse11-64 -v solaris-64-bit -t compile -t unittest -t jsCore
```

_Every_ task or variant can be specified by using the "all" keyword:
```bash
evergreen patch -v all -t all
```

Tasks and variants can also be specified using the regex variants(-rv) and regex tasks(-rt) flags:
```bash
evergreen patch --regex_variants "enterprise.*" --rt "test-.*"
```

When specifying tasks or variants, _both_ must be specified:
```bash
evergreen patch --regex_variants "enterprise.*" // not valid, will not select any tasks
evergreen patch -t unittest // not valid, will not select any tasks
evergreen patch -rv "enterprise.*" --regex-tasks test-.* // valid
evergreen patch --regex_variants "enterprise.*" -t unittest // valid
```

To use the same tasks and variants defined for the previous patch created for this project, you can use the `--reuse` flag. If any tasks/variants were defined for the previous patch but do not exist for the new patch, they will not be added. Note also that aliases will not be re-calculated; this is so if an alias had been given to the previous patch but then the user chose to tweak the specific tasks/variants, the final configuration is the one that we reuse.
```
evergreen patch --reuse
```
To repeat a specific patch id, you can use the '--repeat-patch' flag.
```
evergreen patch --repeat-patch <patch_id>
```
Similarly, using the `--repeat-failed` flag will perform the same behavior as the `--reuse` flag and by default use the last patch as a reference, with the only difference being that it will repeat only the failed tasks and build variants from the most recent patch (if any failures exist).
```
evergreen patch --repeat-failed
```
To repeat the failed of a specific patch, the '--repeat-failed' flag can be used with the '--repeat-patch' flag to specify the patch id.
```
evergreen patch --repeat-failed --repeat-patch <patch_id>
```

To skip all (y/n) prompts, the `-y` keyword can be given:
```
evergreen patch -y
```

To use local changes for an included file from a module, the `--include-modules` flag can be used:
Note that `set-module` command will not work for module includes and this flag must be used instead.
```
evergreen patch --include-modules
```

Working Tree Changes
---
By default patches will include only committed changes, not changes in Git's working tree or index. To include changes from the working tree use the `--uncommitted, -u` flag or set a default by inserting `patch_uncommitted_changes: true` in the `~/.evergreen.yml` file.

Defaults
---
The first time you run a patch, you'll be asked if you want to set the given inputs such as tasks or variants as the default for that project.
After setting defaults, you can omit the flags and the default values will be used, so that just running `evergreen patch` will work.

Defaults may be changed at any time by editing your `~/.evergreen.yml` file.

Additionally, the default project for a directory is also tracked by the first successful patch you perform in that directory. Symlinks are resolved to their absolute path. The defaults are maintained in the `~/.evergreen.yml` file, under the `projects_for_directory` key. The value for this key is a map, where the map keys are absolute paths, and the map values are project identifiers. The automatic defaulting can be disabled by setting disable_auto_defaulting to true.

Prompts
---
Many prompts will ask for a y/n (i.e. yes/no) response. If you hit enter, we will default to yes if the prompt specifies Y/n, and no if the prompt specifies y/N.

Advanced Patch Tips
--

##### Multiple Defaults
While the `evergreen` program has no built-in method of saving multiple configurations of defaults for a project, you can easily mimic this functionality by using multiple local evergreen configurations.
The command line tool allows you to pass in a specific config file with `--config`:
```bash
evergreen --config ~/.many_compiles.yml patch
```
You can use this feature along with shell aliasing to easily manage multiple default sets.

For example, an enterprising server engineer might create a config file called `tests.yml` with the content
```yaml
api_server_host: #api
ui_server_host: #ui
api_key: #apikey
user: #user
projects:
- name: mongodb-mongo-master
  variants:
  - windows-64-2k8-debug
  - enterprise-rhel-62-64-bit
  tasks:
  - all
```
so that running `evergreen --config tests.yml patch` defaults to running all tasks for those two variants.

You might also want to create a config called `compile.yml` with
```yaml
api_server_host: #api
ui_server_host: #ui
api_key: #apikey
user: #user
projects:
- name: mongodb-mongo-master
  variants:
  - windows-64-2k8-debug
  - enterprise-rhel-62-64-bit
  - solaris-64-bit
  - enterprise-osx-108 #and so on...
  tasks:
  - compile
  - unittests
```
for running basic compile/unit tasks for a variety of platforms with `evergreen --config compile.yml patch`.
This setup also makes it easy to do scripting for nice, automatic patch generation.

##### Git Diff
Extra args to the `git diff` command used to generate your patch may be specified by appending them after `--`.  For example, to generate a patch relative to the previous commit:

      evergreen patch -- HEAD~1

Or to patch relative to a specific tag:

      evergreen patch -- r3.0.2

Though keep in mind that the merge base must still exist in the canonical GitHub repository so that Evergreen can apply the patch.


The `--` feature can also be used to pass flags to `git diff`.


#### Task Sync
The task sync feature allows you to upload and download a task's working directory to and from Amazon S3 at the end of a patched task. This can be useful for debugging since it allows you to see the exact files used in the task. In order to use this for patches, it must be enabled in the project settings.

To specify which tasks you wish to sync, include the `--sync_variants` and `--sync_tasks` flags. For example:

      evergreen patch -p <project_id> -t <task_id1> -t <task_id2> -v <variant1> --sync_tasks <task_id1> --sync_variants <variant1>

There is an option to specify a task status for which the sync will run ("success" or "failed") using `--sync_statuses`. A maximum timeout can be specified for the sync before it gets canceled using `--sync_timeout`.

Once the task finishes syncing, the task directory contents can be pulled into a spawn host by requesting it with the host or it can be manually pulled onto the local machine using [evergreen pull](#pull).


#### Local Aliases
Users can define local aliases in their `evergreen.yml` files and even override a patch alias defined by a project admin. Local aliases are defined at the project level. 

```yaml
api_server_host: #api
ui_server_host: #ui
api_key: #apikey
user: #user
projects:
- name: mongodb-mongo-master
  variants:
  - windows-64-2k8-debug
  - enterprise-rhel-62-64-bit
  - solaris-64-bit
  - enterprise-osx-108 #and so on...
  local_aliases:
  - alias: "alias_name"
    variant: ".*"
    task: "^compile$,tests$"
  - alias: "alias_using_tags"
    task_tags:  [ "test", "!smoke" ]
    variant_tags: ["small"]
  tasks:
  - compile
  - unittests
```
Calling the command:

      evergreen patch -a alias_name
 
will use the above local alias and schedule every variant with tasks named "compile" and tasks that end with "tests".

Operating on existing patches
--

To list patches you've created:

      evergreen list-patches


##### To cancel a patch:
 
```
evergreen cancel-patch -i <patch_id>
```
    
##### To finalize a patch:
 
```
evergreen finalize-patch -i <patch_id>
```
Finalizing a patch actually creates and schedules and tasks. Before this the patch only exists as a patch "intent". You can finalize a patch either by passing --finalize or -f or by clicking the "Schedule Patch" button in the UI of an un-finalized patch.

##### To create a patch and add module changes in one command:
```
evergreen patch --include-modules
```
This will attempt to add changes for each module that your project supports. This flag will prompt you to provide your local absolute path to the module, and it will be stored in your evergreen.yml file. For example:
```
projects:
    - name: my_favorite_project
      module_paths:
        my_favorite_module: /Users/first.last/go/src/github.com/my_favorite/module
```

We will then check that directory for changes, confirm them with you, and add them to the patch if confirmed. If there are modules you don’t want to include you can skip them and still continue to check others, or if there are no changes we’ll skip them automatically.
(Note: we won’t set this path for you if you have disable_auto_defaulting set in your evergreen.yml, in which case you will need to add it manually, following the format above.)

##### To add changes to a module on top of an existing  patch:

```
cd ~/projects/module-project-directory
evergreen set-module -i <patch_id> -m <module-name>
```
Note: `set-module` must be run before finalizing the patch.

##### Validating changes to config files

When editing yaml project files, you can verify that the file will work correctly after committing by checking it with the "validate" command.
To validate local changes within modules, use the ``local_modules`` flag to list out module name and path pairs.

Note: Must include a local path for includes that use a module.

```
evergreen validate <path-to-yaml-project-file> -lm <module-name>=<path-to-yaml>
```

The validation step will check for
   * valid yaml syntax
   * correct names for all commands used in the file
   * logical errors, like duplicated variant or task names
   * invalid sets of parameters to commands
   * warning conditions such as referencing a distro pool that does not exist
   * merging errors from include files

Note: validation is server-side and requires a valid evergreen configuration file (by default located at ~/.evergreen.yml). If the configuration file exists but is not valid (malformed, references invalid hosts, invalid api key, etc.) the `evergreen validate` command [will exit with code 0, indicating success, even when the project file is invalid](https://jira.mongodb.org/browse/EVG-6417). The validation is likely not performed at all in this scenario. To check whether a project file is valid, verify that the process exited with code 0 and produced the output "\<project file path\> is valid".

Additionally, the `evaluate` command can be used to locally expand task tags and return a fully evaluated version of a project file.
(Note that this command doesn't support evaluating included files from modules.)

```
evergreen evaluate <path-to-yaml-project-file>
```

Flags `--tasks` and `--variants` can be added to only show expanded tasks and variants, respectively.

Basic Host Usage
--
Evergreen Spawn Hosts can now be managed from the command line, and this can be explored via the command line `--help` arguments. 

### Attaching an EBS Volume

To create a new EBS volume: 
```
evergreen volume create --size <size> --type <default=gp2> --zone <default=us-east-1a> 
```
While the Availability Zone does have a default, this must be in the _same zone as the host_. If you don't know your host's availability zone, this can be found easily at `evergreen host list --mine`. 

To attach the volume to your host (assuming the same availability zone), use:
```
evergreen host attach --host <host_id> --volume <volume_id>
```
If you forget your volume ID, you can find this with `evergreen volume list`. If the volume is already attached, you will see a host ID given with this volume, and a volume can only have one host. 

A volume can only be deleted if detached, so removing a volume would for example be: 
``` 
evergreen host detach --host <host_id> --volume <volume_id>
evergreen volume delete --id <volume_id>
```

### Modify Hosts

Tags can be modified for hosts using the following syntax:
```
evergreen host modify --tag KEY=VALUE
evergreen --delete-tag KEY
```
Note these tags cannot overwrite Evergreen tags. 

Hosts can be set to never expire using the `--no-expire` option. Keep in mind that if making a host unexpirable from the
CLI, you should also set up a [sleep schedule](Hosts/Spawn-Hosts#unexpirable-host-sleep-schedules) from the command line
as well; if you don't set one, your unexpirable host will be automatically assigned a default sleep schedule. For
example, this command will make a host unexpirable and defines a sleep schedule so the host is on from 9 am to 5 pm
between Monday and Friday in Eastern Time:

```sh
evergreen host modify --host "<HOST_ID>" --no-expire --daily-start '09:00' --daily-stop '17:00' --weekdays-off Saturday --weekdays-off Sunday --timezone "America/New_York"
```

Hosts can be set to expire again using the `--expire` option, which will set the host to expire in 24 hours. This
expiration can be extended using `--extend <hours>`, and you can extend its lifetime up to a max of 30 days past host
creation. There are limits on the number of spawn hosts (expirable or unexpirable) that a user can have at once.


### Stop/Start Host to Change Instance Type

Instance type can only be changed if the host is stopped. Hosts can be stopped and started using `evergreen host start/stop --host <id> --wait <set-to-block>`. To change instance type, `host modify --type` (approved types can be configured from the admin settings). 

### Run a script on a host
Run a bash script on a host.
```
evergreen host exec --host <host_id> --script <bash script>
```
This is useful to unblock a host when it can't be reached over SSH.

Other Commands
--

#### Get Update

The command `evergreen get-update` fetches the latest version of the Evergreen CLI binary if the current binary is out of date on a given machine.

Example that downloads the binary:
```
evergreen get-update --auto
```

Specify the optional `--auto` argument to enable automatic CLI upgrades before each command if your current binary is out of date. Once this is done, all future commands will auto update if necessary without the need for specifying this flag.

#### Fetch

The command `evergreen fetch` can automate downloading of the binaries associated with a particular task, or cloning the repo for the task and setting up patches/modules appropriately.

Example that downloads the artifacts for the given task ID and cloning its source:
```
evergreen fetch -t <task-id> --source --artifacts
```

Specify the optional `--dir` argument to choose the destination path where the data is fetched to; if omitted, it defaults to the current working directory.

#### Pull

The command `evergreen pull` can download the task directory contents of a synced task after an [s3.push](Project-Configuration/Project-Commands#s3push) command has finished or after a [patched task that has requested task sync](#task-sync) has completed.

Example that downloads the artifacts for the given task ID and cloning its source:
```
evergreen pull --task <task_id>
```

Specify the optional `--dir` argument to choose the destination path; if omitted, it defaults to the current working directory.
 
#### List

The command `evergreen list` can help you determine what projects, variants, and tasks are available for patching against.
The commands
```
evergreen list --projects
evergreen list --tasks -p <project_id>
evergreen list --variants -p <project_id>
evergreen list --patch-aliases -p <project_id>
evergreen list --trigger-aliases -p <project_id>
```
will all print lists to stdout.

The list command can take an optional `-f/--file` argument for specifying a local project configuration file to use instead of querying the Evergreen server for `-p/--project`.


#### Last Green

The command `evergreen last-green` can help you find an entirely successful commit to patch against.
To use it, specify the project you wish to query along with the set of variants to verify as passing. 
```
evergreen last-green -p <project_id> -v <variant1> -v <variant2> -v <variant...>
```

A run might look something like
```
evergreen last-green -p mci -v ubuntu

   Revision : 97ac269b1e5cf0961fce5bcf985f01c263911efb
    Message : EVG-795 no longer treat conflicting targets as system failures
       Link : https://evergreen.mongodb.com/version/mci_97ac269b1e5cf0961fce5bcf985f01c263911efb

```

#### Tasks
The command `evergeen task` contains subcommands for interacting with task run data, including task output (build) data.

```
# Fetch task logs
evergreen task build TaskLogs --task_id <task_id> --execution <execution> --type <task_log_type>

# Fetch test logs
evergreen task build TestLogs --task_id <task_id> --execution <execution> --log_path <test_log_path>
```

### Server Side (for Evergreen admins)

To enable auto-updating of client binaries, add a section like this to the settings file for your server:


```yaml
api:
    clients:
        latest_revision: "c0110ba937047f99c9a68470f6ec51fc6d98c5cc"
        client_binaries:
           - os: "darwin"
             arch: "amd64"
             url: "https://.evergreen"
           - os: "linux"
             arch: "amd64"
             url: "https://.evergreen"
           - os: "windows"
             arch: "amd64"
             url: "https://.evergreen.exe"
```

The "url" keys in each list item should contain the appropriate URL to the binary for each architecture. The "latest_revision" key should contain the githash that was used to build the binary. It should match the output of `evergreen version` for *all* the binaries at the URLs listed in order for auto-updates to be successful.

### Notifications

The Evergreen CLI has the ability to send slack and email notifications for scripting. These use Evergreen's account, so be cautious about rate limits or being marked as a spammer.

```
# Send a Slack message
evergreen notify slack --target <#channel or @user> --msg <message>

# Send an email
evergreen notify --from <sender> --recipients <to> --subject <subject> --body <body>
```
