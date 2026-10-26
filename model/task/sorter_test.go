package task

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type insertTasksFunc func(context.Context, []any, ...*options.InsertManyOptions) (*mongo.InsertManyResult, error)

func (f insertTasksFunc) InsertMany(ctx context.Context, tasks []any, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
	return f(ctx, tasks, opts...)
}

func makeInsertTestTasks(count int) Tasks {
	tasks := make(Tasks, count)
	for i := range tasks {
		tasks[i] = &Task{Id: fmt.Sprintf("task-%d", i), NumDependents: i, DependsOn: []Dependency{{TaskId: "prerequisite", Status: evergreen.TaskSucceeded}}}
	}
	return tasks
}

func TestInsertUnorderedShouldContinueAfterDuplicateKeysAcrossBatches(t *testing.T) {
	ctx := t.Context()
	collection := evergreen.GetEnvironment().DB().Collection(t.Name())
	t.Cleanup(func() { require.NoError(t, collection.Drop(context.WithoutCancel(t.Context()))) })
	tasks := makeInsertTestTasks(2*taskInsertBatchSize + 1)
	tasks[1].Id = tasks[0].Id
	tasks[taskInsertBatchSize+1].Id = tasks[taskInsertBatchSize].Id

	err := tasks.insertUnordered(ctx, collection)
	var writeErrors mongo.BulkWriteException
	require.ErrorAs(t, err, &writeErrors)
	require.Len(t, writeErrors.WriteErrors, 2)
	assert.Equal(t, 1, writeErrors.WriteErrors[0].Index)
	assert.Equal(t, taskInsertBatchSize+1, writeErrors.WriteErrors[1].Index)
	assert.True(t, mongo.IsDuplicateKeyError(err))
	count, err := collection.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(len(tasks)-2), count)
	var last Task
	require.NoError(t, collection.FindOne(ctx, bson.M{"_id": tasks[len(tasks)-1].Id}).Decode(&last))
	assert.Equal(t, *tasks[len(tasks)-1], last)
}

func TestInsertUnorderedShouldBoundBatchesAndPreserveInput(t *testing.T) {
	tasks := makeInsertTestTasks(2*taskInsertBatchSize + 1)
	var sizes []int
	insert := insertTasksFunc(func(ctx context.Context, payload []any, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
		assert.Equal(t, t.Context(), ctx)
		require.Len(t, opts, 1)
		require.NotNil(t, opts[0].Ordered)
		assert.False(t, *opts[0].Ordered)
		start := 0
		for _, size := range sizes {
			start += size
		}
		for i, value := range payload {
			raw, ok := value.(bson.Raw)
			require.True(t, ok, "batch payload should contain stamped raw documents")
			id, err := raw.LookupErr("_id")
			require.NoError(t, err)
			assert.Equal(t, tasks[start+i].Id, id.StringValue())
		}
		sizes = append(sizes, len(payload))
		return &mongo.InsertManyResult{}, nil
	})
	require.NoError(t, tasks.insertUnordered(t.Context(), insert))
	assert.Equal(t, []int{taskInsertBatchSize, taskInsertBatchSize, 1}, sizes)
}

func TestInsertUnorderedShouldPreserveWriteConcernErrorsAndLabels(t *testing.T) {
	responses := []error{
		mongo.BulkWriteException{WriteConcernError: &mongo.WriteConcernError{Code: 64, Message: "write concern timeout"}, Labels: []string{"first-label"}},
		mongo.BulkWriteException{WriteErrors: []mongo.BulkWriteError{{WriteError: mongo.WriteError{Index: 0, Code: 11000}}}, Labels: []string{"second-label", "first-label"}},
		nil,
	}
	calls := 0
	insert := insertTasksFunc(func(context.Context, []any, ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
		require.Less(t, calls, len(responses))
		err := responses[calls]
		calls++
		return nil, err
	})
	err := makeInsertTestTasks(2*taskInsertBatchSize+1).insertUnordered(t.Context(), insert)
	var writeErrors mongo.BulkWriteException
	require.ErrorAs(t, err, &writeErrors)
	require.NotNil(t, writeErrors.WriteConcernError)
	assert.Equal(t, 64, writeErrors.WriteConcernError.Code)
	require.Len(t, writeErrors.WriteErrors, 1)
	assert.Equal(t, taskInsertBatchSize, writeErrors.WriteErrors[0].Index)
	assert.ElementsMatch(t, []string{"first-label", "second-label"}, writeErrors.Labels)
	assert.Equal(t, 3, calls)
}

func TestInsertUnorderedShouldStopAfterNonWriteErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "CommandError", err: mongo.CommandError{Code: 13, Message: "unauthorized"}},
		{name: "NetworkError", err: mongo.CommandError{Message: "connection closed", Labels: []string{"NetworkError"}}},
		{name: "ContextCanceled", err: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			insert := insertTasksFunc(func(context.Context, []any, ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
				calls++
				if calls == 2 {
					return nil, test.err
				}
				return nil, nil
			})
			err := makeInsertTestTasks(2*taskInsertBatchSize+1).insertUnordered(t.Context(), insert)
			assert.Equal(t, test.err, err)
			assert.Equal(t, 2, calls)
		})
	}
}
