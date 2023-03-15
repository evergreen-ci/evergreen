# Evergreen GraphQL API

Welcome to the beta version of the Evergreen GraphQL API! This API provides both
read and write access to various pieces of data found in Evergreen. You can use
the GraphQL Playground to experiment with the API, which can be found at
https://evergreen.mongodb.com/graphql.

You can read more about GraphQL [here](https://graphql.org/learn/).

## Getting Started

To get started with the API, visit the GraphQL Playground at
https://evergreen.mongodb.com/graphql. Here, you can explore the available data,
execute queries, and see the results.

To use the API in your own code, you can use the endpoint
https://evergreen.mongodb.com/graphql/query. You can also leverage code
generation to generate typings in your own language of choice.

## Authentication

To access any GraphQL endpoint, you must pass the mci-token cookie with your
request. This cookie is used to authenticate your session with the Evergreen
API. Without it, you will not be able to access any data.

## Directives

Evergreen's GraphQL API includes a few directives that manage permissions for
certain pieces of data. Directives in GraphQL are used to add additional
functionality to fields and types.

For example, a common directive used in Evergreen GraphQL API is
`@requireProjectAccess`. This directive checks whether the user has access to a
given project. So you should ensure that the caller has permissions to view a
project before calling a query that has this directive. You can find the other
directives
[here](https://github.com/evergreen-ci/evergreen/blob/d96942bcf0c26b158b8b1313bd27786f7a7c31a7/graphql/schema/directives.graphql)

## Documentation

The Evergreen GraphQL API is self-documenting, meaning that you can explore the
available data and fields using the GraphQL Playground. Additionally, we are
working on more comprehensive documentation to help you get started and make the
most of the API.

### Examples

Below is an example of a GraphQL query to fetch the test result log URLs for the
5 most recent mainline commits. The query filters on successful e2e_test tasks.
Normally, this would require several API calls, as well as some app-side logic
to fetch this data. With this query, however, all of the necessary information
can be retrieved in just one request.

```graphql
{
  mainlineCommits(
    options: {projectIdentifier: "spruce", limit: 5}
    buildVariantOptions: {tasks: "e2e_test", statuses: "success"}
  ) {
    versions {
      version {
        id
        buildVariants(options: {tasks: "e2e_test"}) {
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

Traditionally, fetching the base task and task for a given task would have
required at least 2 fetch requests using the REST API. With GraphQL, this can
now be achieved in just one request, simplifying the process and improving
efficiency.

```graphql
{
  task(taskId:"<task_id>", execution:0){
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

One of the key benefits of using GraphQL is that it provides strong type safety.
GraphQL uses a type system to define the shape of the data available in the API.
This means that you can know exactly what data you can expect to receive from
the API, and you can catch errors related to missing or incorrect data at
compile time.

To make use of this type safety, you can generate client-side code in your own
programming language. This can be done using a variety of code generation tools
available in the GraphQL ecosystem, such as
[graphql-codegen](https://the-guild.dev/graphql/codegen/docs/getting-started).

Code generation tools can generate strongly typed APIs based on our GraphQL
schema. This means that you can write code that directly interacts with the API,
without having to manually parse the response data or worry about type
mismatches.

## Limitations and future improvements.

The Evergreen GraphQL API is currently in beta and is not intended for public
use. As such, we cannot guarantee that fields will not be changed in future
releases. However, when we do deprecate a field, we typically mark it with the
@deprecated directive. You can configure your GraphQL client to issue warnings
when using these fields. Additionally, GraphQL's type safety ensures that you
will be notified if any fields change. We recommend that you stay up to date
with our API changes to ensure that your applications remain compatible with
Evergreen.

### SLA's and SLO's

We cannot guarantee the performance of our GraphQL API since queries are
determined by the client, and the performance may vary from request to request.
However, we typically aim to have a response time of less than 2 seconds for
most requests. More complex or resource-intensive queries may take longer to
complete. While we currently monitor the overall performance of queries, we plan
to introduce field-level monitoring in the future. This will allow us to
identify which parts of queries are slower and make targeted performance
improvements. We recommend that you optimize your queries to minimize their
impact on performance and consider caching where appropriate to improve
performance.

Currently, almost all queries have a tracing parameter attached, which enables
the exposure of field-level performance data for each requested resolver. This
uses the
[apollo-tracing format](https://github.com/apollographql/apollo-tracing). To
better design your queries, you can utilize a Chrome extension tool called
[Apollo Tracing](https://chrome.google.com/webstore/detail/apollo-tracing/cekcgnaofolhdeamjcghebalfmjodgeh?hl=en-US)
which provides a visual representation of field level performance
characteristics.

## Feedback and Support

We welcome your feedback and support as we continue to develop and improve the
Evergreen GraphQL API. If you encounter any issues or have any suggestions,
please let us know by reaching out to the evergreen team.
