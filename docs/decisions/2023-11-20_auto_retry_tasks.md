# 2023-11-20 Auto Retry Tasks

- status: accepted
- date: 2023-11-20
- authors: Malik Hadjri

## Context and Problem Statement

Ticket: DEVPROD-998

It was requested to add a feature to Evergreen to allow users to automatically retry tasks that fail due to a transient error. This
would allow users to avoid having to manually retry these tasks, which can happen at a a semi-common
rate depending on the task environment.

## Considered Options

- Create a new REST endpoint (or repurpose an existing one) that allows for tasks to mark themselves to restart upon failure,
  with the expectation that interested teams would request service users with the needed permissions to hit this endpoint, at which
  point a task could arbitrarily decide when to mark itself for restart via shell scripts

- Use the agent's existing local API, by adding a retryable parameter to its custom end task response route that users already
  leverage to let tasks define their own task end status. This new retryable parameter would be forwarded over to the Evergreen
  app server when setting the custom end task response and would trigger an automatic restart of the task in the end task logic
  on the app server side

- Introduce a new agent command named something like `auto.retry` which will make the agent hit an app server endpoint
  that will mark the task for restart upon failure

- Add a new boolean parameter on the generic command struct, such that if that command fails and the flag is set, we send a
  request to the app server to mark the task for restart upon completion

## Decision Outcome

Option 1 was rejected because it would require interested teams to request service users with the needed permissions to hit the
endpoint, which would both introduce friction for interested teams and also poses a potential security risk if the service user
were to be misused or compromised.

Option 2 was rejected because it would essentially be using the agent's local API as a way to forward requests to the app server's API
and bypass its REST authentication rules, which is not the intended use of the agent's local API. We therefore decided to keep the agent's local API
to its currently limited scope.

Options 3 and 4 are similar in that they both involve adding a new app server endpoint for the agent to ping to mark the task for
restart upon completion. However, option 4 was ultimately chosen because it is more flexible and allows for more granular control over
when the task is marked for restart, as it allows us to identify specific commands that are suspected to contain transient errors, and should
therefore be marked to trigger a restart upon failure.
