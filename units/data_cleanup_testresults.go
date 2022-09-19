package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	testResultsCleanupJobName = "data-cleanup-testresults"
	cleanupBatchSize          = 100 * 1000
)

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
	return j
}

func NewTestResultsCleanupJob(ts time.Time) amboy.Job {
	j := makeTestResultsCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", testResultsCleanupJobName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: time.Minute})
	j.SetScopes([]string{testResultsCleanupJobName})
	j.SetEnqueueAllScopes(true)
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
		j.AddError(errors.Wrap(err, "getting service flags"))
		return
	}

	if flags.BackgroundCleanupDisabled {
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
	timestamp := time.Now().Add(time.Duration(-180*24) * time.Hour)

deleteDocs:
	for {
		select {
		case <-ctx.Done():
			break deleteDocs
		default:
			if time.Since(startAt) >= 45*time.Second {
				break deleteDocs
			}
			opStart := time.Now()
			num, err := testresult.DeleteWithLimit(ctx, j.env, timestamp, cleanupBatchSize)
			j.AddError(errors.Wrap(err, "deleting test results"))

			batches++
			numDocs += num
			timeSpent += time.Since(opStart)
			if num < cleanupBatchSize {
				break deleteDocs
			}
		}
	}

	grip.Info(message.Fields{
		"job_id":             j.ID(),
		"job_type":           j.Type().Name,
		"batch_size":         cleanupBatchSize,
		"total_docs":         totalDocs,
		"collection":         testresult.Collection,
		"message":            "timing-info",
		"run_start_at":       startAt,
		"oid":                primitive.NewObjectIDFromTimestamp(timestamp).Hex(),
		"has_errors":         j.HasErrors(),
		"aborted":            ctx.Err() != nil,
		"total":              time.Since(startAt).Seconds(),
		"run_end_at":         time.Now(),
		"num_batches":        batches,
		"num_docs":           numDocs,
		"time_spent_seconds": timeSpent.Seconds(),
	})
}
