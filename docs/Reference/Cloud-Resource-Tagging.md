# Cloud Resource Tagging

The [MongoDB Cloud Tenant Tag Policy](https://wiki.corp.mongodb.com/spaces/SEC/pages/560370899/Cloud+Tag+Policy)
is the authoritative source for requirements and exceptions. This page summarizes Evergreen's AWS tagging behavior.

For more information, ask in the `#ask-cloud-tagging` Slack channel.

## Required Corporate Tags

| Tag             | Required value                                                                              |
| --------------- | ------------------------------------------------------------------------------------------- |
| `mongodb-owner` | A reachable individual or team email address ending in `@mongodb.com`.                      |
| `mongodb-env`   | One of `dev`, `qa`, `test`, `local`, `poc`, `demo`, `uat`, `sandbox`, `staging`, or `prod`. |

## Evergreen Configuration

Administrators set defaults in global admin settings, not project YAML or distro settings. Example:

```yaml
providers:
  aws:
    resource_tags:
      mongodb_owner: evergreen@mongodb.com
      mongodb_env: staging
```

Use the responsible team email and deployment environment. Evergreen validates email syntax but does not enforce the
`@mongodb.com` domain. Defaults may initially be unset, but admin settings updates cannot clear a value once set.

## Automatic AWS Tagging

Evergreen tags EC2 instances and their launch-time EBS volumes, including root volumes, and separately created EBS volumes.

| Resource                             | Default `mongodb-owner`     |
| ------------------------------------ | --------------------------- |
| CI hosts and hosts spawned by tasks  | Configured team email.      |
| Personal spawn hosts and debug hosts | Requesting user's email.    |
| Launch-time EBS volumes              | Same owner as the instance. |
| Separately created EBS volumes       | Volume creator's email.     |

If a user's email is unavailable or a volume creator lookup fails, Evergreen falls back to the configured owner.
All these resources default to the configured `mongodb-env`, including personal spawn hosts.
