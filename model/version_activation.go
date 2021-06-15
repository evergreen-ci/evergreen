package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func DoProjectActivation(id string) (bool, error) {
	// fetch the most recent, non-ignored version to activate
	activateVersion, err := VersionFindOne(VersionByMostRecentNonIgnored(id))
	if err != nil {
		return false, errors.WithStack(err)
	}
	if activateVersion == nil {
		grip.Info(message.Fields{
			"message":   "no version to activate for repository",
			"project":   id,
			"operation": "project-activation",
		})
		return false, nil
	}
	activated, err := ActivateElapsedBuildsAndTasks(activateVersion)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return activated, nil

}

// Activates any builds/tasks if their BatchTimes have elapsed.
func ActivateElapsedBuildsAndTasks(v *Version) (bool, error) {
	hasActivated := false
	now := time.Now()

	for i, bv := range v.BuildVariants {
		// if there are batchtime tasks, consider if these should/shouldn't be activated, regardless of build
		ignoreTasks := []string{}
		readyTasks := []string{}
		for j, t := range bv.BatchTimeTasks {
			isElapsedTask := t.ShouldActivate(now)
			if isElapsedTask {
				v.BuildVariants[i].BatchTimeTasks[j].Activated = true
				v.BuildVariants[i].BatchTimeTasks[j].ActivateAt = now
				readyTasks = append(readyTasks, t.TaskId)
			} else {
				// this task isn't ready so it shouldn't be activated with the build
				ignoreTasks = append(ignoreTasks, t.TaskId)
			}
		}

		isElapsedBuild := bv.ShouldActivate(now)
		if !isElapsedBuild && len(readyTasks) == 0 {
			grip.Debug(message.Fields{
				"message":          "not activating build",
				"ignore_tasks":     ignoreTasks,
				"is_elapsed_build": isElapsedBuild,
				"build_variant":    bv.BuildId,
			})
			continue
		}
		hasActivated = true
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

			// Don't need to set the version in here since we do it ourselves in a single update
			if err := build.UpdateActivation([]string{bv.BuildId}, true, evergreen.DefaultTaskActivator); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"operation": "project-activation",
					"message":   "problem activating build",
					"variant":   bv.BuildVariant,
					"build":     bv.BuildId,
					"project":   v.Identifier,
				}))
				continue
			}
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

			if err := setTaskActivationForBuilds([]string{bv.BuildId}, true, ignoreTasks, evergreen.DefaultTaskActivator); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"operation": "project-activation",
					"message":   "problem activating tasks for build",
					"variant":   bv.BuildVariant,
					"build":     bv.BuildId,
					"project":   v.Identifier,
				}))
				continue
			}
		} else {
			grip.Info(message.Fields{
				"message":           "activating batchtime tasks",
				"operation":         "project-activation",
				"variant":           bv.BuildVariant,
				"build":             bv.BuildId,
				"project":           v.Identifier,
				"tasks_to_activate": readyTasks,
			})
			// only activate the tasks that are ready to be set
			if err := task.ActivateTasksByIdsWithDependencies(readyTasks, evergreen.DefaultTaskActivator); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"operation": "project-activation",
					"message":   "problem activating batchtime tasks",
					"variant":   bv.BuildVariant,
					"build":     bv.BuildId,
					"project":   v.Identifier,
				}))
				continue
			}
		}
	}

	// If any variants/tasks were activated, update the stored version so that we don't
	// attempt to activate them again
	if hasActivated {
		if err := v.SetActivated(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "project-activation",
				"message":   "problem activating version",
				"version":   v.Id,
			}))
		}
		return true, v.UpdateBuildVariants()
	}
	return false, nil

}
