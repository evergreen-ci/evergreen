package jasper

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/mongodb/grip/send"
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

func yesCreateOpts(timeout time.Duration) CreateOptions {
	return CreateOptions{Args: []string{"yes"}, Timeout: timeout}
}

func procMap() map[string]func(context.Context, *CreateOptions) (Process, error) {
	return map[string]func(context.Context, *CreateOptions) (Process, error){
		"Basic":    newBasicProcess,
		"Blocking": newBlockingProcess,
	}
}

func runIteration(ctx context.Context, makeProc func(context.Context, *CreateOptions) (Process, error), opts *CreateOptions) error {
	proc, err := makeProc(ctx, opts)
	if err != nil {
		return err
	}
	_, _ = proc.Wait(ctx)
	return nil
}

func makeCreateOpts(timeout time.Duration, logger Logger) *CreateOptions {
	opts := yesCreateOpts(timeout)
	opts.Output.Loggers = []Logger{logger}
	return &opts
}

func inMemoryLoggerCase(ctx context.Context, c *caseDefinition) result {
	var logType LogType = LogInMemory
	logOptions := LogOptions{InMemoryCap: 1000}
	res := result{duration: c.timeout}
	size := make(chan int64)
	opts := makeCreateOpts(c.timeout, Logger{Type: logType, Options: logOptions})
	opts.closers = append(opts.closers, func() (_ error) {
		logger := opts.Output.outputSender.Sender.(*send.InMemorySender)
		size <- logger.TotalBytesSent()
		return
	})

	err := runIteration(ctx, c.procMaker, opts)
	if err != nil {
		return result{err: err}
	}
	res.size += float64(<-size)

	return res
}

func fileLoggerCase(ctx context.Context, c *caseDefinition) result {
	res := result{duration: c.timeout}

	var logType LogType = LogFile
	file, err := ioutil.TempFile("build", "bench_out.txt")
	if err != nil {
		return result{err: err}
	}
	defer os.Remove(file.Name())
	logOptions := LogOptions{FileName: file.Name(), Format: LogFormatPlain}

	opts := makeCreateOpts(c.timeout, Logger{Type: logType, Options: logOptions})

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
