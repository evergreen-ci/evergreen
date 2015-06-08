package alerts

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alert"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/version"
	"labix.org/v2/mgo/bson"
	"time"
)

// TODO once model/task.go becomes a model/task package, this should be subsumed into there.
func lastFinishedQ(projectId, taskName, variant string, beforeRevNum int) db.Q {
	return db.Query(
		bson.M{
			model.TaskDisplayNameKey: taskName,
			model.TaskStatusKey: bson.M{
				"$in": []string{
					evergreen.TaskFailed,
					evergreen.TaskSucceeded,
				},
			},
			model.TaskBuildVariantKey: variant,
			model.TaskRevisionOrderNumberKey: bson.M{
				"$lt": beforeRevNum,
			},
		},
	).Sort([]string{"-" + model.TaskRevisionOrderNumberKey}).Limit(1)
}

func RunLastRevisionNotFoundTrigger(proj *model.ProjectRef, v *version.Version) error {
	ctx := triggerContext{
		projectRef: proj,
		version:    v,
	}
	trigger := LastRevisionNotFound{}
	// only one trigger to act on for now
	shouldExec, err := trigger.ShouldExecute(ctx)
	if err != nil {
		return err
	}
	if !shouldExec {
		return nil
	}
	err = alert.EnqueueAlertRequest(&alert.AlertRequest{
		Id:        bson.NewObjectId(),
		Trigger:   trigger.Id(),
		VersionId: v.Id,
		CreatedAt: time.Now(),
	})

	if err != nil {
		return err
	}
	return storeTriggerBookkeeping(ctx, []Trigger{trigger})
}

// RunTaskTriggers queues alerts for any active triggers on the tasks's state change.
func RunTaskFailureTriggers(task *model.Task) error {
	ctx, err := getTaskTriggerContext(task)
	if err != nil {
		return err
	}
	activeTriggers, err := getActiveTaskFailureTriggers(*ctx)
	if err != nil {
		return err
	}
	for _, trigger := range activeTriggers {
		req := &alert.AlertRequest{
			Id:        bson.NewObjectId(),
			Trigger:   trigger.Id(),
			TaskId:    task.Id,
			Execution: task.Execution,
			BuildId:   task.BuildId,
			VersionId: task.Version,
			ProjectId: task.Project,
			PatchId:   "",
			CreatedAt: time.Now(),
		}
		err := alert.EnqueueAlertRequest(req)
		if err != nil {
			return err
		}
		err = storeTriggerBookkeeping(*ctx, []Trigger{trigger})
		if err != nil {
			return err
		}
	}
	return nil
}

func RunHostProvisionFailTriggers(h *host.Host) error {
	ctx := triggerContext{host: h}
	trigger := &ProvisionFailed{}
	// only one provision failure trigger to act on for now
	shouldExec, err := trigger.ShouldExecute(ctx)
	if err != nil {
		return err
	}
	if !shouldExec {
		return nil
	}

	err = alert.EnqueueAlertRequest(&alert.AlertRequest{
		Id:        bson.NewObjectId(),
		Trigger:   trigger.Id(),
		HostId:    h.Id,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return err
	}
	return storeTriggerBookkeeping(ctx, []Trigger{trigger})
}

func RunSpawnWarningTriggers(host *host.Host) error {
	ctx := triggerContext{host: host}
	for _, trigger := range SpawnWarningTriggers {
		shouldExec, err := trigger.ShouldExecute(ctx)
		if err != nil {
			return err
		}
		if shouldExec {
			err := alert.EnqueueAlertRequest(&alert.AlertRequest{
				Id:        bson.NewObjectId(),
				Trigger:   trigger.Id(),
				HostId:    host.Id,
				CreatedAt: time.Now(),
			})
			if err != nil {
				return err
			}
			err = storeTriggerBookkeeping(ctx, []Trigger{trigger})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getTaskTriggerContext(task *model.Task) (*triggerContext, error) {
	ctx := triggerContext{task: task}
	tasks, err := model.FindTasks(lastFinishedQ(task.Project, task.DisplayName, task.BuildVariant, task.RevisionOrderNumber))
	if err != nil {
		return nil, err
	}
	if len(tasks) > 0 {
		ctx.previousCompleted = &tasks[0]
	}
	return &ctx, nil
}

// getActiveTaskTriggers returns a list of the triggers that should be executed for the given task,
// by testing the result of each one's ShouldExecute method.
func getActiveTaskFailureTriggers(ctx triggerContext) ([]Trigger, error) {
	if ctx.task == nil {
		return nil, nil
	}

	activeTriggers := []Trigger{}
	for _, trigger := range AvailableTaskFailTriggers {
		shouldExec, err := trigger.ShouldExecute(ctx)
		if err != nil {
			return nil, err
		}
		if shouldExec {
			activeTriggers = append(activeTriggers, trigger)
		}
	}
	return activeTriggers, nil
}

// storeTriggerBookkeeping runs through any trigger bookkeeping that must be done in order to
// "remember" the state of the trigger for previous executions.
func storeTriggerBookkeeping(ctx triggerContext, triggers []Trigger) error {
	for _, trigger := range triggers {
		alertRecord := trigger.CreateAlertRecord(ctx)
		if alertRecord == nil {
			continue
		}

		err := alertRecord.Insert()
		if err != nil {
			return err
		}
	}
	return nil
}
