# Static Token Deprecation FAQ

## What is being deprecated?

We will transition over to using OAuth for all human users instead of the static api keys we have been using.

## When will static tokens be deprecated?

For updates on this migration, please see [DEVPROD-4160](https://jira.mongodb.org/browse/DEVPROD-4160).

## What does this affect?

- **UI**: No Changes
  - You will be able to continue using the Evergreen and Spruce UI without any changes.
- **CLI**: No Action Needed
  - Until now, you were able to use a static API key saved to a `~/.evergreen.yml` file. This will no longer be accepted after the transition. Please see [here](../CLI.md#authentication).
- **REST API**: Action Needed for Scripting with Human Users
  - **Using the REST API in the browser**: Not affected.
  - **Using the REST API with a service user**: Not affected (e.g., calling it with a service user in scripts that run in Evergreen tasks).
  - **Using the REST API with a human user**: Affected. Please see [here](../API/REST-V1-Usage#authentication).

## I use the Evergreen CLI, what should I do to prepare for the deprecation?

- Please make sure you are on the latest version of the Evergreen CLI. You can run `evergreen update` to update to the latest version.

## I own a script that is used by manual users that uses the Evergreen api, what should I do to prepare for the deprecation?

- During the script, the Evergreen CLI will prompt the user to authenticate via a link that they will need to copy/paste into their browser. After authenticating, they will be able to return to the script and continue using it as normal.
- If a human user can't authenticate via a browser (e.g., running in the background on a server), please reach out to the Evergreen team to have a service user created for you.

## I am using a static Evergreen API key in an Evergreen task, will this continue to work?

If the API key belongs to a [service user](../Project-Configuration/Project-and-Distro-Settings#service-users) it will continue to work as expected. If it belongs to a human user, please reach out to the evergreen team to have a service user created for you.

## Will spawn hosts be affected?

Yes, please see the documentation on [spawn hosts and the Evergreen CLI](../Hosts/Spawn-Hosts.md#evergreen-cli).

For spawn hosts that should fetch task binaries and artifacts automatically, after SSHing into the host you will need to run:

```sh
evergreen host fetch
```

## How often will I be asked to click on a link to authenticate?

Please see [here](https://kanopy.corp.mongodb.com/docs/corpsecure/auth_flow/#refresh-token).

## What if I am in an enviornment where I cannot open a browser to authenticate?

Please follow the documentation [here](../Hosts/Spawn-Hosts.md#evergreen-cli).

## Why did I have to wait a while for the CLI command to load after authenticating?

Due to a polling interval backoff, the longer it takes for a user to click on the link to authenticate, the longer it will take to load. Luckily, this doesn't apply to refreshes which are almost instant.
