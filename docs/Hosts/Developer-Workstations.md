# Developer Workstations

## Overview

Evergreen provides a special class and configuration of spawn hosts for
use as _virtual workstations_ to provide a cloud-managed developer
environment similar to Evergreen's execution environment. These
workstations can use the same configuration as build hosts, but also have:

- a persistent volume attached to each image (mounted at
  `~`) that moves between
  instances, so users can upgrade by terminating their instance and
  starting a new one.

- a web-based IDE based on [Code Server](https://github.com/cdr/code-server),
  which is a distribution of the open source components of
  VSCode. This runs remotely on the workstation and proxies through
  the Evergreen application layer for a fully featured remote editing
  experience.

- Evergreen supports an "unexpirable" spawn host which isn't
  subjected to the termination deadline of most spawn hosts. While
  there is a global per-user limit for unexpirable hosts,
  workstations will, by default.

- Evergreen supports a start/stop mode for spawn hosts. This makes it
  possible for users to change to another instance type, though
  administrators must configure the allowable instance types. Users
  can also opt to stop hosts during vacations and other periods of
  known inactivity.

- Passwordless sudo access for workstation configuration.

Administrators need to configure workstation instances to include the
[IDE](https://github.com/evergreen-ci/ide), and any other software
required for development. There is no limit to the number of distinct
workstation images available in the system.

Evergreen site and project administrators should provide specific
documentation for using these workstations in the course of normal
development.

## Project Setup

To support easier workstation setup, project configurations and the
Evergreen CLI tool have a "project setup" command to help get projects
checked out and running on workstations, though this feature is not
necessarily dependent on workstations, and could be used on local
systems.

Project settings include a ["Workstation Setup" section](../Project-Configuration/Project-and-Distro-Settings#virtual-workstation-commands)
where administrators declare a number of simple commands (and directory
contexts) that will run a project's setup. These commands are _not_ shell
interpolated, and are _not_ meant to provision the development environment (e.g.
install system packages or modify configuration files in `~/`). Use these
commands to clone a repository, generate index files, and/or run a test build.

As a prerequisite, users of the project setup _must_ have configured
their SSH keys with GitHub, with access to the GitHub repositories
required for their project. The commands will assemble a clone
operation for the project's core repository when selected, but
required modules or other repositories would need to be cloned
directly in another command.

The Evergreen CLI would resemble:

    evergreen host configure --project=evergreen --distro=ubuntu1804-workstation

This will run the clone (if configured) and setup commands in the
workstation distro's mounted volume directory
`~/user_home/evergreen`. Directories will be created prior to execution.

Commands that specify a directory will run as a sub-directory of the
project directory. This allows a user to have multiple projects
checked out in their persistent workspace. While this doesn't allow
interaction between the setups of potentially similar projects, it
greatly reduces the internal complexity of running setup.

To test workstation setup commands locally, the `--dry-run` argument
causes all commands to noop, printing the commands that would have
run.

You may also omit the `--distro` argument to use locally. This makes
it possible to set your own `--directory` and setup a project at the
location of your choice. The project directory and intermediate
required directories will be created as normally. The `-distro`
argument only modifies the path of the directory _if_ the distro
specified workstation. If you specify
`--distro` and `--directory` the `--directory` creates a prefix
within the workstation distro's persistent home directory.

## Migrate a volume to a new workstation

To facilitate upgrading your virtual workstations, home volumes can be migrated to a new workstation using the "Migrate" button on the [My Volumes](https://spruce.corp.mongodb.com/spawn/volume) page:

![Screen Shot 2022-11-14 at 11 59 33 AM](https://user-images.githubusercontent.com/9298431/201720970-3303d26e-c9d3-400f-8a50-22a23b05a1f4.png)

This feature will allow you to configure the parameters of a new host. It will then move the volume over to the new workstation and set the old host to expire in 24 hours.

## Mounting Additional Storage

For instructions on mounting additional storage to your workstation, please refer to the [Mounting Additional Storage section in the Spawn Hosts documentation](Spawn-Hosts.md#mounting-additional-storage).
