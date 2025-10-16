# Project Commands

Project Commands are the fundamental units of functionality in an Evergreen task.

## Basic Command Structure

```yaml
- command: shell.exec
  display_name: run my cool script ## optional
  type: system ## optional
  timeout_secs: 10 ## optional
  retry_on_failure: true ## optional
  failure_metadata_tags: ["tag0", "tag1"] ## optional
  params:
    script: echo "my script"
```

Explanation:

- `command`: a command name from the predefined set of commands documented below.
- `display_name`: an optional user defined display name for the command. This will show up in logs and in the UI
  with more details, for example:`'shell.exec' ('run my cool script') (step 1 of 1)`
- `type`: an optional command type. This will affect the [failure colors](Project-Configuration-Files#command-failure-colors)
- `timeout_secs`: an optional timeout that will force the command to fail if it stays "idle" for more than a specified number of
  seconds.
- `retry_on_failure`: an optional field. If set to true, it will automatically restart the task upon failure. The
  automatic restart will process after the command has failed and the task has completed its subsequent post task commands.
- `failure_metadata_tags`: an optional set of tags to attribute to the command if it fails. If these are set and the
  command fails, the tags will appear in the task details returned from the REST API.
- `params`: values for the pre defined set of parameters the command can take. Available parameters vary per command.

## archive.targz_extract

`archive.targz_extract` extracts files from a gzipped tarball.

```yaml
- command: archive.targz_extract
  params:
    path: "jstests.tgz"
    destination: "src/jstestfuzz"
```

Parameters:

- `path`: the path to the tarball
- `destination`: the target directory
- `exclude_files`: a list of filename
  [blobs](https://golang.org/pkg/path/filepath/#Match) to exclude

## archive.targz_pack

`archive.targz_pack` creates a gzipped tarball.

```yaml
- command: archive.targz_pack
  params:
    target: "jstests.tgz"
    source_dir: "src/jstestfuzz"
    include:
      - "out/*.js"
```

Parameters:

- `target`: the tgz file that will be created
- `source_dir`: the directory to archive/compress
- `include`: a list of filename
  [blobs](https://golang.org/pkg/path/filepath/#Match) to include from the
  source directory.
- `exclude_files`: a list of filename
  [blobs](https://golang.org/pkg/path/filepath/#Match) to exclude from the
  source directory.

In addition to the
[filepath.Match](https://golang.org/pkg/path/filepath/#Match) syntax,
`archive.targz_pack` supports using `**` to indicate that
it should recurse into subdirectories. With only `*`, it
will not recurse.

## archive.auto_extract

`archive.auto_extract` extracts an archived/compressed file with an arbitrary
format based on its file extension.

```yaml
- command: archive.targz_extract
  params:
    path: "jstests.tgz"
    destination: "src/jstestfuzz"
```

Parameters:

- `path`: the path to the file to extract.
- `destination`: the target directory.

## archive.auto_pack

`archive.auto_pack` creates an archived/compressed file with an arbitrary
format.

```yaml
- command: archive.auto_pack
  params:
    target: "jstests.tgz"
    source_dir: "src/jstestfuzz"
```

Parameters:

- `target`: the output file that will be created. The extension will be used
  to determine the archiving format. Supported extensions are:
  - `.tgz`, `.tar.gz` (tarball archive with gzip compression)
  - `.tbr`, `.tar.br` (tarball archive with brotli compression)
  - `.tbz2`, `.tar.bz2` (tarball archive with bzip2 compression)
  - `.tar.lz4`, `.tlz4` (tarball archive with lz4 compression)
  - `.tsz`, `.tar.sz` (tarball archive with snappy compression)
  - `.txz`, `.tar.xz` (tarball archive with xz compression)
  - `.tar.zst` (tarball archive with zstandard compression)
  - `.rar` (rar archive)
  - `.tar` (tarball archive)
  - `.zip` (zip archive)
  - `.br` (brotli compression)
  - `.gz` (gzip compression)
  - `.bz2` (bzip2 compression)
  - `.lz4` (lz4 compression)
  - `.sz` (snappy compression)
  - `.xz` (xz compression)
  - `.zst` (zstandard compression)
- `source_dir`: the directory to archive/compress.
- `include`: a list of filename
  [blobs](https://golang.org/pkg/path/filepath/#Match) to include from the
  source directory. If not specified, the entire source directory will be
  archived.
- `exclude_files`: a list of filename
  [blobs](https://golang.org/pkg/path/filepath/#Match) to exclude from the
  source directory.

In addition to the
[filepath.Match](https://golang.org/pkg/path/filepath/#Match) syntax,
`archive.auto_pack` supports using `**` to indicate that
it should recurse into subdirectories. With only `*`, it
will not recurse.

## archive.zip_extract

`archive.zip_extract` extracts files from a zip file.

```yaml
- command: archive.targz_extract
  params:
    path: "jstests.zip"
    destination: "src/jstestfuzz"
```

Parameters:

- `path`: the path to the zip file.
- `destination`: the target directory.

## archive.zip_pack

`archive.zip_pack` creates a zip file.

```yaml
- command: archive.zip_pack
  params:
    target: "jstests.zip"
    source_dir: "src/jstestfuzz"
    include:
      - "out/*.js"
```

Parameters:

- `target`: the zip file that will be created.
- `source_dir`: the directory to archive/compress.
- `include`: a list of filename
  [blobs](https://golang.org/pkg/path/filepath/#Match) to include from the
  source directory.
- `exclude_files`: a list of filename
  [blobs](https://golang.org/pkg/path/filepath/#Match) to exclude from the
  source directory.

In addition to the
[filepath.Match](https://golang.org/pkg/path/filepath/#Match) syntax,
`archive.zip_pack` supports using `**` to indicate that
it should recurse into subdirectories. With only `*`, it
will not recurse.

## attach.artifacts

This command allows users to add files to the "Files" section of the
task page without using the `s3.put` command. Suppose you uploaded a
file to <https://example.com/this-is-my-file> in your task. For
instance, you might be using boto in a Python script. You can then add a
link to the Files element on the task page by:

```yaml
- command: attach.artifacts
  params:
    files:
      - example.json
```

```json
[
  {
    "name": "my-file",
    "link": "https://example.com/this-is-my-file",
    "visibility": "public"
  }
]
```

An additional "ignore_for_fetch" parameter controls whether the file
will be downloaded when spawning a host from the spawn link on a test
page.

- `files`: an array of gitignore file globs. All files that are
  matched - ones that would be ignored by gitignore - are included.
- `prefix`: an optional path to start processing the files, relative
  to the working directory.
- `exact_file_names`: an optional boolean flag which, if set to true,
  indicates to treat the files array as a list of exact filenames to
  match, rather than an array of gitignore file globs.
- `optional`: default false; if set to true, will not error if the
  file(s) specified are not found

## attach.results

This command parses and stores results in Evergreen's JSON test result format. Refer to [Task Output Data Retention Policy](../Reference/Limits#task_output_data_retention_policy) for details on the lifecycle of results uploaded via this command.

The use case for this command is when you wish to link custom test results
with test logs written via the
[file system API for task output](Task-Output-Directory). Evergreen's JSON
format allows you to send test metadata and log paths, relative to the reserved
test logs directory, which Evergreen will then link from the UI and API.

```yaml
- command: attach.results
  params:
    file_location: src/report.json
```

Parameters:

- `file_location`: a JSON file to parse and upload

The JSON file format is as follows:

```json
{
  "results": [
    {
      "status": "pass",
      "test_file": "test_1",
      "log_info": {
        "log_name": "tests/test_1.log",
        "logs_to_merge": ["global", "hooks/test_1.log"],
        "rendering_type": "resmoke"
      },
      "start": 1398782500.359, //epoch_time
      "end": 1398782500.681 //epoch_time
    },
    {
      "etc": "..."
    }
  ]
}
```

The available fields for each JSON object in the "results" array above are the
following. Note that all fields are optional and there is very little
validation on the data, so the server may accept inputs that are logically
nonsensical.

### Result

| Name        | Type          | Description                                                                                                        |
| ----------- | ------------- | ------------------------------------------------------------------------------------------------------------------ |
| `test_file` | string        | The name of the test. This is what will be displayed in the test results section of the UI as the test identifier. |
| `group_id`  | string        | The group ID if the test is associated with a group.                                                               |
| `status`    | string (enum) | The final status of the test. Should be one of: "fail", "pass", "silentfail", "skip".                              |
| `log_info`  | object        | The test's log information as a `Log Info` object, described below.                                                |
| `start`     | float64       | The start time of the test in \<seconds\>.\<fractional_seconds\> from the UNIX epoch.                              |
| `end`       | float64       | The end time of the test in \<seconds\>.\<fractional_seconds\> from the UNIX epoch.                                |

### Log Info

A test result can be linked to log files written to and ingested from the
task's [reserved test logs directory](Task-Output-Directory#test-logs).

Test log URLs are automatically generated and provided via the
[test logs API](../API/REST-V2-Usage#tag/tasks/paths/~1tasks~1%7Btask_id%7D~1build~1TestLogs~1%7Bpath%7D/get)

| Name             | Type          | Description                                                                                                         |
| ---------------- | ------------- | ------------------------------------------------------------------------------------------------------------------- |
| `log_name`       | string        | The principal test log path relative to the reserved test logs directory.                                           |
| `logs_to_merge`  | array         | The log paths, relative to the reserved test logs directory, to merge with the principal test log. Can be prefixes. |
| `line_num`       | int           | The starting line number of the test log if the file contains logs for multiple tests.                              |
| `rendering_type` | string (enum) | The rendering format for the Parsley log view. Should be one of: `default`, `resmoke`.                              |
| `version`        | int           | The log info version. Should be one of: `0`.                                                                        |

## attach.xunit_results

This command parses results in the XUnit format and posts them to the
API server. Refer to [Task Output Data Retention Policy](../Reference/Limits#task_output_data_retention_policy) for details on the lifecycle of results uploaded via this command.

Use this when you use a library in your programming language
to generate XUnit results from tests. Evergreen will parse these XML
files, creating links to individual tests in the test logs in the UI and
API. (Logs are only generated if the test case did not succeed -- this is
part of the XUnit XML file design.)

This command will not error if there are no test results, as XML files can still
be valid. We will error if no file paths given are valid XML files.

```yaml
- command: attach.xunit_results
  params:
    file: src/results.xml
```

Parameters:

- `file`: a .xml file to parse and upload. A filepath glob can also be
  supplied to collect results from multiple files.
- `files`: a list .xml files to parse and upload. Filepath globs can
  also be supplied to collect results from multiple files.

## downstream_expansions.set

downstream_expansions.set is used by parent patches to pass key-value
pairs to child patches. This command only has an effect in manual patches,
GitHub merge queue, and PRs. For all other versions,
it will no-op. The command takes the key-value pairs written in
the file and makes them available to the child patches. Note: these
parameters will be public and viewable on the child patch's page.

```yaml
- command: downstream_expansions.set
  params:
    file: downstream_expansions.yaml
```

Parameters:

- `file`: filename to read the expansions from

## ec2.assume_role

This command calls the aws assumeRole API and returns credentials as
these expansions:

- `AWS_ACCESS_KEY_ID` (not accessible by expansions.write)
- `AWS_SECRET_ACCESS_KEY` (not accessible by expansions.write)
- `AWS_SESSION_TOKEN` (not accessible by expansions.write)
- `AWS_ROLE_EXPIRATION`

See
[here](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html)
for more details on the assume role API.

```yaml
- command: ec2.assume_role
  params:
    role_arn: "aws_arn_123"
```

Parameters:

- `role_arn`: string ARN of the role you want to assume. (required)
- `policy`: string in JSON format that you want to use as an inline
  session policy.
- `duration_seconds`: int in seconds of how long the returned
  credentials will be valid. (default 900)

### AssumeRole AWS Setup

The call to AssumeRole includes an external ID formatted as
`<project_id>-<requester>`. This cannot be modified by the user.
This may be appended to in the future, it's highly recommended to
include a wildcard at the end of your external ID condition.

- An Evergreen project's ID can be found on its General Settings page.
- The list of requesters can be found [here](../Reference/Glossary.md#requesters).

The originating role is:
`arn:aws:iam::<evergreen_account_id>:role/evergreen.role.production`
and your role should only trust that exact role. You should add an
external ID to your role's trust policy to ensure only your project
can assume the role. Evergreen's account ID can be found on the
[wiki page](https://wiki.corp.mongodb.com/spaces/IAMSEC/pages/346197399/AWS+Account+List)
under `Kernel-Build`.

An example of a trust policy with an external ID is below:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::<evergreen_account_id>:role/evergreen.role.production"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringLike": {
          "sts:ExternalId": "<project_id>-<requester>*"
        }
      }
    }
  ]
}
```

> **Note:** Please make sure you use `StringLike` and not `StringEquals` for the
> `sts:ExternalId` condition. As well as including a wildcard at the end of your
> external ID condition. This allows for future additions to the external ID
> format without needing to update your trust policy.

You can allow any requester by using the wildcard earlier in the condition:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::<evergreen_account_id>:role/evergreen.role.production"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringLike": {
          "sts:ExternalId": "<project_id>-*"
        }
      }
    }
  ]
}
```

## expansions.update

`expansions.update` updates the task's expansions at runtime.
Any updates to the expansions made with this command will only persist for the duration of the task.

The order of operations is the `params.updates` field, then the file updates (if a file is given). This
means the file updates take precedence over the `params.updates` field.

Redacting is handled on a per-update basis and stored separately from the current expansion value. This
means that if an expansion is updated multiple times, only updates with `redact` or `redact_file_expansions`
will have their values redacted in the task logs.

For example, the below commands redact `http://s3/static-artifacts.tgz` throughout the task logs, but not
`http://s3/dynamic-artifacts` or `http://s3/dynamic-artifacts.tgz`.

```yaml
- command: expansions.update
  params:
    updates:
      - key: artifact_url
        value: http://s3/static-artifacts.tgz
        redact: true

- command: expansions.update
  params:
    updates:
      - key: artifact_url
        value: http://s3/dynamic-artifacts

- command: expansions.update
  params:
    updates:
      - key: artifact_url
        concat: tgz

- command: expansions.update
  params:
    ignore_missing_file: true
    file: src/ec2_artifacts.yml
    redact_file_expansions: true
```

Parameters:

- `updates`: a list of expansions to update. - `key`: the expansion key to update. (required) - `value`: the new value for the expansion. - `concat`: the string to concatenate to the existing value. Per
  update, only `value` or `concat` can be set. - `redact`: if true, the expansion will be redacted in the task logs.
  By default, this is false. Setting this to false will not unredact
  the expansion if it was already redacted.
- `file`: filename for a YAML file containing expansion updates, one key-value
  pair per expansion. For example, to set expansions `fruit=apple`
  `vegetable=spinach`, and `bread=cornbread`, the file should look like:

  ```yaml
  fruit: apple
  vegetable: spinach
  bread: cornbread
  ```

- `redact_file_expansions`: if true, the expansions added from the file will be redacted in the task logs.
  By default, this is false.
- `ignore_missing_file`: do not error if the file is missing.

## expansions.write

`expansions.write` writes the task's expansions to a file.

`AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` are always redacted for
security reasons.

```yaml
- command: expansions.write
  params:
    file: expansions.yaml
```

Parameters:

- `file`: filename to write expansions to.
- `redacted`: include redacted project variables, defaults to false.

For example, if the expansions are currently `fruit=apple`, `vegetable=spinach`,
and `bread=cornbread`, then the output file will look like this:

```yaml
fruit: apple
vegetable: spinach
bread: cornbread
```

## generate.tasks

This command creates functions, tasks, and variants from a user-provided
JSON file. Consider using one of the following tools to create the JSON
file:

- <https://github.com/evergreen-ci/shrub> (Go, officially maintained)
- <https://github.com/evergreen-ci/shrub.py> (Python, community maintained)

Notes:

- generate.tasks can only be used to append new functions, tasks, and variants,
  or append tasks (new or existing) to an existing variant. generate.tasks cannot
  redefine or modify existing functions, tasks, or variants (except for appending
  tasks to the variant). It is a validation error to define a function, task,
  or variant more than once in the JSON document passed to the command _except_
  to specify a variant multiple times in order to append additional tasks to the variant.
- Generated task's [tags](Project-Configuration-Files#task-and-variant-tags) will not be
  re-evaluated when added or retroactively activated in accordance with the variant's tags.
- The calls to generate.tasks may not in aggregate in a single version
  generate more than 100 variants or more than 1000 tasks.
- Because generate.tasks retries on errors that aren't known to us,
  it may appear that your generate.tasks is hanging until timeout.
  There may be details of this in the task logs; please ask
  if you aren't sure what to do with a hanging generate.tasks.
- If generate.tasks produces many errors, you may not be able to see the full
  error output.
- generate.tasks is idempotent across task restarts. In other words, once
  generate.tasks succeeds once, later task restarts will cause generate.tasks
  to no-op rather than generate duplicate configuration.
- The command does not give any feedback (via logs or the UI) what
  tasks were generated so using [s3.put](#s3put) after the command to upload
  the JSON file used is recommended for debuggability.

```yaml
- command: generate.tasks
  params:
    files:
      - example.json

# Example s3.put command to upload the JSON file used to generate tasks
- command: s3.put
  params:
    role_arn: ${role_arn}
    local_file: example.json
    remote_file: mongodb-mongo-master/${build_variant}/${revision}/generate_tasks/example.json
    bucket: mciuploads
    region: us-east-1
    permissions: private
    visibility: signed
    content_type: application/json
    display_name: Generate Tasks example.json JSON
```

Parameters:

- `files`: the JSON file(s) to generate tasks from
- `optional`: default false; if set to true, will not error if the
  file(s) specified are not found

```json
{
  "functions": {
    "echo-hi": {
      "command": "shell.exec",
      "params": {
        "script": "echo hi"
      }
    }
  },
  "tasks": [
    {
      "commands": [
        {
          "command": "git.get_project",
          "params": {
            "directory": "src"
          }
        },
        {
          "func": "echo-hi"
        }
      ],
      "name": "test"
    }
  ],
  "buildvariants": [
    {
      "tasks": [
        {
          "name": "test"
        }
      ],
      "display_name": "Ubuntu 16.04",
      "run_on": ["ubuntu1604-test"],
      "name": "ubuntu1604"
    }
  ]
}
```

## git.get_project

This command clones the tracked project repository into a given
directory, and checks out the revision associated with the task. Also
applies patches to the source after cloning it, if the task was created
by a patch submission.

```yaml
- command: git.get_project
  params:
    directory: src
    revisions:
      example: ${example_rev} ## or <hash>
```

```yaml
modules:
  - name: example
    owner: 10gen
    repo: mongo-example-modules
    prefix: src/mongo/db/modules
    ref: 12341a65256ff78b6d15ab79a1c7088443b9abcd
    branch: master
```

Parameters:

- `dir`: the directory to clone into
- `revisions`: For commit builds, each module should be passed as
  `<module_name> : ${<module_name>_rev}` (these are loaded from the [manifest](../API/REST-V2-Usage#manifest)
  at the beginning of the command).
  For patch builds, the hash
  must be passed directly as `<module_name> : <hash>`. Note that this
  means that for patch builds, editing the
  ["modules"](Project-Configuration-Files#modules)
  section of the project config will not change the checked out hash.
  If you do not specify any revisions, all of them will be cloned that
  are defined in the [build variant](Project-Configuration-Files#build-variants)'s
  `modules` field.
- `token`: Use a token to clone instead of the Evergreen generated GitHub token.
  This should be created with [github.generate_token](#githubgenerate_token).
  This token is \_only\* used for the source repository, not modules.
  This parameter does not support oauth tokens.
- `clone_depth`: Clone with `git clone --depth <clone_depth>`. For
  patch builds, Evergreen will `git fetch --unshallow` if the base
  commit is older than `<clone_depth>` commits. `clone_depth` takes precedence over `shallow_clone`.
- `shallow_clone`: Sets `clone_depth` to 100, if not already set.
- `recurse_submodules`: automatically initialize and update each
  submodule in the repository, including any nested submodules.

The parameters for each module are:

- `name`: the name of the module
- `owner`: the github owner of the module
- `repo`: the repo of the module
- `prefix`: the subdirectory to clone the repository in. It will be
  the repository name as a top-level directory in `dir` if omitted
- `ref`: must be a commit hash, takes precedence over the `branch`
  parameter if both specified (for commits)
- `branch`: must be the name of branch, commit hashes _are not
  accepted_.

### Module Hash Hierarchy

The hash used for a module during cloning is determined by the following hierarchy:

- For GitHub merge queue patches, Evergreen always uses the module branch name, to ensure accurate testing.
- For other patches, the initial default is to the githash in set-module, if specified.
- For both commits and patches, the next default is to the `<module_name>` set in revisions for the command.
- For commits, if this is not available, the next default is to ref, and then to branch. _Note that this
  doesn't work for patches -- hashes will need to be specified in the revisions section of the command._

## github.generate_token

> **This command will only work if an app ID and key are saved in your project settings. You can follow [the instructions here](Github-Integrations#dynamic-github-access-tokens) to set it up.**

The github.generate_token command will use the github app saved in your [project settings](Github-Integrations#dynamic-github-access-tokens) to dynamically generate a short lived github access token. If you run into any issues, please see the [FAQ](../FAQ.md#dynamic-github-access-tokens).

Parameters:

- `owner`: The account owner of the repository. This will be used to find the installation ID for the app that the token will be generated from. This is an optional field that will default to the project's owner.
- `repo`: The name of the repository without the .git extension. This will be used to find the installation ID for the app that the token will be generated from. This is an optional field that will default to the project's repository.
- `expansion_name`: The name for the expansion the token will be saved in.
- `permissions`: By default, the token will have the full permissions of the GitHub app that it's generated from. If you want the token to have less permissions, specify which permissions it should be restricted to. Permissions can also be restricted in project settings. For more on how to set that up and how it interacts with the permissions defined here, please see [here](Github-Integrations#dynamic-github-access-tokens). For a list of available permission types and levels, please take a look at `properties of permissions` in [the github documentation](https://docs.github.com/en/rest/apps/apps?apiVersion=2022-11-28#create-an-installation-access-token-for-an-app). Expansions cannot be used for this field.

For an example of how to generate a token and use that token to clone a repository, please see below. (Please check if [git.get_project](#gitget_project) or [modules](Project-Configuration-Files#modules) work for your use case before cloning manually).

```yaml
- command: github.generate_token
  params:
    owner: sample-owner # optional
    repo: sample-repo # optional
    expansion_name: generated_token
    permissions: # optional
      contents: read
- command: shell.exec
  params:
    script: |
      git clone https://x-access-token:${generated_token}@github.com/sample-owner/sample-repo.git
```

_While an owner and repository is used when generating a token from a github app (the project owner and repository being the default), you cannot rely on the token being restricted to that repository, as it may have the power to access other repositories in the org as well._

### Token Lifespan

Generated access tokens have a lifespan of one hour. Therefore, for long running tasks we recommend generating a token right before it's needed. A token will also be revoked and the expansion will be removed if it goes out of scope.

### Token Scope

#### Regular Tasks

- Tokens created in any part of the task will be scoped to that task. It will be revoked at the end up the task after the post task commands have finished running.

#### Task Groups

- Tokens created by individual tasks in a task group (including setup_task and teardown_task) will be scoped to that specific task.
- Tokens created by `setup_group` in [task groups](Project-Configuration-Files#task-groups) will be scoped to the entire task group and revoked after `teardown_group` commands have finished running. However, we recommend against generating a single GitHub token for an entire task group. The token may reach its one hour limit and no longer be valid when needed. Shorter token scopes also enhances security.

The following yaml provides a visual breakdown of token scopes.

```yaml
task_groups:
  - name: task_group_name
    setup_group:
      - command: github.generate_token
        params:
          expansion_name: setup_group_token
    setup_task:
      - command: github.generate_token
        params:
          expansion_name: setup_task_token
    tasks:
      - task1
      - task2

tasks:
  - name: task1
    commands:
      - command: github.generate_token
        params:
          expansion_name: task1_token
      - command: shell.exec
        params:
        script: |
          ## ${setup_group_token} is in scope (and one shared token for all tasks in the group)
          ## setup_task_token is in scope (and a fresh token for this task)
          ## task1_token is in scope

  - name: task2
    commands:
      - command: shell.exec
        params:
        script: |
          ## setup_group_token is in scope (and one shared token for all tasks in the group)
          ## setup_task_token is in scope (and a fresh token for this task)
          ## task1_token is **out of** scope (and will be revoked and the expansion undefined)
```

## gotest.parse_files

This command parses Go test results and sends them to the API server. Refer to [Task Output Data Retention Policy](../Reference/Limits#task_output_data_retention_policy) for details on the lifecycle of test results uploaded via this command.
It accepts files generated by saving the output of the `go test -v` command
to a file.

E.g. In a preceding shell.exec command, run `go test -v > result.suite`

```yaml
- command: gotest.parse_files
  params:
    files: ["src/*.suite"]
```

Parameters:

- `files`: a list of files (or blobs) to parse and upload
- `optional_output`: boolean to indicate if having no files found will
  result in a task failure.

## host.create

`host.create` starts a host from a task.

```yaml
- command: host.create
  params:
    distro: rhel70-small
```

Parse From A File:

- `file` - The name of a file containing all the parameters.

```yaml
- command: host.create
  params:
    file: src/host_params.yml
```

Agent Parameters:

- `num_hosts` - Number of hosts to start, 1 &lt;= `num_hosts` &lt;= 10.
  Defaults to 1.
- `retries` - How many times Evergreen should try to create this host
  in EC2 before giving up. Evergreen will wait 1 minute between
  retries.
- `scope` - When Evergreen will tear down the host, i.e., when either
  the task or build is finished. Must be either `task` or `build`.
  Defaults to `task` if not set.
- `timeout_setup_secs` - Stop waiting for hosts to be ready when
  spawning. Must be 60 &lt;= `timeout_setup_secs` &lt;= 3600 (1 hour).
  Default to 600 (10 minutes).
- `timeout_teardown_secs` - Even if the task or build has not
  finished, tear down this host after this many seconds. Must be 60
  &lt;= `timeout_teardown_secs` &lt;= 604800 (7 days). Default to 21600 (6
  hours).
- `ami` - The AMI to start. Must set `ami` or `distro`
  but must not set both.
- `device_name` - name of EBS device
- `distro` - Evergreen distro to start. Must set either `ami` only or
  `distro` but must not set both. Note that the distro setup script will
  not run for hosts spawned by this command, so any required initial
  setup must be done manually.
- `ebs_block_device` - list of the following parameters:
- `ebs_iops` - EBS provisioned IOPS.
- `ebs_size` - Size of EBS volume in GB.
- `ebs_snapshot_id` - EBS snapshot ID to mount.
- `instance_type` - EC2 instance type. Must set if `ami` is set. May
  set if `distro` is set, which will override the value from the
  distro configuration.
- `ipv6`- Set to true if instance should have _only_ an
  IPv6 address, rather than a public IPv4 address.
- `region` - EC2 region. Default is the same as Evergreen's default.
- `security_group_ids` - List of security groups. Must set if `ami` is
  set. May set if `distro` is set, which will override the value from
  the distro configuration.
- `subnet_id` - Subnet ID for the VPC. Must be set if `ami` is set.
- `tenancy` - If set, defines how the hosts are distributed across
  physical hardware. Can be set to `default` or `dedicated`. If not
  set, it uses the `default` (i.e. shared) tenancy.
- `userdata_file` - Path to file to load as EC2 user data on boot. May
  set if `distro` is set, which will override the value from the
  distro configuration. May set if distro is not set.

### Required IAM Policies for `host.create`

To create an on-demand host, the user must have the following
permissions:

- `ec2:CreateTags`
- `ec2:DescribeInstances`
- `ec2:RunInstances`
- `ec2:TerminateInstances`

### Checking SSH Availability for Spawn Hosts

Certain instances require more time for SSH access to become available.
If the user plans to execute commands on the remote host, then waiting
for SSH access to become available is mandatory. Below is an Evergreen
function that probes for SSH connectivity.

Note, however, an important shell caveat! By default Evergreen implements
shell scripting by piping the script into the shell. This means that a command
that reads from stdin, like ssh, will read the script from stdin, and none
of the commands after ssh will execute. To work around this, you can set
`exec_as_string` on `shell.exec`, or in bash you can wrap curly braces around the
script to make sure it is read entirely before executing.

```yaml
functions:
  ## Check SSH availability
  ssh-ready:
    command: shell.exec
    params:
      exec_as_string: true
      script: |
        user=${admin_user_name}
        ## The following hosts.yml file is generated as the output of the host.list command below
        hostname=$(tr -d '"[]{}' < buildhost-configuration/hosts.yml | cut -d , -f 1 | awk -F : '{print $2}')
        identity_file=~/.ssh/mcipacker.pem

        attempts=0
        connection_attempts=${connection_attempts|25}

        ## Check for remote connectivity
        while ! ssh \
          -i "$identity_file" \
          -o ConnectTimeout=10 \
          -o ForwardAgent=yes \
          -o IdentitiesOnly=yes \
          -o StrictHostKeyChecking=no \
          "$(printf "%s@%s" "$user" "$hostname")" \
          exit
        do
          [ "$attempts" -ge "$connection_attempts" ] && exit 1
          ((attempts++))
          printf "SSH connection attempt %d/%d failed. Retrying...\n" "$attempts" "$connection_attempts"
          ## sleep for Permission denied (publickey) errors
          sleep 10
        done
      shell: bash

tasks:
  - name: test
    commands:
      - command: host.create
        params:
          ami: ${ami}
          aws_access_key_id: ${aws_access_key_id}
          aws_secret_access_key: ${aws_secret_access_key}
          instance_type: ${instance_type|m3.medium}
          key_name: ${key_name}
          security_group_ids:
            - ${security_group_id}
          subnet_id: ${subnet_id}
      - command: host.list
        params:
          num_hosts: 1
          path: buildhost-configuration/hosts.yml
          timeout_seconds: 600
          wait: true
      - func: ssh-ready
      - func: other-tasks
```

Note:

- The `${admin_user_name}` expansion should be set to the value of the
  **user** field set for the command's distro, which can be inspected [on Evergreen's distro page](https://evergreen.mongodb.com/distros).
  This is not a default expansion, so it must be set manually.
- The mcipacker.pem key file was created by echoing the value of the
  `${__project_aws_ssh_key_value}` expansion (which gets populated automatically with the ssh private key value) into the file. This
  expansion is automatically set by Evergreen when the host is spawned.

## host.list

`host.list` gets information about hosts created by `host.create`.

```yaml
- command: host.list
  params:
    wait: true
    timeout_seconds: 300
    num_hosts: 1
```

Parameters:

- `num_hosts` - if `wait` is set, the number of hosts to wait to be
  running before the command returns
- `path` - path to file to write host info to
- `silent` - if true, do not log host info to the task logs
- `timeout_seconds` - time to wait for `num_hosts` to be running
- `wait` - if set, wait `timeout_seconds` for `num_hosts` to be
  running

If the `path` directive is specified, then the contents of the file
contain a JSON formatted list of objects. Each object contains the
following keys:

The returned keys represent the instance.

- `dns_name`: The FQDN of the EC2 instance (if IPv6 instance, this
  will not be populated).
- `ip_address`: the IP address of the EC2 instance (currently
  implemented for IPv6).
- `instance_id`: The unique identifier of the EC2 instance.

If there's an error in host.create, these will be available from
host.list in this form:

- `host_id`: The ID of the intent host we were trying to create
  (likely only useful for Evergreen team investigations)
- `error`: The error returned from host.create for this host

```json
[
  {
    "dns_name": "ec2-52-91-50-29.compute-1.amazonaws.com",
    "instance_id": "i-096d6766961314cd5"
  },
  {
    "ip_address": "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47",
    "instance_id": "i-106d6766961312a14"
  }
]
```

## json.send

This command saves JSON-formatted task data, typically used with the
performance plugin.

Parameters:

- `file`: the JSON file to save to Evergreen's DB
- `name`: name of the file you're saving, typically a test name

There is no schema enforced for the file itself - it is simply parsed as
JSON and then saved as BSON.

## keyval.inc

This command is deprecated. It exists to support legacy access to
logkeeper and could be removed at any time.

The keyval.inc command assigns a strictly monotonically increasing value
into the destination parameter name. The value is only strictly
monotonically increasing for the same key but will be strictly
monotonically increasing across concurrent tasks running the command at
the same time. From an implementation perspective, you can thinking of
it as Evergreen running a {findAndModify, query: {key: key}, update:
{\$inc: {counter: 1}}} on its application database.

Parameters:

- `key`: name of the value to increment. Evergreen tracks these
  internally.
- `destination`: expansion name to save the value to.

## papertrail.trace

This command traces artifact releases with the Papertrail service. It is owned
by the Release Infrastructure team, and you may receive assistance with it in
`#ask-devprod-release-tools`.

```yaml
- command: papertrail.trace
  params:
    key_id: ${papertrail_key_id}
    secret_key: ${papertrail_secret_key}
    product: mongosh
    version: 1.0.0
    filenames:
        - mongosh-linux-amd64.tar.gz
        - mongosh-linux-arm64.tar.gz
        - *.zip
```

Parameters:

- `work_dir`: The directory used to search for filenames
- `key_id`: your Papertrail key ID (use private variables to keep this a
  secret).
- `secret_key`: your Papertrail secret key (use private variables to keep this
  a secret).
- `product`: The name of the product these filenames belong to (e.g. mongosh,
  compass, java-driver).
- `version`: The version of the product these filenames belong to (e.g.
  1.0.1).
- `filenames`: A list of filename paths to pass to the service. You may use
  full filepaths in this parameter, the command will label the file with its
  basename only when sent to the service. Wildcard globs are supported within
  a single directory path. For example, the filename `dist/*.zip` would
  locate each zip file within the `dist` directory and individually trace
  those files. Double star globs like `dist/**/*.zip` are not supported. If
  a filename is matched multiple times in the same call to `papertrail.trace`,
  the command will throw an error before any tracing occurs. Note that this
  means that each basename must be unique, regardless of their path on the
  filesystem. For example, `./build-a/file.zip` and `./build-b/file.zip` would
  not be allowed as filenames in the same `papertrail.trace` command. If at least one file cannot be found while using wildcard globs, the command will return an error.

## s3.get

`s3.get` downloads a file from Amazon s3.

```yaml
# Temporary credentials:
- command: s3.get
  params:
    role_arn: ${role_arn}
    remote_file: ${mongo_binaries}
    bucket: mciuploads
    region: us-east-1
    local_file: src/mongo-binaries.tgz
# Or:
- command: s3.get
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    aws_session_token: ${aws_session_token}
    remote_file: ${mongo_binaries}
    bucket: mciuploads
    region: us-east-1
    local_file: src/mongo-binaries.tgz
# Static credentials (deprecated):
- command: s3.get
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    remote_file: ${mongo_binaries}
    bucket: mciuploads
    region: us-east-1
    local_file: src/mongo-binaries.tgz
```

Parameters:

- `aws_key`: your AWS key (use expansions to keep this a secret).
- `aws_secret`: your AWS secret (use expansions to keep this a secret).
- `aws_session_token`: your temporary AWS session token (use expansions to keep this a secret).
  Note: If you are generating temporary credentials using `ec2.assume_role`, you can instead
  pass in the role_arn directly to your s3 commands. If you have multiple S3 commands in quick
  succession, it is recommended you call `ec2.assume_role` once and pass in the credentials
  to each command rather than pass in a `role_arn`.
- `role_arn`: your AWS role to be assumed before and during the s3 operation.
  See [AssumeRole AWS setup](#assumerole-aws-setup) for more information on how
  to configure your role.
  This does not have to be a secret but managing it with expansions is recommended.
  This is the recommended way to authenticate with AWS.
- `local_file`: the local file to save, do not use with `extract_to`.
  This is relative to [task's working directory](./Best-Practices.md#task-directory)
- `extract_to`: the local directory to extract to, do not use with
  `local_file`. This requires the remote file to be a tarball (eg. `.tgz`).
  This is relative to [task's working directory](./Best-Practices.md#task-directory)
- `remote_file`: the S3 path to get the file from
- `bucket`: the S3 bucket to use.
- `region`: AWS region of the bucket, defaults to us-east-1.
- `build_variants`: list of buildvariants to run the command for, if
  missing/empty will run for all
- `optional`: boolean: if set, won't error if the file isn't found or there's an error with downloading.

## s3.put

This command uploads a file to Amazon s3, for use in later tasks or
distribution. Refer to [Task Artifacts Data Retention Policy](../Reference/Limits#task_artifacts_data_retention_policy) for details on the lifecycle of files uploaded via this command. **Files uploaded with this command will also be viewable within the Parsley log viewer if the `content_type` is set to `text/plain`, `application/json` or `text/csv`.**

```yaml
# Temporary credentials:
- command: s3.put
  params:
    role_arn: ${role_arn}
    local_file: src/mongodb-binaries.tgz
    remote_file: mongodb-mongo-master/${build_variant}/${revision}/binaries/mongo-${build_id}.${ext|tgz}
    bucket: mciuploads
    region: us-east-1
    permissions: private
    visibility: signed
    content_type: ${content_type|application/x-gzip}
    display_name: Binaries
# Or:
- command: s3.put
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    aws_session_token: ${aws_session_token}
    local_file: src/mongodb-binaries.tgz
    remote_file: mongodb-mongo-master/${build_variant}/${revision}/binaries/mongo-${build_id}.${ext|tgz}
    bucket: mciuploads
    region: us-east-1
    permissions: private
    visibility: signed
    content_type: ${content_type|application/x-gzip}
    display_name: Binaries
# Static credentials (deprecated):
- command: s3.put
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    local_file: src/mongodb-binaries.tgz
    remote_file: mongodb-mongo-master/${build_variant}/${revision}/binaries/mongo-${build_id}.${ext|tgz}
    bucket: mciuploads
    region: us-east-1
    permissions: private
    visibility: signed
    content_type: ${content_type|application/x-gzip}
    display_name: Binaries
```

Parameters:

- `aws_key`: your AWS key (use expansions to keep this a secret).
- `aws_secret`: your AWS secret (use expansions to keep this a secret)
- `aws_session_token`: your temporary AWS session token (use expansions to keep this a secret).
  Note: If you are generating temporary credentials using `ec2.assume_role`, you can instead
  pass in the role_arn directly to your s3 commands. If you have multiple S3 commands in quick
  succession, it is recommended you call `ec2.assume_role` once and pass in the credentials
  to each command rather than pass in a `role_arn`.
- `role_arn`: your AWS role to be assumed before and during the s3 operation.
  See [AssumeRole AWS setup](#assumerole-aws-setup) for more information on how
  to configure your role.
  This does not have to be a secret but managing it with expansions is recommended.
  This is the recommended way to authenticate with AWS.
- `local_file`: the local file to posts. This is relative to [task's working directory](./Best-Practices.md#task-directory).
- `remote_file`: the S3 path to post the file to
- `bucket`: the S3 bucket to use. Note: buckets created after Sept.
  30, 2020 containing dots (".") are not supported.
- `permissions`: the S3 permissions string to upload with. See [S3 docs](https://docs.aws.amazon.com/AmazonS3/latest/userguide/acl-overview.html#canned-acl)
  for allowed values. We recommend you use private permissions.
- `content_type`: the MIME type of the file. Note it is important that this value accurately reflects the mime type of the file or else the behavior will be unpredictable.
- `optional`: boolean to indicate if failure to find or upload this
  file will result in a task failure. This is intended to be used
  with `local_file`. `local_files_include_filter` be default is
  optional and will not work with this parameter.
- `skip_existing`: boolean to indicate that files that already exist
  in s3 should be skipped.
- `display_name`: the display string for the file in the Evergreen UI
- `local_files_include_filter`: used in place of local_file, an array
  of gitignore file globs. All files that are matched - ones that
  would be ignored by gitignore - are included in the put. If no
  files are found, the task continues execution.
  This is relative to [task's working directory](./Best-Practices.md#task-directory).
- `local_files_include_filter_prefix`: an optional path to start
  processing the `local_files_include_filter`.
  This is relative to [task's working directory](./Best-Practices.md#task-directory).
- `region`: AWS region for the bucket. We suggest us-east-1, since
  that is where ec2 hosts are located. If you would like to override,
  you can use this parameter.
- `visibility`: "public" (default) which provides a link to the
  s3 path in the UI for all Evergreen users. "private" which is a legacy option that now does the
  same as "public". "none" which hides the file from the UI for everybody but does not
  affect the underlying s3 permissions (see `permissions` parameter). "signed" which creates
  a pre signed url with the provided role*arn or credentials, allowing users to see the file
  (even if it's private on S3). Visibility: signed should not be combined with
  permissions: public-read or permissions: public-read-write. It can be combined with aws_session_token
  but only if the generated credentials are from a previous `ec2.assume_role` command in this task or if
  `role_arn` was passed in, otherwise Evergreen won't know the associated role to assume when generating
  the presigned url.
  Note: This parameter does \_not* affect the underlying permissions of the file
  on S3, only the visibility in the Evergreen UI. To change the permissions of the file on S3, use the `permissions` parameter.
- `patchable`: defaults to true. If set to false, the command will
  no-op for patches (i.e. continue without performing the s3 put).
- `patch_only`: defaults to false. If set to true, the command will
  no-op for non-patches (i.e. continue without performing the s3 put).
- `preserve_path`: defaults to false. If set to true, causes multi part uploads uploaded with
  `LocalFilesIncludeFilter` to preserve the original folder structure instead
  of putting all the files into the same folder

## s3.put with multiple files

Using the s3.put command in this uploads multiple files to an s3 bucket.

```yaml
- command: s3.put
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    aws_session_token: ${aws_session_token}
    local_files_include_filter:
      - slow_tests/coverage/*.tgz
      - fast_tests/coverage/*.tgz
    remote_file: mongodb-mongo-master/test_coverage-
    preserve_path: true
    bucket: mciuploads
    permissions: public-read
    content_type: ${content_type|application/x-gzip}
    display_name: coverage-
```

Each file is displayed in evergreen as the file's name prefixed with the
`display_name` field. Each file is uploaded to a path made of the local
file's name, in this case whatever matches the `*.tgz`, prefixed with
what is set as the `remote_file` field (or, to preserve the original folder
structure, use the `preserve_path` field). The filter uses the same
specification as gitignore when matching files. In this way, all files
that would be marked to be ignored in a gitignore containing the lines
`slow_tests/coverage/*.tgz` and `fast_tests/coverage/*.tgz` are uploaded
to the s3 bucket.

## s3Copy.copy (Deprecated)

`s3Copy.copy` is deprecated. Please use `s3.put` instead.

```yaml
- command: s3Copy.copy
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    aws_session_token: ${aws_session_token}
    s3_copy_files:
      - {
          "optional": true,
          "source":
            {
              "path": "${push_path}-STAGE/${push_name}/mongodb-${push_name}-${push_arch}-${suffix}-${task_id}.${ext|tgz}",
              "bucket": "build-push-testing",
            },
          "destination":
            {
              "path": "${push_path}/mongodb-${push_name}-${push_arch}-${suffix}.${ext|tgz}",
              "bucket": "${push_bucket}",
            },
        }
```

Parameters:

- `aws_key`: your AWS key (use expansions to keep this a secret).
- `aws_secret`: your AWS secret (use expansions to keep this a secret).
- `aws_session_token`: your temporary AWS session token (use expansions to keep this a secret).
- `s3_copy_files`: a map of `source` (`bucket` and `path`),
  `destination`, `build_variants` (a list of strings), `display_name`,
  and `optional` (suppresses errors). Note: destination buckets
  created after Sept. 30, 2020 containing dots (".") are not
  supported.

## shell.exec

This command runs a shell script. To follow [Evergreen best practices](Best-Practices#subprocessexec), we recommend using [subprocess.exec](#subprocessexec).

```yaml
- command: shell.exec
  params:
    working_dir: src
    script: |
      echo "this is a 2nd command in the function! Hello from ${author}!"
      ls
```

Parameters:

- `script`: the script to run. Expansions can be used in the script (it is not neccessary to specify `include_expansions_in_env` to do so)
- `working_dir`: the directory to execute the shell script in
- `env`: a map of environment variables and their values. In case of
  conflicting environment variables defined by `add_expansions_to_env` or
  `include_expansions_in_env`, this has the lowest priority. Unless
  overridden, the following environment variables will be set by default:
  - "CI" will be set to "true".
  - "GOCACHE" will be set to `${workdir}/.gocache`.
  - "EVR_TASK_ID" will be set to the running task's ID.
  - "TMP", "TMPDIR", and "TEMP" will be set to `${workdir}/tmp`.
- `add_expansions_to_env`: when true, add all expansions to the
  command's environment. In case of conflicting environment variables
  defined by `env` or `include_expansions_in_env`, this has higher
  priority than `env` and lower priority than `include_expansions_in_env`.
- `include_expansions_in_env`: specify one or more expansions to
  include in the environment. If you specify an expansion that does
  not exist, it is ignored. In case of conflicting environment
  variables defined by `env` or `add_expansions_to_env`, this has
  highest priority.
- `add_to_path`: specify one or more paths to prepend to the command `PATH`,
  which has the following effects:
  - If `PATH` is explicitly set in `env`, that `PATH` is ignored.
  - The command automatically inherits the runtime environment's `PATH`
    environment variable. Then, any paths specified in `add_to_path` are
    prepended in the given order.
- `background`: if set to true, the script runs in the background
  instead of the foreground. `shell.exec` starts the script but
  does not wait for the script to exit before running the next command.
  If the background script exits with an error while the
  task is still running, the task will continue running.
- `silent`: if set to true, does not log any shell output during
  execution; useful to avoid leaking sensitive info. Note that you should
  not pass secrets as command-line arguments but instead as environment
  variables or from a file, as Evergreen runs `ps` periodically, which
  will log command-line arguments.
- `continue_on_err`: by default, a task will fail if the script returns
  a non-zero exit code; for scripts that set `background`, the task will
  fail only if the script fails to start. If `continue_on_err`
  is true and the script fails, it will be ignored and task
  execution will continue.
- `system_log`: if set to true, the script's output will be written to
  the task's system logs, instead of inline with logs from the test
  execution.
- `shell`: shell to use. Defaults to sh if not set. Note that this is
  usually bash but is dash on Debian, so it's good to explicitly pass
  this parameter
- `ignore_standard_out`: if true, discards output sent to stdout
- `ignore_standard_error`: if true, discards output sent to stderr
- `redirect_standard_error_to_output`: if true, sends stderr to
  stdout. Can be used to synchronize these 2 streams
- `exec_as_string`: if true, executes as "sh -c 'your script
  here'". By default, shell.exec runs sh then pipes your script to
  its stdin. Use this parameter if your script will be doing something
  that may change stdin, such as sshing

## subprocess.exec

The subprocess.exec command executes a binary file. On a Unix-like OS,
you can also run a `#!` script as if it were a binary. To
get similar behavior on Windows, try `bash.exe -c
yourScript.sh`.

```yaml
- command: subprocess.exec
  params:
    working_dir: "src"
    env:
      FOO: bar
      BAZ: qux
    binary: "command"
    args:
      - "arg1"
      - "arg2"
```

Parameters:

- `binary`: a binary to run
- `args`: a list of arguments to the binary
- `env`: a map of environment variables and their values. In case of
  conflicting environment variables defined by `add_expansions_to_env` or
  `include_expansions_in_env`, this has the lowest priority. Unless
  overridden, the following environment variables will be set by default:
  - "CI" will be set to "true".
  - "GOCACHE" will be set to `${workdir}/.gocache`.
  - "EVR_TASK_ID" will be set to the running task's ID.
  - "TMP", "TMPDIR", and "TEMP" will be set to `${workdir}/tmp`.
- `command`: a command string (cannot use with `binary` or `args`), split
  according to shell rules for use as arguments.
  - Note: Expansions will _not_ be split on spaces; each expansion represents
    its own argument.
  - Note: on Windows, the shell splitting rules may not parse the command
    string as desired (e.g. for Windows paths containing `\`).
- `background`: if set to true, the process runs in the background
  instead of the foreground. `subprocess.exec` starts the process but
  does not wait for the process to exit before running the next command.
  If the background process exits with an error while the
  task is still running, the task will continue running.
- `silent`: do not log output of command. Note that you should
  not pass secrets as command-line arguments but instead as environment
  variables or from a file, as Evergreen runs `ps` periodically, which
  will log command-line arguments.
- `system_log`: write output to system logs instead of task logs
- `working_dir`: working directory to start shell in
- `ignore_standard_out`: if true, do not log standard output
- `ignore_standard_error`: if true, do not log standard error
- `redirect_standard_error_to_output`: if true, redirect standard
  error to standard output
- `continue_on_err`: by default, a task will fail if the command returns
  a non-zero exit code; for command that set `background`, the task will
  fail only if the command fails to start. If `continue_on_err`
  is true and the command fails, it will be ignored and task
  execution will continue.
- `add_expansions_to_env`: when true, add all expansions to the
  command's environment. In case of conflicting environment variables
  defined by `env` or `include_expansions_in_env`, this has higher
  priority than `env` and lower priority than `include_expansions_in_env`.
- `include_expansions_in_env`: specify one or more expansions to
  include in the environment. If you specify an expansion that does
  not exist, it is ignored. In case of conflicting environment variables
  defined by `env` or `add_expansions_to_env`, this has highest
  priority.
- `add_to_path`: specify one or more paths to prepend to the command `PATH`,
  which has the following effects:
  - If `PATH` is explicitly set in `env`, that `PATH` is ignored.
  - The command automatically inherits the runtime environment's `PATH`
    environment variable. Then, any paths specified in `add_to_path` are
    prepended in the given order.
  - This can be used to specify fallback paths to search for the `binary`
    executable (see [PATH special case](#path-environment-variable-special-case)).

### PATH Environment Variable Special Case

The `PATH` environment variable (specified either via explicitly setting `PATH`
in `env` or via `add_to_path`) is a special variable that has two effects:

- It sets the `PATH` environment variable for the command that runs.
- It adds fallback paths to search for the command's `binary`. If the `binary`
  is not found in the default runtime environment's `PATH`, it will try
  searching for a matching executable `binary` in any of the paths in
  `add_to_path` or in the `PATH` specified in `env`.

## test_selection.get

**Note: this feature is experimental and subject to change.**

This command allows a task to get a recommended list of tests from the [test selection
service](https://wiki.corp.mongodb.com/spaces/DBDEVPROD/pages/385846915/Test+Selection+Services). The command will
populate an output JSON file, which your task can use to decide which tests should run.

This command can only be used if [the test selection feature is enabled by the project](Project-and-Distro-Settings#test-selection-settings).

```yaml
- command: test_selection.get
  params:
    output_file: path/to/output/file.json
```

Parameters:

- `output_file`: the JSON output file containing the list of recommended tests. The schema for this file is:

  ```json
  {
      "tests": [
        {"name": "test1"},
        {"name": "test2"},
        ...
      ]
  }
  ```

- `tests`: a string list of input tests for test selection to consider. Optional. Cannot be combined with `tests_file`
- `tests_file`: a JSON file containing a list of input tests to consider. Optional. Cannot be combined with `tests`. The
  expected schema for this file is:

  ```json
  {
      "tests": ["test1", "test2", ...]
  }
  ```

- `strategies`: A comma-separated list of strategy names to use. Optional. If no strategy is explicitly chosen, test
  selection by default will return the same set of tests that were given in the input (via `tests` or `tests_file`).
- `usage_rate`: Define a string proportion (between 0 and 1) of how often the command should actually request a list of
  recommended tests. Even if it does not request a list of recommended tests, it will still produce an output file but
  that file will not contain any tests. Optional. If undefined, the command will always run.

### Example Integration of test_selection.get

Just adding this command will not immediately change which tests run or what test results are produced in your task. All
it does is produce a file containing the list of tests that the test selection service recommends running. Your
project's testing infrastructure will need to be updated to read and use the file to decide which tests to run.

A working simple example of the test selection command can be found [here](https://github.com/evergreen-ci/commit-queue-sandbox/blob/686ec45e27533294398ca0e83788f2b427cc6a2e/evergreen.yml#L337-L352).
Note that your integration may look very different, this is just meant to be a starting point.

## timeout.update

This command sets `exec_timeout_secs` or `timeout_secs` of a task from
within that task.

```yaml
- command: timeout.update
  params:
    exec_timeout_secs: ${my_exec_timeout_secs}
    timeout_secs: ${my_timeout_secs}
```

Parameters:

- `exec_timeout_secs`: set `exec_timeout_secs` for the task, which is
  the maximum amount of time the task may run. May be int, string, or
  expansion
- `timeout_secs`: set `timeout_secs` for the task, which is the
  maximum amount of time that can elapse without any output on stdout.
  May be int, string, or expansion

Both parameters are optional. If not set, the task will use the
definition from the project config.

Commands can also be configured to run if timeout occurs, as documented [here](Project-Configuration-Files#timeout-handler).

Note: CLI tools that run on Evergreen (such as DSI) might also have their own timeout configurations. Please check the documentation of the CLI tools you use for more details.
