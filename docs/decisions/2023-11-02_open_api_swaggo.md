# 2023-11-02 Use swaggo to generate REST v2 documentation

* status: accepted
* date: 2023-11-02
* authors: Brian Samek

## Context and Problem Statement

There are three different definitions of Evergreen's REST v2 API: Evergreen's
code, Evergreen's documentation, and the evergreen.py client. Since these are
developed independently, they drift, and the documentation and client are
incomplete. Furthermore, there are 95 endpoints in Evergreen's API docs, so
producing correct documentation or a correct spec is a significant amount of
work.

## Considered Options

### Auto-generate OpenAPI spec from code

[go-swagger](https://github.com/go-swagger/go-swagger) supports generating an
OpenAPI spec from code. It can generate an entire OpenAPI response object from a
`// swagger: response` comment on a struct, for example, and a `route` object
from inline YAML in a `// swagger: route` comment placed anywhere. You still
need to add comments to fields in order to get them to show up in the spec, but
much of the rest of the work is done by the tool.

Unfortunately, it only supports OpenAPI 2, and it is failing to document
embedded structs, even though the docs imply that this should work. I have an
[issue](https://github.com/go-swagger/go-swagger/issues/2980) open for this.
Development of this tool seems to have slowed. Therefore although it is cheaper
than the spec-from-docs solution in the short-term (that is, if the open issue
is solved), it's possible that in the long-term this approach is not
sustainable, since the tooling may grow stale and not receive updates.

Cloud uses a similar approach to generate an OpenAPI spec for their public API
from code comments using
[swagger-core](https://github.com/swagger-api/swagger-core). This Java tool is
under more active development than the equivalent Golang tool. 

Although I have been unable to generate docs for embedded structs with
go-swagger, another approach would be to generate the partial OpenAPI spec and
then fill in the rest by hand. This approach, however, would require continuous
manual intervention.

### Auto-generate OpenAPI spec from comments

In this approach, instead of relying on a tool that can understand Go code as
fully as go-swagger, we document the API entirely in comments near the code, and
then use a tool like [swaggo](https://github.com/swaggo/swag) (also OpenAPI 2)
to generate an OpenAPI spec from the comments. Note that swaggo does understand
structs as return types and parameters, and can parse field comments.

The downside is that the comments are tied to the code only by the developers
editing them. They can drift from the API code, though they will not drift from
documentation or clients, since both documentation and clients will be generated
by the spec, which in turn is generated from the comments. This suggests that in
this approach we should also write contract tests to ensure that the
implementation has not drifted from the comments.

I believe this is a reasonable compromise if the spec cannot be generated from
code.  

### Write the OpenAPI spec by hand 

This gives us the flexibility to write down exactly how we want the API to
behave. It is also the recommended approach for using OpenAPI: Design your API
first so that it is clear and correct, and then generate everything else. 

However, this would require rewriting the API by generating stubs from the spec,
and is therefore a much larger amount of work.

### Generate the spec from browser behavior

There are ways of bootstrapping the work of writing the spec to make it faster.
For example, there are ways of using Chrome's Developer tools to generate a
spec, like openapi-devtools. In the limit it is conceivable, but is also kind of
a hack, to build a tool that does this programmatically (run a web crawler in CI
that generates a HAR file, and then generates a spec from the HAR file). This
violates the spirit of OpenAPI as a spec, and instead uses it as a documentation
language.

### Do nothing

If auto-generating from code is not possible, I think there is a difficult cost
question to analyze: Is the large upfront cost of the spec-from-comments
approach more or less expensive than the ongoing cost to the Evergreen team and
to users of developing, maintaining, and using separate sources of truth for the
API?

## Decision Outcome

Given that swaggo will substantially increase the amount of validation done to
our API docs, and can enable future work like generating REST clients, it is the
fullest working solution, and therefore the one we have chosen.