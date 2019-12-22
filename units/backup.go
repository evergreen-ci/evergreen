package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/backup"
)

const backupMDBJobName = "backup-mdb-collection"

func init() {
	registry.AddJobType(backupMDBJobName, func() amboy.Job {
		return makeBackupMDBCollectionJob()
	})
}

type backupMDBCollectionJob struct {
	opts     backup.Options `bson:"options" json:"options" yaml:"options"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	env evergreen.Environment
}

func makeBackupMDBCollectionJob() *backupMDBCollectionJob {
	j := &backupMDBCollectionJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    backupMDBJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

func NewBackupMDBCollectionJob(opts backup.Options, ts time.Time) amboy.Job {
	j := makeBackupMDBCollectionJob()
	j.opts = opts
	j.SetID(fmt.Sprintf("%s.%s.%s", backupMDBJobName, opts.NS.String(), ts.Format(TSFormat)))
	return j
}

func (j *backupMDBCollectionJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	j.AddError(backup.Collection(ctx, j.env.Client(), j.opts))
}
