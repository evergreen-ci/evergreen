# 2024-11-11 Enforce Project Permissions in GraphQL API

- status: accepted
- date: 2024-11-11
- authors: Minna K-T

## Context and Problem Statement

In February 2024, it was brought to the attention of the Evergreen team that all users had the ability to modify tasks and patches in restricted projects on Spruce ([DEVPROD-5221](https://jira.mongodb.org/browse/DEVPROD-5221)). Notably, this behavior differed from that of the legacy UI, which correctly enforced project permissions.

An investigation revealed that the GraphQL API only had partial support for project permissions, as outlined in the table below:

| Project Permission                      | Implemented |
| :-------------------------------------- | :---------- |
| View/Edit access to project settings    | ✅          |
| View/Edit access to project tasks       | ❌          |
| View/Edit access to project annotations | ❌          |
| View/Edit access to project patches     | ❌          |
| View access to project logs             | ❌          |

This meant that the Evergreen team needed to retroactively add the missing permission logic to the GraphQL API, which had already undergone significant development since its introduction.

## Decision Outcome

### Mirroring the REST API

To enforce project permissions, the REST API uses a middleware function called [`RequiresProjectPermission`](https://github.com/evergreen-ci/evergreen/blob/af18e60d63f99d5c0fbfe4679d86a96829bc7098/rest/route/middleware.go#L605-L615). This function has two main roles:

1. Determine the project ID from the given path parameters via [`urlVarsToProjectScopes`](https://github.com/evergreen-ci/evergreen/blob/af18e60d63f99d5c0fbfe4679d86a96829bc7098/rest/route/middleware.go#L639).
2. Check the user's permission for the project associated with that ID.

Since [directives](https://the-guild.dev/graphql/tools/docs/schema-directives) were already being used as a form of access control in the GraphQL schema, the team decided to implement a new directive that would mirror the functionality of `RequiresProjectPermission`. Directives are evaluated before the execution of a query or mutation and can be used as middleware to restrict access.

The [`@requireProjectAccess` directive](https://github.com/evergreen-ci/evergreen/blob/7fd7c2065599850a41b445de4b1ff75e624fa622/graphql/schema/directives.graphql#L6-L23) was introduced and applied on top of existing queries and mutations in [DEVPROD-5459](https://jira.mongodb.org/browse/DEVPROD-5459). Similar to `RequiresProjectPermission`, the directive looks up the project ID based on the provided input arguments, and then determines if the user has the appropriate permissions for that project and GraphQL operation.

### Standardization of GraphQL Arguments

The REST API's implementation relies on uniform path parameter names. For example, an operation on a task will always expect the path parameter `task_id`. This proved to be the first obstacle for implementing permission checks in the GraphQL API, as argument names were not standardized. Many queries simply used an argument of `id`, making it impossible to tell if the object being operated on was a host, a task, a patch, or otherwise.

Before any permission work could be completed, all queries and mutations had to be updated with more descriptive argument names (e.g. `taskId`, `versionId`, `distroId`) to identify the target object. This work was completed in [DEVPROD-5460](https://jira.mongodb.org/browse/DEVPROD-5460).

## More Information

For more information about completed and ongoing GraphQL permission work, please refer to [this document](https://docs.google.com/document/d/10xqHg94Ynqcd33-qSkK6XtelGcajGWteNBdG-gUGUHI/edit?usp=sharing).
