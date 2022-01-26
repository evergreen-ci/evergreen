package units

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	pprofCheckName     = "pprof-check"
	goroutineThreshold = 1500
	pprofPath          = "/srv/evergreen/pprof"
	totalBytesLimit    = 1000000000
	checkInterval      = 5 * time.Minute
)

func init() {
	registry.AddJobType(pprofCheckName,
		func() amboy.Job { return makePprofCheckJob() })
}

type pprofCheckJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makePprofCheckJob() *pprofCheckJob {
	j := &pprofCheckJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    pprofCheckName,
				Version: 0,
			},
		},
	}
	return j
}

func NewPprofCheckJob(ts string) amboy.Job {
	j := makePprofCheckJob()
	j.SetID(fmt.Sprintf("%s.%s", pprofCheckName, ts))

	return j
}

func (j *pprofCheckJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	goroutineCount := runtime.NumGoroutine()
	if goroutineCount < goroutineThreshold {
		return
	}

	if err := os.MkdirAll(pprofPath, os.ModePerm); err != nil {
		j.AddError(errors.Wrapf(err, "creating pprof path '%s'", pprofPath))
		return
	}

	contents, err := ioutil.ReadDir(pprofPath)
	if err != nil {
		j.AddError(errors.Wrapf(err, "listing contents of '%s'", pprofPath))
		return
	}

	var totalSize int64
	for _, file := range contents {
		age := time.Now().Sub(file.ModTime())
		if age < checkInterval {
			grip.Info(message.Fields{
				"message":              "not collecting pprof",
				"job_id":               j.ID(),
				"reason":               "most recent file is newer then the threshold",
				"most_recent_file":     file.Name(),
				"age_in_minutes":       age.Minutes(),
				"threshold_in_minutes": checkInterval.Minutes(),
			})
			return
		}
		totalSize += file.Size()
	}

	if totalSize > totalBytesLimit {
		grip.Info(message.Fields{
			"message":    "not collecting pprof",
			"reason":     "already over the size limit",
			"job_id":     j.ID(),
			"total_size": totalSize,
			"limit":      totalBytesLimit,
		})
		return
	}

	pprofFilePath := filepath.Join(pprofPath, time.Now().Format(time.RFC3339))
	pprofFile, err := os.Create(pprofFilePath)
	if err != nil {
		j.AddError(errors.Wrapf(err, "creating pprof file '%s'", pprofFilePath))
		return
	}

	if err := pprof.Lookup("goroutine").WriteTo(pprofFile, 2); err != nil {
		j.AddError(errors.Wrap(err, "writing pprof to file"))
	}

	grip.Info(message.Fields{
		"message":         "wrote pprof to file",
		"job_id":          j.ID(),
		"path":            pprofFilePath,
		"goroutine_count": goroutineCount,
	})
}
