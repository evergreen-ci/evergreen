# Run Tests with Github

Specific Github pull request behavior can trigger behavior in Evergreen.

## Help Text

```
evergreen help
```

We have documentation here but we also provide it on the PR itself. It will display commands that are available for your project, with some context about when to use them. If the commit queue is disabled but there is an available message, we will still display the message. If PR testing isn't enabled for the branch but [PR testing is set up for untracked branches](../01-Configure-a-Project/04-Using-Repo-Level-Settings#how-to-use-pr-testing-for-untracked-branches) then we will also still show the related Github Pull request commands (detailed below).


## Github Pull Request Testing

Evergreen has an option to create patches for pull requests and this can be defined on the project page. 

If "Automated Testing" is enabled, Evergreen will automatically create a patch for each pull request opened in the repository as well as each subsequent push to each pull request.

If you'd like the option of creating patches but wouldn't like it to happen automatically, you can enable "Manual Testing".

You can read more about these options [here](../01-Configure-a-Project/03-Project-and-Distro-Settings.md#github-pull-request-testing).

#### Retry a patch

```
evergreen retry
```
Sometimes Evergreen has trouble creating a PR patch, due to internal server errors or Github flakiness. Commenting `evergreen retry` will attempt to recreate this patch. 

Note that this is specific to Github PR checks; it won't retry a commit queue patch. For that, re-type `evergreen merge` (detailed below).

#### Create a patch for manual testing

```
evergreen patch
```
If your project is configured for manual testing, then Evergreen will only add Github checks to the PR when prompted, as opposed to for every commit. Commenting `evergreen patch` will trigger this.

#### Refresh Github checks

```
evergreen refresh
```
Sometimes Evergreen has trouble sending updated Github statuses, so the checks on the PR may not accurately reflect the state of patch on Evergreen. This is especially troublesome when the repository requires passing checks. To re-sync the Github checks with Evergreen, comment `evergreen refresh` on the PR.

## Commit Queue 

Evergreen's commit queue merges changes after the code has passed a set of tests. You can read more about this [here](01-Commit-Queue#commit-queue).

#### Add a PR to the commit queue

```
evergreen merge
<any text here will be added as the commit message>
```

To add a PR to the commit queue, comment `evergreen merge`. Any text after the newline will be added as the commit message.




