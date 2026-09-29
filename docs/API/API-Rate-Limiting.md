# Rate Limiting

Evergreen applies per-user API rate limits to protect the service from abusive, high-volume request patterns.

REST and GraphQL requests are rate-limited independently, meaning that making REST requests does not affect how many GraphQL requests a user can make, and vice versa.

## User Tiers

Evergreen uses different limits for different types of users:

- Human users use the standard per-user limits.
- Service (API-only) users use separate limits per service user.

### Elevated Users

A small number of workflows may legitimately need more headroom than the default limits allow. Users may request to be added to the "elevated" users list, which grants double the request volume for their user tier. This list is kept deliberately small, so requests should demonstrate a genuine need for higher limits.

To request elevated user status, open a DEVPROD Jira ticket describing which user should be elevated, the API surface (REST or GraphQL), the workflow being throttled, and why the baseline limits are insufficient.

## Burst vs. Per-Hour Limits

For each API surface and user type, there are two limits: one burst limit, and one per-hour limit. Burst indicates the number of requests that can be made without throttling. Once the burst limit starts to deplete, the user accumulates a new request "token" at the hourly rate.

Note that tokens are refilled continuously, not reset on a fixed schedule (e.g. at the top of the hour).

> **Example:** if the burst limit is 20 and the per-hour limit is 600 (for a particular API surface and user type), a user's burst limit will "refill" at a rate of 600 requests/hour: the user is allowed one new request every 6 seconds until the bucket reaches 20 again.

## GraphQL Query Complexity

GraphQL requests are additionally subject to a ["complexity"](https://gqlgen.com/reference/complexity) limit, which prevents the execution of queries that could create stressful workloads for the system. Complexity is computed by traversing the query AST and summing a cost of 1 per field, across all levels of nesting. Each task whose test results (`Task.tests`) are fetched adds an extra cost, counted once per task in `taskHistory` and `Version.tasks` based on their `limit`, or 100 tasks if `limit` is unset or 0. This is a stateless, per-query ceiling rather than a limit bucket that the user exhausts over time.

### Example

```graphql
query TaskHistoryExample {
  taskHistory(options: { limit: 10 }) {
    tasks {
      id
      tests {
        totalTestCount
      }
    }
  }
}
```

The complexity of this query is 1 per field (`taskHistory`, `tasks`, `id`, `tests`, `totalTestCount`) plus the test results cost for each of the 10 tasks. If the configured complexity limit is lower than that total, this query will return an error, independent of any other requests and the GraphQL rate-limiting state. Requesting 5 tasks instead would halve the test results cost.

## API Response

If a request is blocked due to rate limiting, it will be rejected with a 429 HTTP response. All requests that are subject to rate-limiting, regardless of whether they are blocked, carry headers providing information on the current limit state. If rate-limiting is disabled across the service, these headers serve only as a warning.

### Response Headers

- `X-RateLimit-Limit`: The hourly request limit for the current user.
- `X-RateLimit-Burst`: The maximum number of requests that can be made without being throttled.
- `X-RateLimit-Remaining`: The current number of burst requests remaining.
- `X-RateLimit-Reset`: The absolute Unix timestamp, in seconds, when the user's burst limit is completely refilled.
- `X-RateLimit-Exceeded`: Indicates that the limit has been exceeded. If rate-limiting is disabled, the request may still succeed even when this header is present.
- `Retry-After`: Present only alongside a 429 response. This is the number of seconds until a single request can succeed (not when the bucket will be full).

## CLI Behavior

Some Evergreen CLI commands may fail because the underlying REST API calls are rate limited. When the user hits their rate limit via the CLI, it will print the refill rate and time at which the next request can be fulfilled.

Users can monitor their own REST rate limit status using `GET /rest/v2/users/{user_id}/rate_limit`.
