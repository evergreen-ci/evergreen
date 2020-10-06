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

func DoProjectActivation(id string) error {
	// fetch the most recent, non-ignored version to activate
	activateVersion, err := VersionFindOne(VersionByMostRecentNonIgnored(id))
	if err != nil {
		return errors.WithStack(err)
	}
	if activateVersion == nil {
		grip.Info(message.Fields{
			"message":   "no version to activate for repository",
			"project":   id,
			"operation": "project-activation",
		})
		return nil
	}

	if err = ActivateElapsedBuildsAndTasks(activateVersion); err != nil {
		return errors.WithStack(err)
	}

	return nil

}

// Activates any builds/tasks if their BatchTimes have elapsed.
func ActivateElapsedBuildsAndTasks(v *Version) error {
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

		// SHORT CIRCUIT
		if !isElapsedBuild && len(readyTasks) == 0 {
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
		// UPDATE BUILD
		if !bv.Activated {
			grip.Info(message.Fields{
				"message":       "activating build",
				"operation":     "project-activation",
				"variant":       bv.BuildVariant,
				"build":         bv.BuildId,
				"project":       v.Identifier,
				"elapsed_build": isElapsedBuild,
			})
			// Go copies the slice value, we want to modify the actual value
			v.BuildVariants[i].Activated = true
			v.BuildVariants[i].ActivateAt = now

			// ONLY DO THIS IF WE'RE UPDATING BUILD
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
		// if it's an elapsed build, update all tasks minus failed batch time tasks
		// if it's elapsed tasks, update only those tasks
		if isElapsedBuild {
			grip.Info(message.Fields{
				"message":      "activating tasks for build",
				"operation":    "project-activation",
				"variant":      bv.BuildVariant,
				"build":        bv.BuildId,
				"project":      v.Identifier,
				"ignore_tasks": ignoreTasks,
			})
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
		return v.UpdateBuildVariants()
	}
	return nil

}
