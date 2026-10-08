# Authorization

Every request path that reads or changes an object needs its own authorization check. A check on a related path does not protect the one you are adding or changing.

## Surfaces and where checks live

| Surface | Where it is registered | How authorization is applied |
|---|---|---|
| REST v2 | `rest/route/service.go` `AttachHandler` | Middleware in `.Wrap(...)`: `viewTasks`, `editTasks`, `submitPatches`, `viewProjectSettings`, `editProjectSettings`, `view/editRepoSettings`, `view/edit/removeDistroSettings`, `requireProjectAdmin`, `adminSettings`, `requireTask`, `requireHost`, ... |
| Legacy API / UI | `service/api.go`, `service/ui.go` `GetServiceApp` | Same `route.RequiresProjectPermission` values plus `service/` wrappers (`requireUser`, `requireProject`, `ownsHost`, ...) |
| GraphQL | `graphql/schema/**/*.graphql` and resolvers | Directives (`@requireProjectAccess`, `@requireProjectSettingsAccess`, `@requireRepoAccess`, `@requireDistroAccess`, `@requireHostAccess`, `@requireVolumeAccess`, `@requirePatchOwner`, `@requireAdmin`) plus helpers in `graphql/util.go` (`checkProjectAccess`, `userHasProjectSettingsPermission`, `userCanModifyPatch`, `userHasHostPermission`, `userHasVolumePermission`) |

`requireUser`, `rateLimit`, `addProject`, CORS, and compression middleware do **not** authorize anything.

## Rules

### 1. Every route that names a resource needs an authorization check

If the path has `{task_id}`, `{project_id}`, `{patch_id}`, `{volume_id}`, or similar, the `Wrap(...)` list needs the matching permission middleware. If the handler deliberately checks permission itself (for example, an ownership check on a user-owned object), say so in a comment next to the route and add a test for the denial case.

List endpoints count too. A list route that returns objects the caller could not fetch one at a time is a data leak. Filter results per item with the same check the single-object route uses.

### 2. Authorize the object you act on, derived from where you read it

The permission middleware resolves a project from the request. The handler must act on the same object the middleware authorized.

- Never let a query-string or body value override the path-bound ID used for authorization.
- Never branch authorization on the GraphQL operation name (`graphql.GetOperationContext(ctx).OperationName`). The client chooses it.
- If a GraphQL argument holds an ID with a different name than the directive expects, make sure the directive actually resolves it.

```go
// BAD: caller can point authorization at a project they can access.
id := gimlet.GetVars(r)["build_id"]
if q := r.URL.Query().Get("build_id"); q != "" {
    id = q
}

// GOOD: the path variable is the only source.
id := gimlet.GetVars(r)["build_id"]
```

### 3. User-owned objects need an owner check, not a project check

Project-level permissions such as patch submit or task edit say nothing about who owns a specific patch, spawn host, volume, or personal subscription.

| Object | Owner check |
|---|---|
| Patch | `model.UserCanModifyPatch(ctx, user, patch)` (REST/legacy), `@requirePatchOwner` or `userCanModifyPatch` (GraphQL) |
| Spawn host | `host.CanUpdateSpawnHost`, `data.FindHostByIdWithOwner`, `@requireHostAccess`, `userHasHostPermission` |
| Volume | compare `volume.CreatedBy` to the user, `@requireVolumeAccess`, `userHasVolumePermission` |
| Subscription | compare the complete owner identity before reading or overwriting |

Look up by ID **and** owner, or look up then compare before any write. Check ownership before reusing or overwriting an existing record found by caller-supplied ID.

A permission check that only logs is not a check:

```go
// BAD
if !canView(ctx, u, item) {
    grip.Warning(...)
}
result = append(result, item)

// GOOD
if !canView(ctx, u, item) {
    continue
}
result = append(result, item)
```

### 4. Lists and pairs: authorize every element and both ends

- A mutation that takes `ids: [String!]!` must check every ID, against the project that ID actually belongs to. A directive that checks one parent object does not cover child IDs that might belong elsewhere. Load each child and confirm it belongs to the authorized parent, or check its own project.
- Copy, move, attach, promote, link, rebind, and "create with reference to existing X" operations need a check on the **destination or referenced object**, not just the source. Examples: the destination of a copy or move, an existing object referenced by ID in a create request, or a new parent an object is attached to.
- Copying a privileged object must not grant the caller more than they had on the source.

### 5. GraphQL nested fields are separate entry points

A field resolver that loads and returns another object is reachable by anyone who can reach the parent. Check access to the **returned** object in the field resolver, or return a reduced type that holds only safe fields.

```go
// Illustrative field resolver that returns a task related to its parent object.
func (r *widgetResolver) RelatedTask(ctx context.Context, obj *restModel.APIWidget) (*restModel.APITask, error) {
    t, err := task.FindOneId(ctx, utility.FromStringPtr(obj.RelatedTaskID))
    if err != nil || t == nil {
        return nil, err
    }
    // The parent was authorized; the task may belong to a project the caller cannot see.
    if err := checkProjectAccess(ctx, t.Project, ProjectPermissionTasks, AccessLevelView); err != nil {
        return nil, err
    }
    return getAPITaskFromTask(ctx, r.sc.GetURL(), *t)
}
```

Returning a full type through a nested field exposes every field of that type, including admin-only ones. Generate signed URLs only after the access check on the object they grant access to.

### 6. Responses must not carry more than the caller may see

Project refs, settings, subscriptions, and webhook configs can contain secrets. Run the model's redaction (covering both private and admin-only values) on every response path, and test that it does. A secret-bearing field should be write-only unless the caller holds the settings-edit permission. Restoring redacted placeholders on save must use the stored value for the same object, never a value from another object.

## Tests

For every new route, resolver, or field, add tests that:

- deny a user with no permission on the target project
- deny a user who has permission on a different project (cross-project ID)
- deny a non-owner for user-owned objects
- for list arguments, deny when one ID belongs to a forbidden project
