package task

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	numRevisionsToSearch = 10
)

func (t *Task) setGenerateTasksEstimations() error {
	if !t.GenerateTask || (t.EstimatedNumGeneratedTasks == nil && t.EstimatedNumActivatedGeneratedTasks == nil) {
		return nil
	}

	query := db.Query(
		ByPreviousCommit(
			t.BuildVariant,
			t.DisplayName,
			t.Project,
			evergreen.RepotrackerVersionRequester,
			t.RevisionOrderNumber,
		),
	).Sort([]string{"-" + RevisionOrderNumberKey}).Limit(numRevisionsToSearch)

	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "finding tasks '%s' in '%s'", t.DisplayName, t.Project)
	}

	generatedTotal := 0
	activatedTotal := 0
	for _, task := range tasks {
		generatedTotal += task.NumGeneratedTasks
		activatedTotal += task.NumActivatedGeneratedTasks
	}

	t.EstimatedNumGeneratedTasks = utility.ToIntPtr(generatedTotal / len(tasks))
	t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(activatedTotal / len(tasks))

	if err = t.cacheGenerateTasksEstimations(); err != nil {
		return errors.Wrap(err, "caching generate tasks estimations")
	}

	return nil
}

func (t *Task) cacheGenerateTasksEstimations() error {
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				EstimatedNumGeneratedTasksKey:          t.EstimatedNumGeneratedTasks,
				EstimatedNumActivatedGeneratedTasksKey: t.EstimatedNumActivatedGeneratedTasks,
			},
		},
	)
}
