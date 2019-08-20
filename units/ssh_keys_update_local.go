package units

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const localUpdateSSHKeysJobName = "update-ssh-key-pairs-local"

type localUpdateSSHKeysJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	env evergreen.Environment
}

func init() {
	registry.AddJobType(localUpdateSSHKeysJobName, func() amboy.Job {
		return makeLocalUpdateSSHKeysJob()
	})
}

func makeLocalUpdateSSHKeysJob() *localUpdateSSHKeysJob {
	j := &localUpdateSSHKeysJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    localUpdateSSHKeysJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewLocalUpdateSSHKeysJob updates the SSH key files locally.
func NewLocalUpdateSSHKeysJob(id string) amboy.Job {
	j := makeLocalUpdateSSHKeysJob()
	j.SetID(fmt.Sprintf("%s.%s", localUpdateSSHKeysJobName, id))
	return j
}

func (j *localUpdateSSHKeysJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	writeFileWithPerm := func(path string, content []byte, perm os.FileMode) error {
		if stat, err := os.Stat(path); err == nil {
			if stat.Mode().Perm() != perm {
				return errors.Wrap(os.Chmod(path, perm), "could not change file permissions")
			}
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return errors.Wrap(err, "could not make parent directories")
		}

		if err := ioutil.WriteFile(path, content, 0200); err != nil {
			return errors.Wrap(err, "could not write file")
		}

		return errors.Wrap(os.Chmod(path, perm), "could not change file permissions")
	}

	for _, pair := range settings.SSHKeyPairs {
		j.AddError(errors.Wrap(writeFileWithPerm(pair.PublicPath, []byte(pair.Public), 0644), "could not make public key file"))
		j.AddError(errors.Wrap(writeFileWithPerm(pair.PrivatePath, []byte(pair.Private), 0600), "could not make private key file"))
	}

	return
}
