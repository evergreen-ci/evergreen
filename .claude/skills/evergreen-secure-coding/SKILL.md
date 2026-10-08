---
name: evergreen-secure-coding
description: Use when writing, changing, or reviewing Evergreen Go code that adds or modifies REST or legacy routes, GraphQL fields or resolvers, agent or host endpoints, permission checks, spawn hosts or volumes, patches, project or repo settings and variables, shell or git commands, host setup scripts, outbound HTTP or webhooks, file paths, notifications, or secrets; also when fixing a security bug or reviewing a pull request for security.
---

# Evergreen Secure Coding

## Overview

Evergreen's security bugs cluster into a small set of repeat mistakes: an operation guarded on one surface but not its twins, authorization checked against a different object than the one acted on, caller-controlled data reaching a shell, URL, path, or template, and secrets expanded or returned where the caller can read them. This skill turns those mistakes into invariants and gives the Evergreen helper to use for each.

**Core principle:** authorize the exact object you act on, on every surface that can reach it, before you act.

## The Sibling Rule (most-missed step)

Most Evergreen operations are reachable through several surfaces: REST v2 (`rest/route`), the legacy API and UI server (`service/`), GraphQL (`graphql/`), agent routes, and CLI-backed endpoints. A fix or check added to one surface does nothing for the others.

Whenever you add, change, or fix a check for an operation:

1. Name the underlying model mutation or read (for example "update a field on a user-owned object" or "copy data between two objects").
2. Find every caller of it from a request path: `rg` for the model function plus its REST, `service/`, and GraphQL wrappers.
3. Give each caller the same check, in the same PR. List the surfaces you checked in the PR description.

"The ticket only names one route" and "the others can be a follow-up" are not acceptable reasons to stop. An incomplete fix is a new bug report.

## Quick Reference

| You are... | Invariant | Use | Details |
|---|---|---|---|
| Registering a route with an ID in its path | Authorization middleware in `Wrap(...)`, not just `requireUser` | `viewTasks`/`editTasks`/... from `RequiresProjectPermission` etc. | authorization.md |
| Resolving which project/resource to authorize | Derive it only from the object the handler acts on | path vars; never query string, op name, or rewritten vars | authorization.md |
| Mutating a user-owned object (patch, host, volume, subscription) | Owner check on that object | `model.UserCanModifyPatch`, `host.CanUpdateSpawnHost`, `data.FindHostByIdWithOwner`, `@requirePatchOwner`, `@requireVolumeAccess` | authorization.md |
| Taking a list of IDs, or a source and destination | Authorize every ID, and both ends | per-item permission check | authorization.md |
| Adding a GraphQL field that returns another object | Field resolver checks access to the returned object | `checkProjectAccess`, redacted API models | authorization.md |
| Writing an agent or host route | Identity comes from auth middleware, never body or headers | `MustHaveTask(ctx)`, `MustHaveHost(ctx)` | agent-and-credentials.md |
| Minting a token or credential for a task | Target derived from the task's project, not the request | project ref fields | agent-and-credentials.md |
| Building a shell, ssh, or git command | Quote or use argv; `--` before positional args | `util.ShellQuote`, `util.PowerShellQuotedString` | commands.md |
| Expanding `${...}` or returning config | Admin expansions never touch user-influenced strings; secrets never returned | explicit allowlist maps, the model and API redaction helpers, `@redactSecrets` | secrets.md |
| Fetching a URL that a user can influence | Guarded client: dial-time IP check, redirect re-check, no proxy | `util.ValidateWebhookURL`, the webhook sender's transport | outbound-requests.md |
| Joining a path from task, patch, or archive data | Containment check; don't follow symlinks | `filepath.IsLocal`, `filepath.Rel`, `os.Lstat` | files.md |
| Rendering email, HTML, links, or served files | Escape; allowlist URL schemes and content types | `html/template`, `util.CheckURL` | web-output.md |
| Reading request bodies, webhooks, keys, locks | Bound sizes; verify signatures and sources; fail closed | `http.MaxBytesReader`, `subtle.ConstantTimeCompare` | robustness.md |
| Gating CI for PRs from forks | Check permissions on the base repo and the PR author | base owner/repo | fork-prs.md |

Reference files are in `references/`. Read the ones matching your change before writing code.

## Reviewing a PR

1. For every new or changed route, resolver, or field, write down the object it acts on and the check that authorizes that exact object. No check means a blocking finding.
2. Apply the Sibling Rule to every check the PR adds or changes.
3. For each list argument, confirm every element is authorized. For each copy, move, attach, or link, confirm both source and destination are authorized.
4. Trace every request-derived value that reaches a shell, URL fetch, file path, template, `${...}` expansion, or DB filter.
5. Confirm tests cover the denial case (non-owner, wrong project, foreign ID), not only the happy path.
6. If Semgrep comments on the PR, fix the code, or explain in the PR why the finding does not apply.

## Red Flags (stop and fix)

- `Wrap(requireUser, rateLimit)` on a route whose path contains an `{..._id}`
- `r.URL.Query()` or `OperationName` feeding a permission decision
- A permission check that logs and continues
- `fmt.Sprintf` with `'%s'` or `"%s"` going into a script
- `settings.Expansions` applied to anything not constant
- `http.Get`, `http.DefaultClient`, or `utility.GetHTTPClient()` used with a stored URL
- A body field named `TaskID` or `HostID` used to look something up in an agent route
- "Only this route is in scope for the ticket"
