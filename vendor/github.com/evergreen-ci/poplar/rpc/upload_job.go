package rpc

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/poplar"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	uploadJobName = "convert-and-upload-artifact"
)

type uploadJob struct {
	Artifact poplar.TestArtifact        `bson:"artifact_info" json:"artifact_info" yaml:"artifact_info"`
	Conf     poplar.BucketConfiguration `bson:"conf" json:"conf" yaml:"conf"`
	DryRun   bool                       `bson:"dry_run" json:"dry_run" yaml:"dry_run"`

	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func init() {
	registry.AddJobType(uploadJobName, func() amboy.Job { return makeUploadJob() })
}

func makeUploadJob() *uploadJob {
	j := &uploadJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    uploadJobName,
				Version: 1,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewUploadJob(artifact poplar.TestArtifact, conf poplar.BucketConfiguration, dryRun bool) amboy.Job {
	j := makeUploadJob()

	j.Artifact = artifact
	j.Conf = conf
	j.DryRun = dryRun

	j.SetID(fmt.Sprintf("convert-and-upload-artifact.%s.%s", j.Artifact.LocalFile, j.Artifact.Path))

	return j
}

func (j *uploadJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.Artifact.LocalFile == "" {
		return
	}

	grip.Info(message.Fields{
		"op":     "uploading file",
		"path":   j.Artifact.Path,
		"bucket": j.Artifact.Bucket,
		"prefix": j.Artifact.Prefix,
		"file":   j.Artifact.LocalFile,
	})

	j.AddError(errors.Wrapf(j.Artifact.Upload(ctx, j.Conf, j.DryRun), "problem uploading artifact"))
}
