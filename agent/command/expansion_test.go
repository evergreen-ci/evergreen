package command

import (
	"context"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpansionUpdate(t *testing.T) {
	for tName, tCase := range map[string]struct {
		updates    []updateParams
		expansions util.Expansions
		expected   util.Expansions
		redact     []string
		err        error
	}{
		"EmptyUpdate": {
			updates:    []updateParams{},
			expansions: util.Expansions{},
			expected:   util.Expansions{},
		},
		"NewValuesWithValueAndEmptyConcat": {
			updates: []updateParams{
				{
					Key:   "base",
					Value: "not eggs",
				},
				{
					Key:    "toppings",
					Concat: ", chicken",
				},
			},
			expansions: util.Expansions{},
			expected:   util.Expansions{"base": "not eggs", "toppings": ", chicken"},
		},
		"ReplaceValuesAndConcatValues": {
			updates: []updateParams{
				{
					Key:   "base",
					Value: "not eggs",
				},
				{
					Key:    "toppings",
					Concat: ", chicken",
				},
			},
			expansions: util.Expansions{"base": "eggs", "toppings": "bacon"},
			expected:   util.Expansions{"base": "not eggs", "toppings": "bacon, chicken"},
		},
		"RedactionNewValuesAndEmptyConcat": {
			updates: []updateParams{
				{
					Key:    "base",
					Value:  "not eggs",
					Redact: true,
				},
				{
					Key:    "toppings",
					Concat: ", chicken",
					Redact: true,
				},
			},
			expansions: util.Expansions{},
			expected:   util.Expansions{"base": "not eggs", "toppings": ", chicken"},
			redact:     []string{"base", "toppings"},
		},
		"RedactionReplaceValuesAndConcat": {
			updates: []updateParams{
				{
					Key:    "base",
					Value:  "not eggs",
					Redact: true,
				},
				{
					Key:    "toppings",
					Concat: ", chicken",
					Redact: true,
				},
				{
					Key:   "not-redacted",
					Value: "other",
				},
			},
			expansions: util.Expansions{"base": "eggs", "toppings": "bacon"},
			expected:   util.Expansions{"base": "not eggs", "toppings": "bacon, chicken", "not-redacted": "other"},
			redact:     []string{"base", "toppings"},
		},
	} {
		t.Run(tName, func(t *testing.T) {
			updateCommand := update{
				Updates: tCase.updates,
			}

			taskConfig := internal.TaskConfig{
				Expansions:    tCase.expansions,
				NewExpansions: agentutil.NewDynamicExpansions(tCase.expansions),
			}

			err := updateCommand.executeUpdates(context.Background(), &taskConfig)
			require.NoError(t, err)
			assert.Equal(t, tCase.expected, taskConfig.DynamicExpansions)
			assert.Equal(t, tCase.redact, taskConfig.NewExpansions.GetRedacted())
		})
	}

}

func TestExpansionUpdateFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{Expansions: util.Expansions{}, Task: task.Task{}, Project: model.Project{}}
	conf.NewExpansions = agentutil.NewDynamicExpansions(conf.Expansions)
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	Convey("When running Update commands", t, func() {
		Convey("if there is no expansion, the file name is not changed", func() {
			So(conf.Expansions, ShouldResemble, util.Expansions{})
			cmd := &update{YamlFile: "foo"}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "foo")
		})

		Convey("With an Expansion, the file name is expanded", func() {
			conf.NewExpansions.Put("foo", "bar")
			cmd := &update{YamlFile: "${foo}"}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "bar")
		})
	})
}

func TestExpansionWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, &task.Task{}, nil)
	require.NoError(t, err)
	tc := &internal.TaskConfig{
		Expansions: util.Expansions{
			"foo":                                "bar",
			"baz":                                "qux",
			"password":                           "hunter2",
			evergreen.GlobalGitHubTokenExpansion: "sample_token",
			evergreen.GithubAppToken:             "app_token",
			globals.AWSAccessKeyId:               "aws_key_id",
			globals.AWSSecretAccessKey:           "aws_secret_key",
			globals.AWSSessionToken:              "aws_token",
		},
		Redacted: []string{"password"},
	}
	f, err := os.CreateTemp("", t.Name())
	require.NoError(t, err)
	defer os.Remove(f.Name())

	writer := &expansionsWriter{File: f.Name()}
	err = writer.Execute(ctx, comm, logger, tc)
	assert.NoError(t, err)
	out, err := os.ReadFile(f.Name())
	assert.NoError(t, err)
	assert.Equal(t, "baz: qux\nfoo: bar\n", string(out))

	writer = &expansionsWriter{File: f.Name(), Redacted: true}
	err = writer.Execute(ctx, comm, logger, tc)
	assert.NoError(t, err)
	out, err = os.ReadFile(f.Name())
	assert.NoError(t, err)
	assert.Equal(t, "baz: qux\nfoo: bar\npassword: hunter2\n", string(out))
}
