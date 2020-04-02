package units

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/sometimes"
)

const testResultsCleanupJobName = "data-cleanup-testresults"

func init() {
	registry.AddJobType(testResultsCleanupJobName, func() amboy.Job {
		return makeTestResultsCleanupJob()
	})
}

type dataCleanupTestResults struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	env evergreen.Environment
}

func makeTestResultsCleanupJob() *dataCleanupTestResults {
	j := &dataCleanupTestResults{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    testResultsCleanupJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

func NewTestResultsCleanupJob(ts time.Time) amboy.Job {
	j := makeTestResultsCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", testResultsCleanupJobName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.TimeInfo{MaxTime: 50 * time.Second})
	return j
}

func (j *dataCleanupTestResults) Run(ctx context.Context) {
	defer j.MarkComplete()
	startAt := time.Now()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.BackgroundCleanup {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job_type": testResultsCleanupJobName,
			"job_id":   j.ID(),
			"message":  "disaster recovery backups disabled, also disabling cleanup",
		})
		return
	}

	var (
		batches   int
		numDocs   int
		timeSpent time.Duration
	)

	totalDocs, _ := j.env.DB().Collection(testresult.Collection).EstimatedDocumentCount(ctx)

	lock := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer func() { j.AddError(recovery.HandlePanicWithError(recover(), nil, "double batch deletes")) }()
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					break
				default:
					if time.Since(startAt) >= 45*time.Second {
						break
					}
					opStart := time.Now()
					num, err := testresult.DeleteWithLimit(ctx, j.env, time.Now().Add(time.Duration(-365*24)*time.Hour), 100*1000)
					j.AddError(err)
					opTime := time.Since(opStart)
					func() {
						lock.Lock()
						defer lock.Unlock()
						batches++
						numDocs += num
						timeSpent += opTime
					}()
				}
			}
		}()
	}

	wg.Wait()
	grip.Info(message.Fields{
		"job_id":             j.ID(),
		"job_type":           j.Type().Name,
		"total_docs":         totalDocs,
		"message":            "timing-info",
		"run_start_at":       startAt,
		"has_errors":         j.HasErrors(),
		"aborted":            ctx.Err() != nil,
		"total":              time.Since(startAt).Seconds(),
		"run_end_at":         time.Now(),
		"num_batches":        batches,
		"num_docs":           numDocs,
		"time_spent_seconds": timeSpent.Seconds(),
	})

}
