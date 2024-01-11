# Task Priority

Evergreen orders its task queues by taking into account a number of properties
of a task. These properties are given weights and are summed. Evergreen orders
the queues according to this sum. Weights are editable by Evergreen admins.
This process means that it's not possible for an Evergreen user or admin to
easily reason about the relative positions of specific tasks in the queue.

Properties of tasks that can influence their order include the following.
However, note that these properties can overlap in complex ways.

- Tasks that generate tasks are prioritized over tasks that do not.
- Patches are prioritized over mainline commits.
- Commit queue patches are prioritized over patches and mainline.
- Newer mainline builds are prioritized over older mainline builds.
- Stepped back tasks are prioritized over non-stepped back tasks.
- Tasks with more dependencies are prioritized over tasks with fewer dependencies.
- Tasks with longer runtimes are prioritized over tasks with shorter runtimes.
- Tasks with higher priorities set by users are prioritized over tasks with lower priorities.

It is possible for a user to change the priority of their task, which affects
one part of the sum. The default priority is 0. Valid priorities are 0-100 for
standard users, and higher for project admins. -1 will prevent a task from being
scheduled, e.g., by stepback.

Please be conservative when setting high priorities, as this will deprioritize
other users' tasks relative to yours. Please stay below 50 if the change is not
high priority.

Priority can be set in the UI on the version and task pages from the three dots
menu -> Set priority. 

![set priority](https://private-user-images.githubusercontent.com/624531/295986802-cd5ecf2d-c7f8-484e-943c-c8ad2cc3370c.png?jwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJnaXRodWIuY29tIiwiYXVkIjoicmF3LmdpdGh1YnVzZXJjb250ZW50LmNvbSIsImtleSI6ImtleTUiLCJleHAiOjE3MDQ5OTE3MDQsIm5iZiI6MTcwNDk5MTQwNCwicGF0aCI6Ii82MjQ1MzEvMjk1OTg2ODAyLWNkNWVjZjJkLWM3ZjgtNDg0ZS05NDNjLWM4YWQyY2MzMzcwYy5wbmc_WC1BbXotQWxnb3JpdGhtPUFXUzQtSE1BQy1TSEEyNTYmWC1BbXotQ3JlZGVudGlhbD1BS0lBVkNPRFlMU0E1M1BRSzRaQSUyRjIwMjQwMTExJTJGdXMtZWFzdC0xJTJGczMlMkZhd3M0X3JlcXVlc3QmWC1BbXotRGF0ZT0yMDI0MDExMVQxNjQzMjRaJlgtQW16LUV4cGlyZXM9MzAwJlgtQW16LVNpZ25hdHVyZT1lNTY5ZDkwYTgyZTQxMGQzZmVlNjRiYTA3MjU3MDg1ZWE4ZGRjOTAzMGVmOGMwMTlkZDI2YzhkMmI0OWQxZThlJlgtQW16LVNpZ25lZEhlYWRlcnM9aG9zdCZhY3Rvcl9pZD0wJmtleV9pZD0wJnJlcG9faWQ9MCJ9.t66CQdsEc-HHN2EsMrAFgrSbPrqYOlocPEklWHAI1FU)

It can also be set with the
[API](../API/REST-V2-Usage.md).