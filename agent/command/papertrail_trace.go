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

	papertrailBuildID := getPapertrailBuildID(task.Id, task.Execution)

	files, err := getTraceFiles(workdir, t.Filenames)
	if err != nil {
		return errors.Wrap(err, "getting trace files")
	}

	for _, f := range files {
		args := thirdparty.TraceArgs{
			Build:     papertrailBuildID,
			Platform:  platform,
			Filename:  f.filename,
			Sha256:    f.sha256,
			Product:   t.Product,
			Version:   t.Version,
			Submitter: task.ActivatedBy,
		}

		if err := pclient.Trace(ctx, args); err != nil {
			return errors.Wrap(err, "running trace")
		}

		logger.Task().Infof("Successfully traced '%s' (sha256://%s)", f.filename, f.sha256)
	}

	return nil
}

type papertrailTraceFile struct {
	filename string
	sha256   string
}

func getTraceFiles(workdir string, patterns []string) ([]papertrailTraceFile, error) {
	files := make([]papertrailTraceFile, 0, len(patterns))

	seen := make(map[string]string)

	for _, path := range patterns {
		patternpath := filepath.Join(workdir, path)

		matches, err := filepath.Glob(patternpath)
		if err != nil {
			return nil, errors.Wrap(err, "globbing filepath")
		}

		for _, matchpath := range matches {
			sha256sum, err := getSHA256(matchpath)
			if err != nil {
				return nil, errors.Wrap(err, "getting sha256")
			}

			basename := filepath.Base(matchpath)

			if v, ok := seen[basename]; ok {
				return nil, errors.Errorf("file '%s' matched multiple filename patterns ('%s' and '%s'); papertrail.trace requires uploaded filenames to be unique regardless of their path, please provide only unique file names", basename, path, v)
			}

			seen[basename] = path

			f := papertrailTraceFile{
				filename: basename,
				sha256:   sha256sum,
			}

			files = append(files, f)
		}
	}

	return files, nil
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
