package management

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	job.RegisterDefaultJobs()
	registry.AddJobType(testJobName, func() amboy.Job { return makeTestJob() })
}

type ManagerSuite struct {
	queue   amboy.Queue
	manager Manager
	ctx     context.Context
	cancel  context.CancelFunc

	factory func() Manager
	setup   func()
	cleanup func()
	suite.Suite
}

func TestManagerImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(defaultMongoDBTestOptions().URI))
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, client.Disconnect(ctx))
	}()

	teardownDB := func(ctx context.Context) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(client.Database(defaultMongoDBTestOptions().DB).Drop(ctx))
		return catcher.Resolve()
	}

	const queueSize = 128

	getInvalidFilters := func() []string {
		return []string{"", "foo", "inprog"}
	}

	type filteredJob struct {
		job            amboy.Job
		matchedFilters []StatusFilter
	}

	matchesFilter := func(fj filteredJob, f StatusFilter) bool {
		for _, mf := range fj.matchedFilters {
			if f == mf {
				return true
			}
		}
		return false
	}

	getFilteredJobs := func() []filteredJob {
		pending := newTestJob("pending")

		inProg := newTestJob("in-progress")
		inProg.SetStatus(amboy.JobStatusInfo{
			InProgress:       true,
			ModificationTime: time.Now(),
		})

		stale := newTestJob("stale")
		stale.SetStatus(amboy.JobStatusInfo{
			InProgress:       true,
			ModificationTime: time.Now().Add(-time.Hour),
		})

		completed := newTestJob("completed")
		completed.SetStatus(amboy.JobStatusInfo{
			Completed: true,
		})
		retrying := newTestJob("retrying")
		retrying.SetStatus(amboy.JobStatusInfo{
			Completed: true,
		})
		retrying.UpdateRetryInfo(amboy.JobRetryOptions{
			Retryable:  utility.TruePtr(),
			NeedsRetry: utility.TruePtr(),
		})

		completedRetryable := newTestJob("completed-retryable")
		completedRetryable.SetStatus(amboy.JobStatusInfo{
			Completed: true,
		})
		completedRetryable.UpdateRetryInfo(amboy.JobRetryOptions{
			Retryable: utility.TruePtr(),
		})
		return []filteredJob{
			{
				job:            pending,
				matchedFilters: []StatusFilter{All, Pending},
			},
			{
				job:            inProg,
				matchedFilters: []StatusFilter{All, InProgress},
			},
			{
				job:            stale,
				matchedFilters: []StatusFilter{All, InProgress, Stale},
			},
			{
				job:            completed,
				matchedFilters: []StatusFilter{All, Completed},
			},
			{
				job:            retrying,
				matchedFilters: []StatusFilter{All, Completed, Retrying},
			},
			{
				job:            completedRetryable,
				matchedFilters: []StatusFilter{All, Completed},
			},
		}
	}

	partitionByFilter := func(fjs []filteredJob, f StatusFilter) (matched []filteredJob, unmatched []filteredJob) {
		for _, fj := range fjs {
			if matchesFilter(fj, f) {
				matched = append(matched, fj)
			} else {
				unmatched = append(unmatched, fj)
			}
		}
		return matched, unmatched
	}

	name := uuid.New().String()

	for managerName, managerCase := range map[string]struct {
		makeQueue   func(context.Context) (amboy.Queue, error)
		makeManager func(context.Context, amboy.Queue) (Manager, error)
		teardown    func(context.Context) error
	}{
		"MongoDB": {
			makeQueue: func(ctx context.Context) (amboy.Queue, error) {
				queueOpts := queue.MongoDBQueueCreationOptions{
					Size:   queueSize,
					Name:   name,
					MDB:    defaultMongoDBTestOptions(),
					Client: client,
				}
				return queue.NewMongoDBQueue(ctx, queueOpts)
			},
			makeManager: func(ctx context.Context, _ amboy.Queue) (Manager, error) {
				mgrOpts := DBQueueManagerOptions{
					Options: defaultMongoDBTestOptions(),
					Name:    name,
				}
				return MakeDBQueueManager(ctx, mgrOpts, client)
			},
			teardown: teardownDB,
		},
		"MongoDBSingleGroup": {
			makeQueue: func(ctx context.Context) (amboy.Queue, error) {
				opts := defaultMongoDBTestOptions()
				opts.UseGroups = true
				opts.GroupName = "group"
				queueOpts := queue.MongoDBQueueCreationOptions{
					Size:   queueSize,
					Name:   name,
					MDB:    opts,
					Client: client,
				}
				return queue.NewMongoDBQueue(ctx, queueOpts)
			},
			makeManager: func(ctx context.Context, _ amboy.Queue) (Manager, error) {
				opts := defaultMongoDBTestOptions()
				opts.UseGroups = true
				opts.GroupName = "group"
				mgrOpts := DBQueueManagerOptions{
					Options:     opts,
					Name:        name,
					Group:       opts.GroupName,
					SingleGroup: true,
				}
				return MakeDBQueueManager(ctx, mgrOpts, client)
			},
			teardown: teardownDB,
		},
		// TODO (EVG-14270): defer testing this until remote management
		// interface is improved, since it may be deleted.
		// "MongoDBMultiGroup": {
		//     makeQueue: func(ctx context.Context) (amboy.Queue, error) {
		//         opts := defaultMongoDBTestOptions()
		//         opts.UseGroups = true
		//         opts.GroupName = "group"
		//         queueOpts := queue.MongoDBQueueCreationOptions{
		//             Size:   queueSize,
		//             Name:   name,
		//             MDB:    opts,
		//             Client: client,
		//         }
		//         return queue.NewMongoDBQueue(ctx, queueOpts)
		//     },
		//     makeManager: func(ctx context.Context, _ amboy.Queue) (Manager, error) {
		//         opts := defaultMongoDBTestOptions()
		//         opts.UseGroups = true
		//         opts.GroupName = "group"
		//         mgrOpts := DBQueueManagerOptions{
		//             Options:  opts,
		//             Name:     name,
		//             Group:    opts.GroupName,
		//             ByGroups: true,
		//         }
		//         return MakeDBQueueManager(ctx, mgrOpts, client)
		//     },
		//     teardown: teardownDB,
		// },
		"Queue-Backed": {
			makeQueue: func(ctx context.Context) (amboy.Queue, error) {
				return queue.NewLocalLimitedSizeSerializable(2, queueSize)
			},
			makeManager: func(ctx context.Context, q amboy.Queue) (Manager, error) {
				return NewQueueManager(q), nil
			},
			teardown: func(context.Context) error { return nil },
		},
	} {
		t.Run(managerName, func(t *testing.T) {
			t.Run("CompleteJobSucceedsWithFilter", func(t *testing.T) {
				for _, f := range ValidStatusFilters() {
					t.Run("Filter="+string(f), func(t *testing.T) {
						q, err := managerCase.makeQueue(ctx)
						require.NoError(t, err)
						mgr, err := managerCase.makeManager(ctx, q)
						require.NoError(t, err)

						defer func() {
							assert.NoError(t, managerCase.teardown(ctx))
						}()

						fjs := getFilteredJobs()
						for _, fj := range fjs {
							require.NoError(t, q.Put(ctx, fj.job))
						}

						require.NoError(t, mgr.CompleteJobs(ctx, f))

						matched, unmatched := partitionByFilter(fjs, f)
						var numJobs int
						for info := range q.JobInfo(ctx) {
							for _, fj := range matched {
								if fj.job.ID() == info.ID {
									assert.True(t, info.Status.Completed, "job '%s should be complete'", info.ID)
									assert.NotZero(t, info.Status.ModificationCount, "job '%s' should be complete", info.ID)
								}
							}
							for _, fj := range unmatched {
								if fj.job.ID() == info.ID {
									if matchesFilter(fj, Completed) {
										assert.True(t, info.Status.Completed, "job '%s' should be complete", info.ID)
										continue
									}
									assert.False(t, info.Status.Completed, "job '%s' should not be complete", info.ID)
									assert.Zero(t, info.Status.ModificationCount, info.ID)
								}
							}
							numJobs++
						}
						assert.Equal(t, len(fjs), numJobs)
					})
				}
			})

			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue){
				"JobIDsByStateSucceeds": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					fjs := getFilteredJobs()
					for _, fj := range fjs {
						require.NoError(t, q.Put(ctx, fj.job))
					}

					for _, f := range ValidStatusFilters() {
						groupedIDs, err := mgr.JobIDsByState(ctx, testJobName, f)
						require.NoError(t, err)

						matched, unmatched := partitionByFilter(fjs, f)
						for _, fj := range matched {
							var foundJobID bool
							for _, gid := range groupedIDs {
								if strings.Contains(gid.ID, fj.job.ID()) {
									foundJobID = true
									break
								}
							}
							assert.True(t, foundJobID)
						}
						for _, fj := range unmatched {
							for _, gid := range groupedIDs {
								assert.NotContains(t, gid.ID, fj.job.ID())
							}
						}
					}
				},
				"JobIDsByStateSucceedsWithEmptyResult": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range ValidStatusFilters() {
						counts, err := mgr.JobIDsByState(ctx, "foo", f)
						assert.NoError(t, err)
						assert.Empty(t, counts)
					}
				},
				"JobIDsByStateFailsWithInvalidFilter": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range getInvalidFilters() {
						r, err := mgr.JobIDsByState(ctx, "foo", StatusFilter(f))
						assert.Error(t, err)
						assert.Empty(t, r)
					}
				},
				"JobStatusSucceedsWithEmptyResult": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range ValidStatusFilters() {
						counts, err := mgr.JobStatus(ctx, f)
						assert.NoError(t, err)
						assert.Empty(t, counts)
					}
				},
				"JobStatusFailsWithInvalidFilter": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range getInvalidFilters() {
						counts, err := mgr.JobStatus(ctx, StatusFilter(f))
						assert.Error(t, err)
						assert.Empty(t, counts)
					}
				},
				"CompleteJobSucceeds": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					j0 := job.NewShellJob("ls", "")
					j1 := newTestJob("complete")
					j2 := newTestJob("incomplete")

					require.NoError(t, q.Put(ctx, j0))
					require.NoError(t, q.Put(ctx, j1))
					require.NoError(t, q.Put(ctx, j2))

					require.NoError(t, mgr.CompleteJob(ctx, j1.ID()))

					var numJobs int
					for info := range q.JobInfo(ctx) {
						switch info.ID {
						case j1.ID():
							assert.True(t, info.Status.Completed)
							if _, ok := mgr.(*dbQueueManager); ok {
								assert.NotZero(t, info.Status.ModificationCount)
							}
						default:
							assert.False(t, info.Status.Completed)
							assert.Zero(t, info.Status.ModificationCount)
						}
						numJobs++
					}
					assert.Equal(t, 3, numJobs)
				},
				"CompleteJobWithNonexistentJob": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					assert.Error(t, mgr.CompleteJob(ctx, "nonexistent"))
				},
				"CompleteJobsSucceeds": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					j0 := job.NewShellJob("ls", "")
					j1 := newTestJob("pending1")
					j2 := newTestJob("pending2")
					require.NoError(t, q.Put(ctx, j0))
					require.NoError(t, q.Put(ctx, j1))
					require.NoError(t, q.Put(ctx, j2))

					require.NoError(t, mgr.CompleteJobs(ctx, Pending))
					var numJobs int
					for info := range q.JobInfo(ctx) {
						assert.True(t, info.Status.Completed)
						if _, ok := mgr.(*dbQueueManager); ok {
							assert.NotZero(t, info.Status.ModificationCount)
						}
						numJobs++
					}
					assert.Equal(t, 3, numJobs)
				},
				"CompleteJobsSucceedsWithoutMatches": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range ValidStatusFilters() {
						assert.NoError(t, mgr.CompleteJobs(ctx, f))
					}
				},
				"CompleteJobsFailsWithInvalidFilter": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range getInvalidFilters() {
						assert.Error(t, mgr.CompleteJobs(ctx, StatusFilter(f)))
					}
				},
				"CompleteJobsByTypeFailsWithInvalidFilte": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range getInvalidFilters() {
						assert.Error(t, mgr.CompleteJobsByType(ctx, StatusFilter(f), "type"))
					}
				},
				"CompleteJobsByTypeSucceedsWithoutMatches": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range ValidStatusFilters() {
						assert.NoError(t, mgr.CompleteJobsByType(ctx, f, "type"))
					}
				},
				"CompleteJobByPatternSucceeds": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					j0 := job.NewShellJob("ls", "")
					j1 := newTestJob("prefix1")
					j2 := newTestJob("prefix2")

					require.NoError(t, q.Put(ctx, j0))
					require.NoError(t, q.Put(ctx, j1))
					require.NoError(t, q.Put(ctx, j2))

					require.NoError(t, mgr.CompleteJobsByPattern(ctx, Pending, "prefix"))

					var numJobs int
					for info := range q.JobInfo(ctx) {
						switch info.ID {
						case j1.ID(), j2.ID():
							assert.True(t, info.Status.Completed)
							if _, ok := mgr.(*dbQueueManager); ok {
								assert.NotZero(t, info.Status.ModificationCount)
							}
						default:
							assert.False(t, info.Status.Completed)
							assert.Zero(t, info.Status.ModificationCount)
						}
						numJobs++
					}
					assert.Equal(t, 3, numJobs)
				},
				"CompleteJobByPatternFailsWithInvalidFilter": func(ctx context.Context, t *testing.T, mgr Manager, q amboy.Queue) {
					for _, f := range getInvalidFilters() {
						assert.Error(t, mgr.CompleteJobsByPattern(ctx, StatusFilter(f), "prefix"))
					}
				},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
					defer tcancel()

					q, err := managerCase.makeQueue(tctx)
					require.NoError(t, err)

					mgr, err := managerCase.makeManager(tctx, q)
					require.NoError(t, err)

					defer func() {
						assert.NoError(t, managerCase.teardown(tctx))
					}()

					testCase(tctx, t, mgr, q)
				})
			}
		})
	}
}

const testJobName = "test"

type testJob struct {
	job.Base
}

func makeTestJob() *testJob {
	j := &testJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    testJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func newTestJob(id string) *testJob {
	j := makeTestJob()
	j.SetID(id)
	return j
}

func (j *testJob) Run(ctx context.Context) {
	time.Sleep(time.Minute)
	return
}
