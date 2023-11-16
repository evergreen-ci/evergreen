# Project Commands

Project Commands are the fundamental units of functionality in an Evergreen task.

## archive.targz_extract

`archive.targz_extract` extracts files from a gzipped tarball.

``` yaml
- command: archive.targz_extract
  params:
    path: "jstests.tgz"
    destination: "src/jstestfuzz"
```

Parameters:

-   `path`: the path to the tarball
-   `destination`: the target directory
-   `exclude_files`: a list of filename
    [blobs](https://golang.org/pkg/path/filepath/#Match) to exclude

## archive.targz_pack

`archive.targz_pack` creates a gzipped tarball.

``` yaml
- command: archive.targz_pack
  params:
    target: "jstests.tgz"
    source_dir: "src/jstestfuzz"
    include:
      - "out/*.js"
```

Parameters:

-   `target`: the tgz file that will be created
-   `source_dir`: the directory to compress
-   `include`: a list of filename
    [blobs](https://golang.org/pkg/path/filepath/#Match) to include
-   `exclude_files`: a list of filename
    [blobs](https://golang.org/pkg/path/filepath/#Match) to exclude

In addition to the
[filepath.Match](https://golang.org/pkg/path/filepath/#Match) syntax,
`archive.targz_pack` supports using \*\* to indicate that
it should recurse into subdirectories. With only \*, it
will not recurse.

## attach.artifacts

This command allows users to add files to the "Files" section of the
task page without using the `s3.put` command. Suppose you uploaded a
file to <https://example.com/this-is-my-file> in your task. For
instance, you might be using boto in a Python script. You can then add a
link to the Files element on the task page by:

``` yaml
- command: attach.artifacts
  params:
    files:
      - example.json
```

``` json
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

#### Lifecycle Policy 

These artifacts are stored in an S3 bucket which has the following lifecycle policy:

* Day 0 - Object uploaded
* Day 60 - Object moved to Standard-IA
* Day 365 - Object moved to Deep Glacier Archive
* Day 1095 - Object expires

If you would like to download an artifact after it has been moved to Glacier, please create a BUILD ticket requesting download as it will no longer be available via the link under the Files tab on the task page.

## attach.results

This command parses results in Evergreen's JSON test result format and
posts them to the API server. The use case for this command is when you
wish to store test logs yourself elsewhere. Evergreen's JSON format
allows you to send test metadata and a link to the test logs to
Evergreen, which Evergreen will then link from the UI and API.

The format is as follows:

``` json
{
    "results":[
    {
        "status":"pass",
        "test_file":"test_1",
        "exit_code":0,
        "elapsed":0.32200002670288086, //end-start
        "start":1398782500.359, //epoch_time
        "end":1398782500.681 //epoch_time
    },
    {
        "etc":"..."
    },
    ]
}
```

The available fields for each json object in the "results" array above
are the following. Note that all fields are optional and there is very
little validation on the data, so the server may accept inputs that are
logically nonsensical.

| Name        | Type          | Description                                                                                                            |
| ----------- | ------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `status`    | string (enum) | The final status of the test. Should be one of: "fail", "pass", "silentfail", "skip".                                  |
| `test_file` | string        | The name of the test. This is what will be displayed in the test results section of the UI as the test identifier.     |
| `group_id`  | string        | The group ID if the test is associated with a group. This is mostly used for tests logging directly to cedar.          |
| `url`       | string        | The URL containing the rich-text view of the test logs.                                                                |
| `url_raw`   | string        | The URL containing the plain-text view of the test logs.                                                               |
| `line_num`  | int           | The line number of the test within the "url" parameter, if the URL actually contains the logs for multiple tests.      |
| `exit_code` | int           | The status with which the test command exited. For the most part this does nothing.                                    |
| `task_id`   | string        | The ID of the task with which this test should be associated. The test will appear on the page for the specified task. |
| `execution` | int           | The execution of the task above with which this test should be associated.                                             |

``` yaml
- command: attach.results
  params:
    file_location: src/report.json
```

Parameters:

-   `file_location`: a .json file to parse and upload

## attach.xunit_results

This command parses results in the XUnit format and posts them to the
API server. Use this when you use a library in your programming language
to generate XUnit results from tests. Evergreen will parse these XML
files, creating links to individual tests in the test logs in the UI and
API. (Logs are only generated if the test case did not succeed -- this is
 part of the XUnit XML file design.)

This command will not error if there are no test results, as XML files can still
be valid. We will error if no file paths given are valid XML files.

``` yaml
- command: attach.xunit_results
  params:
    file: src/results.xml
```

Parameters:

-   `file`: a .xml file to parse and upload. A filepath glob can also be
    supplied to collect results from multiple files.
-   `files`: a list .xml files to parse and upload. Filepath globs can
    also be supplied to collect results from multiple files.

## ec2.assume_role

This command calls the aws assumeRole API and returns credentials as
these expansions:

-   `AWS_ACCESS_KEY_ID` (not accessible by expansion.write)
-   `AWS_SECRET_ACCESS_KEY` (not accessible by expansion.write)
-   `AWS_SESSION_TOKEN` (not accessible by expansion.write)
-   `AWS_ROLE_EXPIRATION`

See
[here](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html)
for more details on the assume role API.

``` yaml
- command: ec2.assume_role
  params:
    role_arn: "aws_arn_123"
```

Parameters:

-   `role_arn`: string ARN of the role you want to assume. (required)
-   `external_id`: string of external ID that can be specified in the
    role.
-   `policy`: string in JSON format that you want to use as an inline
    session policy.
-   `duration_seconds`: int in seconds of how long the returned
    credentials will be valid. (default 900)

## expansions.update

`expansions.update` updates the task's expansions at runtime. 
Any updates to the expansions made with this command will only persist for the duration of the task.

``` yaml
- command: expansions.update
  params:
    ignore_missing_file: true
    file: src/ec2_artifacts.yml

- command: expansions.update
  params:
    updates:
    - key: artifact_url
      value: http://s3/static-artifacts.tgz
```

Parameters:

-   `updates`: key-value pairs for updating the task's parameters
-   `file`: filename for a YAML file containing expansion updates
-   `ignore_missing_file`: do not error if the file is missing

## expansions.write

`expansions.write` writes the task's expansions to a file.

`global_github_oauth_token`, `github_app_token`, `AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` are always redacted for
security reasons.

``` yaml
- command: expansions.write
  params:
    file: expansions.yaml
```

Parameters:

-   `file`: filename to write expansions to
-   `redacted`: include redacted project variables, defaults to false

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

-   <https://github.com/evergreen-ci/shrub> (Go, officially maintained)
-   <https://github.com/evergreen-ci/shrub.py> (Python, community maintained)

Notes:

-   generate.tasks can only be used to append new functions, tasks, and variants,
    or append tasks (new or existing) to an existing variant. generate.tasks cannot
    redefine or modify existing functions, tasks, or variants (except for appending
    tasks to the variant). It is a validation error to define a function, task,
    or variant more than once in the JSON document passed to the command _except_
    to specify a variant multiple times in order to append additional tasks to the variant.
-   The calls to generate.tasks may not in aggregate in a single version
    generate more than 100 variants or more than 1000 tasks.
-   Because generate.tasks retries on errors that aren't known to us,
    it may appear that your generate.tasks is hanging until timeout.
    There may be details of this in the task logs; please ask in
    #evergreen-users if you aren't sure what to do with a hanging
    generate.tasks.

``` yaml
- command: generate.tasks
  params:
    files:
      - example.json
```

Parameters:

-   `files`: the JSON file(s) to generate tasks from
-   `optional`: default false; if set to true, will not error if the
    file(s) specified are not found

``` json
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
            "run_on": [
                "ubuntu1604-test"
            ],
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

``` yaml
- command: git.get_project
  params:
    directory: src
    revisions:
      example: ${example_rev} ## or <hash>
```

``` yaml
- modules: 
  - name: example
    owner: 10gen
    repo: mongo-example-modules
    prefix: src/mongo/db/modules
    ref: 12341a65256ff78b6d15ab79a1c7088443b9abcd
    branch: master
```

Parameters:

-   `dir`: the directory to clone into
-   `revisions`: For commit builds, each module should be passed as
    `<module_name> : ${<module_name>_rev}` (these are loaded from the [manifest](../API/REST-V2-Usage.md#manifest) 
    at the beginning of the command). 
    For patch builds, the hash
    must be passed directly as `<module_name> : <hash>`. Note that this
    means that for patch builds, editing the
    ["modules"](Project-Configuration-Files.md#modules)
    section of the project config will not change the checked out hash.
-   `token`: Use a token to clone instead of the ssh key on the host.
    Since this is a secret, it should be provided as a project
    expansion. For example, you could provide an expansion called
    "github_token" and then set this field to \${github_token}.
    Evergreen will populate the expansion when it parses the project
    yaml.
-   `clone_depth`: Clone with `git clone --depth <clone_depth>`. For
    patch builds, Evergreen will `git fetch --unshallow` if the base
    commit is older than `<clone_depth>` commits. `clone_depth` takes precedence over `shallow_clone`.
-   `shallow_clone`: Sets `clone_depth` to 100, if not already set.
-   `recurse_submodules`: automatically initialize and update each
    submodule in the repository, including any nested submodules.

The parameters for each module are:

-   `name`: the name of the module
-   `owner`: the github owner of the module
-   `repo`: the repo of the module
-   `prefix`: the subdirectory to clone the repository in. It will be
    the repository name as a top-level directory in `dir` if omitted
-   `ref`: must be a commit hash, takes precedence over the `branch`
    parameter if both specified (for commits)
-   `branch`: must be the name of branch, commit hashes _are not
    accepted_.

#### Module Hash Hierarchy
The hash used for a module during cloning is determined by the following hierarchy:
* For commit queue and GitHub merge queue patches, Evergreen always uses the module branch name, to ensure accurate testing.
* For other patches, the initial default is to the githash in set-module, if specified.
* For both commits and patches, the next default is to the `<module_name>` set in revisions for the command.
* For commits, if this is not available, the next default is to ref, and then to branch. *Note that this 
doesn't work for patches -- hashes will need to be specified in the revisions section of the command.*

## gotest.parse_files

This command parses Go test results and sends them to the API server. It
accepts files generated by saving the output of the `go test -v` command
to a file.

E.g. In a preceding shell.exec command, run `go test -v > result.suite`

``` yaml
- command: gotest.parse_files
  params:
    files: ["src/*.suite"]
```

Parameters:

-   `files`: a list of files (or blobs) to parse and upload
-   `optional_output`: boolean to indicate if having no files found will
    result in a task failure.

## host.create

`host.create` starts a host from a task.

``` yaml
- command: host.create
  params:
    provider: ec2
    distro: rhel70-small
```

Parse From A File:

-   `file` - The name of a file containing all the parameters.

``` yaml
- command: host.create
  params:
    file: src/host_params.yml 
```

Agent Parameters:

-   `num_hosts` - Number of hosts to start, 1 &lt;= `num_hosts` &lt;= 10.
    Defaults to 1 (must be 1 if provider is Docker).
-   `provider` - Cloud provider. Must set `ec2` or `docker`.
-   `retries` - How many times Evergreen should try to create this host
    in EC2 before giving up. Evergreen will wait 1 minute between
    retries.
-   `scope` - When Evergreen will tear down the host, i.e., when either
    the task or build is finished. Must be either `task` or `build`.
    Defaults to `task` if not set.
-   `timeout_setup_secs` - Stop waiting for hosts to be ready when
    spawning. Must be 60 &lt;= `timeout_setup_secs` &lt;= 3600 (1 hour).
    Default to 600 (10 minutes).
-   `timeout_teardown_secs` - Even if the task or build has not
    finished, tear down this host after this many seconds. Must be 60
    &lt;= `timeout_teardown_secs` &lt;= 604800 (7 days). Default to 21600 (6
    hours).

EC2 Parameters:

-   `ami` - EC2 AMI to start. Must set `ami` or `distro` but must not
    set both.
-   `aws_access_key_id` - AWS access key ID. May set to use a
    non-default account. Must set if `aws_secret_access_key` is set.
-   `aws_secret_access_key` - AWS secret key. May set to use a
    non-default account. Must set if `aws_access_key_id` is set.
-   `device_name` - name of EBS device
-   `distro` - Evergreen distro to start. Must set `ami` or `distro` but
    must not set both. Note that the distro setup script will not run for 
    hosts spawned by this command, so any required initial setup must be done manually.
-   `ebs_block_device` - list of the following parameters:
-   `ebs_iops` - EBS provisioned IOPS.
-   `ebs_size` - Size of EBS volume in GB.
-   `ebs_snapshot_id` - EBS snapshot ID to mount.
-   `instance_type` - EC2 instance type. Must set if `ami` is set. May
    set if `distro` is set, which will override the value from the
    distro configuration.
-   `ipv6`- Set to true if instance should have _only_ an
    IPv6 address, rather than a public IPv4 address.
-   `key_name` - EC2 Key name. Must set if `aws_access_key_id` or
    `aws_secret_access_key` is set. Must not set otherwise.
-   `region` - EC2 region. Default is the same as Evergreen's default.
-   `security_group_ids` - List of security groups. Must set if `ami` is
    set. May set if `distro` is set, which will override the value from
    the distro configuration.
-   `subnet_id` - Subnet ID for the VPC. Must be set if `ami` is set.
-   `userdata_file` - Path to file to load as EC2 user data on boot. May
    set if `distro` is set, which will override the value from the
    distro configuration. May set if distro is not set.
-   `vpc_id` - EC2 VPC. Must set if `ami` is set. May set if `distro` is
    set, which will override the value from the distro configuration.

Docker Parameters:

-   `background` - Set to wait for logs in the background, rather than
    blocking. Default is true.
-   `container_wait_timeout_secs` - Time to wait for the container to
    finish running the given command. Must be &lt;= 3600 (1 hour). Default
    to 600 (10 minutes).
-   `command` - The command to run on the container. Does not not
    support shell interpolation. If not specified, will use the default
    entrypoint.
-   `distro` - Required. The distro's container pool is used to
    find/create parents for the container.
-   `image` - Required. The image to use for the container. If image is
    a URL, then the image is imported, otherwise it is pulled.
-   `poll_frequency_secs` - Check for running container and logs at this
    interval. Must be &lt;= 60 (1 second). Default to 30.
-   `publish_ports` - Set to make ports available by mapping container
    ports to ports on the Docker host. Default is false.
-   `extra_hosts` - Optional. This is a list of hosts to be added to
    /etc/hosts on the container (each should be of the form
    hostname:IP).
-   `registry_name` - The registry from which to pull/import the image.
    Defaults to Dockerhub.
-   `registry_username` - Username for the `registry_name` if it
    requires authentication. Must set if `registry_password` is set.
-   `registry_password` - Password for the `registry_name` if it
    requires authentication. Must set if `registry_username` is set.
-   `stdin_file_name` - The file containing the content to provide as stdin to
    the container command. By default, the container command has no input to
    stdin. Note that if you try to start a spawn host and also choose to start
    containers started by host.create for the task, the running container will
    *not* have the stdin file content passed to it.
-   `stdout_file_name` - The file path to write stdout logs from the
    container. Default is &lt;container_id&gt;.out.log.
-   `stderr_file_name` - The file path to write stderr logs from the
    container. Default is &lt;container_id&gt;.err.log.
-   `environment_vars` - Environment variables to pass to the container command.
    By default, no environment variables are passed.

### Required IAM Policies for `host.create`

To create an on-demand host, the user must have the following
permissions:

-   `ec2:CreateTags`
-   `ec2:DescribeInstances`
-   `ec2:RunInstances`
-   `ec2:TerminateInstances`

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

``` yaml
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
          provider: ec2
          security_group_ids:
            - ${security_group_id}
          subnet_id: ${subnet_id}
          vpc_id: ${vpc_id}
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
`${__project_aws_ssh_key_value}` expansion (which gets populated automatically with the ssh public key value) into the file. This
expansion is automatically set by Evergreen when the host is spawned.

## host.list

`host.list` gets information about hosts created by `host.create`.

``` yaml
- command: host.list
  params:
    wait: true
    timeout_seconds: 300
    num_hosts: 1
```

Parameters:

-   `num_hosts` - if `wait` is set, the number of hosts to wait to be
    running before the command returns
-   `path` - path to file to write host info to
-   `silent` - if true, do not log host info to the task logs
-   `timeout_seconds` - time to wait for `num_hosts` to be running
-   `wait` - if set, wait `timeout_seconds` for `num_hosts` to be
    running

If the `path` directive is specified, then the contents of the file
contain a JSON formatted list of objects. Each object contains the
following keys:

For EC2, these keys represent the instance. For Docker, they represent
the Docker host that the container is running on.

-   `dns_name`: The FQDN of the EC2 instance (if IPv6 instance, this
    will not be populated).
-   `ip_address`: the IP address of the EC2 instance (currently
    implemented for IPv6).

EC2 Only:

-   `instance_id`: The unique identifier of the EC2 instance.

Docker Only:

-   `host_id`: The unique identifier of the container.
-   `parent_id`: The unique identifier of the parent of the container
    (may be given as the parent host's tag, i.e. evergreen-assigned
    ID).
-   `image`: The image used for the container.
-   `command`: The command run on the container.
-   `port_bindings`: The map of docker ports (formatted
    `<port>/<protocol>`) to ports on the container host. Only available
    if `publish_ports` was set for `host.create`.

If there's an error in host.create, these will be available from
host.list in this form:

-   `host_id`: The ID of the intent host we were trying to create
    (likely only useful for Evergreen team investigations)
-   `error`: The error returned from host.create for this host

``` json
[
    {
        "dns_name": "ec2-52-91-50-29.compute-1.amazonaws.com",
        "instance_id": "i-096d6766961314cd5"
    },
    {
        "ip_address": "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47",
        "instance_id": "i-106d6766961312a14"
    }
    {
        "dns_name": "ec2-55-123-99-55.compute-1.amazonaws.com",
        "host_id": "container-7919139205343971456",
        "parent_id": "evg-archlinux-parent-20190513171239-2182711601109995555",
        "image": "hello-world",
        "command": "/hello",
        "port_bindings": {
            "4444/tcp": [
               "32769"
            ],
            "5900/tcp": [
               "32768"
            ]
         }
    }
] 
```

## json.send

This command saves JSON-formatted task data, typically used with the
performance plugin.

Parameters:

-   `file`: the JSON file to save to Evergreen's DB
-   `name`: name of the file you're saving, typically a test name

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

-   `key`: name of the value to increment. Evergreen tracks these
    internally.
-   `destination`: expansion name to save the value to.

## mac.sign

`mac.sign` signs and/or notarizes the mac OS artifacts. It calls
internal macOS signing and notarization service.

**Note**: This command is maintained by the BUILD team.

``` yaml
- command: mac.sign
  params:
    key_id: ${key_id}
    secret: ${secret}
    service_url: ${service_url}
    client_binary: /local/path/to/client_binary
    local_zip_file: /local/path/to/file_to_be_singed
    output_zip_file: /local/path/to/output_file
    artifact_type: binary
    entitlements_file: /local/path/to/entitlements_file
    verify: false
    notarize: true
    bundle_id: bundle_id_for_notarization
    working_directory: /local/path/to/working_directory
```

Parameters:

-   `key_id`: the id of the key needs to be used for signing
-   `secret`: secret associated with the key
-   `service_url`: url of the signing and notarization service
-   `client_binary`: path to the client binary, if not given this value
    is used - `/usr/local/bin/macnotary`
-   `local_zip_file`: path to the local zip file contains the list of
    artifacts for signing
-   `output_zip_file`: local path to the file returned by service
-   `artifact_type`: type of the artifact. Either `binary` or `app`. If
    not given `binary` is taken as a value
-   `entitlements_file`: path to the local entitlements file to be used
    during signing. This can be omitted for default entitlements
-   `verify`: boolean param defines whether the signing/notarization
    should be verified. Only supported on macOS. If not given or OS is
    none mac OS, the `false` value will be taken
-   `notarize`: boolean param defines whether notarization should be
    performed after signing. The default value is `false`
-   `bundle_id`: bundle id used during notarization. Must be given if
    notarization requested
-   `build_variants`: list of buildvariants to run the command for, if
    missing/empty will run for all
-   `working_directory`: local path to the working directory


## perf.send

This command sends performance test data, as either JSON or YAML, to
Cedar. Note that if the tests do not contain artifacts, the AWS
information is not necessary.

``` yaml
- command: perf.send
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    bucket: mciuploads
    prefix: perf_reports
    file: cedar_report.json
```

Parameters:

-   `file`: the JSON or YAML file containing the test results, see
    below for more info.
-   `aws_key`: your AWS key (use expansions to keep this a secret)
-   `aws_secret`: your AWS secret (use expansions to keep this a secret)
-   `region`: AWS region of the bucket, defaults to us-east-1.
-   `bucket`: the S3 bucket to use.
-   `prefix`: prefix, if any, within the s3 bucket.

Example dummy content of a test results JSON file containing `test` objects:

```json
[
    {
        "info": {
            "test_name": "foo",
            "trial": 0,
            "tags": [],
            "args": {
                "mongod": 0,
                "genny_phase_id": 1
            }
        },
        "created_at": "2023-01-01T00:00:14.993882+00:00",
        "completed_at": "2023-01-01T00:03:15+00:00",
        "artifacts": [],
        "metrics": [
            {
                "name": "lorem",
                "type": "COUNT",
                "value": 12345.0,
                "user_submitted": false
            },
            {
                "name": "ipsum",
                "type": "PERCENTILE_99TH",
                "value": 6.78910,
                "user_submitted": false
            },
            {
                "name": "dolor",
                "type": "MIN",
                "value": 1.11213,
                "user_submitted": false
            }
        ],
        "sub_tests": []
    },
    {
        "info": {
            "test_name": "bar",
            "trial": 0,
            "tags": [],
            "args": {}
        },
        "created_at": "2023-01-01T09:50:01.720954+00:00",
        "completed_at": "2023-01-01T10:04:37.015088+00:00",
        "artifacts": [],
        "metrics": [],
        "sub_tests": [
            {
                "info": {
                    "test_name": "WaldoActor.QuxOperation",
                    "trial": 0,
                    "tags": [],
                    "args": {}
                },
                "created_at": "2023-01-01T09:51:15.021000+00:00",
                "completed_at": "2023-01-01T09:51:15.021000+00:00",
                "artifacts": [
                    {
                        "bucket": "genny-metrics",
                        "path": "WaldoActor.QuxOperation",
                        "prefix": "foobar_variant.2022_11_bar_patch_ghijk67890_23_01_01_06_24_48_0",
                        "tags": [],
                        "created_at": "2023-01-01T09:50:04.992453+00:00",
                        "local_path": "/data/mci/12345abcdef/build/WorkloadOutput/reports/bar/CedarMetrics/WaldoActor.QuxOperation.ftdc",
                        "permissions": "public-read",
                        "convert_bson_to_ftdc": false
                    }
                ],
                "metrics": [],
                "sub_tests": []
            },
            {
                "info": {
                    "test_name": "FredActor.BazOperation",
                    "trial": 0,
                    "tags": [],
                    "args": {}
                },
                "created_at": "2023-01-01T09:51:11.175000+00:00",
                "completed_at": "2023-01-01T09:51:14.552000+00:00",
                "artifacts": [
                    {
                        "bucket": "genny-metrics",
                        "path": "FredActor.BazOperation",
                        "prefix": "foobar_variant.2022_11_bar_patch_ghijk67890_23_01_01_06_24_48_0",
                        "tags": [],
                        "created_at": "2023-01-01T09:50:04.992453+00:00",
                        "local_path": "/data/mci/12345abcdef/build/WorkloadOutput/reports/bar/CedarMetrics/FredActor.BazOperation.ftdc",
                        "permissions": "public-read",
                        "convert_bson_to_ftdc": false
                    }
                ],
                "metrics": [],
                "sub_tests": []
            }
        ]
    }
]
```
### test

Each `test` object holds data about a specific test and its
subtests. It is represented by the
[`Test` struct](https://pkg.go.dev/github.com/evergreen-ci/poplar#Test),
and contains these fields:

| Name           | Type   | Description                                                                                                    |
| -------------- | ------ | -------------------------------------------------------------------------------------------------------------- |
| `info`         | object | The test's `info` object, described below.                                                                     |
| `created_at`   | string | The test's creation timestamp.                                                                                 |
| `completed_at` | string | The test's completion timestamp.                                                                               |
| `artifacts`    | array  | The test's list of `artifact` objects, described below.                                                        |
| `metrics`      | array  | The test's list of `metric` objects, described below.                                                          |
| `sub_tests`    | array  | The test's list of subtest `test` objects, which recursively have the same format as the parent's `test` object. |

**Note:** Although the `Test` struct includes the `_id` field, you
should not populate it. It would be populated automatically by
`perf.send`.

### info

Each `info` object holds metadata about the test configuration and
execution. It is represented by the
[`TestInfo` struct](https://pkg.go.dev/github.com/evergreen-ci/poplar#TestInfo),
and contains these fields:

| Name        | Type    | Description                                                                                    |
| ----------- | ------- | ---------------------------------------------------------------------------------------------- |
| `test_name` | string  | The test's name.                                                                               |
| `trial`     | integer | (Optional) Representing a repeated test run (first run is 0).                                  |
| `tags`      | array   | (Optional) The test's list of tags.                                                            |
| `args`      | object  | (Optional) The test's configuration arguments, as an object with string keys & integer values. |

**Note:** Although the `TestInfo` struct includes the `parent` field,
you should not populate it. It stores a subtest's parent test ID, and
would be populated automatically by `perf.send`.

### artifact

Each `artifact` object allows you to upload and attach metadata to
results files. It's frequently used to upload FTDC files representing
the test's intra-run data. This object is represented by the
[`TestArtifact` struct](https://pkg.go.dev/github.com/evergreen-ci/poplar#TestInfo),
and contains these fields:

| Name                      | Type      | Description                                                                                                                                                                                   |
| ------------------------- | --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `local_path`              | string    | The artifact's local filepath.                                                                                                                                                                |
| `permissions`             | string    | The S3 canned ACL permission to apply to the uploaded artifact. See the list of valid permissions [here](https://docs.aws.amazon.com/AmazonS3/latest/userguide/acl-overview.html#canned-acl). |
| `created_at`              | timestamp | The artifact's creation time.                                                                                                                                                                 |
| `bucket`                  | string    | (Optional) The S3 bucket to upload to. If not provided, the top level bucket information (provided above as part of the evergreen command config) will be used.                               |
| `prefix`                  | string    | (Optional) The S3 bucket's prefix (if any). If not provided, also uses top level prefix.                                                                                                      |
| `path`                    | string    | (Optional) The artifact's unique S3 object key (usually the metric name / FTDC filename). If not provided, the filename is used.                                                              |
| `tags`                    | array     | (Optional) The artifact's list of tags.                                                                                                                                                       |
| `is_text`                 | boolean   | (Optional) The artifact is a plain text file.                                                                                                                                                 |
| `is_ftdc`                 | boolean   | (Optional) The artifact is an FTDC file.                                                                                                                                                      |
| `is_bson`                 | boolean   | (Optional) The artifact is a BSON file.                                                                                                                                                       |
| `is_json`                 | boolean   | (Optional) The artifact is a JSON file.                                                                                                                                                       |
| `is_csv`                  | boolean   | (Optional) The artifact is a CSV file.                                                                                                                                                        |
| `is_uncompressed`         | boolean   | (Optional) The artifact is an uncompressed file.                                                                                                                                              |
| `is_gzip`                 | boolean   | (Optional) The artifact is a GZIP file.                                                                                                                                                       |
| `is_tarball`              | boolean   | (Optional) The artifact is a tarball.                                                                                                                                                         |
| `convert_gzip`            | boolean   | (Optional) Should gzip the artifact before uploading.                                                                                                                                         |
| `convert_bson_to_ftdc`    | boolean   | (Optional) Should convert the BSON artifact file to FTDC before uploading.                                                                                                                    |
| `convert_json_to_ftdc`    | boolean   | (Optional) Should convert the JSON artifact file to FTDC before uploading.                                                                                                                    |
| `convert_csv_to_ftdc`     | boolean   | (Optional) Should convert the CSV artifact file to FTDC before uploading.                                                                                                                     |
| `events_raw`              | boolean   | (Unused)                                                                                                                                                                                      |
| `events_histogram`        | boolean   | (Unused)                                                                                                                                                                                      |
| `events_interval_summary` | boolean   | (Unused)                                                                                                                                                                                      |
| `events_collapsed`        | boolean   | (Unused)                                                                                                                                                                                      |

### metric

Each `metric` object holds a computed summary statistic / metric for
a test. It is represented by the
[`TestMetrics` struct](https://pkg.go.dev/github.com/evergreen-ci/poplar#TestMetrics),
and contains these fields:

| Name      | Type      | Description                                                                                                                                                                                                                                                                                                                                                                        |
| --------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`    | string    | The metric's name.                                                                                                                                                                                                                                                                                                                                                                 |
| `type`    | string    | The metric's type. Valid types are: `SUM`, `MEAN`, `MEDIAN`, `MAX`, `MIN`, `STANDARD_DEVIATION`, `THROUGHPUT`, `LATENCY`, `PERCENTILE_99TH`, `PERCENTILE_95TH`, `PERCENTILE_90TH`, `PERCENTILE_80TH`, `PERCENTILE_50TH`. This is represented by the [`RollupType` enum](https://github.com/evergreen-ci/cedar/blob/bf4b115ab032fca375e6a86c40f9f8944e55a483/perf.proto#L103-L117). |
| `value`   | int/float | The metric's value.                                                                                                                                                                                                                                                                                                                                                                |
| `version` | int       | (Optional) The metric's version.                                                                                                                                                                                                                                                                                                                                                   |

## downstream_expansions.set

downstream_expansions.set is used by parent patches to pass key-value
pairs to child patches. The command takes the key-value pairs written in
the file and makes them available to the child patches. Note: these
parameters will be public and viewable on the patch page.

``` yaml
- command: downstream_expansions.set
  params:
    file: downstream_expansions.yaml
```

Parameters:

-   `file`: filename to read the expansions from

## s3.get

`s3.get` downloads a file from Amazon s3.

``` yaml
- command: s3.get
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    remote_file: ${mongo_binaries}
    bucket: mciuploads
    local_file: src/mongo-binaries.tgz
```

Parameters:

-   `aws_key`: your AWS key (use expansions to keep this a secret)
-   `aws_secret`: your AWS secret (use expansions to keep this a secret)
-   `local_file`: the local file to save, do not use with `extract_to`
-   `extract_to`: the local directory to extract to, do not use with
    `local_file`
-   `remote_file`: the S3 path to get the file from
-   `bucket`: the S3 bucket to use.
-   `build_variants`: list of buildvariants to run the command for, if
    missing/empty will run for all
-   `optional`: boolean: if set, won't error if the file isn't found or there's an error with downloading.

## s3.put

This command uploads a file to Amazon s3, for use in later tasks or
distribution. Files uploaded with this command will also be viewable within the Parsley log viewer if the `content_type` is set to `text/plain`, `application/json` or `text/csv`.

``` yaml
- command: s3.put
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    local_file: src/mongodb-binaries.tgz
    remote_file: mongodb-mongo-master/${build_variant}/${revision}/binaries/mongo-${build_id}.${ext|tgz}
    bucket: mciuploads
    permissions: public-read
    content_type: ${content_type|application/x-gzip}
    display_name: Binaries
```

Parameters:

-   `aws_key`: your AWS key (use expansions to keep this a secret)
-   `aws_secret`: your AWS secret (use expansions to keep this a secret)
-   `local_file`: the local file to post
-   `remote_file`: the S3 path to post the file to
-   `bucket`: the S3 bucket to use. Note: buckets created after Sept.
    30, 2020 containing dots (".") are not supported.
-   `permissions`: the S3 permissions string to upload with. See [S3 docs](https://docs.aws.amazon.com/AmazonS3/latest/userguide/acl-overview.html#canned-acl)
    for allowed values.
-   `content_type`: the MIME type of the file. Note it is important that this value accurately reflects the mime type of the file or else the behavior will be unpredictable.
-   `optional`: boolean to indicate if failure to find or upload this
    file will result in a task failure. This is intended to be used
    with `local_file`. `local_files_include_filter` be default is 
    optional and will not work with this parameter.
-   `skip_existing`: boolean to indicate that files that already exist
    in s3 should be skipped.
-   `display_name`: the display string for the file in the Evergreen UI
-   `local_files_include_filter`: used in place of local_file, an array
    of gitignore file globs. All files that are matched - ones that
    would be ignored by gitignore - are included in the put. If no
    files are found, the task continues execution.
-   `local_files_include_filter_prefix`: an optional path to start
    processing the `local_files_include_filter`, relative to the working
    directory.
-   `region`: AWS region for the bucket. We suggest us-east-1, since
    that is where ec2 hosts are located. If you would like to override,
    you can use this parameter.
-   `visibility`: one of "private", which allows logged-in users to
    see the file; "public" (the default), which allows anyone to see
    the file; "none", which hides the file from the UI for everybody;
    or "signed", which creates a pre signed url, allowing logged-in
    users to see the file (even if it's private on s3). Visibility:
    signed should not be combined with permissions: public-read or
    permissions: public-read-write.
-   `patchable`: defaults to true. If set to false, the command will
    no-op for patches (i.e. continue without performing the s3 put).
-   `patch_only`: defaults to false. If set to true, the command will
    no-op for non-patches (i.e. continue without performing the s3 put).

## s3.put with multiple files

Using the s3.put command in this uploads multiple files to an s3 bucket.

``` yaml
- command: s3.put
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
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

## s3.push

This command supports the task sync feature, which allows users to
upload and download their task directory to and from Amazon S3. It must
be enabled in the project settings before it can be used.

This command uploads the task directory to S3. This can later be used in
dependent tasks using the [s3.pull](#s3pull) command. There is only one
latest copy of the task sync in S3 per task - if the task containing
s3.push is restarted, it will replace the existing one.

Users also have the option to inspect the task working directory after
it has finished pushing (e.g. for debugging a failed task). This can be
achieved by either pulling the task working directory from S3 onto a
spawn host (from the UI) or their local machines (using [evergreen pull](../CLI.md#pull)).

The working directory is put in a private S3 bucket shared between all
projects. Any other logged in user can pull and view the directory
contents of an s3.push command once it has been uploaded.

``` yaml
- command: s3.push
  params:
     exclude: path/to/directory/to/ignore
     max_retries: 50
```

Parameters:

-   `max_retries`: Optional. The maximum number of times it will attempt
    to push a file to S3.
-   `exclude`: Optional. Specify files to exclude within the working
    directory in [Google RE2
    syntax](https://github.com/google/re2/wiki/Syntax).

## s3.pull

This command helps support the task sync feature, which allows users to
upload and download their task directory to and from Amazon S3. It must
be enabled in the project settings before it can be used.

This command downloads the task directory from S3 that was uploaded by
[s3.push](#s3push). It can only be used in a task which depends on a
task that runs s3.push first and must explicitly specify the dependency
using `depends_on`.

``` yaml
- command: s3.pull
  params:
     task: my_s3_push_task
     from_build_variant: some_other_build_variant
     working_directory: path/to/working/directory
     delete_on_sync: false
     exclude: path/to/directory/to/ignore
     max_retries: 50
```

Parameters:

-   `working_directory`: Required. Specify the location where the task
    directory should be pulled to.
-   `task`: Required. The name of the task to be pulled from. Does not
    accept expansions.
-   `from_build_variant`: Optional. Specify the build variant to pull
    from. If none is provided, it defaults to the build variant on which
    s3.pull runs. Does not accept expansions.
-   `delete_on_sync`: Optional. If set, anything already in the working
    directory that is not in the remote task sync directory will be
    deleted. Defaults to false.
-   `exclude`: Optional. Specify files to exclude from the synced task
    directory in [Google RE2
    syntax](https://github.com/google/re2/wiki/Syntax).
-   `max_retries`: Optional. The maximum number of times it will attempt
    to pull a file from S3.

## s3Copy.copy

`s3Copy.copy` copies files from one s3 location to another

``` yaml
- command: s3Copy.copy
  params:
    aws_key: ${aws_key}
    aws_secret: ${aws_secret}
    s3_copy_files:
        - {'optional': true, 'source': {'path': '${push_path}-STAGE/${push_name}/mongodb-${push_name}-${push_arch}-${suffix}-${task_id}.${ext|tgz}', 'bucket': 'build-push-testing'},
           'destination': {'path': '${push_path}/mongodb-${push_name}-${push_arch}-${suffix}.${ext|tgz}', 'bucket': '${push_bucket}'}}
```

Parameters:

-   `aws_key`: your AWS key (use expansions to keep this a secret)
-   `aws_secret`: your AWS secret (use expansions to keep this a secret)
-   `s3_copy_files`: a map of `source` (`bucket` and `path`),
    `destination`, `build_variants` (a list of strings), `display_name`,
    and `optional` (suppresses errors). Note: destination buckets
    created after Sept. 30, 2020 containing dots (".") are not
    supported.

## shell.exec

This command runs a shell script. To follow [Evergreen best practices](Best-Practices.md#subprocessexec), we recommend using [subprocess.exec](#subprocess.exec).

``` yaml
- command: shell.exec
  params:
    working_dir: src
    script: |
      echo "this is a 2nd command in the function!"
      ls
```

Parameters:

-   `script`: the script to run
-   `working_dir`: the directory to execute the shell script in
-   `env`: a map of environment variables and their values.  In case of
    conflicting environment variables defined by `add_expansions_to_env` or
    `include_expansions_in_env`, this has the lowest priority. Unless
    overridden, the following environment variables will be set by default:
    - "CI" will be set to "true".
    - "GOCACHE" will be set to `${workdir}/.gocache`.
    - "EVR_TASK_ID" will be set to the running task's ID.
    - "TMP", "TMPDIR", and "TEMP" will be set to `${workdir}/tmp`.
-   `add_expansions_to_env`: when true, add all expansions to the
    command's environment. In case of conflicting environment variables
    defined by `env` or `include_expansions_in_env`, this has higher
    priority than `env` and lower priority than `include_expansions_in_env`.
-   `include_expansions_in_env`: specify one or more expansions to
    include in the environment. If you specify an expansion that does
    not exist, it is ignored. In case of conflicting environment
    variables defined by `env` or `add_expansions_to_env`, this has
    highest priority.
-   `background`: if set to true, the script runs in the background
    instead of the foreground. `shell.exec` starts the script but
    does not wait for the script to exit before running the next command. 
    If the background script exits with an error while the
    task is still running, the task will continue running.
-   `silent`: if set to true, does not log any shell output during
    execution; useful to avoid leaking sensitive info. Note that you should 
    not pass secrets as command-line arguments but instead as environment
    variables or from a file, as Evergreen runs `ps` periodically, which
    will log command-line arguments.
-   `continue_on_err`: by default, a task will fail if the script returns
    a non-zero exit code; for scripts that set `background`, the task will
    fail only if the script fails to start. If `continue_on_err`
    is true and the script fails, it will be ignored and task
    execution will continue.
-   `system_log`: if set to true, the script's output will be written to
    the task's system logs, instead of inline with logs from the test
    execution.
-   `shell`: shell to use. Defaults to sh if not set. Note that this is
    usually bash but is dash on Debian, so it's good to explicitly pass
    this parameter
-   `ignore_standard_out`: if true, discards output sent to stdout
-   `ignore_standard_error`: if true, discards output sent to stderr
-   `redirect_standard_error_to_output`: if true, sends stderr to
    stdout. Can be used to synchronize these 2 streams
-   `exec_as_string`: if true, executes as "sh -c 'your script
    here'". By default, shell.exec runs sh then pipes your script to
    its stdin. Use this parameter if your script will be doing something
    that may change stdin, such as sshing

## subprocess.exec

The subprocess.exec command executes a binary file. On a Unix-like OS,
you can also run a `#!` script as if it were a binary. To
get similar behavior on Windows, try `bash.exe -c
yourScript.sh`.

``` yaml
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

-   `binary`: a binary to run
-   `args`: a list of arguments to the binary
-   `env`: a map of environment variables and their values. In case of
    conflicting environment variables defined by `add_expansions_to_env` or
    `include_expansions_in_env`, this has the lowest priority. Unless
    overridden, the following environment variables will be set by default:
    - "CI" will be set to "true".
    - "GOCACHE" will be set to `${workdir}/.gocache`.
    - "EVR_TASK_ID" will be set to the running task's ID.
    - "TMP", "TMPDIR", and "TEMP" will be set to `${workdir}/tmp`.
-   `command`: a command string (cannot use with `binary` or `args`), split
    according to shell rules for use as arguments.
    - Note: Expansions will *not* be split on spaces; each expansion represents
      its own argument.
    - Note: on Windows, the shell splitting rules may not parse the command
      string as desired (e.g. for Windows paths containing `\`).
-   `background`: if set to true, the process runs in the background
    instead of the foreground. `subprocess.exec` starts the process but
    does not wait for the process to exit before running the next command. 
    If the background process exits with an error while the
    task is still running, the task will continue running.
-   `silent`: do not log output of command. Note that you should
    not pass secrets as command-line arguments but instead as environment
    variables or from a file, as Evergreen runs `ps` periodically, which
    will log command-line arguments.
-   `system_log`: write output to system logs instead of task logs
-   `working_dir`: working directory to start shell in
-   `ignore_standard_out`: if true, do not log standard output
-   `ignore_standard_error`: if true, do not log standard error
-   `redirect_standard_error_to_output`: if true, redirect standard
    error to standard output
-   `continue_on_err`: by default, a task will fail if the command returns
    a non-zero exit code; for command that set `background`, the task will
    fail only if the command fails to start. If `continue_on_err`
    is true and the command fails, it will be ignored and task
    execution will continue.
-   `add_expansions_to_env`: when true, add all expansions to the
    command's environment. In case of conflicting environment variables
    defined by `env` or `include_expansions_in_env`, this has higher
    priority than `env` and lower priority than `include_expansions_in_env`.
-   `include_expansions_in_env`: specify one or more expansions to
    include in the environment. If you specify an expansion that does
    not exist, it is ignored. In case of conflicting environment variables
    defined by `env` or `add_expansions_to_env`, this has highest
    priority.
-   `add_to_path`: specify one or more paths to prepend to the command `PATH`,
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

## timeout.update

This command sets `exec_timeout_secs` or `timeout_secs` of a task from
within that task.

``` yaml
- command: timeout.update
  params:
    exec_timeout_secs: ${my_exec_timeout_secs}
    timeout_secs: ${my_timeout_secs}
```

Parameters:

-   `exec_timeout_secs`: set `exec_timeout_secs` for the task, which is
    the maximum amount of time the task may run. May be int, string, or
    expansion
-   `timeout_secs`: set `timeout_secs` for the task, which is the
    maximum amount of time that can elapse without any output on stdout.
    May be int, string, or expansion

Both parameters are optional. If not set, the task will use the
definition from the project config.

Commands can also be configured to run if timeout occurs, as documented [here](Project-Configuration-Files.md#timeout-handler).
