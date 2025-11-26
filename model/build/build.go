package build

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// IdTimeLayout is used time time.Time.Format() to produce timestamps for our ids.
const IdTimeLayout = "06_01_02_15_04_05"

// TaskCache references the id of a task in the build
type TaskCache struct {
	Id string `bson:"id" json:"id"`
}

// Build represents a set of tasks on one variant of a Project
// e.g. one build might be "Ubuntu with Python 2.4" and
// another might be "OSX with Python 3.0", etc.
type Build struct {
	Id                  string        `bson:"_id" json:"_id"`
	CreateTime          time.Time     `bson:"create_time" json:"create_time,omitempty"`
	StartTime           time.Time     `bson:"start_time" json:"start_time,omitempty"`
	FinishTime          time.Time     `bson:"finish_time" json:"finish_time,omitempty"`
	Version             string        `bson:"version" json:"version,omitempty"`
	Project             string        `bson:"branch" json:"branch,omitempty"`
	Revision            string        `bson:"gitspec" json:"gitspec,omitempty"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant,omitempty"`
	Status              string        `bson:"status" json:"status,omitempty"`
	Activated           bool          `bson:"activated" json:"activated,omitempty"`
	ActivatedBy         string        `bson:"activated_by" json:"activated_by,omitempty"`
	ActivatedTime       time.Time     `bson:"activated_time" json:"activated_time,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty" json:"order,omitempty"`
	Tasks               []TaskCache   `bson:"tasks" json:"tasks"`
	TimeTaken           time.Duration `bson:"time_taken" json:"time_taken,omitempty"`
	DisplayName         string        `bson:"display_name" json:"display_name,omitempty"`
	PredictedMakespan   time.Duration `bson:"predicted_makespan" json:"predicted_makespan,omitempty"`
	ActualMakespan      time.Duration `bson:"actual_makespan" json:"actual_makespan,omitempty"`
	Aborted             bool          `bson:"aborted" json:"aborted,omitempty"`

	// Tags that describe the variant
	Tags []string `bson:"tags,omitempty" json:"tags,omitempty"`

	// The status of the subset of the build that's used for github checks
	GithubCheckStatus string `bson:"github_check_status,omitempty" json:"github_check_status,omitempty"`
	// does the build contain tasks considered for mainline github checks
	IsGithubCheck bool `bson:"is_github_check,omitempty" json:"is_github_check,omitempty"`

	// build requester - this is used to help tell the
	// reason this build was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"r,omitempty"`

	// builds that are part of a child patch will store the id and number of the parent patch
	ParentPatchID     string `bson:"parent_patch_id,omitempty" json:"parent_patch_id,omitempty"`
	ParentPatchNumber int    `bson:"parent_patch_number,omitempty" json:"parent_patch_number,omitempty"`

	// Fields set if triggered by an upstream build
	TriggerID    string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	TriggerType  string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`

	// Set to true if all tasks in the build are blocked.
	// Should not be exposed, only for internal use.
	AllTasksBlocked bool `bson:"all_tasks_blocked"`
	// HasUnfinishedEssentialTask tracks whether or not this build has at least
	// one unfinished essential task. The build cannot be in a finished state
	// until all of its essential tasks have finished.
	HasUnfinishedEssentialTask bool `bson:"has_unfinished_essential_task"`
}

func (b *Build) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(b) }
func (b *Build) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, b) }

// Returns whether or not the build has finished, based on its status.
// In spite of the name, a build with status BuildFailed may still be in
// progress; use AllCachedTasksFinished
func (b *Build) IsFinished() bool {
	return evergreen.IsFinishedBuildStatus(b.Status)
}

// FindBuildOnBaseCommit returns the build that a patch build is based on.
func (b *Build) FindBuildOnBaseCommit(ctx context.Context) (*Build, error) {
	return FindOne(ctx, ByRevisionAndVariant(b.Revision, b.BuildVariant))
}

// Find the most recent b on with the same build variant + requester +
// project as the current build, with any of the specified statuses.
func (b *Build) PreviousSuccessful(ctx context.Context) (*Build, error) {
	return FindOne(ctx, ByRecentlySuccessfulForProjectAndVariant(
		b.RevisionOrderNumber, b.Project, b.BuildVariant))
}

func getSetBuildActivatedUpdate(active bool, caller string) bson.M {
	return bson.M{
		ActivatedKey:     active,
		ActivatedTimeKey: time.Now(),
		ActivatedByKey:   caller,
	}
}

// UpdateActivation updates builds with the given ids
// to the given activation setting.
func UpdateActivation(ctx context.Context, buildIds []string, active bool, caller string) error {
	if len(buildIds) == 0 {
		return nil
	}
	query := bson.M{IdKey: bson.M{"$in": buildIds}}
	if !active && evergreen.IsSystemActivator(caller) {
		query[ActivatedByKey] = bson.M{"$in": evergreen.SystemActivators}
	}

	err := UpdateAllBuilds(
		ctx,
		query,
		bson.M{
			"$set": getSetBuildActivatedUpdate(active, caller),
		},
	)
	return errors.Wrapf(err, "setting build activation to %t", active)
}

// UpdateStatus sets the build status to the given string.
func (b *Build) UpdateStatus(ctx context.Context, status string) error {
	if b.Status == status {
		return nil
	}
	b.Status = status
	return UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{StatusKey: status}},
	)
}

// SetAborted sets the build aborted field to the given boolean.
func (b *Build) SetAborted(ctx context.Context, aborted bool) error {
	if b.Aborted == aborted {
		return nil
	}
	b.Aborted = aborted
	return UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{AbortedKey: aborted}},
	)
}

// SetActivated sets the build activated field to the given boolean.
func (b *Build) SetActivated(ctx context.Context, activated bool) error {
	if b.Activated == activated {
		return nil
	}
	b.Activated = activated
	return UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{ActivatedKey: activated}},
	)
}

// SetAllTasksBlocked sets the build AllTasksBlocked field to the given boolean.
func (b *Build) SetAllTasksBlocked(ctx context.Context, blocked bool) error {
	if b.AllTasksBlocked == blocked {
		return nil
	}
	b.AllTasksBlocked = blocked
	return UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{AllTasksBlockedKey: blocked}},
	)
}

// SetTasksCache updates one build with the given id
// to contain the given task caches.
func SetTasksCache(ctx context.Context, buildId string, tasks []TaskCache) error {
	return UpdateOne(
		ctx,
		bson.M{IdKey: buildId},
		bson.M{"$set": bson.M{TasksKey: tasks}},
	)
}

func (b *Build) UpdateGithubCheckStatus(ctx context.Context, status string) error {
	if b.GithubCheckStatus == status {
		return nil
	}
	b.GithubCheckStatus = status
	return UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{GithubCheckStatusKey: status}},
	)
}

func (b *Build) SetIsGithubCheck(ctx context.Context) error {
	if b.IsGithubCheck {
		return nil
	}
	b.IsGithubCheck = true
	return UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{IsGithubCheckKey: true}},
	)
}

// SetHasUnfinishedEssentialTask sets whether or not the build has at least one
// unfinished essential task.
func (b *Build) SetHasUnfinishedEssentialTask(ctx context.Context, hasUnfinishedEssentialTask bool) error {
	if b.HasUnfinishedEssentialTask == hasUnfinishedEssentialTask {
		return nil
	}
	if err := UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{HasUnfinishedEssentialTaskKey: hasUnfinishedEssentialTask}},
	); err != nil {
		return err
	}

	b.HasUnfinishedEssentialTask = hasUnfinishedEssentialTask

	return nil
}

// UpdateMakespans sets the builds predicted and actual makespans to given durations
func (b *Build) UpdateMakespans(ctx context.Context, predictedMakespan, actualMakespan time.Duration) error {
	b.PredictedMakespan = predictedMakespan
	b.ActualMakespan = actualMakespan

	return UpdateOne(
		ctx,
		bson.M{IdKey: b.Id},
		bson.M{"$set": bson.M{PredictedMakespanKey: predictedMakespan, ActualMakespanKey: actualMakespan}},
	)
}

// TryMarkStarted attempts to mark a build as started if it
// isn't already marked as such
func TryMarkStarted(ctx context.Context, buildId string, startTime time.Time) error {
	selector := bson.M{
		IdKey:     buildId,
		StatusKey: bson.M{"$ne": evergreen.BuildStarted},
	}
	update := bson.M{"$set": bson.M{
		StatusKey:    evergreen.BuildStarted,
		StartTimeKey: startTime,
	}}
	err := UpdateOne(ctx, selector, update)
	if adb.ResultsNotFound(err) {
		return nil
	}
	return err
}

// MarkFinished sets the build to finished status in the database (this does
// not update task or version data).
func (b *Build) MarkFinished(ctx context.Context, status string, finishTime time.Time) error {
	b.Status = status
	b.FinishTime = finishTime
	b.TimeTaken = finishTime.Sub(b.StartTime)
	return UpdateOne(
		ctx,
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

func (b *Build) GetTimeSpent(ctx context.Context) (time.Duration, time.Duration, error) {
	query := db.Query(task.ByBuildId(b.Id)).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey)
	tasks, err := task.FindAllFirstExecution(ctx, query)
	if err != nil {
		return 0, 0, errors.Wrap(err, "getting tasks")
	}

	timeTaken, makespan := task.GetTimeSpent(tasks)
	return timeTaken, makespan, nil
}

// GetURL returns a url to the build page.
func (b *Build) GetURL(uiBase string) string {
	return fmt.Sprintf("%s/build/%s", uiBase, url.PathEscape(b.Id))
}

// Insert writes the b to the db.
func (b *Build) Insert(ctx context.Context) error {
	return db.Insert(ctx, Collection, b)
}

type Builds []*Build

func (b Builds) getPayload() []any {
	payload := make([]any, len(b))
	for idx := range b {
		payload[idx] = any(b[idx])
	}

	return payload
}

func (b Builds) InsertMany(ctx context.Context, ordered bool) error {
	if len(b) == 0 {
		return nil
	}
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertMany(ctx, b.getPayload(), options.InsertMany().SetOrdered(ordered))
	return errors.Wrap(err, "bulk inserting builds")
}

// GetPRNotificationDescription returns a GitHub PR status description based on
// the statuses of tasks in the build, to be used by jobs and notification
// processing.
func (b *Build) GetPRNotificationDescription(tasks []task.Task) string {
	success := 0
	failed := 0
	other := 0
	runningOrWillRun := 0
	unscheduledEssential := 0
	for _, t := range tasks {
		switch {
		case t.Status == evergreen.TaskSucceeded:
			success++

		case t.Status == evergreen.TaskFailed:
			failed++

		case utility.StringSliceContains(evergreen.TaskUncompletedStatuses, t.Status):
			if utility.StringSliceContains(evergreen.TaskInProgressStatuses, t.Status) || (t.Activated && !t.Blocked() && !t.IsFinished()) {
				runningOrWillRun++
			}
			if t.IsUnscheduled() && t.IsEssentialToSucceed {
				unscheduledEssential++
			}
		default:
			other++
		}
	}

	grip.ErrorWhen(other > 0, message.Fields{
		"source":   "status updates",
		"message":  "unknown task status",
		"build_id": b.Id,
	})

	if runningOrWillRun > 0 {
		return evergreen.PRTasksRunningDescription
	}

	if success == 0 && failed == 0 && other == 0 {
		if unscheduledEssential > 0 {
			return unscheduledEssentialTaskStatusSubformat(unscheduledEssential)
		}
		return "no tasks were run"
	}

	desc := fmt.Sprintf("%s, %s", taskStatusSubformat(success, "succeeded"),
		taskStatusSubformat(failed, "failed"))
	if unscheduledEssential > 0 {
		desc = fmt.Sprintf("%s, %s", desc, unscheduledEssentialTaskStatusSubformat(unscheduledEssential))
	}
	if other > 0 {
		desc += fmt.Sprintf(", %d other", other)
	}

	return b.appendTime(desc)
}

// unscheduledEssentialTaskStatusSubformat returns a GitHub PR status
// description indicating that the build has some essential tasks that are not
// scheduled to run.
func unscheduledEssentialTaskStatusSubformat(numEssentialTasksNeeded int) string {
	return fmt.Sprintf("%d essential task(s) not scheduled", numEssentialTasksNeeded)
}

func taskStatusSubformat(n int, verb string) string {
	if n == 0 {
		return fmt.Sprintf("none %s", verb)
	}
	return fmt.Sprintf("%d %s", n, verb)
}

func (b *Build) appendTime(txt string) string {
	finish := b.FinishTime
	// In case the build is actually blocked, but we are triggering the finish event
	if utility.IsZeroTime(b.FinishTime) {
		finish = time.Now()
	}
	return fmt.Sprintf("%s in %s", txt, finish.Sub(b.StartTime).String())
}
