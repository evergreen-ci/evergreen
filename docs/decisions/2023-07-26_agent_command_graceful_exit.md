# 2023-07-26 Agent Task Execution and Command Graceful Exit

- status: accepted
- date: 2023-07-26
- authors: Kim Tao

## Context and Problem Statement

The agent was implemented run pre and main block commands in a separate goroutine and then waited until it either
completed normally or hit a context error (e.g. due to aborting the task). It seems to have been put in a separate
goroutine out of uncertainty about whether the pre and main blocks could hang forever, which would prevent the task from
making forward progress as it waits forever for them to finish.

## Decision Outcome

The agent has been fixed over time to ensure that all logic does respect the context (or has other timeout mechanisms),
so all logic to run a task does eventually return when the context errors. Therefore, pre and main no longer have the
original risk of hanging forever as they once did, meaning it is possible to run the pre and main blocks in the
foreground rather than as a background goroutine.
