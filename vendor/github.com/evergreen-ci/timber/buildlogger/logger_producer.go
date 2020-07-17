package buildlogger

import (
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// JasperLoggerOptions wraps LoggerOptions and implements a jasper
// options.LoggerProducer for BuildloggerV3.
type JasperLoggerOptions struct {
	Name          string         `json:"name" bson:"name"`
	Level         send.LevelInfo `json:"level" bson:"level"`
	BuildloggerV3 LoggerOptions  `json:"buildloggerv3" bson:"buildloggerv3"`
}

// NewBuildloggerV3LoggerProducer returns a jasper options.LoggerProducer
// backed by JasperLoggerOptions.
func NewBuildloggerV3LoggerProducer() *JasperLoggerOptions { return &JasperLoggerOptions{} }

func (opts *JasperLoggerOptions) validate() error {
	if opts.Name == "" {
		opts.Name = "jasper"
	}
	if opts.Level.Threshold == 0 && opts.Level.Default == 0 {
		opts.Level = send.LevelInfo{Default: level.Trace, Threshold: level.Trace}
	}

	return opts.BuildloggerV3.validate()
}

func (opts *JasperLoggerOptions) Type() string { return "BuildloggerV3" }

func (opts *JasperLoggerOptions) Configure() (send.Sender, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return NewLogger(opts.Name, opts.Level, &opts.BuildloggerV3)
}
