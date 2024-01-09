# Stepback Bisection
Evergreen's stepback bisection performs stepback by continuously reducing the amount of commits needed to test by half. To enable it, go to the project settings 'general' tab.

## Motivation
Traditionally, Evergreen performed linear stepback only. This is tasking each previous commit one by one. If stepback bisection is enabled, Evergreen will use bisection instead of linear.
Evergreen performs linear stepback by default, which takes O(n) tasks at worst to find the offending commit. With bisection, Evergreen performs binary search on the remaining tasks, cutting down the worst time to O(logn).

## Definitions
'Last Passing Stepback Task' is the last known task that passes.
'Last Failing Stepback Task' is the task of the recent commit that causes stepback, the failing mainline commit.
'Next Stepback Task' and 'Previous Stepback Task' helps navigation on Spruce to track the path of stepback.

## Strategy
Bisection takes the failed commit that triggers it and the last known passing commit for that task. It then gets the version in the middle and activates the same task. If it passes, Evergreen knows that the offending commit is between the same failed commit and this new recent passing commit. If it fails, Evergreen knows the offending commit is between the same last known passing commit and this new recent failing commit. Evergreen stops when the next task to activate is the same as the last passing task.

## Navigation
Tasks involved in stepback bisection will have 'Last Failing Stepback Task', 'Last Passing Stepback Task', 'Previous Stepback Task', and 'Next Stepback Task' when applicable on their task metadata.

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