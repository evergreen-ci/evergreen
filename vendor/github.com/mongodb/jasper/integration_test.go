package jasper

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/evergreen-ci/bond"
	"github.com/evergreen-ci/bond/recall"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Returns path to release and to mongod
func downloadMongoDB(t *testing.T) (string, string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var target string
	var edition string
	platform := runtime.GOOS
	switch platform {
	case "darwin":
		target = "osx"
		edition = "enterprise"
	case "linux":
		edition = "base"
		target = platform
	default:
		edition = "enterprise"
		target = platform
	}
	arch := "x86_64"
	release := "4.0-stable"

	dir, err := ioutil.TempDir("", "mongodb")
	require.NoError(t, err)

	opts := bond.BuildOptions{
		Target:  target,
		Arch:    bond.MongoDBArch(arch),
		Edition: bond.MongoDBEdition(edition),
		Debug:   false,
	}
	releases := []string{release}
	require.NoError(t, recall.DownloadReleases(releases, dir, opts))

	catalog, err := bond.NewCatalog(ctx, dir)
	require.NoError(t, err)

	path, err := catalog.Get("4.0-current", edition, target, arch, false)
	require.NoError(t, err)

	var mongodPath string
	if platform == "windows" {
		mongodPath = filepath.Join(path, "bin", "mongod.exe")
	} else {
		mongodPath = filepath.Join(path, "bin", "mongod")
	}
	_, err = os.Stat(mongodPath)
	require.NoError(t, err)

	return dir, mongodPath
}

func setupMongods(numProcs int, mongodPath string) ([]options.Create, []string, error) {
	dbPaths := make([]string, numProcs)
	optslist := make([]options.Create, numProcs)
	for i := 0; i < numProcs; i++ {
		procName := "mongod"
		port := testutil.GetPortNumber()

		dbPath, err := ioutil.TempDir(testutil.BuildDirectory(), procName)
		if err != nil {
			return nil, nil, err
		}
		dbPaths[i] = dbPath

		opts := options.Create{Args: []string{mongodPath, "--port", fmt.Sprintf("%d", port), "--dbpath", dbPath}}
		optslist[i] = opts
	}

	return optslist, dbPaths, nil
}

func removeDBPaths(dbPaths []string) {
	for _, dbPath := range dbPaths {
		grip.Error(os.RemoveAll(dbPath))
	}
}

func TestMongod(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mongod tests in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, mongodPath := downloadMongoDB(t)
	defer os.RemoveAll(dir)

	for name, makeProc := range map[string]ProcessConstructor{
		"Basic":    newBasicProcess,
		"Blocking": newBlockingProcess,
	} {
		t.Run(name, func(t *testing.T) {
			for _, test := range []struct {
				id          string
				numProcs    int
				signal      syscall.Signal
				sleep       time.Duration
				expectError bool
			}{
				{
					id:          "WithSingleMongod",
					numProcs:    1,
					signal:      syscall.SIGKILL,
					sleep:       0,
					expectError: true,
				},
				{
					id:          "With10MongodsAndSigkill",
					numProcs:    10,
					signal:      syscall.SIGKILL,
					sleep:       2 * time.Second,
					expectError: true,
				},
				{
					id:          "With30MongodsAndSigkill",
					numProcs:    30,
					signal:      syscall.SIGKILL,
					sleep:       3 * time.Second,
					expectError: true,
				},
			} {
				t.Run(test.id, func(t *testing.T) {
					optslist, dbPaths, err := setupMongods(test.numProcs, mongodPath)
					defer removeDBPaths(dbPaths)
					require.NoError(t, err)

					// Spawn concurrent mongods
					procs := make([]Process, test.numProcs)
					for i, opts := range optslist {
						proc, err := makeProc(ctx, &opts)
						require.NoError(t, err)
						assert.True(t, proc.Running(ctx))
						procs[i] = proc
					}

					waitError := make(chan error, test.numProcs)
					for _, proc := range procs {
						go func(proc Process) {
							_, err := proc.Wait(ctx)
							waitError <- err
						}(proc)
					}

					// Signal to stop mongods
					time.Sleep(test.sleep)
					for _, proc := range procs {
						err := proc.Signal(ctx, test.signal)
						require.NoError(t, err)
					}

					// Check that the processes all received signal
					for i := 0; i < test.numProcs; i++ {
						err := <-waitError
						if test.expectError {
							assert.Error(t, err)
						} else {
							assert.NoError(t, err)
						}
					}

					// Check that the processes have all noted a unsuccessful run
					for _, proc := range procs {
						if test.expectError {
							assert.False(t, proc.Info(ctx).Successful)
						}
					}
				})
			}
		})
	}
}
