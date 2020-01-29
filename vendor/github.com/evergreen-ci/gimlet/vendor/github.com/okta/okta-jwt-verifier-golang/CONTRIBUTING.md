Contributing to Okta Open Source Repos
======================================

Sign the CLA
------------

If you haven't already, [sign the CLA](https://developer.okta.com/cla/).  Common questions/answers are also listed on the CLA page.

Summary
-------
This document covers how to contribute to an Okta Open Source project. These instructions assume you have a GitHub
.com account, so if you don't have one you will have to create one. Your proposed code changes will be published to
your own fork of the Okta JWT verifier for Golang project and you will submit a Pull Request for your changes to be added.

_Lets get started!!!_


Fork the code
-------------

In your browser, navigate to: [https://github.com/okta/okta-jwt-verifier-golang](https://github.com/okta/okta-jwt-verifier-golang)

Fork the repository by clicking on the 'Fork' button on the top right hand side.  The fork will happen and you will be taken to your own fork of the repository.  Copy the Git repository URL by clicking on the clipboard next to the URL on the right hand side of the page under '**HTTPS** clone URL'.  You will paste this URL when doing the following `git clone` command.

On your computer, follow these steps to setup a local repository for working on the Okta JWT verifier for Golang:

``` bash
$ git clone https://github.com/YOUR_ACCOUNT/okta-jwt-verifier-golang.git
$ cd okta-jwt-verifier-golang
$ git remote add upstream https://github.com/okta/okta-jwt-verifier-golang.git
$ git checkout develop
$ git fetch upstream
$ git rebase upstream/develop
```


Making changes
--------------

It is important that you create a new branch to make changes on and that you do not change the `develop`
branch (other than to rebase in changes from `upstream/develop`).  In this example I will assume you will be making
your changes to a branch called `feature/myFeature`.  This `feature/myFeature` branch will be created on your local repository and will be pushed to your forked repository on GitHub.  Once this branch is on your fork you will create a Pull Request for the changes to be added to the Okta JWT verifier for Golang project.

It is best practice to create a new branch each time you want to contribute to the project and only track the changes for that pull request in this branch.

``` bash
$ git checkout develop
$ git checkout -b feature/myFeature
   (make your changes)
$ git status
$ git add <files>
$ git commit -m "descriptive commit message for your changes"
```

> The `-b` specifies that you want to create a new branch called `feature/myFeature`.  You only specify `-b` the first time you checkout because you are creating a new branch.  Once the `feature/myFeature` branch exists, you can later switch to it with only `git checkout feature/myFeature`.


Rebase `feature/myFeature` to include updates from `upstream/develop`
------------------------------------------------------------

It is important that you maintain an up-to-date `develop` branch in your local repository.  This is done by rebasing in
 the code changes from `upstream/develop` (the official Okta JWT verifier for Golang repository) into your local repository.
 You will want to do this before you start working on a feature as well as right before you submit your changes as a pull request.  I recommend you do this process periodically while you work to make sure you are working off the most recent project code.

This process will do the following:

1. Checkout your local `develop` branch
2. Synchronize your local `develop` branch with the `upstream/develop` so you have all the latest changes from the
project
3. Rebase the latest project code into your `feature/myFeature` branch so it is up-to-date with the upstream code

``` bash
$ git checkout develop
$ git fetch upstream
$ git rebase upstream/develop
$ git checkout feature/myFeature
$ git rebase develop
```

> Now your `feature/myFeature` branch is up-to-date with all the code in `upstream/develop`.


Make a GitHub Pull Request to contribute your changes
-----------------------------------------------------

When you are happy with your changes and you are ready to contribute them, you will create a Pull Request on GitHub to do so.  This is done by pushing your local changes to your forked repository (default remote name is `origin`) and then initiating a pull request on GitHub.

> **IMPORTANT:** Make sure you have rebased your `feature/myFeature` branch to include the latest code from `upstream/develop`
_before_ you do this.

``` bash
$ git push origin feature/myFeature
```

Now that the `feature/myFeature` branch has been pushed to your GitHub repository, you can initiate the pull request.

To initiate the pull request, do the following:

1. In your browser, navigate to your forked repository: [https://github.com/YOUR_ACCOUNT/okta-jwt-verifier-golang](https://github
.com/YOUR_ACCOUNT/okta-jwt-verifier-golang)
2. Click the new button called '**Compare & pull request**' that showed up just above the main area in your forked repository
3. Validate the pull request will be into the upstream `develop` and will be from your `feature/myFeature` branch
4. Enter a detailed description of the work you have done and then click '**Send pull request**'

If you are requested to make modifications to your proposed changes, make the changes locally on your `feature/myFeature` branch, re-push the `feature/myFeature` branch to your fork.  The existing pull request should automatically pick up the change and update accordingly.


Cleaning up after a successful pull request
-------------------------------------------

Once the `feature/myFeature` branch has been committed into the `upstream/develop` branch, your local `feature/myFeature` branch and
the `origin/feature/myFeature` branch are no longer needed.  If you want to make additional changes, restart the process with a new branch.

> **IMPORTANT:** Make sure that your changes are in `upstream/develop` before you delete your `feature/myFeature` and
`origin/feature/myFeature` branches!

You can delete these deprecated branches with the following:

``` bash
$ git checkout master
$ git branch -D feature/myFeature
$ git push origin :feature/myFeature
```