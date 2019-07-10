package jasper

import (
	"context"
	"io"
	"io/ioutil"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// OutputOptions provides a common way to define and represent the
// output behavior of a evergreen/subprocess.Command operation.
type OutputOptions struct {
	Output            io.Writer `bson:"-" json:"-" yaml:"-"`
	Error             io.Writer `bson:"-" json:"-" yaml:"-"`
	SuppressOutput    bool      `bson:"suppress_output" json:"suppress_output" yaml:"suppress_output"`
	SuppressError     bool      `bson:"suppress_error" json:"suppress_error" yaml:"suppress_error"`
	SendOutputToError bool      `bson:"redirect_output_to_error" json:"redirect_output_to_error" yaml:"redirect_output_to_error"`
	SendErrorToOutput bool      `bson:"redirect_error_to_output" json:"redirect_error_to_output" yaml:"redirect_error_to_output"`
	Loggers           []Logger  `bson:"loggers" json:"loggers" yaml:"loggers"`

	outputSender *send.WriterSender
	errorSender  *send.WriterSender
	outputMulti  io.Writer
	errorMulti   io.Writer
}

// LogType is a type for representing various logging options.
// See the documentation for grip/send for more information on the various
// LogType's.
type LogType string

const (
	LogBuildloggerV2 = "buildloggerv2" // nolint
	LogBuildloggerV3 = "buildloggerv3" // nolint
	LogDefault       = "default"       // nolint
	LogFile          = "file"          // nolint
	LogInherit       = "inherit"       // nolint
	LogSplunk        = "splunk"        // nolint
	LogSumologic     = "sumologic"     // nolint
	LogInMemory      = "inmemory"      // nolint
)

const (
	// DefaultLogName is the default name for logs emitted by Jasper.
	DefaultLogName = "jasper"
)

// LogFormat specifies a certain format for logging by Jasper.
// See the documentation for grip/send for more information on the various
// LogFormat's.
type LogFormat string

const (
	// LogFormatPlain refers to the plain logging format (no formatting).
	LogFormatPlain = "plain"
	// LogFormatDefault refers to the default logging format.
	LogFormatDefault = "default"
	// LogFormatJSON refers to the JSON logging format.
	LogFormatJSON = "json"
	// LogFormatInvalid refers to an invalid logging format. This should not be
	// used.
	LogFormatInvalid = "invalid"
)

// LogOptions contains options related to the logging done by Jasper.
//
// By default, logger reads from both standard output and standard error.
type LogOptions struct {
	BufferOptions      BufferOptions             `json:"buffer_options"`
	BuildloggerOptions send.BuildloggerConfig    `json:"buildlogger_options"`
	DefaultPrefix      string                    `json:"default_prefix"`
	FileName           string                    `json:"file_name"`
	Format             LogFormat                 `json:"format"`
	InMemoryCap        int                       `json:"in_memory_cap"`
	SplunkOptions      send.SplunkConnectionInfo `json:"splunk_options"`
	SumoEndpoint       string                    `json:"sumo_endpoint"`
}

// Validate ensures that BufferOptions is valid.
func (opts BufferOptions) Validate() error {
	if opts.Buffered && opts.Duration < 0 || opts.MaxSize < 0 {
		return errors.New("cannot have negative buffer duration or size")
	}
	return nil
}

// Validate ensures that LogOptions is valid.
func (opts LogOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.Wrap(opts.BufferOptions.Validate(), "invalid buffering options")
	catcher.Wrap(opts.Format.Validate(), "invalid log format")
	return catcher.Resolve()
}

// Logger is a wrapper struct around a grip/send.Sender.
type Logger struct {
	Type    LogType    `bson:"log_type" json:"log_type" yaml:"log_type"`
	Options LogOptions `bson:"log_options" json:"log_options" yaml:"log_options"`

	sender send.Sender
}

// NewInMemoryLogger is a basic constructor that constructs a logger
// configuration for plain formatted in-memory buffered logger. The
// logger will capture up to maxSize messages.
func NewInMemoryLogger(maxSize int) Logger {
	return Logger{
		Type: LogInMemory,
		Options: LogOptions{
			Format:      LogFormatPlain,
			InMemoryCap: maxSize,
		},
	}

}

// Validate ensures that LogOptions is valid.
func (l Logger) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(l.Type.Validate())
	catcher.Add(l.Options.Validate())
	return catcher.Resolve()
}

// Validate ensures that the LogType is valid.
func (l LogType) Validate() error {
	switch l {
	case LogBuildloggerV2, LogBuildloggerV3, LogDefault, LogFile, LogInherit, LogSplunk, LogSumologic, LogInMemory:
		return nil
	default:
		return errors.New("unknown log type")
	}
}

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

// Configure will configure the grip/send.Sender used by the Logger to use the
// specified LogType as specified in Logger.Type.
func (l *Logger) Configure() (send.Sender, error) {
	if l.sender != nil {
		return l.sender, nil
	}

	var sender send.Sender
	var err error

	switch l.Type {
	case LogBuildloggerV2, LogBuildloggerV3:
		if l.Options.BuildloggerOptions.Local == nil {
			l.Options.BuildloggerOptions.Local = send.MakeNative()
		}
		if l.Options.BuildloggerOptions.Local.Name() == "" {
			l.Options.BuildloggerOptions.Local.SetName(DefaultLogName)
		}
		sender, err = send.MakeBuildlogger(DefaultLogName, &l.Options.BuildloggerOptions)
		if err != nil {
			return nil, err
		}
	case LogDefault:
		if l.Options.DefaultPrefix == "" {
			l.Options.DefaultPrefix = DefaultLogName
		}
		sender, err = send.NewNativeLogger(l.Options.DefaultPrefix, send.LevelInfo{Default: level.Trace, Threshold: level.Trace})
		if err != nil {
			return nil, err
		}
	case LogFile:
		sender, err = send.MakePlainFileLogger(l.Options.FileName)
		if err != nil {
			return nil, err
		}
		sender.SetName(DefaultLogName)
	case LogInherit:
		sender = grip.GetSender()
	case LogSplunk:
		if !l.Options.SplunkOptions.Populated() {
			return nil, errors.New("missing connection info for output type splunk")
		}
		sender, err = send.NewSplunkLogger(DefaultLogName, l.Options.SplunkOptions, send.LevelInfo{Default: level.Trace, Threshold: level.Trace})
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
	case LogInMemory:
		if l.Options.InMemoryCap <= 0 {
			return nil, errors.New("invalid inmemory capacity")
		}
		sender, err = send.NewInMemorySender(DefaultLogName, send.LevelInfo{Default: level.Trace, Threshold: level.Trace}, l.Options.InMemoryCap)
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

	if l.Options.BufferOptions.Buffered {
		if l.Options.BufferOptions.Duration < 0 || l.Options.BufferOptions.MaxSize < 0 {
			return nil, errors.New("buffer options cannot be negative")
		}
		sender = send.NewBufferedSender(sender, l.Options.BufferOptions.Duration, l.Options.BufferOptions.MaxSize)
	}

	l.sender = sender

	return l.sender, nil
}

// LogStream represents the output of reading the in-memory log buffer as a
// stream, containing the logs (if any) and whether or not the stream is done
// reading.
type LogStream struct {
	Logs []string `json:"logs,omitempty"`
	Done bool     `json:"done"`
}

// GetInMemoryLogStream gets at most count logs from the in-memory output logs
// for the given Process proc. If the process has not been called with
// Process.Wait(), this is not guaranteed to produce all the logs. This function
// assumes that there is exactly one in-memory logger attached to this process's
// output. It returns io.EOF if the stream is done.
func GetInMemoryLogStream(ctx context.Context, proc Process, count int) ([]string, error) {
	if proc == nil {
		return nil, errors.New("cannot get output logs from nil process")
	}
	for _, logger := range proc.Info(ctx).Options.Output.Loggers {
		if logger.Type != LogInMemory {
			continue
		}

		inMemorySender, ok := logger.sender.(*send.InMemorySender)
		if !ok {
			continue
		}

		msgs, _, err := inMemorySender.GetCount(count)
		if err != nil {
			if err != io.EOF {
				err = errors.Wrap(err, "failed to get logs from in-memory stream")
			}
			return nil, err
		}

		strs := make([]string, 0, len(msgs))
		for _, msg := range msgs {
			str, err := inMemorySender.Formatter(msg)
			if err != nil {
				return nil, err
			}
			strs = append(strs, str)
		}

		return strs, nil
	}
	return nil, errors.New("could not find in-memory output logs")
}

// BufferOptions packages options for whether or not a Logger should be
// buffered and the duration and size of the respective buffer in the case that
// it should be.
type BufferOptions struct {
	Buffered bool          `bson:"buffered" json:"buffered" yaml:"buffered"`
	Duration time.Duration `bson:"duration" json:"duration" yaml:"duration"`
	MaxSize  int           `bson:"max_size" json:"max_size" yaml:"max_size"`
}

func (o OutputOptions) outputIsNull() bool {
	if o.Output == nil {
		return true
	}

	if o.Output == ioutil.Discard {
		return true
	}

	return false
}

func (o OutputOptions) outputLogging() bool {
	return len(o.Loggers) > 0 && !o.SuppressOutput
}

func (o OutputOptions) errorLogging() bool {
	return len(o.Loggers) > 0 && !o.SuppressError
}

func (o OutputOptions) errorIsNull() bool {
	if o.Error == nil {
		return true
	}

	if o.Error == ioutil.Discard {
		return true
	}

	return false
}

// Validate ensures that the OutputOptions it is called on has reasonable
// values.
func (o OutputOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	if o.SuppressOutput && (!o.outputIsNull() || o.outputLogging()) {
		catcher.Add(errors.New("cannot suppress output if output is defined"))
	}

	if o.SuppressError && (!o.errorIsNull() || o.errorLogging()) {
		catcher.Add(errors.New("cannot suppress error if error is defined"))
	}

	if o.SuppressOutput && o.SendOutputToError {
		catcher.Add(errors.New("cannot suppress output and redirect it to error"))
	}

	if o.SuppressError && o.SendErrorToOutput {
		catcher.Add(errors.New("cannot suppress error and redirect it to output"))
	}

	if o.SendOutputToError && o.Error == nil && !o.errorLogging() {
		catcher.Add(errors.New("cannot redirect output to error without a defined error writer"))
	}

	if o.SendErrorToOutput && o.Output == nil && !o.outputLogging() {
		catcher.Add(errors.New("cannot redirect error to output without a defined output writer"))
	}

	if o.SendOutputToError && o.SendErrorToOutput {
		catcher.Add(errors.New("cannot create redirect cycle between output and error"))
	}

	for _, logger := range o.Loggers {
		catcher.Wrap(logger.Validate(), "invalid logger")
	}

	return catcher.Resolve()
}

// GetOutput returns a Writer that has the stdout output from the process that the
// OutputOptions that this method is called on is attached to.
func (o *OutputOptions) GetOutput() (io.Writer, error) {
	if o.SendOutputToError {
		return o.GetError()
	}

	if o.outputIsNull() && !o.outputLogging() {
		return ioutil.Discard, nil
	}

	if o.outputMulti != nil {
		return o.outputMulti, nil
	}

	if o.outputLogging() {
		outSenders := []send.Sender{}

		for i := range o.Loggers {
			sender, err := o.Loggers[i].Configure()
			if err != nil {
				return ioutil.Discard, err
			}
			outSenders = append(outSenders, sender)
		}

		var outMulti send.Sender
		if len(outSenders) == 1 {
			outMulti = outSenders[0]
		} else {
			var err error
			outMulti, err = send.NewMultiSender(DefaultLogName, send.LevelInfo{Default: level.Info, Threshold: level.Trace}, outSenders)
			if err != nil {
				return ioutil.Discard, err
			}
		}
		o.outputSender = send.NewWriterSender(outMulti)
	}

	if !o.outputIsNull() && o.outputLogging() {
		o.outputMulti = io.MultiWriter(o.Output, o.outputSender)
	} else if !o.outputIsNull() {
		o.outputMulti = o.Output
	} else {
		o.outputMulti = o.outputSender
	}

	return o.outputMulti, nil
}

// GetError returns an io.Writer that can be used for standard error, depending on
// the output configuration.
func (o *OutputOptions) GetError() (io.Writer, error) {
	if o.SendErrorToOutput {
		return o.GetOutput()
	}

	if o.errorIsNull() && !o.errorLogging() {
		return ioutil.Discard, nil
	}

	if o.errorMulti != nil {
		return o.errorMulti, nil
	}

	if o.errorLogging() {
		errSenders := []send.Sender{}

		for i := range o.Loggers {
			sender, err := o.Loggers[i].Configure()
			if err != nil {
				return ioutil.Discard, err
			}
			errSenders = append(errSenders, sender)
		}

		errMulti, err := send.NewMultiSender(DefaultLogName, send.LevelInfo{Default: level.Error, Threshold: level.Trace}, errSenders)
		if err != nil {
			return ioutil.Discard, err
		}
		o.errorSender = send.NewWriterSender(errMulti)
	}

	if !o.errorIsNull() && o.errorLogging() {
		o.errorMulti = io.MultiWriter(o.Error, o.errorSender)
	} else if !o.errorIsNull() {
		o.errorMulti = o.Error
	} else {
		o.errorMulti = o.errorSender
	}

	return o.errorMulti, nil
}

// Copy returns a copy of the options for only the exported fields. Unexported
// fields are cleared.
func (o *OutputOptions) Copy() *OutputOptions {
	optsCopy := *o

	optsCopy.outputSender = nil
	optsCopy.errorSender = nil
	optsCopy.outputMulti = nil
	optsCopy.errorMulti = nil

	if o.Loggers != nil {
		optsCopy.Loggers = make([]Logger, len(o.Loggers))
		_ = copy(optsCopy.Loggers, o.Loggers)
	}

	return &optsCopy
}
