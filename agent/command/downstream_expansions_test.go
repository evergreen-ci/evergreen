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
			"FilenameIsExpanded": func(t *testing.T, ctx context.Context, comm *client.Mock, conf *internal.TaskConfig, logger client.LoggerProducer, cwd string) {
				path := filepath.Join(cwd, "testdata", "git", "test_expansions.yml")
				conf.Expansions = util.NewExpansions(map[string]string{"foo": path})
				cmd := &setDownstream{YamlFile: "${foo}"}
				cmdExecute := cmd.Execute(ctx, comm, logger, conf)
				assert.Nil(t, cmdExecute)
				assert.Equal(t, cmd.YamlFile, path)
			},
			"ContentsAreStored": func(t *testing.T, ctx context.Context, comm *client.Mock, conf *internal.TaskConfig, logger client.LoggerProducer, cwd string) {
				path := filepath.Join(cwd, "testdata", "git", "test_expansions.yml")
				cmd := &setDownstream{YamlFile: path}
				assert.Nil(t, cmd.Execute(ctx, comm, logger, conf))
				assert.Equal(t, cmd.downstreamParams[0].Key, "key_1")
				assert.Equal(t, cmd.downstreamParams[0].Value, "value_1")
				assert.Equal(t, cmd.downstreamParams[1].Key, "my_docker_image")
				assert.Equal(t, cmd.downstreamParams[1].Value, "my_image")
				assert.NotNil(t, comm.DownstreamParams)
			},
		} {
			t.Run(testName, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				comm := client.NewMock("http://localhost.com")
				conf := &internal.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
				logger, _ := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
				cwd := testutil.GetDirectoryOfFile()
				testCase(t, ctx, comm, conf, logger, cwd)
			})
		}
	})
}
