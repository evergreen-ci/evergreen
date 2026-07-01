# Authentication

Human users authenticate with Evergreen's REST API using OAuth tokens. Service users, also called API users, authenticate using static credentials.

## OAuth

OAuth is for human users. [Service users](../Project-Configuration/Project-and-Distro-Settings#service-users) should use [static API keys](#static-api-keys).

To authenticate using OAuth, include a valid OAuth token as the `Authorization` header in your request. You can get one by running `evergreen client get-oauth-token`.

OAuth tokens can only be used when authenticating for evergreen.corp.mongodb.com, they cannot be used with evergreen.mongodb.com.

### Examples

> Note: Please make sure to use `https://evergreen.corp.mongodb.com`.

#### Rest API v1

```bash
curl -H "Authorization: Bearer $(evergreen client get-oauth-token)" https://evergreen.corp.mongodb.com/rest/v1/projects/my_private_project
```

> Note, your session may be expired. You should run `evergreen login` to refresh your session before running the above command.

#### Rest API v2

```bash
curl -H "Authorization: Bearer $(evergreen client get-oauth-token)" https://evergreen.corp.mongodb.com/rest/v2/projects/my_private_project
```

> Note, your session may be expired. You should run `evergreen login` to refresh your session before running the above command.

## Static API Keys

Static API keys are for [service users](../Project-Configuration/Project-and-Distro-Settings#service-users), also called API users. Human users should use [OAuth](#oauth).

Use the `user` and `api_key` fields from your Evergreen configuration file, typically located at `~/.evergreen.yml`.
Authenticated REST access requires setting two headers, `Api-User` and `Api-Key`.

Static API keys can only be used when authenticating for evergreen.mongodb.com, they cannot be used with evergreen.corp.mongodb.com.

### Example

> Note: Please make sure to use `https://evergreen.mongodb.com`.

#### Rest API v1

```bash
    curl -H Api-User:my.name -H Api-Key:21312mykey12312 https://evergreen.mongodb.com/rest/v1/projects/my_private_project
```

#### Rest API v2

```bash
    curl -H "Api-User:my.name" -H "Api-Key:21312mykey12312" https://evergreen.mongodb.com/rest/v2/projects/my_private_project
```
