# Authentication

There are two methods to authenticate with Evergreen's REST API: OAuth tokens and Static API Keys. Static API Keys will soon be deprecated for human users, who should use OAuth instead.

## OAuth

_Note_: This is only available for human users, [service users](../Project-Configuration/Project-and-Distro-Settings#service-users), should use [Static API Keys](#static-api-keys).

To authenticate using OAuth, you must include a valid OAuth token as the `Authorization` header in your request. You can get one by running `evergreen client get-oauth-token`

OAuth tokens can only be used when authenticating for evergreen.corp.mongodb.com, they cannot be used with evergreen.mongodb.com.

### Examples

> Note: Please make sure to use `https://evergreen.corp.mongodb.com`.

#### Rest API v1

```bash
    curl -H "Authorization: Bearer $(evergreen client get-oauth-token)" https://evergreen.corp.mongodb.com/rest/v1/projects/my_private_project
```

#### Rest API v2

```bash
    curl -H "Authorization: Bearer $(evergreen client get-oauth-token)" https://evergreen.corp.mongodb.com/rest/v2/projects/my_private_project
```

## Static API Keys

_Note_: This will soon be deprecated for human users (everyone except [service users](../Project-Configuration/Project-and-Distro-Settings#service-users)), who should use [OAuth](#oauth).

Use the `user` and `api_key` fields from the Settings page or your Evergreen configuration file (typically located at ~/.evergreen.yml).
Authenticated REST access requires setting two headers, `Api-User` and `Api-Key`.

Static api keys can only be used when authenticating for evergreen.mongodb.com, it cannot be used with evergreen.corp.mongodb.com.

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
