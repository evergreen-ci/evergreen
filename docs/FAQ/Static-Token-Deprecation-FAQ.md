# Static Token Deprecation FAQ

## What is being deprecated?

We will transition over to using personal access tokens for all human users instead of the static api keys we have been using and downloading from [preferences/cli](https://spruce.mongodb.com/preferences/cli) until now.

## When will static tokens be deprecated?

For updates on this migration, please see [DEVPROD-4160](https://jira.mongodb.org/browse/DEVPROD-4160).

## What does this affect?

- **UI**: No Changes
  - You will be able to continue using the Evergreen and Spruce UI without any changes.
- **CLI**: Action Needed
  - Until now, you were able to use a static API key saved to a `~/.evergreen.yml` file. This will no longer be accepted after the transition. Please see [here](../CLI.md#authentication).
- **REST API**: Action Needed for Scripting with Human Users
  - **Using the REST API in the browser**: Not affected.
  - **Using the REST API with a service user**: Not affected (e.g., calling it with a service user in scripts that run in Evergreen tasks).
  - **Using the REST API with a human user**: Affected. Please see [here](../API/REST-V1-Usage#authentication).

## I use the Evergreen CLI, what should I do to prepare for the deprecation?

- Ensure that your Evergreen CLI is not out of date, you can use `evergreen get-update` to upgrade.
- [Install kanopy-oidc](../CLI.md##install-kanopy-oidc).
- Try to run an evergreen command, for example `evergreen volume list` and [authenticate when prompted](../CLI.md#authenticate-when-prompted).

## I own a script that is used by manual users that uses the Evergreen api, what should I do to prepare for the deprecation?

- Switch your script to use evergreen.mongodb.com for service users along with the headers specified [here](../API/REST-V1-Usage#static-api-keys), and evergreen.corp.mongodb.com for non service users with the headers specified [here](<https://wiki.corp.mongodb.com/spaces/DBDEVPROD/pages/384992097/Kanopy+Auth+On+Evergreen#KanopyAuthOnEvergreen-RESTAPI(V1andV2)>).
- If you use [evergreen.py](https://github.com/evergreen-ci/evergreen.py) in your script, it will attempt to generate a personal access token and use that to authenticate when no api key is saved in the .evergreen.yml config file. Please comment out the api key in your config file and make sure it works as expected.
- We are working on providing a CLI command that will make it easier for scripts that rely on the .evergreen.yml config file. Please follow [DEVPROD-17996](https://jira.mongodb.org/browse/DEVPROD-17996) for updates.

## I am using a static Evergreen API key in an Evergreen task, will this continue to work?

If the API key belongs to a [service user](../Project-Configuration/Project-and-Distro-Settings#service-users) it will continue to work as expected. If it belongs to a human user, please reach out to the evergreen team to have a service user created for you.

## Will spawn hosts be affected?

Yes, to use the evergreen CLI on a spawn host, you will need to copy/paste the link provided into your laptop's browser when prompted. You will not need to do any other additional setup.

## How often will I be asked to click on a link to authenticate?

Please see [here](https://kanopy.corp.mongodb.com/docs/corpsecure/auth_flow/#refresh-token).

## Why am I being asked to copy and paste a link instead of the link being opened in a browser automatically?

We implemented it this way so that it is compatible with spawn hosts.

## Why did I have to wait a while for the CLI command to load after authenticating?

Due to a polling interval backoff, the longer it takes for a user to click on the link to authenticate, the longer it will take to load. Luckily, this doesn't apply to refreshes which are almost instant.
