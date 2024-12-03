package scheduler

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	testutil.Setup()
}

func TestDistroAliases(t *testing.T) {
	tasks := []task.Task{
		{
			Id:               "other",
			DistroId:         "one",
			Priority:         200,
			Version:          "foo",
			SecondaryDistros: []string{"two"},
		},
		{
			Id:               "one",
			DistroId:         "one",
			Priority:         2000,
			Version:          "foo",
			SecondaryDistros: []string{"two"},
		},
	}

	require.NoError(t, db.Clear(model.TaskQueuesCollection))
	require.NoError(t, db.Clear(model.TaskSecondaryQueuesCollection))

	require.NoError(t, db.Clear(model.VersionCollection))
	require.NoError(t, (&model.Version{Id: "foo"}).Insert())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("VerifyPrimaryQueue", func(t *testing.T) {
		distroOne := &distro.Distro{
			Id: "one",
		}

		t.Run("Tunable", func(t *testing.T) {
			require.NoError(t, db.Clear(model.TaskQueuesCollection))

			distroOne.PlannerSettings.Version = evergreen.PlannerVersionTunable
			output, err := PrioritizeTasks(ctx, distroOne, tasks, TaskPlannerOptions{ID: "tunable-0"})
			require.NoError(t, err)
			require.Len(t, output, 2)
			require.Equal(t, "one", output[0].Id)
			require.Equal(t, "other", output[1].Id)

			ct, err := db.Count(model.TaskQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 1, ct)

			ct, err = db.Count(model.TaskSecondaryQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 0, ct)
		})
		t.Run("UseLegacy", func(t *testing.T) {
			require.NoError(t, db.Clear(model.TaskQueuesCollection))

			distroOne.PlannerSettings.Version = evergreen.PlannerVersionLegacy
			output, err := PrioritizeTasks(ctx, distroOne, tasks, TaskPlannerOptions{ID: "legacy-1"})
			require.NoError(t, err)
			require.Len(t, output, 2)
			require.Equal(t, "one", output[0].Id)
			require.Equal(t, "other", output[1].Id)

			ct, err := db.Count(model.TaskQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 1, ct)

			ct, err = db.Count(model.TaskSecondaryQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 0, ct)
		})
	})

	require.NoError(t, db.Clear(model.TaskQueuesCollection))
	require.NoError(t, db.Clear(model.TaskSecondaryQueuesCollection))

	t.Run("DistroAlias", func(t *testing.T) {
		distroTwo := &distro.Distro{
			Id: "two",
		}

		t.Run("Tunable", func(t *testing.T) {
			require.NoError(t, db.Clear(model.TaskSecondaryQueuesCollection))

			distroTwo.PlannerSettings.Version = evergreen.PlannerVersionTunable
			output, err := PrioritizeTasks(ctx, distroTwo, tasks, TaskPlannerOptions{ID: "tunable-0", IsSecondaryQueue: true})
			require.NoError(t, err)
			require.Len(t, output, 2)
			require.Equal(t, "one", output[0].Id)
			require.Equal(t, "other", output[1].Id)

			ct, err := db.Count(model.TaskSecondaryQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 1, ct)

			ct, err = db.Count(model.TaskQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 0, ct)

		})
		t.Run("UseLegacy", func(t *testing.T) {
			require.NoError(t, db.Clear(model.TaskSecondaryQueuesCollection))

			distroTwo.PlannerSettings.Version = evergreen.PlannerVersionLegacy
			output, err := PrioritizeTasks(ctx, distroTwo, tasks, TaskPlannerOptions{ID: "legacy-0", IsSecondaryQueue: true})
			require.NoError(t, err)
			require.Len(t, output, 2)
			require.Equal(t, "one", output[0].Id)
			require.Equal(t, "other", output[1].Id)

			ct, err := db.Count(model.TaskSecondaryQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 1, ct)

			ct, err = db.Count(model.TaskQueuesCollection, bson.M{})
			require.NoError(t, err)
			require.Equal(t, 0, ct)
		})
	})
}
