# Simple Configurations for Simple Projects

Evergreen provides functionality to help simple projects easily create project configurations. Suitable projects are
ones that use a single build/testing framework and does not rely on additional setup, testing infrastructure, or
orchestration.

The supported build/testing frameworks are:
* Go
* Make

To generate project configurations, you will need:
* The Evergreen CLI
* Build files

In order to generate project configs, you will have to do the following:
1. Create the [build file(s)](#build-files).
2. Pass the build file(s) to the Evergreen CLI to create the project configuration.

## Build Files
Build files are intermediate YAML files used to generate project configurations. You can write the build files
in two different ways:
1. With a _generator file_: a single YAML file containing all the build configuration information.
2. With a _control file_: a single YAML file containing references to multiple other YAML files, each of which have a
   part of the build configuration information.

### Variables Section
Build files are parsed using [strict YAML semantics](https://godoc.org/gopkg.in/yaml.v2#UnmarshalStrict). In order to
facilitate usage of other YAML constructs such as defining anchors to use as aliases, all build files support a
`variables` top-level section which can be populated with arbitrary configuration as needed.

---

## Golang Projects
This is built for projects that rely solely on the Go binary to do CI testing. Each task generated in the Evergreen
configuration corresponds to a package defined in the build file(s). After the Go tests have run, they will be
automatically uploaded as test results.

Go also supports [automatic package discovery](#automatic-package-discovery) as a convenience so that you do not need to
explicitly define all the packages that need to be tested.

---

### Automatic Package Discovery
When you [use the CLI](#generating-project-configurations-with-the-cli) to generate your project configuration,
package can be automatically defined based on the structure of your local project copy so that you do not have to
explicitly define every single package that you wish to test. Any package within the root package that contains a Go
test file (i.e. `*_test.go`) will be defined if there is not already an explicit definition for that package in the
build file. That way, you can reference packages within your variants without needing to write boilerplate definitions.
Automatically discovered packages are equivalent to package definitions that have only the path specified.

It is also possible to create package definitions for packages containing only Go source files and no tests (i.e.
`*.go`). This can be achieved with the `discover_source_files` option in the [top-level
configuration](#top-level-configuration).

#### Example
The root directory is laid out as follows:
```txt
.
├── util
├── foo
│   ├── bar
│   │   └── bar.go
│   ├── bat
│   │   ├── foo.go
│   │   └── foo_test.go
│   ├── foo.go
│   └── foo_test.go
├── util.go
└── util_test.go
```
Assuming there are no packages explicitly defined, package discovery will create the following [package
definitions](#package-definition):
```yaml
packages:
  - path: .
  - path: util
  - path: foo
  - path: foo/bat
```
`foo/bar` will not be defined as a package since it does not contain any Go test files.

---

### Generator File
Use a generator file if you wish to define the entire intermediate build in a single file.

#### Top-Level Configuration
This defines the top-level configuration sections and settings.

| Key                   | Required | Type            | Description                                                                                                                                                  |
|-----------------------|----------|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|
| working_dir           | N        | String          | The directory relative to the task working directory where the project should be cloned. Defaults to `${GOPATH}/src/${root_package}`.                        |
| root_package          | Y        | String          | The name of the project's top-level package (e.g. for Evergreen, it's `github.com/evergreen-ci/evergreen`).                                                  |
| variants              | Y        | List            | Define variants that should run tests and which packages should be tested.                                                                                   |
| env                   | Y        | Map             | Define environment variables that will apply globally. `GOROOT` and `GOPATH` are required. `GOPATH` should be a path relative to the task working directory. |
| packages              | N        | List            | Explicitly define Go packages. Can be automatically populated using [package discovery](#automatic-package-discovery).                                       |
| discover_source_files | N        | Bool            | Discover packages even if they contain only Go source files when discovering packages.                                                                       |
| default_tags          | N        | List of Strings | Define tags that apply to all packages by default (unless explicitly excluded by a package with `exclude_tags`).                                             |
| variables             | N        | None            | See [variables section](#variables-section).

##### Example
```yaml
root_package: github.com/owner/repo
packages:
  - name: test-root
    path: .
variants:
  - name: ubuntu
    distros: ["ubuntu1804-small", "ubuntu1804-large"]
    packages:
      - name: test-root
env:
  GOPATH: relative/path/to/gopath
  GOROOT: /absolute/path/to/goroot
```

#### Variant Definition
Tasks within the variant are created based on the specified packages that the variant should run.

| Key      | Required | Type            | Description                                                                                                                  |
|----------|----------|-----------------|------------------------------------------------------------------------------------------------------------------------------|
| name     | Y        | String          | The variant name.                                                                                                            |
| distros  | Y        | List of Strings | The distros this variant can run on.                                                                                         |
| packages | Y        | List            | References to packages to run on this variant by name, path, or tag.                                                         |
| env      | N        | Map             | Define environment variables for this variant. This has higher precedence than the global and package environment variables. |
| flags    | N        | List of Strings | Additional flags to pass to the underlying Go binary for each package.                                                       |

##### Example
```yaml
variants:
  - name: ubuntu
    distros: ["ubuntu1804-small", "ubuntu1804-large"]
    packages:
      - tag: coverage
  - name: arch
    distros: ["archlinux-small"]
    packages:
      - name: test-util
      - path: .
    flags: ["-race"]
```

#### Package Definition
Packages specify configuration for each Go package that should be tested.

| Key          | Required | Type            | Description                                                                                                                                                                                                      |
|--------------|----------|-----------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| path         | Y        | String          | The path to the package to test, relative to the root package. The root package can be referenced using a dot (`"."`).                                                                                           |
| name         | N        | String          | An alias for the package.                                                                                                                                                                                        |
| tags         | N        | List of Strings | Define tags for the package to logically group it with other packages.                                                                                                                                           |
| exclude_tags | N        | List of Strings | Top-level default tags that should not be applied to the package.                                                                                                                                                |
| env          | N        | Map             | Define environment variables for this package. `GOPATH` cannot be redefined for a package. This has higher precedence that global environment variables but lower precedence than variant environment variables. |
| flags        | N        | List of Strings | Additional flags to pass to the underlying Go binary.                                                                                                                                                            |


##### Example
```yaml
packages:
  - name: test-util
    path: ./util
    tags: ["race-detector", "test"]
    flags: ["-race"]
  - path: .
    tags: ["test"]
    flags: ["-timeout=20m"]
    env:
      GOROOT: /path/to/GOROOT
```

---

### Control File
Use a control file if you wish to define the intermediate build with multiple separate files.

| Key      | Required | Type            | Description                                                                                       |
|----------|----------|-----------------|---------------------------------------------------------------------------------------------------|
| general  | Y        | String          | The path to the file containing the top-level configuration. Supports gitignore-style globbing.   |
| variants | Y        | List of Strings | The path(s) to the file(s) containing the variant definitions. Supports gitignore-style globbing. |
| packages | Y        | List of Strings | The path(s) to the file(s) containing the package definitions. Supports gitignore-style globbing. |

#### Example
```yaml
general: general.yaml
variants:
  - variants1.yaml
  - variants2.yaml
packages:
  - packages*.yaml
```

#### General File
The general file defines top-level miscellaneous configuration.

| Key                   | Required | Type            | Description                                                                                                                                                         |
|-----------------------|----------|-----------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| root_package          | Y        | String          | The name of the project's top-level package (e.g. for Evergreen, it's `github.com/evergreen-ci/evergreen`).                                                         |
| env                   | Y        | Map             | Define environment variables that will apply to all packages. `GOROOT` and `GOPATH` are required. `GOPATH` should be a path relative to the task working directory. |
| discover_source_files | N        | Bool            | Include packages containing only source files when discovering packages. By default, this is false.                                                                 |
| default_tags          | N        | List of Strings | Define tags that apply to all packages by default (unless explicitly excluded by a package with `exclude_tags`).                                                    |

#### Variant File
See [variant definition](#variant-definition). Also supports a [variables section](#variables-section).

#### Package File
See [package definition](#package-definition). Also supports a [variables section](#variables-section).

---

### Generating Project Configurations with the CLI
The CLI is used to generate project configurations once you have the build files. You must give it either a generator
file or a control file.

| Flag           | Required | Type   | Description                                                                    |
|----------------|----------|--------|--------------------------------------------------------------------------------|
| discovery_dir  | Y        | String | The directory where package discovery should occur.                            |
| generator_file | N        | String | The path to the generator file. Either this or control_file must be specified. |
| control_file   | N        | String | The path to the control file. Either this or generator_file must be specified. |
| output_file    | N        | String | The file where the configuration should be written.                            |
| output_format  | N        | String | The output format ("json" or "yaml"). By default, this is "yaml".              |

#### Quick Usage
Create a project configuration from a [generator file](#generator-file):
```sh
evergreen generate golang --discovery_dir <path_to_discovery_directory> --generator_file <path_to_generator_file>
```
Alternatively, if you use a [control file](#control-file), use the following command:
```sh
evergreen generate golang --discovery_dir <path_to_discovery_directory> --control_file <path_to_control_file>
```

---

## Make Projects
This is built for projects that rely solely on Make to do CI testing. Each task generated in the
project configuration corresponds to a package defined in the build file(s). After running tests, there is an option
to upload test result files.

---

### Generator File
Use a generator file if you wish to define the entire intermediate build in a single file.

#### Top-Level Configuration
This defines the top-level configuration sections and settings.

| Key          | Required | Type            | Description                                                                                                                              |
|--------------|----------|-----------------|------------------------------------------------------------------------------------------------------------------------------------------|
| working_dir  | N        | String          | The directory relative to the task working directory where the project should be cloned. By default, this is the task working directory. |
| tasks        | Y        | List            | Define tasks to run.                                                                                                                     |
| variants     | Y        | List            | Define variants that should run tasks.                                                                                                   |
| sequences    | N        | List            | Define sequences of targets to run.                                                                                                      |
| env          | N        | Map             | Define environment variables that will apply to all task.                                                                                |
| default_tags | N        | List of Strings | Define tags that apply to all tasks by default (unless explicitly excluded by a task with `exclude_tags`).                               |
| variables    | N        | None            | See [variables section](#variables-section).                                                                                             |

##### Example
```yaml
tasks:
- name: foo
  targets:
    - name: setup
    - name: test
variants:
  - name: ubuntu
    distros: ["ubuntu1804-small", "ubuntu1804-large"]
    tasks:
      - name: foo
```

### Task Definition
Tasks are defined in terms of a sequence of Make targets that should run for each task.

| Key          | Required | Type            | Description                                                                                                                                                       |
|--------------|----------|-----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| name         | Y        | String          | The name of the task.                                                                                                                                             |
| targets      | Y        | List            | The targets or target sequences to run.                                                                                                                           |
| tags         | N        | List of Strings | Define tags for the task to logically group it with other tasks.                                                                                                  |
| exclude_tags | N        | List of Strings | Top-level default tags that should not be applied to the task.                                                                                                    |
| env          | N        | Map             | Define environment variables for this task. This has higher precedence that global environment variables but lower precedence than variant environment variables. |
| flags        | N        | List of Strings | Additional flags to pass to the underlying Make binary for each target in the task.                                                                               |
| reports      | N        | List            | Describe how test results should be reported after the task has finished running all targets.                                                                     |

#### Example
```yaml
tasks:
  - name: test
    targets:
      - name: setup
      - sequence: test
```

#### Task Target Definition
Task targets are a way to reference a Make target or [a sequence of targets](#target-sequence-definition) to run.

| Key      | Required | Type            | Description                                                                                                |
|----------|----------|-----------------|------------------------------------------------------------------------------------------------------------|
| name     | N        | String          | The name of the target to run. Either this or sequence must be specified.                                  |
| sequence | N        | String          | The name of the target sequence to run. Either this or name must be specified.                             |
| flags    | N        | List of Strings | Additional flags to pass to the underlying Make binary for this target or sequence of targets.             |
| reports  | N        | List            | Describe how test results should be reported after the target or sequence of targets has finished running. |

##### Example
```yaml
targets:
  - name: setup
    flags: ["-k"]
  - sequence: test
    reports:
      - files: ["output.txt"]
        format: gotest
```

### Target Sequence Definition
Target sequences are ordered sequences of Make targets.

| Key     | Required | Type            | Description                      |
|---------|----------|-----------------|----------------------------------|
| Name    | Y        | String          | The name of the target sequence. |
| Targets | Y        | List of Strings | The targets to run.              |

#### Example
```yaml
sequences:
  - name: test
    targets:
      - setup
      - test
      - teardown
```

### File Report Definition
File reports provide a way to upload test results.

| Key    | Required | Type   | Description                                                                                                |
|--------|----------|--------|------------------------------------------------------------------------------------------------------------|
| files  | Y        | String | The path(s) to the file(s) to upload.                                                                      |
| format | Y        | String | The format of the file(s) to upload. Recognized formats are "artifact", "evg-json", "gotest", and "xunit". |

#### Example
```yaml
tasks:
  - name: test
    targets:
      - name: setup
      - name: test
        reports:
          - files: ["output.txt"]
            format: xunit
      - name: teardown
    reports:
      - files: ["artifacts.json"]
        format: artifact

```

### Variant Definition
Variants run sets of Make tasks.

| Key     | Required | Type            | Description                                                                                                               |
|---------|----------|-----------------|---------------------------------------------------------------------------------------------------------------------------|
| name    | Y        | String          | The variant name.                                                                                                         |
| distros | Y        | List of Strings | The distros this variant can run on.                                                                                      |
| tasks   | Y        | List            | References to tasks to run on this variant by name or tag.                                                                |
| env     | N        | Map             | Define environment variables for this variant. This has higher precedence than the global and task environment variables. |
| flags   | N        | List of Strings | Additional flags to pass to the underlying Make binary for each Make target run in each task.                             |

#### Example
```yaml
variants:
  - name: ubuntu
    distros: ["ubuntu1604-small", "ubuntu1604-large"]
    tasks:
      - name: lint
      - tag: test
```

---

### Control File
Use a control file if you wish to define the intermediate build with multiple separate files.

| Key      | Required | Type            | Description                                                                                       |
|----------|----------|-----------------|---------------------------------------------------------------------------------------------------|
| general  | N        | String          | The path to the file containing the top-level configuration. Supports gitignore-style globbing.   |
| variants | Y        | List of Strings | The path(s) to the file(s) containing the variant definitions. Supports gitignore-style globbing. |
| tasks    | Y        | List of Strings | The path(s) to the file(s) containing the task definitions. Supports gitignore-style globbing.    |

#### Example
```yaml
general: general.yaml
variants:
  - variants1.yaml
  - variants2.yaml
tasks:
  - tasks*.yaml
```

#### General File
The general file defines top-level miscellaneous configuration.

| Key                   | Required | Type            | Description                                                                                              |
|-----------------------|----------|-----------------|----------------------------------------------------------------------------------------------------------|
| env                   | N        | Map             | Define environment variables that will apply to all task.                                                |
| discover_source_files | N        | Bool            | Discover packages containing only source files when discovering packages. By default, this is false.     |
| default_tags          | N        | List of Strings | Define tags that apply to all tasks by default (unless explicitly excluded by a task with `exclude_tags`). |

#### Variant File
See [variant definition](#variant-definition-1). Also supports a [variables section](#variables-section).

#### Task File
See [task definition](#task-definition). Also supports a [variables section](#variables-section).

---

### Generating Project Configurations with the CLI
The CLI is used to generate project configurations once you have the build files. You must give it either a
generator file or a control files.

| Flag           | Required | Type   | Description                                                                         |
|----------------|----------|--------|-------------------------------------------------------------------------------------|
| discovery_dir  | Y        | String | The directory where [package discovery](#automatic-package-discovery) should occur. |
| output_file    | N        | String | The file where the configuration should be written.                                 |
| generator_file | N        | String | The path to the generator file. Either this or control_file must be specified.      |
| control_file   | N        | String | The path to the control file. Either this or generator_file must be specified.      |

#### Quick Usage
Create a project configuration from a [generator file](#generator-file):
```sh
evergreen generate make --generator_file <path_to_generator_file>
```
Alternatively, if you use a [control file](#control-file), use the following command:
```sh
evergreen generate make --control_file <path_to_control_file>
```
