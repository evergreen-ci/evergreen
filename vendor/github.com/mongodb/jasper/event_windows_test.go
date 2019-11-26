package jasper

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mongodStartupTime         = 15 * time.Second
	mongodShutdownEventPrefix = "Global\\Mongo_"
)

func TestMongodShutdownEvent(t *testing.T) {
	for procName, makeProc := range map[string]ProcessConstructor{
		"Basic":    newBasicProcess,
		"Blocking": newBlockingProcess,
	} {
		t.Run(procName, func(t *testing.T) {
			if testing.Short() {
				t.Skip("skipping mongod shutdown event tests in short mode")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var opts options.Create
			dir, mongodPath := downloadMongoDB(t)
			defer os.RemoveAll(dir)

			optslist, dbPaths, err := setupMongods(1, mongodPath)
			require.NoError(t, err)
			defer removeDBPaths(dbPaths)
			require.Equal(t, 1, len(optslist))

			opts = optslist[0]
			opts.Output.Loggers = []options.Logger{{Type: options.LogDefault, Options: options.Log{Format: options.LogFormatPlain}}}

			proc, err := makeProc(ctx, &opts)
			require.NoError(t, err)

			// Give mongod time to start up its signal processing thread.
			time.Sleep(mongodStartupTime)

			pid := proc.Info(ctx).PID
			mongodShutdownEvent := mongodShutdownEventPrefix + strconv.Itoa(pid)

			require.NoError(t, SignalEvent(ctx, mongodShutdownEvent))

			exitCode, err := proc.Wait(ctx)
			assert.NoError(t, err)
			assert.Zero(t, exitCode)
			assert.False(t, proc.Running(ctx))
		})
	}
}
