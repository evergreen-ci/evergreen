# 2026-07-02 Rate Limit Configuration

- status: proposed
- date: 2026-07-02
- authors: Shreeya Chand

## Context and Problem Statement

Evergreen exposes both REST (`/rest/v2/...`) and GraphQL (`/graphql/query`) APIs. Both surfaces have seen rapidly-growing request volume from users running personal AI agents (Claude, Cursor, etc.) that issue API calls on the user's behalf. 

To prevent AI agents from overloading Evergreen and its resources, we are implementing server-side rate limiting. Each API surface and user type (normal vs. service) combination gets its own configured burst and per-hour limit in order to accomodate for different use cases and usage patterns.

## Considered Options

A range of limits were considered. 

## Decision Outcome

These limits balance allowing existing user traffic to proceed normally while throttling 99th percenticle users and preventing agents from abusing the system as agentic usage of Evergreen increases. 

### REST
|  | User (Human) | Service User |
| :---- | :---- | :---- |
| **Burst** | 200 | 3,000 |
| **Hourly** | 2,000 | 60,000 |
| Rationale | Covers legitimate user use (typically \< 300 req/hr), throttles users running a script that constantly hits routes. Could be even more conservative with this, only a few outliers who reach the 1k+/hr rate. Users hitting this limit may have some use case that warrants a service user. | Covers the big services that average thousands of reqs per hour (mongodb-mongo-ci-user, build-baron-tools-user, auto-revert-tools-user, etc)  Some of these services would definitely exceed this when they peak, may be added to elevated users.|

### GraphQL
|  | User (Human) | Service User |
| :---- | :---- | :---- |
| **Burst** | 500 | 100 |
| **Hourly** | 2,000 | 1,000 |
| Rationale | The heaviest sustained user usually makes around \~500 req/hr, so regular users should be comfortably under this limit. There are some outliers who make 2k+ requests in an hour occasionally and should probably be throttled. There’s a small percentage of requests that come from Python so the rate limit makes sure that scripts/agents don’t abuse the endpoints. The majority of traffic comes from the UI, and those navigation patterns fall well below this limit.  Burst limit is slightly higher than the REST counterpart because UI navigation sends multiple requests per load \+ requests to poll.  | GraphQL endpoints are almost never hit by service users (Sage/MCP being the exception), so this should be sufficient, and help alert if the service users start behaving strangely by spamming GQL endpoints. As MCP usage of GQL increases, this may need to be adjusted. For now, it’s quite liberal for service user usage patterns.

### GraphQL Complexity
|  | Any user |
| :---- | :---- |
| **Cap** | 1000 |
| Rationale | Blocks rare high complexity queries which have high latency. All regular Sage queries are < 500 complexity.|