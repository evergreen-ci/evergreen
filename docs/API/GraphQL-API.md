# GraphQL API

Welcome to the beta version of the Evergreen GraphQL API! This API provides read
and write access to various pieces of data found in Evergreen. You can use the
GraphQL Playground, which can be found at <https://evergreen.corp.mongodb.com/graphql>,
to experiment with and explore the available data.

⚠️ Please note that the Evergreen GraphQL API is currently in beta and is not
officially supported for production-level code.

If you intend to use this API
for production, we recommend reaching out to us for proper support and
authorization. However, for exploratory operations, you are welcome to use the
API without prior authorization.

## Authentication

To access any GraphQL endpoint, ensure you pass the mci-token cookie along with
your request to authenticate your session with the Evergreen API, providing read
and/or write access as required.

## Directives

Evergreen's GraphQL API includes directives that manage permissions for specific
pieces of data. These directives add additional functionality to fields and
types. A common directive used in Evergreen's GraphQL API is
`@requireProjectAccess`. Before calling a query that has this directive, ensure
the caller has permissions to view a project. You can find the other directives
[here](https://github.com/evergreen-ci/evergreen/blob/d96942bcf0c26b158b8b1313bd27786f7a7c31a7/graphql/schema/directives.graphql).

## Limitations and Future Improvements

The Evergreen GraphQL API is currently in beta and not intended for public use,
so we cannot guarantee field consistency between releases. However, when we
deprecate a field, we mark it with the @deprecated directive. You can configure
your GraphQL client to issue warnings when using these fields. Additionally,
GraphQL's type safety ensures that you will be notified if fields change. We
recommend staying up to date with our API changes to ensure application
compatibility with Evergreen.

## Documentation

The Evergreen GraphQL API is self-documenting, meaning that you can use the
GraphQL Playground to explore the available data and fields. More comprehensive
documentation is currently being created to help fully utilize the API.

## Examples

GraphQL queries offer efficient ways to retrieve information in single requests,
compared to its REST counterpart. Below are a couple of examples:

This query retrieves the latest 5 mainline commits and filters the tasks to only
include those with a successful `e2e_test` task. Additionally, it fetches the
log links for each test associated with these tasks. Previously, achieving this
functionality would require multiple synchronous API calls and application-level
logic utilizing the traditional REST API. However, with our GraphQL API, it can
be accomplished in a single declarative request.

```graphql
# Query with variables defined
query (
  $options: MainlineCommitsOptions!
  $buildVariantOptions: BuildVariantOptions!
) {
  # Pass in the options to the mainlineCommits query
  mainlineCommits(
    options: $options
    buildVariantOptions: $buildVariantOptions
  ) {
    versions {
      version {
        id
        # Pass in the options to the buildVariants query
        buildVariants(options: $buildVariantOptions) {
          tasks {
            displayName
            execution
            tests {
              testResults {
                testFile
                status
                logs {
                  urlParsley
                  urlRaw
                }
              }
              totalTestCount
            }
          }
        }
      }
    }
  }
}
```

```json
// Query Variables
{
  "options": {
    "projectIdentifier": "spruce",
    "limit": 5
  },
  "buildVariantOptions": {
    "tasks": ["e2e_test"],
    "statuses": ["success"]
  }
}
```

Take the below query which fetches both a task and its base task. Traditionally,
fetching the desired data would have required a minimum of two requests for a
given task using the REST API. With GraphQL, however, it can be achieved in just
one request. Additionally, we only return the data that is required by the
application requesting the data.

```graphql
{
  task(taskId: "<task_id>", execution: 0) {
    id
    status
    execution
    displayName
    baseTask {
      id
      execution
      status
      displayName
    }
  }
}
```

## Type Safety and Code Generation

One of the key benefits of using GraphQL is its strong type safety. GraphQL uses
a type system to define the shape of the data available in the API. This means
you can catch errors related to missing or incorrect data at compile time. To
utilize this type safety, you can generate client-side code in your preferred
programming language.

Code generation tools are available in the GraphQL ecosystem, including
[graphql-codegen](https://the-guild.dev/graphql/codegen/docs/getting-started),
which generate strongly typed APIs based on our GraphQL schema. This means you
can write code that directly interacts with the API without manually parsing the
response data or worrying about type mismatches.

## SLA's and SLO's

As queries are determined by the client, we cannot guarantee the performance of
our GraphQL API. We aim for a response time of less than 2 seconds for most
requests, complex or resource-intensive queries may take longer. Although we
currently monitor query performance, we plan to introduce field-level monitoring
for targeted performance improvements. Optimizing queries to minimize
performance impact and caching (if appropriate) is advisable.

Due to tracing parameters included in almost all queries, exposing field-level
performance data for each requested resolver using the
[apollo-tracing format](https://github.com/apollographql/apollo-tracing), you
can utilize a Chrome extension tool called
[Apollo Tracing](https://chrome.google.com/webstore/detail/apollo-tracing/cekcgnaofolhdeamjcghebalfmjodgeh?hl=en-US)
to better design queries, providing a helpful visual representation of
field-level performance characteristics.

## Feedback and Support

We welcome your feedback and support during Evergreen GraphQL API development.
If an issue arises or you have any suggestions or feedback, please contact the
Evergreen team.
