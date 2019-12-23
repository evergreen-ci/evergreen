package units

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/backup"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const backupMDBJobName = "backup-mdb-collection"

func init() {
	registry.AddJobType(backupMDBJobName, func() amboy.Job {
		return makeBackupMDBCollectionJob()
	})
}

type backupMDBCollectionJob struct {
	Options  backup.Options `bson:"options" json:"options" yaml:"options"`
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
	j.Options = opts
	j.SetID(fmt.Sprintf("%s.%s.%s", backupMDBJobName, opts.NS.String(), ts.Format(TSFormat)))
	return j
}

func (j *backupMDBCollectionJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrapf(err, "Can't get degraded mode flags"))
		return
	}

	if flags.DRBackupDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     backupMDBJobName,
			"message": "disaster recovery backup job disabled",
		})
		return
	}
	conf := j.env.Settings().Backup

	client := util.GetHTTPClient()
	client.Timeout = 60 * time.Minute
	defer util.PutHTTPClient(client)

	bucket, err := pail.NewS3MultiPartBucketWithHTTPClient(client, pail.S3Options{
		Credentials: pail.CreateAWSCredentials(conf.Key, conf.Secret, ""),
		Permissions: pail.S3PermissionsPrivate,
		Name:        conf.BucketName,
		Compress:    conf.Compress,
		Prefix:      path.Join(conf.Prefix, j.TimeInfo().Created.Format(TSFormat), "dump"),
	})
	if err != nil {
		j.AddError(err)
		return
	}

	if err := bucket.Check(ctx); err != nil {
		j.AddError(err)
		return
	}

	j.Options.Target = bucket.Writer
	j.AddError(backup.Collection(ctx, j.env.Client(), j.Options))
}
