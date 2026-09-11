# 2026-08-31 Convert the generated REST v2 spec to OpenAPI 3

- status: accepted
- date: 2026-08-31
- authors: Annie Black

## Context and Problem Statement

Evergreen's REST v2 spec is generated from swaggo annotations, as decided in
[2023-11-02_open_api_swaggo.md](2023-11-02_open_api_swaggo.md). swaggo v1 only
emits Swagger 2.0, so the spec we publish is Swagger 2.0. Most modern tooling —
including the client generators users need in order to move off evergreen.py —
targets OpenAPI 3, so publishing 2.0 makes that migration harder. We want to be
on OpenAPI 3 ahead of the evergreen.py deprecation (DEVPROD-42403).

## Considered Options

### Upgrade to swaggo v2

swaggo v2 can emit OpenAPI 3.1 directly via `swag init --v3.1`, which would let
a single tool own the whole pipeline. However, its latest tag is still a release
candidate, and running it against Evergreen panics with a nil pointer
dereference while parsing our annotations. It is not usable today.

### Convert the generated spec as a post-processing step

Keep generating Swagger 2.0 with swaggo v1, then convert to OpenAPI 3 before
rendering and publishing. The conversion is mechanical and lossless for our
spec: it preserves all paths, schemas, tags, and security schemes, and the
result passes `redocly lint`.

## Decision Outcome

We convert as a post-processing step, using `getkin/kin-openapi`'s
`openapi2conv` in `cmd/swagger-to-openapi`. A Go converter keeps the API doc
build within the toolchain the build already uses, rather than adding a Node
dependency to it.

The published artifact keeps the name `swagger.json` so that existing consumers
of its URL keep working; only its contents changed.

If swaggo v2 stabilizes and can parse our annotations, we should revisit this
and drop the conversion step.

## More Information

The `@version` and `@host` annotations use placeholders that are substituted at
publish time by `scripts/prepare-swagger-push.sh`. These placeholders cannot be
wrapped in curly braces, because OpenAPI 3 would interpret a braced token in a
server URL as a server variable.
