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

func DoProjectActivation(ctx context.Context, id string, ts time.Time) (bool, error) {
	// Find the most recently activated version to use as a reference point
	lastActivatedVersion, err := VersionFindOne(ctx, VersionByMostRecentActivated(id, ts))
	if err != nil {
		return false, errors.Wrap(err, "finding most recently activated version")
	}

	var activateVersions []Version
	if lastActivatedVersion == nil {
		// No previously activated versions - this might be a new project or first activation
		// Activate ALL unactivated non-ignored versions to ensure complete coverage
		activateVersions, err = VersionFind(ctx, VersionsAllUnactivatedNonIgnored(id, ts))
		if err != nil {
			return false, errors.WithStack(err)
		}
	} else {
		// Find all unactivated versions since the last activated one
		activateVersions, err = VersionFind(ctx, VersionsUnactivatedSinceLastActivated(id, ts, lastActivatedVersion.RevisionOrderNumber))
		if err != nil {
			return false, errors.WithStack(err)
		}
	}

	if len(activateVersions) == 0 {
		grip.Debug(message.Fields{
			"message":   "no versions to activate for repository",
			"project":   id,
			"operation": "project-activation",
		})
		return false, nil
	}

	// Activate all eligible versions
	anyActivated := false
	activatedCount := 0
	for _, version := range activateVersions {
		activated, err := ActivateElapsedBuildsAndTasks(ctx, &version)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "error activating version",
				"project":   id,
				"version":   version.Id,
				"revision":  version.Revision,
				"operation": "project-activation",
			}))
			// Continue with other versions even if one fails
			continue
		}
		if activated {
			anyActivated = true
			activatedCount++
			grip.Info(message.Fields{
				"message":   "activated version",
				"project":   id,
				"version":   version.Id,
				"revision":  version.Revision,
				"operation": "project-activation",
			})
		}
	}

	if anyActivated {
		lastActivatedInfo := "none"
		if lastActivatedVersion != nil {
			lastActivatedInfo = lastActivatedVersion.Id
		}
		grip.Info(message.Fields{
			"message":              "project activation completed",
			"project":              id,
			"versions_checked":     len(activateVersions),
			"versions_activated":   activatedCount,
			"last_activated_version": lastActivatedInfo,
			"operation":            "project-activation",
		})
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
		// Skip ignored build variants (similar to how ignored versions are skipped)
		if bv.Ignored {
			continue
		}

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
	// Update the stored version so that we don't attempt to reactivate any variants/tasks
	return true, v.ActivateAndSetBuildVariants(ctx)
}
