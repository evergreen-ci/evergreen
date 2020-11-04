package command

import (
	"context"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/poplar"
	"github.com/evergreen-ci/poplar/rpc"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type perfSend struct {
	// AWSKey and AWSSecret are the user's credentials for authenticating
	// interactions with s3. These are required if any of the tests have
	// artifacts.
	AWSKey    string `mapstructure:"aws_key" plugin:"expand"`
	AWSSecret string `mapstructure:"aws_secret" plugin:"expand"`

	// Region is the s3 region where the global bucket is located. It
	// defaults to "us-east-1".
	Region string `mapstructure:"region" plugin:"expand"`

	// Bucket is the global s3 bucket to use when storing any artifacts
	// without a bucket specified.
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// Prefix specifies the global prefix to use within the s3 bucket for
	// any artifacts without a prefix specified.
	Prefix string `mapstructure:"region" plugin:"expand"`

	// File is the file containing either the json or yaml representation
	// of the performance report tests.
	File string `mapstructure:"file" plugin:"expand"`

	base
}

func perfSendFactory() Command { return &perfSend{} }
func (*perfSend) Name() string { return "perf.send" }

func (c *perfSend) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", c.Name())
	}

	if c.File == "" {
		return errors.New("'file' param must not be blank")
	}

	return nil
}

func (c *perfSend) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	// Read the file and add the evergreen info.
	filename := filepath.Join(conf.WorkDir, c.File)
	report, err := poplar.LoadTests(filename)
	if err != nil {
		return errors.Wrapf(err, "problem reading tests from '%s'", filename)
	}
	c.addEvgData(report, conf)

	// Send data to Cedar.
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrap(err, "problem connecting to cedar")
	}
	opts := rpc.UploadReportOptions{
		Report:     report,
		ClientConn: conn,
	}
	return errors.Wrap(rpc.UploadReport(ctx, opts), "failed to upload report to cedar")
}

func (c *perfSend) addEvgData(report *poplar.Report, conf *internal.TaskConfig) {
	report.Project = conf.Task.Project
	report.Version = conf.Task.Version
	report.Order = conf.Task.RevisionOrderNumber
	report.Variant = conf.Task.BuildVariant
	report.TaskName = conf.Task.DisplayName
	report.TaskID = conf.Task.Id
	report.Execution = conf.Task.Execution
	report.Mainline = !conf.Task.IsPatchRequest()

	report.BucketConf.APIKey = c.AWSKey
	report.BucketConf.APISecret = c.AWSSecret
	report.BucketConf.Name = c.Bucket
	report.BucketConf.Prefix = c.Prefix
	report.BucketConf.Region = c.Region
}
