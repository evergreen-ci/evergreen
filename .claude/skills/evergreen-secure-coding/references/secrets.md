# Secrets, Expansions, and Redaction

## Rules

### 1. Admin expansions never touch user-influenced strings

`evergreen.Settings.Expansions` holds service-level secrets. Expanding `${...}` in any string a user, task, patch, or project can influence lets the caller read those secrets. They can then read the result on a host they control, in a log, or in an API response.

- Expand user-influenced content only with an explicit **allowlist** map of non-secret keys built for that purpose.
- Build the map from scratch with only the keys you need. Denylists go stale the moment someone adds a new secret.
- Expand each piece of content once, with the map appropriate to its source. Never re-expand combined output.
- Project variables follow the same rule across trust boundaries. Admin-only and private project variables must not reach patches or tasks that aren't entitled to them. That includes fork PRs, untrusted patches, and modules from other projects.

```go
// BAD
exp := util.NewExpansions(serviceSecrets)
out, _ := exp.ExpandString(userProvided)

// GOOD
exp := util.NewExpansions(map[string]string{
    "name": obj.Name,
})
out, _ := exp.ExpandString(userProvided)
```

### 2. Secrets are write-only in APIs

- Any API, GraphQL, or event response containing project vars, settings, webhook configs, third-party integration credentials, or subscriber secrets must be redacted. Use the model and API redaction helpers, and confirm they cover both private and admin-only values.
- New GraphQL input fields that carry secrets get `@redactSecrets`, so request logging doesn't record them.
- When adding a secret field to an existing struct, find every serializer for that struct: REST model, GraphQL model, events, and audit logs.
- When reading merged project variables for a task, use the accessor that applies the task's privilege. Don't read the raw map.

### 3. Secrets stay out of logs and telemetry

- Don't log request bodies, AWS or HTTP requests, user data, or GraphQL variables that may contain credentials. Log selected fields instead.
- Header redaction must be case-insensitive (`http.CanonicalHeaderKey` or `strings.EqualFold`).

### 4. Fail closed when keys are missing

- If a signing or verification key (HMAC, CSRF, webhook secret, artifact signing secret) is empty, return an error. Never sign or verify with an empty key, and never skip the control. Validate the key in the config section's `ValidateAndDefault`.
- Compare secrets with `subtle.ConstantTimeCompare` or `hmac.Equal`.

## Tests

- Expanding `${<admin secret key>}` in user content yields an empty or literal string.
- API responses for users without edit permission contain no secret values.
- An empty key fails closed.
