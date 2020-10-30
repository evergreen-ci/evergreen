package options

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

///////////////////////////////////////////////////////////////////////////////
// Default Logger
///////////////////////////////////////////////////////////////////////////////

// LogDefault is the name for the default logger.
const LogDefault = "default"

// DefaultLoggerOptions encapsulates the options for creating a logger that logs
// to the native standard streams (i.e. stdout, stderr).
type DefaultLoggerOptions struct {
	Prefix string      `json:"prefix" bson:"prefix"`
	Base   BaseOptions `json:"base" bson:"base"`
}

// NewDefaultLoggerProducer returns a LoggerProducer backed by
// DefaultLoggerOptions.
func NewDefaultLoggerProducer() LoggerProducer { return &DefaultLoggerOptions{} }

// Validate checks that a default prefix is specified and that the common base
// options are valid.
func (opts *DefaultLoggerOptions) Validate() error {
	if opts.Prefix == "" {
		opts.Prefix = DefaultLogName
	}

	return opts.Base.Validate()
}

// Type returns the log type for default loggers.
func (*DefaultLoggerOptions) Type() string { return LogDefault }

// Configure returns a send.Sender based on the default logging options.
func (opts *DefaultLoggerOptions) Configure() (send.Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	sender, err := send.NewNativeLogger(opts.Prefix, opts.Base.Level)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating base default logger")
	}

	sender, err = NewSafeSender(sender, opts.Base)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating safe default logger")
	}
	return sender, nil
}

///////////////////////////////////////////////////////////////////////////////
// File Logger
///////////////////////////////////////////////////////////////////////////////

// LogFile is the name for the file logger.
const LogFile = "file"

// FileLoggerOptions encapsulates the options for creating a file logger.
type FileLoggerOptions struct {
	Filename string      `json:"filename" bson:"filename"`
	Base     BaseOptions `json:"base" bson:"base"`
}

// NewFileLoggerProducer returns a LoggerProducer backed by FileLoggerOptions.
func NewFileLoggerProducer() LoggerProducer { return &FileLoggerOptions{} }

// Validate checks that the file name is given and that the common base options
// are valid.
func (opts *FileLoggerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(opts.Filename == "", "must specify a filename")
	catcher.Add(opts.Base.Validate())
	return catcher.Resolve()
}

// Type returns the log type for file loggers.
func (*FileLoggerOptions) Type() string { return LogFile }

// Configure returns a send.Sender based on the file options.
func (opts *FileLoggerOptions) Configure() (send.Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	sender, err := send.NewPlainFileLogger(DefaultLogName, opts.Filename, opts.Base.Level)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating base file logger")
	}

	sender, err = NewSafeSender(sender, opts.Base)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating safe file logger")
	}
	return sender, nil
}

///////////////////////////////////////////////////////////////////////////////
// Inherited Logger
///////////////////////////////////////////////////////////////////////////////

// LogInherited is the name for the inherited logger.
const LogInherited = "inherited"

// InheritedLoggerOptions encapsulates the options for creating a logger
// inherited from the globally-configured grip logger.
type InheritedLoggerOptions struct {
	Base BaseOptions `json:"base" bson:"base"`
}

// NewInheritedLoggerProducer returns a LoggerProducer for creating inherited
// loggers.
func NewInheritedLoggerProducer() LoggerProducer { return &InheritedLoggerOptions{} }

// Type returns the log type for inherited loggers.
func (*InheritedLoggerOptions) Type() string { return LogInherited }

// Configure returns a send.Sender based on the inherited logger options.
func (opts *InheritedLoggerOptions) Configure() (send.Sender, error) {
	var (
		sender send.Sender
		err    error
	)

	if err = opts.Base.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	sender = grip.GetSender()
	if err = sender.SetLevel(opts.Base.Level); err != nil {
		return nil, errors.Wrap(err, "problem creating base inherited logger")
	}

	sender, err = NewSafeSender(sender, opts.Base)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating safe inherited logger")
	}
	return sender, nil
}

///////////////////////////////////////////////////////////////////////////////
// In-Memory Logger
///////////////////////////////////////////////////////////////////////////////

// LogInMemory is the name for the in-memory logger.
const LogInMemory = "in-memory"

// InMemoryLoggerOptions encapsulates the options for creating an in-memory
// logger.
type InMemoryLoggerOptions struct {
	InMemoryCap int         `json:"in_memory_cap" bson:"in_memory_cap"`
	Base        BaseOptions `json:"base" bson:"base"`
}

// NewInMemoryLoggerProducer returns a LoggerProducer for creating in-memory
// loggers.
func NewInMemoryLoggerProducer() LoggerProducer { return &InMemoryLoggerOptions{} }

// Validate checks that the in-memory capacity is specified and that the common
// base options are valid.
func (opts *InMemoryLoggerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(opts.InMemoryCap <= 0, "invalid in-memory capacity")
	catcher.Add(opts.Base.Validate())
	return catcher.Resolve()
}

// Type returns the log type for in-memory loggers.
func (*InMemoryLoggerOptions) Type() string { return LogInMemory }

// Configure returns a send.Sender based on the in-memory logger options.
func (opts *InMemoryLoggerOptions) Configure() (send.Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	sender, err := send.NewInMemorySender(DefaultLogName, opts.Base.Level, opts.InMemoryCap)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating base in-memory logger")
	}

	sender, err = NewSafeSender(sender, opts.Base)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating safe in-memory logger")
	}
	return sender, nil
}

///////////////////////////////////////////////////////////////////////////////
// Sumo Logic Logger
///////////////////////////////////////////////////////////////////////////////

// LogSumoLogic is the name for the Sumo Logic logger.
const LogSumoLogic = "sumo-logic"

// SumoLogicLoggerOptions encapsulates the options for creating a Sumo Logic
// logger.
type SumoLogicLoggerOptions struct {
	SumoEndpoint string      `json:"sumo_endpoint" bson:"sumo_endpoint"`
	Base         BaseOptions `json:"base" bson:"base"`
}

// NewSumoLogicLoggerProducer returns a LoggerProducer for creating Sumo Logic
// loggers.
func NewSumoLogicLoggerProducer() LoggerProducer { return &SumoLogicLoggerOptions{} }

// Validate checks that the Sumo Log endpoint is specified and that the common
// base options are valid.
func (opts *SumoLogicLoggerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(opts.SumoEndpoint == "", "must specify a sumo endpoint")
	catcher.Add(opts.Base.Validate())
	return catcher.Resolve()
}

// Type returns the log type for Sumo Logic.
func (*SumoLogicLoggerOptions) Type() string { return LogSumoLogic }

// Configure returns a send.Sender based on the Sumo Logic options.
func (opts *SumoLogicLoggerOptions) Configure() (send.Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	sender, err := send.NewSumo(DefaultLogName, opts.SumoEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating base sumo logic logger")
	}
	if err = sender.SetLevel(opts.Base.Level); err != nil {
		return nil, errors.Wrap(err, "problem setting level for sumo logic logger")
	}

	sender, err = NewSafeSender(sender, opts.Base)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating safe sumo logic logger")
	}
	return sender, nil
}

///////////////////////////////////////////////////////////////////////////////
// Splunk Logger
///////////////////////////////////////////////////////////////////////////////

// LogSplunk is the name for the Splunk logger.
const LogSplunk = "splunk"

// SplunkLoggerOptions encapsulates the options for creating a Splunk logger.
type SplunkLoggerOptions struct {
	Splunk send.SplunkConnectionInfo `json:"splunk" bson:"splunk"`
	Base   BaseOptions               `json:"base" bson:"base"`
}

// NewSplunkLoggerProducer returns a LoggerProducer for creating Splunk loggers.
func NewSplunkLoggerProducer() LoggerProducer { return &SplunkLoggerOptions{} }

// Validate checks that the Splunk options are populated and that the common
// base options are valid.
func (opts *SplunkLoggerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(!opts.Splunk.Populated(), "missing connection info for output type splunk")
	catcher.Add(opts.Base.Validate())
	return catcher.Resolve()
}

// Type returns the log type for Splunk.
func (*SplunkLoggerOptions) Type() string { return LogSplunk }

// Configure returns a send.Sender based on the Splunk options.
func (opts *SplunkLoggerOptions) Configure() (send.Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	sender, err := send.NewSplunkLogger(DefaultLogName, opts.Splunk, opts.Base.Level)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating base splunk logger")
	}

	sender, err = NewSafeSender(sender, opts.Base)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating safe splunk logger")
	}
	return sender, nil
}

///////////////////////////////////////////////////////////////////////////////
// BuildloggerV2 Logger
///////////////////////////////////////////////////////////////////////////////

// LogBuildloggerV2 is the name for the Buildlogger v2 logger.
const LogBuildloggerV2 = "buildloggerv2"

// BuildloggerV2Options encapsulates the options for creating a Buildlogger v2
// logger.
type BuildloggerV2Options struct {
	Buildlogger send.BuildloggerConfig `json:"buildlogger" bson:"buildlogger"`
	Base        BaseOptions            `json:"base" bson:"base"`
}

// NewBuildloggerV2LoggerProducer returns a LoggerProducer for creating
// Buildlogger v2 loggers.
func NewBuildloggerV2LoggerProducer() LoggerProducer { return &BuildloggerV2Options{} }

// Type returns the log type for the Buildlogger v2.
func (*BuildloggerV2Options) Type() string { return LogBuildloggerV2 }

// Configure returns a send.Sender based on the Buildlogger v2 options.
func (opts *BuildloggerV2Options) Configure() (send.Sender, error) {
	if opts.Buildlogger.Local == nil {
		opts.Buildlogger.Local = send.MakeNative()
	}
	if opts.Buildlogger.Local.Name() == "" {
		opts.Buildlogger.Local.SetName(DefaultLogName)
	}

	sender, err := send.NewBuildlogger(DefaultLogName, &opts.Buildlogger, opts.Base.Level)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating base buildlogger logger")
	}

	sender, err = NewSafeSender(sender, opts.Base)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating safe buildlogger logger")
	}
	return sender, nil
}
