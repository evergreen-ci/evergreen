package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func DoProjectActivation(ctx context.Context, projectRef *ProjectRef, ts time.Time) (bool, error) {
	if projectRef.RunEveryMainlineCommit {
		return activateEveryRecentMainlineCommitForProject(ctx, projectRef, ts)
	}
	return activateMostRecentNonIgnoredCommitForProject(ctx, projectRef, ts)
}

func activateMostRecentNonIgnoredCommitForProject(ctx context.Context, projectRef *ProjectRef, ts time.Time) (bool, error) {
	// fetch the most recent, non-ignored version (before the given time) to activate
	activateVersion, err := VersionFindOne(ctx, VersionByMostRecentNonIgnored(projectRef.Id, ts))
	if err != nil {
		return false, errors.WithStack(err)
	}
	if activateVersion == nil {
		grip.Info(message.Fields{
			"message":   "no version to activate for repository",
			"project":   projectRef.Id,
			"operation": "project-activation",
		})
		return false, nil
	}
	activated, err := ActivateElapsedBuildsAndTasks(ctx, activateVersion)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return activated, nil
}

// activateEveryRecentMainlineCommitForProject activates all unactivated non-ignored versions
// up to the limit specified in projectRef.RunEveryMainlineCommitLimit or up to the last activated version.
func activateEveryRecentMainlineCommitForProject(ctx context.Context, projectRef *ProjectRef, ts time.Time) (bool, error) {
	lastActivatedVersion, err := VersionFindOne(ctx, VersionByMostRecentActivated(projectRef.Id, ts))
	if err != nil {
		return false, errors.Wrap(err, "finding most recently activated version")
	}

	var activateVersions []Version
	if lastActivatedVersion == nil {
		// If there is no previous activated versions, this may be a new project or a project's first activation.
		// In that case, activate previous unactivated non-ignored versions rather than since last activated.
		activateVersions, err = VersionFind(ctx, VersionsAllUnactivatedNonIgnored(projectRef.Id, ts, projectRef.RunEveryMainlineCommitLimit))
		if err != nil {
			return false, errors.Wrapf(err, "finding all unactivated non-ignored versions")
		}
	} else {
		// If there is a last activated version, activate only versions since that one.
		activateVersions, err = VersionFind(ctx, VersionsUnactivatedSinceLastActivated(projectRef.Id, ts, lastActivatedVersion.RevisionOrderNumber, projectRef.RunEveryMainlineCommitLimit))
		if err != nil {
			return false, errors.Wrapf(err, "finding unactivated versions since last activated version '%s'", lastActivatedVersion.Id)
		}
	}

	if len(activateVersions) == 0 {
		grip.Info(message.Fields{
			"message":   "no versions to activate for repository",
			"project":   projectRef.Id,
			"operation": "project-activation-every-commit",
		})
		return false, nil
	}
	// Activate all eligible versions
	anyActivated := false
	for _, version := range activateVersions {
		activated, err := ActivateElapsedBuildsAndTasks(ctx, &version)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "error activating version",
				"project":   projectRef.Id,
				"version":   version.Id,
				"revision":  version.Revision,
				"operation": "project-activation-every-commit",
			}))
			// Continue with other versions even if one fails
			continue
		}
		if activated {
			anyActivated = true
		}
	}

	return anyActivated, nil
}

// ActivateElapsedBuildsAndTasks activates any builds/tasks if their BatchTimes have elapsed.
func ActivateElapsedBuildsAndTasks(ctx context.Context, v *Version) (bool, error) {
	now := time.Now()

	buildIdsToActivate := []string{}
	elapsedBuildIds := []string{}
	allReadyTaskIds := []string{}
	allIgnoreTaskIds := []string{}

	if _, err := v.GetBuildVariants(ctx); err != nil {
		return false, errors.Wrap(err, "getting build variant info for version")
	}

	for i, bv := range v.BuildVariants {
		// If there are batchtime tasks, consider if these should/shouldn't be activated, regardless of build
		ignoreTasks := []string{}
		readyTasks := []string{}
		for j, t := range bv.BatchTimeTasks {
			isElapsedTask := t.ShouldActivate(now)
			if isElapsedTask {
				v.BuildVariants[i].BatchTimeTasks[j].Activated = true
				v.BuildVariants[i].BatchTimeTasks[j].ActivateAt = now
				readyTasks = append(readyTasks, t.TaskId)
			} else {
				// This task isn't ready, so it shouldn't be activated with the build
				ignoreTasks = append(ignoreTasks, t.TaskId)
			}
		}

		isElapsedBuild := bv.ShouldActivate(now)
		if !isElapsedBuild && len(readyTasks) == 0 {
			continue
		}
		grip.Info(message.Fields{
			"message":   "activating revision",
			"operation": "project-activation",
			"variant":   bv.BuildVariant,
			"project":   v.Identifier,
			"revision":  v.Revision,
		})

		// we only get this far if something in the build is being updated
		if !bv.Activated {
			grip.Info(message.Fields{
				"message":       "activating build",
				"operation":     "project-activation",
				"variant":       bv.BuildVariant,
				"build":         bv.BuildId,
				"project":       v.Identifier,
				"elapsed_build": isElapsedBuild,
			})

			buildIdsToActivate = append(buildIdsToActivate, bv.BuildId)
		}
		// If it's an elapsed build, update all tasks for the build, minus batch time tasks that aren't ready.
		// If it's elapsed tasks, update only those tasks.
		if isElapsedBuild {
			grip.Info(message.Fields{
				"message":      "activating tasks for build",
				"operation":    "project-activation",
				"variant":      bv.BuildVariant,
				"build":        bv.BuildId,
				"project":      v.Identifier,
				"ignore_tasks": ignoreTasks,
			})

			// only update build variant activation in the version if we activated the whole variant
			v.BuildVariants[i].Activated = true
			v.BuildVariants[i].ActivateAt = now

			elapsedBuildIds = append(elapsedBuildIds, bv.BuildId)
			allIgnoreTaskIds = append(allIgnoreTaskIds, ignoreTasks...)
		} else {
			grip.Info(message.Fields{
				"message":           "activating batchtime tasks",
				"operation":         "project-activation",
				"variant":           bv.BuildVariant,
				"build":             bv.BuildId,
				"project":           v.Identifier,
				"tasks_to_activate": readyTasks,
			})
			allReadyTaskIds = append(allReadyTaskIds, readyTasks...)
		}
	}

	if len(elapsedBuildIds) == 0 && len(allReadyTaskIds) == 0 {
		// Nothing was changed so we can return
		return false, nil
	}
	if len(buildIdsToActivate) > 0 {
		// Don't need to set the version in here since we do it ourselves in a single update
		if err := build.UpdateActivation(ctx, buildIdsToActivate, true, evergreen.BuildActivator); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "project-activation",
				"message":   "problem activating builds",
				"build_ids": buildIdsToActivate,
				"project":   v.Identifier,
			}))
		}
	}

	if len(elapsedBuildIds) > 0 {
		if err := setTaskActivationForBuilds(ctx, elapsedBuildIds, true, true, allIgnoreTaskIds, evergreen.ElapsedBuildActivator); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "project-activation",
				"message":   "problem activating tasks for builds",
				"builds":    elapsedBuildIds,
				"project":   v.Identifier,
			}))
		}
	}
	if len(allReadyTaskIds) > 0 {
		if err := task.ActivateTasksByIdsWithDependencies(ctx, allReadyTaskIds, evergreen.ElapsedTaskActivator); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "project-activation",
				"message":   "problem activating batchtime tasks",
				"tasks":     allReadyTaskIds,
				"project":   v.Identifier,
			}))
		}
	}
	if err := v.UpdateAggregateTaskCosts(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "failed to update version expected costs after task activation",
			"version": v.Id,
		}))
	}
	// Update the stored version so that we don't attempt to reactivate any variants/tasks
	return true, v.ActivateAndSetBuildVariants(ctx)
}
