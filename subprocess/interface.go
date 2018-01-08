package subprocess

import (
	"context"
	"errors"
	"io"
	"io/ioutil"

	"github.com/mongodb/grip"
)

// Command provides a common interface for the three methods of
// managing commands in evergreen
type Command interface {
	// Run Provides a wrapper around start and wait, that provides
	// a blocking operation, with the ability to cancel the operation
	// via the context.
	Run(context.Context) error

	Start(context.Context) error
	Wait() error

	Stop() error
	GetPid() int
	SetOutput(OutputOptions) error
}

// OutputOptions provides a common way to define and represent the
// output behavior of a evergreen/subprocess.Command operation.
type OutputOptions struct {
	Output            io.Writer
	Error             io.Writer
	SuppressOutput    bool
	SuppressError     bool
	SendOutputToError bool
	SendErrorToOutput bool
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

func (o OutputOptions) errorIsNull() bool {
	if o.Error == nil {
		return true
	}

	if o.Error == ioutil.Discard {
		return true
	}

	return false
}

func (o OutputOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	if o.SuppressOutput && !o.outputIsNull() {
		catcher.Add(errors.New("cannot suppress output if output is defined"))
	}

	if o.SuppressError && !o.errorIsNull() {
		catcher.Add(errors.New("cannot suppress error if error is defined"))
	}

	if o.Error == o.Output && !o.errorIsNull() {
		catcher.Add(errors.New("cannot specify the same value for error and output"))
	}

	if o.SuppressOutput && o.SendOutputToError {
		catcher.Add(errors.New("cannot suppress output and redirect it to error"))
	}

	if o.SuppressError && o.SendErrorToOutput {
		catcher.Add(errors.New("cannot suppress output and redirect it to error"))
	}

	if o.SendOutputToError && o.Error == nil {
		catcher.Add(errors.New("cannot redirect output to error without a defined error writer"))
	}

	if o.SendErrorToOutput && o.Output == nil {
		catcher.Add(errors.New("cannot redirect error to output without a defined output writer"))
	}

	return catcher.Resolve()
}

func (o OutputOptions) GetOutput() io.Writer {
	if o.outputIsNull() {
		return ioutil.Discard
	}

	return o.Output
}

func (o OutputOptions) GetError() io.Writer {
	if o.errorIsNull() {
		return ioutil.Discard
	}

	return o.Error
}
