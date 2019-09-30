// +build none

package jasper

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

const (
	one             = 1
	five            = 5
	ten             = 2 * five
	thirty          = 3 * ten
	hundred         = ten * ten
	thousand        = ten * hundred
	tenThousand     = ten * thousand
	hundredThousand = hundred * thousand
	million         = hundred * hundredThousand
	halfMillion     = five * hundredThousand

	executionTimeout = five * time.Minute
	minIterations    = one
)

func yesCreateOpts(timeout time.Duration) options.Create {
	return options.Create{Args: []string{"yes"}, Timeout: timeout}
}

func procMap() map[string]func(context.Context, *options.Create) (Process, error) {
	return map[string]func(context.Context, *options.Create) (Process, error){
		"Basic":    newBasicProcess,
		"Blocking": newBlockingProcess,
	}
}

func runIteration(ctx context.Context, makeProc func(context.Context, *options.Create) (Process, error), opts *options.Create) error {
	proc, err := makeProc(ctx, opts)
	if err != nil {
		return err
	}
	exitCode, err := proc.Wait(ctx)
	if err != nil && !proc.Info(ctx).Timeout {
		return errors.Wrapf(err, "process with id '%s' exited unexpectedly with code %d", proc.ID(), exitCode)
	}
	return nil
}

func makeCreateOpts(timeout time.Duration, logger options.Logger) *options.Create {
	opts := yesCreateOpts(timeout)
	opts.Output.Loggers = []options.Logger{logger}
	return &opts
}

func inMemoryLoggerCase(ctx context.Context, c *caseDefinition) result {
	var logType options.LogType = options.LogInMemory
	logOptions := options.Log{InMemoryCap: 1000, Format: options.LogFormatPlain}
	res := result{duration: c.timeout}
	size := make(chan int64)
	opts := makeCreateOpts(c.timeout, options.Logger{Type: logType, Options: logOptions})
	opts.closers = append(opts.closers, func() (_ error) {
		defer close(size)
		logger := opts.Output.outputSender.Sender.(*send.InMemorySender)
		select {
		case size <- logger.TotalBytesSent():
		case <-ctx.Done():
		}
		return
	})

	sizeAdded := make(chan struct{})
	go func(res *result) {
		defer close(sizeAdded)
		select {
		case numBytes := <-size:
			res.size += float64(numBytes)
		case <-ctx.Done():
		}
	}(&res)

	err := runIteration(ctx, c.procMaker, opts)
	if err != nil {
		return result{err: err}
	}

	select {
	case <-sizeAdded:
	case <-ctx.Done():
	}

	return res
}

func fileLoggerCase(ctx context.Context, c *caseDefinition) result {
	res := result{duration: c.timeout}

	var logType options.LogType = options.LogFile
	file, err := ioutil.TempFile("build", "bench_out.txt")
	if err != nil {
		return result{err: err}
	}
	defer os.Remove(file.Name())
	logOptions := options.Log{FileName: file.Name(), Format: options.LogFormatPlain}

	opts := makeCreateOpts(c.timeout, options.Logger{Type: logType, Options: logOptions})

	err = runIteration(ctx, c.procMaker, opts)
	if err != nil {
		return result{err: err}
	}

	info, err := file.Stat()
	if err != nil {
		return result{err: err}
	}
	numBytes := info.Size()
	res.size += float64(numBytes)

	return res
}

func logCases() map[string]func(context.Context, *caseDefinition) result {
	return map[string]func(context.Context, *caseDefinition) result{
		"InMemoryLogger": inMemoryLoggerCase,
		"FileLogger":     fileLoggerCase,
	}
}

func getAllCases() []*caseDefinition {
	cases := make([]*caseDefinition, 0)
	for procName, makeProc := range procMap() {
		for logName, logCase := range logCases() {
			cases = append(cases,
				&caseDefinition{
					name:               fmt.Sprintf("%s/%s/Send1Second", logName, procName),
					bench:              logCase,
					procMaker:          makeProc,
					timeout:            one * time.Second,
					requiredIterations: ten,
				},
				&caseDefinition{
					name:               fmt.Sprintf("%s/%s/Send5Seconds", logName, procName),
					bench:              logCase,
					procMaker:          makeProc,
					timeout:            five * time.Second,
					requiredIterations: five,
				},
				&caseDefinition{
					name:               fmt.Sprintf("%s/%s/Send30Seconds", logName, procName),
					bench:              logCase,
					procMaker:          makeProc,
					timeout:            thirty * time.Second,
					requiredIterations: one,
				},
			)
		}
	}
	return cases
}
