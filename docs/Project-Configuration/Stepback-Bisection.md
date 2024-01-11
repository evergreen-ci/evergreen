# Stepback Bisection
Evergreen's stepback bisection performs stepback by continuously reducing the amount of commits needed to test by half. To enable it, go to the project settings 'general' tab and enable the 'Stepback Bisection' flag. Note that the 'Stepback' flag has to be enabled as well for bisection to take place.

## Motivation
Traditionally, Evergreen performed linear stepback only. This is tasking each previous commit one by one. If stepback bisection is enabled, Evergreen will use bisection instead of linear.
Evergreen performs linear stepback by default, which takes O(n) steps in the worst case to find the offending commit. With bisection, Evergreen performs binary search on the remaining tasks, cutting down the worst time to O(logn) steps.

## Definitions
The following are all task-specific fields that populate separately for every task. Only tasks that are in the middle or have finished the bisection process will have these fields.
'Last Passing Stepback Task' is the last task that is known to pass (related to a mainline commit).
'Last Failing Stepback Task' is the failing mainline commit.
'Next Stepback Task' is the task that comes next in stepback (i.e. the current task is either a last passing/failing and bisection has picked the next task).
'Previous Stepback Task' is the task that happened before the current stepback task (i.e. the previous last passing/failing that caused this stepback task to activate)

All of these fields are populated as Evergreen does stepback bisection.

## Strategy
Bisection starts with the failed commit that triggers it and the last known passing commit for that task. It then gets the version in the middle and activates the same task. This can be broken down to the following cases.

- If the task passes, Evergreen knows the offending commit is between the same failing commit and the task that just passed.
    - Evergreen activates the task (current) between the same failing commit and the task that just passed.
- If the task fails, Evergreen knows the offending commit is between the same passing commit and the task that just failed.
    - Evergreen activates the task (current) between the same passing commit and the task that just failed.
- Then in both cases:
    - The 'current' task's previous stepback task is set to the task that activated this step in stepback.
    - The task that activated this step in Stepback is updated to point their 'Next Stepback Task' to the 'current' task.
    - Once this task fails or passes, repeats the case that applies (with 'Previous' being the 'current' task).

## Navigation
Tasks involved in stepback will have corresponding data in their task metadata when selecting the task. To access it, click on the version (or mainline commit) that failed and activated stepback, then go to the task(s) that failed and view the task metadata.

## Example
Below is an example where there is a last known passing commit 'Passing commit'. 10 inactive commits labeled 1-10. A failing mainline commit 'Latest commit'. The commit '3' is the offending commit bisection stepback is finding.

![stepback-bisection-1.png](../images/stepback-bisection-1.png)

Once the latest commit finishes and reports a failing status, Evergreen will activate the task associated with the commit between our 'Latest commit' and 'Passing commit'.

![stepback-bisection-2.png](../images/stepback-bisection-2.png)

As the middle commit task (aka commit labeled '5' fails), Evergreen will activate the task between '5' and 'Passing Commit'.

![stepback-bisection-3.png](../images/stepback-bisection-3.png)

As commit '2' passes, Evergreen will activate the task between '5' and '2'.

![stepback-bisection-4.png](../images/stepback-bisection-4.png)

Finally, commit 3 fails, ending stepback and declaring commit labeled '3' as the offending commit.

![stepback-bisection-5.png](../images/stepback-bisection-5.png)