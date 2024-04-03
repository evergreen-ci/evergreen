package command

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type papertrailTrace struct {
	Address   string   `mapstructure:"address" plugin:"expand"`
	KeyID     string   `mapstructure:"key_id" plugin:"expand"`
	SecretKey string   `mapstructure:"secret_key" plugin:"expand"`
	Product   string   `mapstructure:"product" plugin:"expand"`
	Version   string   `mapstructure:"version" plugin:"expand"`
	Filenames []string `mapstructure:"filenames" plugin:"expand"`
	WorkDir   string   `mapstructure:"work_dir" plugin:"expand"`
	base
}

// This command is owned by the Dev Prod Release Infrastructure team
func (t *papertrailTrace) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(t, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	pclient := thirdparty.NewPapertrailClient(t.KeyID, t.SecretKey, "")

	task := conf.Task

	workdir := t.WorkDir
	if workdir == "" {
		workdir = conf.WorkDir
	}

	const platform = "evergreen"

	for _, file := range t.Filenames {
		fullname := filepath.Join(workdir, file)

		sha256sum, err := getSHA256(fullname)
		if err != nil {
			return errors.Wrap(err, "getting sha256")
		}

		papertrailBuildID := getPapertrailBuildID(task.Id, task.Execution)

		// should already be the basename, but just in case
		basename := filepath.Base(file)

		args := thirdparty.TraceArgs{
			Build:     papertrailBuildID,
			Platform:  platform,
			Filename:  basename,
			Sha256:    sha256sum,
			Product:   t.Product,
			Version:   t.Version,
			Submitter: task.ActivatedBy,
		}

		if err := pclient.Trace(ctx, args); err != nil {
			return errors.Wrap(err, "running trace")
		}

		logger.Task().Infof("Successfully traced '%s' (sha256://%s)", fullname, sha256sum)
	}

	return nil
}

func getSHA256(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}

	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	sha256sum := fmt.Sprintf("%x", h.Sum(nil))

	return sha256sum, nil
}

func (t *papertrailTrace) ParseParams(params map[string]any) error {
	conf := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           t,
	}

	decoder, err := mapstructure.NewDecoder(conf)
	if err != nil {
		return errors.Wrap(err, "initializing new decoder")
	}

	if err := decoder.Decode(params); err != nil {
		return errors.Wrap(err, "decoding params")
	}

	catcher := grip.NewSimpleCatcher()

	if t.KeyID == "" {
		catcher.New("must specify key_id")
	}

	if t.SecretKey == "" {
		catcher.New("must specify secret_key")
	}

	if t.Product == "" {
		catcher.New("must specify product")
	}

	if t.Version == "" {
		catcher.New("must specify version")
	}

	if len(t.Filenames) == 0 {
		catcher.New("must specify at least one filename")
	}

	return catcher.Resolve()
}

func getPapertrailBuildID(taskID string, execution int) string {
	return fmt.Sprintf("%s_%d", taskID, execution)
}

func (t *papertrailTrace) Name() string { return "papertrail.trace" }

func papertrailTraceFactory() Command { return &papertrailTrace{} }
