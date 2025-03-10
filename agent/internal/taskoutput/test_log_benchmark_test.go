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

func BenchmarkTestLogDirectoryHandler(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, bench := range []struct {
		name              string
		version           int
		preloadRedactions bool
	}{
		{
			name:    "OriginalImplementation(SequentialFileReadWithSendCallPerLine)",
			version: 0,
		},
		{
			name:    "PartitionedFileReadWithSendCallPerLine",
			version: 1,
		},
		{
			name:              "PartitionedFileReadWithSendCallPerLineAndPreloadedRedactions",
			version:           1,
			preloadRedactions: true,
		},
		{
			name:    "PartitionedFileReadWithSingleSendCallPerPartition",
			version: 2,
		},
		{
			name:              "PartitionedFileReadWithSingleSendCallPerPartitionAndPreloadedRedactions",
			version:           2,
			preloadRedactions: true,
		},
		{
			name:    "PartitionedFileReadWithSingleSendCallPerPartitionAndBytesMessage",
			version: 3,
		},
		{
			name:              "PartitionedFileReadWithSingleSendCallPerPartitionAndBytesMessageAndPreloadedRedactions",
			version:           3,
			preloadRedactions: true,
		},
	} {
		b.Run(bench.name, func(b *testing.B) {
			h := setupTestLogBenchmark(b, testLogBenchmarkConfiguration{
				filePath:          "benchmark_log.txt",
				numRedactionKeys:  40,
				version:           bench.version,
				preloadRedactions: bench.preloadRedactions,
			})
			for n := 0; n < b.N; n++ {
				require.NoError(b, h.run(ctx))
			}
		})
	}
}

type testLogBenchmarkConfiguration struct {
	filePath          string
	numRedactionKeys  int
	version           int
	preloadRedactions bool
}

func setupTestLogBenchmark(b *testing.B, config testLogBenchmarkConfiguration) directoryHandler {
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

	switch config.version {
	case 0:
		return &testLogDirectoryHandlerV0{h}
	case 1:
		return &testLogDirectoryHandlerV1{h}
	case 2:
		return &testLogDirectoryHandlerV2{h}
	}
	return h
}
