package units

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const localUpdateSSHKeysJobName = "update-ssh-keys-local"

type localUpdateSSHKeysJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
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
	return j
}

// NewLocalUpdateSSHKeysJob updates the SSH key files locally.
func NewLocalUpdateSSHKeysJob(id string) amboy.Job {
	j := makeLocalUpdateSSHKeysJob()
	j.SetID(fmt.Sprintf("%s.%s", localUpdateSSHKeysJobName, id))
	return j
}

func (j *localUpdateSSHKeysJob) Run(ctx context.Context) {
	settings := evergreen.GetEnvironment().Settings()
	for _, pair := range settings.SSHKeyPairs {
		j.AddError(errors.Wrap(writeFileWithPerm(pair.PrivatePath(settings), []byte(pair.Private), 0600), "could not make private key file"))
	}
	return
}

// writeFileWithPerm writes the contents to the file path if it does not exist
// yet and sets the permissions.
func writeFileWithPerm(path string, content []byte, perm os.FileMode) error {
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
