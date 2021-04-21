package options

import (
	"io"
	"io/ioutil"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/util"
)

// Output provides a common way to define and represent the
// output behavior of a evergreen/subprocess.Command operation.
type Output struct {
	Output            io.Writer `bson:"-" json:"-" yaml:"-"`
	Error             io.Writer `bson:"-" json:"-" yaml:"-"`
	SuppressOutput    bool      `bson:"suppress_output" json:"suppress_output" yaml:"suppress_output"`
	SuppressError     bool      `bson:"suppress_error" json:"suppress_error" yaml:"suppress_error"`
	SendOutputToError bool      `bson:"redirect_output_to_error" json:"redirect_output_to_error" yaml:"redirect_output_to_error"`
	SendErrorToOutput bool      `bson:"redirect_error_to_output" json:"redirect_error_to_output" yaml:"redirect_error_to_output"`
	// Loggers are self-contained and specific to the process they are attached
	// to. They are closed and cleaned up when the process exits. If this
	// behavior is not desired, use Output instead of Loggers.
	Loggers []*LoggerConfig `bson:"loggers" json:"loggers,omitempty" yaml:"loggers"`

	outputSender *send.WriterSender
	errorSender  *send.WriterSender
	outputMulti  io.Writer
	errorMulti   io.Writer
}

func (o Output) outputIsNull() bool {
	if o.Output == nil {
		return true
	}

	if o.Output == ioutil.Discard {
		return true
	}

	return false
}

func (o Output) outputLogging() bool {
	return len(o.Loggers) > 0 && !o.SuppressOutput
}

func (o Output) errorLogging() bool {
	return len(o.Loggers) > 0 && !o.SuppressError
}

func (o Output) errorIsNull() bool {
	if o.Error == nil {
		return true
	}

	if o.Error == ioutil.Discard {
		return true
	}

	return false
}

// Validate ensures that the Output it is called on has reasonable
// values.
func (o *Output) Validate() error {
	catcher := grip.NewBasicCatcher()

	if o.SuppressOutput && (!o.outputIsNull() || o.outputLogging()) {
		catcher.New("cannot suppress output if output is defined")
	}

	if o.SuppressError && (!o.errorIsNull() || o.errorLogging()) {
		catcher.New("cannot suppress error if error is defined")
	}

	if o.SuppressOutput && o.SendOutputToError {
		catcher.New("cannot suppress output and redirect it to error")
	}

	if o.SuppressError && o.SendErrorToOutput {
		catcher.New("cannot suppress error and redirect it to output")
	}

	if o.SendOutputToError && o.errorIsNull() && !o.errorLogging() {
		catcher.New("cannot redirect output to error without a defined error writer")
	}

	if o.SendErrorToOutput && o.outputIsNull() && !o.outputLogging() {
		catcher.New("cannot redirect error to output without a defined output writer")
	}

	if o.SendOutputToError && o.SendErrorToOutput {
		catcher.New("cannot create redirect cycle between output and error")
	}

	for _, l := range o.Loggers {
		catcher.Wrap(l.validate(), "invalid logger")
	}

	return catcher.Resolve()
}

// GetOutput returns a Writer that has the stdout output from the process that
// the Output that this method is called on is attached to. The caller is
// responsible for calling Close when the loggers are not needed anymore.
func (o *Output) GetOutput() (io.Writer, error) {
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
		outLoggers := []send.Sender{}

		for i := range o.Loggers {
			sender, err := o.Loggers[i].Resolve()
			if err != nil {
				return ioutil.Discard, err
			}
			outLoggers = append(outLoggers, sender)
		}

		var outMulti send.Sender
		if len(outLoggers) == 1 {
			outMulti = outLoggers[0]
		} else {
			var err error
			outMulti, err = send.NewMultiSender(DefaultLogName, send.LevelInfo{Default: level.Info, Threshold: level.Trace}, outLoggers)
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
func (o *Output) GetError() (io.Writer, error) {
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
			sender, err := o.Loggers[i].Resolve()
			if err != nil {
				return ioutil.Discard, err
			}
			errSenders = append(errSenders, sender)
		}

		errMulti, err := send.NewMultiSender(DefaultLogName, send.LevelInfo{Default: level.Error, Threshold: level.Trace}, errSenders)
		if err != nil {
			return ioutil.Discard, err
		}
		// This will not close the Loggers' underlying senders.
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
func (o *Output) Copy() *Output {
	optsCopy := *o

	optsCopy.outputSender = nil
	optsCopy.errorSender = nil
	optsCopy.outputMulti = nil
	optsCopy.errorMulti = nil

	if o.Loggers != nil {
		optsCopy.Loggers = make([]*LoggerConfig, len(o.Loggers))
		_ = copy(optsCopy.Loggers, o.Loggers)
	}

	return &optsCopy
}

// Close calls all of the processes' output senders' Close method.
func (o *Output) Close() error {
	catcher := grip.NewBasicCatcher()
	// Close the outputSender and errorSender, which does not close the
	// underlying send.Sender.
	if o.outputSender != nil {
		catcher.Wrap(o.outputSender.Close(), "problem closing output sender")
	}
	if o.errorSender != nil {
		catcher.Wrap(o.errorSender.Close(), "problem closing error sender")
	}
	// Close the sender wrapped by the send.WriterSender.
	if o.outputSender != nil {
		catcher.Wrap(o.outputSender.Sender.Close(), "problem closing wrapped output sender")
	}
	// Since senders are shared, only close error's senders if output hasn't
	// already closed them.
	if o.errorSender != nil && (o.SuppressOutput || o.SendOutputToError) {
		catcher.Wrap(o.errorSender.Sender.Close(), "problem closing wrapped error sender")
	}

	return catcher.Resolve()
}

// CachedLogger returns a cached logger with the given ID with its error and
// output derived from Output.
func (o *Output) CachedLogger(id string) *CachedLogger {
	return &CachedLogger{
		ID:       id,
		Accessed: time.Now(),
		Error:    util.ConvertWriter(o.GetError()),
		Output:   util.ConvertWriter(o.GetOutput()),
	}
}
