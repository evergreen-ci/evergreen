# GitHub Integration

Specific GitHub pull request behavior can trigger behavior in Evergreen.

## Help Text

```
evergreen help
```

We have documentation here but we also provide it on the PR itself. It will display commands that are available for your project, with some context about when to use them. If the commit queue is disabled but there is an available message, we will still display the message. If PR testing isn't enabled for the branch but [PR testing is set up for untracked branches](../Project-Configuration/Repo-Level-Settings#how-to-use-pr-testing-for-untracked-branches) then we will also still show the related GitHub Pull request commands (detailed below).


## GitHub Pull Request Testing

Evergreen has an option to create patches for pull requests and this can be defined on the project page.

If "Automated Testing" is enabled, Evergreen will automatically create a patch for each pull request opened in the repository as well as each subsequent push to each pull request.

If you'd like the option of creating patches but wouldn't like it to happen automatically, you can enable "Manual Testing".

You can read more about these options [here](../Project-Configuration/Project-and-Distro-Settings.md#github-pull-request-testing).

#### Retry a patch

```
evergreen retry
```

Sometimes Evergreen has trouble creating a PR patch, due to internal server errors or GitHub flakiness. Commenting `evergreen retry` will attempt to recreate this patch. This can also be used to submit a new patch.

Note that this is specific to GitHub PR checks; it won't retry a commit queue patch. For that, re-type `evergreen merge` (detailed below).

#### Set PR patches to reuse a patch definition

```
evergreen keep-definitions
```

By default, `evergreen retry` will create a new patch using the default GitHub PR patch definition. If you comment `evergreen keep-definitions`, the latest patch you ran (including all tasks that were manually scheduled) will be used to select tasks to run in newer PR patch definitions. Any new patches in the PR will reuse that template patch's definition.

Note that if you schedule more tasks in a patch created after `evergreen keep-definitions` and wish to overwrite the existing patch definition to use the new one, you'll have to comment `evergreen keep-definitions` again.

#### Stop PR patches from reusing a patch definition

```
evergreen reset-definitions
```

If you used `evergreen keep-definitions`, then `evergreen reset-definitions` will reset your PR patches back to using the original GitHub PR patch definition.

#### Skip CI Testing

Sometimes you may want to avoid having Evergreen create patches (perhaps because the work is in progress, or testing isn't relevant yet). 
Simply including `[skip-ci]` or `[skip ci]` in the PR title or the first 100 characters of the description will prevent Evergreen from creating a patch (both from commits and `evergreen retry` comments) 
until the label is removed and a new commit or `evergreen retry` comment is pushed.


#### Create a patch for manual testing

```
evergreen patch
```

If your project is configured for manual testing, then Evergreen will only add GitHub checks to the PR when prompted, as opposed to for every commit. Commenting `evergreen patch` will trigger this.

#### Refresh GitHub checks

```
evergreen refresh
```

Sometimes Evergreen has trouble sending updated GitHub statuses, so the checks on the PR may not accurately reflect the state of patch on Evergreen. This is especially troublesome when the repository requires passing checks. To re-sync the GitHub checks with Evergreen, comment `evergreen refresh` on the PR.

## Commit Queue 

Evergreen's commit queue merges changes after the code has passed a set of tests. You can read more about this [here](Commit-Queue#commit-queue).

#### Add a PR to the commit queue

```
evergreen merge
<any text here will be added as the commit message>
```

To add a PR to the commit queue, comment `evergreen merge`. Any text after the newline will be added as the commit message.

