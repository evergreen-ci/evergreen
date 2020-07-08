package jasper

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInMemoryLogStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for procType, makeProc := range map[string]ProcessConstructor{
		"Basic":    newBasicProcess,
		"Blocking": newBlockingProcess,
	} {
		t.Run(procType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor, output string){
				"FailsWithNilProcess": func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor, output string) {
					logs, err := GetInMemoryLogStream(ctx, nil, 1)
					assert.Error(t, err)
					assert.Nil(t, logs)
				},
				"FailsWithInvalidCount": func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor, output string) {
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 0)
					assert.Error(t, err)
					assert.Nil(t, logs)
				},
				"FailsWithoutInMemoryLogger": func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor, output string) {
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 100)
					assert.Error(t, err)
					assert.Nil(t, logs)
				},
				"SucceedsWithInMemoryLogger": func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor, output string) {
					loggerProducer := &options.InMemoryLoggerOptions{
						InMemoryCap: 100,
						Base: options.BaseOptions{
							Format: options.LogFormatPlain,
						},
					}
					config := &options.LoggerConfig{}
					require.NoError(t, config.Set(loggerProducer))
					opts.Output.Loggers = []*options.LoggerConfig{config}
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 100)
					assert.NoError(t, err)
					assert.Contains(t, logs, output)
				},
				"MultipleInMemoryLoggersReturnLogsFromOnlyOne": func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor, output string) {
					config1 := &options.LoggerConfig{}
					require.NoError(t, config1.Set(&options.InMemoryLoggerOptions{
						InMemoryCap: 100,
						Base: options.BaseOptions{
							Format: options.LogFormatPlain,
						},
					}))
					config2 := &options.LoggerConfig{}
					require.NoError(t, config2.Set(&options.InMemoryLoggerOptions{
						InMemoryCap: 100,
						Base: options.BaseOptions{
							Format: options.LogFormatPlain,
						},
					}))
					opts.Output.Loggers = []*options.LoggerConfig{config1, config2}
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 100)
					assert.NoError(t, err)
					assert.Contains(t, logs, output)

					outputCount := 0
					for _, log := range logs {
						if strings.Contains(log, output) {
							outputCount++
						}
					}
					assert.Equal(t, 1, outputCount)
				},
				// "": func(ctx context.Context, t *testing.T, opts *options.Create, output string) {},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, tcancel := context.WithTimeout(ctx, testutil.ProcessTestTimeout)
					defer tcancel()

					output := t.Name() + " " + filepath.Join(procType, testName)
					opts := &options.Create{Args: []string{"echo", output}}
					testCase(tctx, t, opts, makeProc, output)
				})
			}

		})
	}
}
