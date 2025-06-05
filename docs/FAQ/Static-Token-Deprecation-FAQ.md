# Static Token Deprecation FAQ

## What is being deprecated?

We will transition over to using personal access tokens for all human users instead of the static api keys we have been using and downloading from [preferences/cli](https://spruce.mongodb.com/preferences/cli) until now.


## What does this affect?

- **UI**: No Changes
  - You will be able to continue using the Evergreen and Spruce UI without any changes.
- **CLI**: Action Needed 
  - Until now, you were able to use a static API key saved to a `~/.evergreen.yml` file. This will no longer be accepted after the transition. Please see [here](../CLI.md#authentication).
- **REST API**: Action Needed for Scripting with Human Users
  - **Using the REST API in the browser**: Not affected.
  - **Using the REST API with a service user**: Not affected (e.g., calling it with a service user in scripts that run in Evergreen tasks).
  - **Using the REST API with a human user**: Affected. Please see [here](../API/REST-V1-Usage#authentication).


## I am using a static Evergreen API key in an Evergreen task, will this continue to work?

If the API key belongs to a [service user](../Project-Configuration/Project-and-Distro-Settings#service-users) it will continue to work as expected. If it belongs to a human user, please reach out to the evergreen team to have a service user created for you. 


## Will spawn hosts be affected?

Yes, to use the evergreen CLI on a spawn host, you will need to copy/paste the link provided into your laptop's browser when prompted. You will not need to do any other additional setup. 

## How often will I be asked to click on a link to authenticate?

Please see [here](https://kanopy.corp.mongodb.com/docs/corpsecure/auth_flow/#refresh-token).

## Why am I being asked to copy and paste a link instead of the link being opened in a browser automatically?

We implemented it this way so that it is compatible with spawn hosts. 