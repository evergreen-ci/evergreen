# Cloud Resource Tagging

The [MongoDB Cloud Tenant Tag Policy](https://wiki.corp.mongodb.com/spaces/SEC/pages/560370899/Cloud+Tag+Policy)
is the authoritative source for requirements and exceptions. This page summarizes Evergreen's AWS tagging behavior.

For more information, ask in the `#ask-cloud-tagging` Slack channel.

## Required Corporate Tags

| Tag             | Required value                                                                              |
| --------------- | ------------------------------------------------------------------------------------------- |
| `mongodb-owner` | A reachable individual or team email address ending in `@mongodb.com`.                      |
| `mongodb-env`   | One of `dev`, `qa`, `test`, `local`, `poc`, `demo`, `uat`, `sandbox`, `staging`, or `prod`. |

## Automatic AWS Tagging

Evergreen tags EC2 instances and their launch-time EBS volumes, including root volumes, and separately created EBS volumes.

| Default `mongodb-owner`    | Resources                                                                                              |
| -------------------------- | ------------------------------------------------------------------------------------------------------ |
| User's email               | Personal spawn hosts, debug hosts, their launch-time EBS volumes, and separately created user volumes. |
| The Evergreen team's email | CI hosts, hosts spawned by tasks, and their launch-time EBS volumes.                                   |

Evergreen also automatically adds a `mongodb-env` tag to these EC2 instances and EBS volumes.
