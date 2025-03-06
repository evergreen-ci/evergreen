package taskoutput

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func BenchmarkTestLog(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, bench := range []struct {
		name   string
		config testLogBenchmarkConfiguration
	}{
		{
			name: "PartitionedFileReadAndPreloadedRedactions",
			config: testLogBenchmarkConfiguration{
				filePath:          "benchmark_log.txt",
				sequenceSize:      1e7,
				numRedactionKeys:  40,
				preloadRedactions: true,
			},
		},
		{
			name: "PartitionedFileRead",
			config: testLogBenchmarkConfiguration{
				filePath:         "benchmark_log.txt",
				sequenceSize:     1e7,
				numRedactionKeys: 40,
			},
		},
		{
			name: "SequentialFileRead",
			config: testLogBenchmarkConfiguration{
				filePath:         "benchmark_log.txt",
				sequenceSize:     1e10,
				numRedactionKeys: 40,
			},
		},
	} {
		b.Run(bench.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				require.NoError(b, setupTestLogBenchmark(b, bench.config).run(ctx))
			}
		})
	}

	// Total file size.
	// Sequence size.
	// Number of redaction keys.
	// With and without preloaded expansions.
	// Number of workers (maybe).
	// Number of files (with old way).
}

func benchmarkTestLog(b *testing.B, config testLogBenchmarkConfiguration) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := setupTestLogBenchmark(b, testLogBenchmarkConfiguration{
		filePath:          "benchmark_log.txt",
		sequenceSize:      1e7,
		numRedactionKeys:  40,
		preloadRedactions: true,
	})

	for n := 0; n < b.N; n++ {
		require.NoError(b, h.run(ctx))
	}
}

type testLogBenchmarkConfiguration struct {
	filePath          string
	sequenceSize      int64
	numRedactionKeys  int
	preloadRedactions bool
}

func setupTestLogBenchmark(b *testing.B, config testLogBenchmarkConfiguration) *testLogDirectoryHandler {
	expansions := map[string]string{}
	keys := make([]string, config.numRedactionKeys)
	for i := 0; i < config.numRedactionKeys; i++ {
		key := fmt.Sprintf("secret_%d", i)
		expansions[key] = "abcdefghijklmnopqrstuvwxyz"
		keys[i] = key
	}

	tsk := &task.Task{
		Project: "project",
		Id:      utility.RandomString(),
		TaskOutputInfo: &taskoutput.TaskOutput{
			TestLogs: taskoutput.TestLogOutput{
				Version: 1,
				BucketConfig: evergreen.BucketConfig{
					Name: b.TempDir(),
					Type: evergreen.BucketTypeLocal,
					//Name: "julian-push-test",
					//Type: evergreen.BucketTypeS3,
				},
			},
		},
	}

	logger, err := client.NewMock("url").GetLoggerProducer(context.TODO(), tsk, nil)
	require.NoError(b, err)

	h := newTestLogDirectoryHandler(
		b.TempDir(),
		tsk.TaskOutputInfo,
		taskoutput.TaskOptions{
			ProjectID: tsk.Project,
			TaskID:    tsk.Id,
			Execution: tsk.Execution,
		},
		redactor.RedactionOptions{
			Expansions:         util.NewDynamicExpansions(expansions),
			Redacted:           keys,
			InternalRedactions: util.NewDynamicExpansions(map[string]string{"another_secret": "DEADC0DE"}),
			PreloadExpansions:  config.preloadRedactions,
		},
		logger,
	).(*testLogDirectoryHandler)
	h.sequenceSize = config.sequenceSize

	data, err := yaml.Marshal(testLogSpec{Format: testLogFormatTextTimestamp})
	require.NoError(b, err)
	require.NoError(b, os.WriteFile(filepath.Join(h.dir, testLogSpecFilename), data, 0777))

	src, err := os.Open(config.filePath)
	require.NoError(b, err)
	dst, err := os.Create(filepath.Join(h.dir, config.filePath))
	require.NoError(b, err)
	_, err = io.Copy(dst, src)
	require.NoError(b, err)
	require.NoError(b, src.Close())
	require.NoError(b, dst.Close())

	return h
}

func generateExpansions(n int) (*util.DynamicExpansions, []string) {
	expansions := map[string]string{}
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("secret_%d", i)
		expansions[key] = "abcdefghijklmnopqrstuvwxyz"
		keys[i] = key
	}

	return util.NewDynamicExpansions(expansions), keys
}
