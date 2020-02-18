package options

import (
	"errors"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
)

// LogType is a type for representing various logging options.
// See the documentation for grip/send for more information on the various
// LogType's.
type LogType string

const (
	LogBuildloggerV2 LogType = "buildloggerv2"
	LogBuildloggerV3 LogType = "buildloggerv3"
	LogDefault       LogType = "default"
	LogFile          LogType = "file"
	LogInherit       LogType = "inherit"
	LogSplunk        LogType = "splunk"
	LogSumologic     LogType = "sumologic"
	LogInMemory      LogType = "inmemory"
)

// Validate ensures that the LogType is valid.
func (l LogType) Validate() error {
	switch l {
	case LogBuildloggerV2, LogBuildloggerV3, LogDefault, LogFile, LogInherit, LogSplunk, LogSumologic, LogInMemory:
		return nil
	default:
		return errors.New("unknown log type")
	}
}

const (
	// DefaultLogName is the default name for logs emitted by Jasper.
	DefaultLogName = "jasper"
)

// LogFormat specifies a certain format for logging by Jasper.
// See the documentation for grip/send for more information on the various
// LogFormat's.
type LogFormat string

const (
	LogFormatPlain   LogFormat = "plain"
	LogFormatDefault LogFormat = "default"
	LogFormatJSON    LogFormat = "json"
	LogFormatInvalid LogFormat = "invalid"
)

// Validate ensures that the LogFormat is valid.
func (f LogFormat) Validate() error {
	switch f {
	case LogFormatDefault, LogFormatJSON, LogFormatPlain:
		return nil
	case LogFormatInvalid:
		return errors.New("invalid log format")
	default:
		return errors.New("unknown log format")
	}
}

// MakeFormatter creates a grip/send.MessageFormatter for the specified
// LogFormat on which it is called.
func (f LogFormat) MakeFormatter() (send.MessageFormatter, error) {
	switch f {
	case LogFormatDefault:
		return send.MakeDefaultFormatter(), nil
	case LogFormatPlain:
		return send.MakePlainFormatter(), nil
	case LogFormatJSON:
		return send.MakeJSONFormatter(), nil
	case LogFormatInvalid:
		return nil, errors.New("cannot make log format for invalid format")
	default:
		return nil, errors.New("unknown log format")
	}
}

// Buffer packages options for whether or not a Logger should be
// buffered and the duration and size of the respective buffer in the case that
// it should be.
type Buffer struct {
	Buffered bool          `bson:"buffered" json:"buffered" yaml:"buffered"`
	Duration time.Duration `bson:"duration" json:"duration" yaml:"duration"`
	MaxSize  int           `bson:"max_size" json:"max_size" yaml:"max_size"`
}

// Validate ensures that BufferOptions is valid.
func (opts Buffer) Validate() error {
	if opts.Buffered && opts.Duration < 0 || opts.MaxSize < 0 {
		return errors.New("cannot have negative buffer duration or size")
	}
	return nil
}

// Log contains options related to the logging done by Jasper.
//
// By default, logger reads from both standard output and standard error.
type Log struct {
	BufferOptions        Buffer                    `json:"buffer_options,omitempty" bson:"buffer_options"`
	BuildloggerOptions   send.BuildloggerConfig    `json:"buildlogger_options,omitempty" bson:"buildlogger_options"`
	BuildloggerV3Options timber.LoggerOptions      `json:"buildlogger_v3_options" bson:"buildlogger_v_3_options"`
	DefaultPrefix        string                    `json:"default_prefix,omitempty" bson:"default_prefix"`
	FileName             string                    `json:"file_name,omitempty" bson:"file_name"`
	Format               LogFormat                 `json:"format" bson:"format"`
	InMemoryCap          int                       `json:"in_memory_cap,omitempty" bson:"in_memory_cap"`
	Level                send.LevelInfo            `json:"level,omitempty" bson:"level"`
	SplunkOptions        send.SplunkConnectionInfo `json:"splunk_options,omitempty" bson:"splunk_options"`
	SumoEndpoint         string                    `json:"sumo_endpoint,omitempty" bson:"sumo_endpoint"`
}

// Validate ensures that LogOptions is valid.
func (opts *Log) Validate() error {
	catcher := grip.NewBasicCatcher()
	if opts.Level.Threshold == 0 && opts.Level.Default == 0 {
		opts.Level = send.LevelInfo{Default: level.Trace, Threshold: level.Trace}
	}
	if !opts.Level.Valid() {
		catcher.New("invalid log level")
	}
	catcher.Wrap(opts.BufferOptions.Validate(), "invalid buffering options")
	catcher.Wrap(opts.Format.Validate(), "invalid log format")
	return catcher.Resolve()
}

// Logger is a wrapper around a grip/send.Sender. It is not thread-safe.
type Logger struct {
	Type    LogType `bson:"log_type" json:"log_type" yaml:"log_type"`
	Options Log     `bson:"log_options" json:"log_options" yaml:"log_options"`

	// sender may be the actual send.Sender or the wrapping send.Sender.
	sender send.Sender
	// baseSender stores the underlying send.Senders. It must be stored to work
	// around the fact that wrapping Senders do not necessarily close their
	// underlying send.Sender, but we have to clean up the resources when the
	// Logger is finished.
	baseSender send.Sender
}

// Validate ensures that LogOptions is valid.
func (l Logger) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(l.Type.Validate())
	catcher.Add(l.Options.Validate())
	return catcher.Resolve()
}

// Configure will configure the grip/send.Sender used by the Logger to use the
// specified LogType as specified in Logger.Type. If the error is nil, callers
// are expected to call Close for the Logger once complete.
func (l *Logger) Configure() (send.Sender, error) { //nolint: gocognit
	if l.sender != nil {
		return l.sender, nil
	}

	var sender send.Sender
	var err error

	if l.Options.Level.Threshold == 0 && l.Options.Level.Default == 0 {
		l.Options.Level.Threshold = level.Trace
		l.Options.Level.Default = level.Trace
	}

	switch l.Type {
	case LogBuildloggerV2:
		if l.Options.BuildloggerOptions.Local == nil {
			l.Options.BuildloggerOptions.Local = send.MakeNative()
		}
		if l.Options.BuildloggerOptions.Local.Name() == "" {
			l.Options.BuildloggerOptions.Local.SetName(DefaultLogName)
		}
		sender, err = send.NewBuildlogger(DefaultLogName, &l.Options.BuildloggerOptions, l.Options.Level)
		if err != nil {
			return nil, err
		}
	case LogDefault:
		if l.Options.DefaultPrefix == "" {
			l.Options.DefaultPrefix = DefaultLogName
		}
		sender, err = send.NewNativeLogger(l.Options.DefaultPrefix, l.Options.Level)
		if err != nil {
			return nil, err
		}
	case LogFile:
		sender, err = send.NewPlainFileLogger(DefaultLogName, l.Options.FileName, l.Options.Level)
		if err != nil {
			return nil, err
		}
	case LogInherit:
		sender = grip.GetSender()
		if err = sender.SetLevel(l.Options.Level); err != nil {
			return nil, err
		}
	case LogSplunk:
		if !l.Options.SplunkOptions.Populated() {
			return nil, errors.New("missing connection info for output type splunk")
		}
		sender, err = send.NewSplunkLogger(DefaultLogName, l.Options.SplunkOptions, l.Options.Level)
		if err != nil {
			return nil, err
		}
	case LogSumologic:
		if l.Options.SumoEndpoint == "" {
			return nil, errors.New("missing endpoint for output type sumologic")
		}
		sender, err = send.NewSumo(DefaultLogName, l.Options.SumoEndpoint)
		if err != nil {
			return nil, err
		}
		if err = sender.SetLevel(l.Options.Level); err != nil {
			return nil, err
		}
	case LogInMemory:
		if l.Options.InMemoryCap <= 0 {
			return nil, errors.New("invalid inmemory capacity")
		}
		sender, err = send.NewInMemorySender(DefaultLogName, l.Options.Level, l.Options.InMemoryCap)
		if err != nil {
			return nil, err
		}
	case LogBuildloggerV3:
		sender, err = timber.NewLogger(DefaultLogName, l.Options.Level, &l.Options.BuildloggerV3Options)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown log type")
	}

	formatter, err := l.Options.Format.MakeFormatter()
	if err != nil {
		return nil, err
	}
	if err := sender.SetFormatter(formatter); err != nil {
		return nil, errors.New("failed to set log format")
	}

	baseSender := sender

	if l.Type != LogBuildloggerV3 && l.Options.BufferOptions.Buffered {
		if l.Options.BufferOptions.Duration < 0 || l.Options.BufferOptions.MaxSize < 0 {
			return nil, errors.New("buffer options cannot be negative")
		}
		sender = send.NewBufferedSender(sender, l.Options.BufferOptions.Duration, l.Options.BufferOptions.MaxSize)
	}

	l.sender = sender
	l.baseSender = baseSender

	return l.sender, nil
}

// Close closes its send.Senders, closing the wrapper send.Sender and then the
// underlying base send.Sender. This should be called once the Logger is
// finished logging.
func (l *Logger) Close() error {
	catcher := grip.NewBasicCatcher()
	if l.sender != nil {
		catcher.Wrap(l.sender.Close(), "could not close sender")
	}
	if l.baseSender != nil && l.sender != l.baseSender {
		catcher.Wrap(l.baseSender.Close(), "could not close underlying sender")
	}
	return catcher.Resolve()
}
