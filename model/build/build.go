package build

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// IdTimeLayout is used time time.Time.Format() to produce timestamps for our ids.
const IdTimeLayout = "06_01_02_15_04_05"

// TaskCache represents some duped information about tasks,
// mainly for ui purposes.
type TaskCache struct {
	Id            string                  `bson:"id" json:"id"`
	DisplayName   string                  `bson:"d" json:"display_name"`
	Status        string                  `bson:"s" json:"status"`
	StatusDetails apimodels.TaskEndDetail `bson:"ed" json:"task_end_details"`
	StartTime     time.Time               `bson:"st" json:"start_time"`
	TimeTaken     time.Duration           `bson:"tt" json:"time_taken"`
	Activated     bool                    `bson:"a" json:"activated"`
}

// Build represents a set of tasks on one variant of a Project
// 	e.g. one build might be "Ubuntu with Python 2.4" and
//  another might be "OSX with Python 3.0", etc.
type Build struct {
	Id                  string        `bson:"_id" json:"_id"`
	CreateTime          time.Time     `bson:"create_time" json:"create_time,omitempty"`
	StartTime           time.Time     `bson:"start_time" json:"start_time,omitempty"`
	FinishTime          time.Time     `bson:"finish_time" json:"finish_time,omitempty"`
	Version             string        `bson:"version" json:"version,omitempty"`
	Project             string        `bson:"branch" json:"branch,omitempty"`
	Revision            string        `bson:"gitspec" json:"gitspec,omitempty"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant,omitempty"`
	BuildNumber         string        `bson:"build_number" json:"build_number,omitempty"`
	Status              string        `bson:"status" json:"status,omitempty"`
	Activated           bool          `bson:"activated" json:"activated,omitempty"`
	ActivatedBy         string        `bson:"activated_by" json:"activated_by,omitempty"`
	ActivatedTime       time.Time     `bson:"activated_time" json:"activated_time,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty" json:"order,omitempty"`
	Tasks               []TaskCache   `bson:"tasks" json:"tasks,omitempty"`
	TimeTaken           time.Duration `bson:"time_taken" json:"time_taken,omitempty"`
	DisplayName         string        `bson:"display_name" json:"display_name,omitempty"`
	PredictedMakespan   time.Duration `bson:"predicted_makespan" json:"predicted_makespan,omitempty"`
	ActualMakespan      time.Duration `bson:"actual_makespan" json:"actual_makespan,omitempty"`

	// build requester - this is used to help tell the
	// reason this build was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"r,omitempty"`

	// Fields set if triggered by an upstream build
	TriggerID    string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	TriggerType  string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`
}

// Returns whether or not the build has finished, based on its status.
// In spite of the name, a build with status BuildFailed may still be in
// progress; use AllCachedTasksOrCompileFinished
func (b *Build) IsFinished() bool {
	return b.Status == evergreen.BuildFailed ||
		b.Status == evergreen.BuildSucceeded
}

// AllUnblockedTasksOrCompileFinished returns true when either:
//  1. if there is a compile task, the compile task's status is one the ones
//     listed in IsFailedTaskStatus
//  2. or all activated tasks in the task cache have one of the statuses in
//     IsFinishedTaskStatus or the task is considered blocked
// returns boolean to indicate if tasks are complete, string with either
// BuildFailed or BuildSucceded. The string is only valid when the boolean is
// true
func (b *Build) AllUnblockedTasksFinished(tasksWithDeps []task.Task) (bool, string, error) {
	if !b.Activated {
		return false, b.Status, nil
	}
	allFinished := true
	status := evergreen.BuildSucceeded
	catcher := grip.NewSimpleCatcher()
	for i := range b.Tasks {
		if !b.Tasks[i].Activated {
			continue
		}
		if evergreen.IsFailedTaskStatus(b.Tasks[i].Status) {
			status = evergreen.BuildFailed
		}
		if !evergreen.IsFinishedTaskStatus(b.Tasks[i].Status) {
			t, err := task.FindOneNoMerge(task.ById(b.Tasks[i].Id))
			if err != nil {
				return false, status, err
			}
			if t == nil {
				return false, status, errors.Errorf("task %s doesn't exist", b.Tasks[i].Id)
			}

			blockedStatus, err := t.BlockedState(tasksWithDeps)
			if err != nil {
				return false, status, err
			}
			if blockedStatus != "blocked" {
				allFinished = false
			}
		}
	}
	err := catcher.Resolve()
	if allFinished && err != nil {
		return false, status, err
	}

	return allFinished, status, nil
}

// Find

// FindBuildOnBaseCommit returns the build that a patch build is based on.
func (b *Build) FindBuildOnBaseCommit() (*Build, error) {
	return FindOne(ByRevisionAndVariant(b.Revision, b.BuildVariant))
}

// Find all builds on the same project + variant + requester between
// the current b and the specified previous build.
func (b *Build) FindIntermediateBuilds(previous *Build) ([]Build, error) {
	return Find(ByBetweenBuilds(b, previous))
}

// Find the most recent activated build with the same variant +
// requester + project as the current build.
func (b *Build) PreviousActivated(project string, requester string) (*Build, error) {
	return FindOne(ByRecentlyActivatedForProjectAndVariant(
		b.RevisionOrderNumber, project, b.BuildVariant, requester))
}

// Find the most recent b on with the same build variant + requester +
// project as the current build, with any of the specified statuses.
func (b *Build) PreviousSuccessful() (*Build, error) {
	return FindOne(ByRecentlySuccessfulForProjectAndVariant(
		b.RevisionOrderNumber, b.Project, b.BuildVariant))
}

// UpdateActivation updates one build with the given id
// to the given activation setting.
func UpdateActivation(buildId string, active bool, caller string) error {
	var err error
	if !active && (evergreen.IsSystemActivator(caller)) {
		_, err = UpdateAllBuilds(
			bson.M{IdKey: buildId,
				ActivatedByKey: caller,
			},
			bson.M{
				"$set": bson.M{
					ActivatedKey:     active,
					ActivatedTimeKey: time.Now(),
					ActivatedByKey:   caller,
				},
			},
		)
	} else {
		_, err = UpdateAllBuilds(
			bson.M{IdKey: buildId},
			bson.M{
				"$set": bson.M{
					ActivatedKey:     active,
					ActivatedTimeKey: time.Now(),
					ActivatedByKey:   caller,
				},
			},
		)
	}
	return err

}

// UpdateStatus sets the build status to the given string.
func (b *Build) UpdateStatus(status string) error {
	b.Status = status
	return UpdateOne(
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{StatusKey: status}},
	)
}

// UpdateMakespans sets the builds predicted and actual makespans to given durations
func (b *Build) UpdateMakespans(predictedMakespan, actualMakespan time.Duration) error {
	b.PredictedMakespan = predictedMakespan
	b.ActualMakespan = actualMakespan

	return UpdateOne(
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{PredictedMakespanKey: predictedMakespan, ActualMakespanKey: actualMakespan}},
	)
}

// TryMarkBuildStarted attempts to mark a b as started if it
// isn't already marked as such
func TryMarkStarted(buildId string, startTime time.Time) error {
	selector := bson.M{
		IdKey:     buildId,
		StatusKey: evergreen.BuildCreated,
	}
	update := bson.M{"$set": bson.M{
		StatusKey:    evergreen.BuildStarted,
		StartTimeKey: startTime,
	}}
	err := UpdateOne(selector, update)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

// MarkFinished sets the build to finished status in the database (this does
// not update task or version data).
func (b *Build) MarkFinished(status string, finishTime time.Time) error {
	b.Status = status
	b.FinishTime = finishTime
	b.TimeTaken = finishTime.Sub(b.StartTime)
	return UpdateOne(
		bson.M{IdKey: b.Id},
		bson.M{
			"$set": bson.M{
				StatusKey:     status,
				FinishTimeKey: finishTime,
				TimeTakenKey:  b.TimeTaken,
			},
		},
	)
}

// Create

// Insert writes the b to the db.
func (b *Build) Insert() error {
	return db.Insert(Collection, b)
}

// Checks if the build is active (has any active task)
func (b *Build) IsActive() bool {
	for _, task := range b.Tasks {
		if task.Activated {
			return true
		}
	}
	return false
}

func (b *Build) SetCachedTaskFinished(taskID, status string, detail *apimodels.TaskEndDetail, timeTaken time.Duration) error {
	if err := SetCachedTaskFinished(b.Id, taskID, status, detail, timeTaken); err != nil {
		return err
	}
	for i := range b.Tasks {
		if b.Tasks[i].Id != taskID {
			continue
		}

		b.Tasks[i].Id = taskID
		b.Tasks[i].Status = status
		b.Tasks[i].TimeTaken = timeTaken
		b.Tasks[i].StatusDetails = *detail
		break
	}
	return nil
}

func (b *Build) IsPatchBuild() bool {
	return evergreen.IsPatchRequester(b.Requester)
}
