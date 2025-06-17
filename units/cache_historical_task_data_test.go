package units

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheHistoricalTaskDataJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	defer func() {
		assert.NoError(t, db.ClearCollections(evergreen.ConfigCollection, task.Collection, taskstats.DailyTaskStatsCollection, taskstats.DailyStatsStatusCollection))
	}()

	now := time.Now().UTC()
	t0 := utility.GetUTCDay(now.Add(-7 * 24 * time.Hour))
	for _, test := range []struct {
		name   string
		pre    func(ctx context.Context, t *testing.T)
		post   func(ctx context.Context, t *testing.T)
		hasErr bool
	}{
		{
			name: "CacheStatsJobDisabled",
			pre: func(ctx context.Context, t *testing.T) {
				flags, err := evergreen.GetServiceFlags(ctx)
				require.NoError(t, err)
				flags.CacheStatsJobDisabled = true
				require.NoError(t, flags.Set(ctx))
			},
			post: func(ctx context.Context, t *testing.T) {
				flags, err := evergreen.GetServiceFlags(ctx)
				require.NoError(t, err)
				flags.CacheStatsJobDisabled = false
				require.NoError(t, flags.Set(ctx))
			},
			hasErr: true,
		},
		{
			name: "OnlyMainlineCommits",
			pre: func(ctx context.Context, t *testing.T) {
				for i, requester := range evergreen.AllRequesterTypes {
					tsk := task.Task{
						Id:           fmt.Sprintf("id%d", i),
						DisplayName:  "task0",
						Project:      "p0",
						BuildVariant: "variant",
						Requester:    requester,
						CreateTime:   t0,
						FinishTime:   now,
						Status:       "success",
					}
					require.NoError(t, tsk.Insert(t.Context()))
				}

				lastJobTime := now.Add(-time.Hour)
				require.NoError(t, taskstats.UpdateStatsStatus(t.Context(), "p0", lastJobTime, lastJobTime, time.Minute))
			},
			post: func(ctx context.Context, t *testing.T) {
				for _, requester := range evergreen.AllRequesterTypes {
					ts, err := taskstats.GetDailyTaskDoc(t.Context(), taskstats.DBTaskStatsID{
						TaskName:     "task0",
						BuildVariant: "variant",
						Project:      "p0",
						Requester:    requester,
						Date:         t0,
					})
					require.NoError(t, err)

					if requester == evergreen.RepotrackerVersionRequester {
						assert.NotNil(t, ts)
					} else {
						assert.Nil(t, ts)
					}

				}

				status, err := taskstats.GetStatsStatus(t.Context(), "p0")
				require.NoError(t, err)
				assert.WithinDuration(t, time.Now(), status.LastJobRun, time.Minute)
				assert.WithinDuration(t, time.Now(), status.ProcessedTasksUntil, time.Minute)
			},
		},
		{
			name: "UpdateWindowNoMoreThan12Hours",
			pre: func(ctx context.Context, t *testing.T) {
				for _, tsk := range []task.Task{
					{
						Id:           "id0",
						DisplayName:  "task0",
						Project:      "p0",
						BuildVariant: "variant",
						Requester:    evergreen.RepotrackerVersionRequester,
						CreateTime:   t0.Add(2 * time.Hour),
						FinishTime:   t0.Add(3 * time.Hour),
						Status:       "success",
					},
					{
						Id:           "id1",
						DisplayName:  "task1",
						Project:      "p0",
						BuildVariant: "variant",
						Requester:    evergreen.RepotrackerVersionRequester,
						CreateTime:   t0.Add(2 * time.Hour),
						FinishTime:   t0.Add(23 * time.Hour),
						Status:       "success",
						Execution:    1,
					},
				} {
					require.NoError(t, tsk.Insert(t.Context()))
				}

				lastJobTime := t0.Add(-2 * time.Hour)
				require.NoError(t, taskstats.UpdateStatsStatus(t.Context(), "p0", lastJobTime, lastJobTime, time.Minute))
			},
			post: func(ctx context.Context, t *testing.T) {
				ts, err := taskstats.GetDailyTaskDoc(t.Context(), taskstats.DBTaskStatsID{
					TaskName:     "task0",
					BuildVariant: "variant",
					Project:      "p0",
					Requester:    evergreen.RepotrackerVersionRequester,
					Date:         t0,
				})
				require.NoError(t, err)
				require.NotNil(t, ts)
				assert.Equal(t, 1, ts.NumSuccess)

				ts, err = taskstats.GetDailyTaskDoc(t.Context(), taskstats.DBTaskStatsID{
					TaskName:     "task1",
					BuildVariant: "variant",
					Project:      "p0",
					Requester:    evergreen.RepotrackerVersionRequester,
					Date:         t0,
				})
				require.NoError(t, err)
				assert.Nil(t, ts)

				status, err := taskstats.GetStatsStatus(t.Context(), "p0")
				require.NoError(t, err)
				assert.WithinDuration(t, time.Now(), status.LastJobRun, time.Minute)
				assert.WithinDuration(t, t0.Add(10*time.Hour), status.ProcessedTasksUntil, time.Minute)
			},
		},
		{
			name: "MultipleTasksAndDates",
			pre: func(ctx context.Context, t *testing.T) {
				for _, tsk := range []task.Task{
					{
						Id:           "id0",
						DisplayName:  "task0",
						Project:      "p0",
						BuildVariant: "variant",
						Requester:    evergreen.RepotrackerVersionRequester,
						CreateTime:   t0.Add(2 * time.Hour),
						FinishTime:   now.Add(-90 * time.Minute),
						Status:       "success",
					},
					{
						Id:           "id1",
						DisplayName:  "task0",
						Project:      "p0",
						BuildVariant: "variant",
						Requester:    evergreen.RepotrackerVersionRequester,
						CreateTime:   now.Add(-3 * time.Hour),
						FinishTime:   now.Add(-time.Hour),
						Status:       "success",
					},
					{
						Id:           "id2",
						DisplayName:  "task1",
						Project:      "p0",
						BuildVariant: "variant",
						Requester:    evergreen.RepotrackerVersionRequester,
						CreateTime:   t0.Add(2 * time.Hour),
						FinishTime:   now.Add(-5 * time.Minute),
						Status:       "success",
						Execution:    1,
					},
					{
						Id:           "id3",
						DisplayName:  "task1",
						Project:      "p0",
						BuildVariant: "variant",
						Requester:    evergreen.RepotrackerVersionRequester,
						CreateTime:   t0.Add(2 * time.Hour),
						FinishTime:   now.Add(-30 * time.Minute),
						Status:       "success",
						Execution:    1,
					},
				} {
					require.NoError(t, tsk.Insert(t.Context()))
				}

				lastJobTime := now.Add(-2 * time.Hour)
				require.NoError(t, taskstats.UpdateStatsStatus(t.Context(), "p0", lastJobTime, lastJobTime, time.Minute))
			},
			post: func(ctx context.Context, t *testing.T) {
				ts, err := taskstats.GetDailyTaskDoc(t.Context(), taskstats.DBTaskStatsID{
					TaskName:     "task0",
					BuildVariant: "variant",
					Project:      "p0",
					Requester:    evergreen.RepotrackerVersionRequester,
					Date:         t0,
				})
				require.NoError(t, err)
				require.NotNil(t, ts)
				assert.Equal(t, 1, ts.NumSuccess)

				ts, err = taskstats.GetDailyTaskDoc(t.Context(), taskstats.DBTaskStatsID{
					TaskName:     "task0",
					BuildVariant: "variant",
					Project:      "p0",
					Requester:    evergreen.RepotrackerVersionRequester,
					Date:         utility.GetUTCDay(now.Add(-3 * time.Hour)),
				})
				require.NoError(t, err)
				require.NotNil(t, ts)
				assert.Equal(t, 1, ts.NumSuccess)

				ts, err = taskstats.GetDailyTaskDoc(t.Context(), taskstats.DBTaskStatsID{
					TaskName:     "task1",
					BuildVariant: "variant",
					Project:      "p0",
					Requester:    evergreen.RepotrackerVersionRequester,
					Date:         t0,
				})
				require.NoError(t, err)
				require.NotNil(t, ts)
				assert.Equal(t, 2, ts.NumSuccess)

				status, err := taskstats.GetStatsStatus(t.Context(), "p0")
				require.NoError(t, err)
				assert.WithinDuration(t, time.Now(), status.LastJobRun, time.Minute)
				assert.WithinDuration(t, time.Now(), status.ProcessedTasksUntil, time.Minute)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tctx := testutil.TestSpan(ctx, t)

			require.NoError(t, db.ClearCollections(task.Collection, taskstats.DailyTaskStatsCollection, taskstats.DailyStatsStatusCollection))
			if test.pre != nil {
				test.pre(tctx, t)
			}

			j := NewCacheHistoricalTaskDataJob("id", "p0")
			if test.hasErr {
				j.Run(tctx)
				require.Error(t, j.Error())
			} else {
				j.Run(tctx)
				require.NoError(t, j.Error())
			}

			if test.post != nil {
				test.post(tctx, t)
			}
		})
	}
}
