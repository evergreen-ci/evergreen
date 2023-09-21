package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
)

func TestDownstreamExpansions(t *testing.T) {
	t.Run("SetDownstreamParams", func(t *testing.T) {
		for testName, testCase := range map[string]func(t *testing.T, ctx context.Context, comm *client.Mock, conf *internal.TaskConfig, logger client.LoggerProducer, cwd string){
			"FilenameIsExpanded": func(t *testing.T, ctx context.Context, comm *client.Mock, conf *internal.TaskConfig, logger client.LoggerProducer, path string) {
				conf.Expansions = *util.NewExpansions(map[string]string{"foo": path})
				cmd := &setDownstream{YamlFile: "${foo}"}
				assert.Nil(t, cmd.Execute(ctx, comm, logger, conf))
				assert.Equal(t, cmd.YamlFile, path)
			},
			"ContentsAreStored": func(t *testing.T, ctx context.Context, comm *client.Mock, conf *internal.TaskConfig, logger client.LoggerProducer, path string) {
				cmd := &setDownstream{YamlFile: path}
				assert.Nil(t, cmd.Execute(ctx, comm, logger, conf))
				paramsCmd := map[string]string{}
				paramsComm := map[string]string{}
				for i := range cmd.downstreamParams {
					paramsCmd[cmd.downstreamParams[i].Key] = cmd.downstreamParams[i].Value
					paramsComm[comm.DownstreamParams[i].Key] = comm.DownstreamParams[i].Value
				}
				assert.Equal(t, "newValue1", paramsCmd["key1"])
				assert.Equal(t, "newValue2", paramsCmd["key2"])
				assert.Equal(t, "newValue1", paramsComm["key1"])
				assert.Equal(t, "newValue2", paramsComm["key2"])
			},
		} {
			t.Run(testName, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				comm := client.NewMock("http://localhost.com")
				conf := &internal.TaskConfig{Expansions: util.Expansions{}, Task: task.Task{Requester: "patch_request"}, Project: model.Project{}}
				logger, _ := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
				cwd := testutil.GetDirectoryOfFile()
				path := filepath.Join(cwd, "testdata", "git", "test_expansions.yml")
				testCase(t, ctx, comm, conf, logger, path)
			})
		}

		t.Run("OnlyOpForPatches", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			comm := client.NewMock("http://localhost.com")
			conf := &internal.TaskConfig{Expansions: util.Expansions{}, Task: task.Task{Requester: "gitter_request"}, Project: model.Project{}}
			logger, _ := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
			cwd := testutil.GetDirectoryOfFile()
			path := filepath.Join(cwd, "testdata", "git", "test_expansions.yml")

			cmd := &setDownstream{YamlFile: path}
			assert.Nil(t, cmd.Execute(ctx, comm, logger, conf))
			paramsCmd := map[string]string{}
			for i := range cmd.downstreamParams {
				paramsCmd[cmd.downstreamParams[i].Key] = cmd.downstreamParams[i].Value
			}
			assert.Equal(t, "newValue1", paramsCmd["key1"])
			assert.Equal(t, "newValue2", paramsCmd["key2"])
			assert.Nil(t, comm.DownstreamParams)
		})
	})
}
