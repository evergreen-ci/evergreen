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
	t.Run("ParseParams", func(t *testing.T) {
		for tName, tCase := range map[string]struct {
			params map[string]interface{}
			update []updateParams
			err    string
		}{
			"EmptyParams": {
				params: map[string]interface{}{},
			},
			"MissingKey": {
				params: map[string]interface{}{
					"updates": []map[string]interface{}{
						{
							"value": "value",
						},
					},
				},
				err: "expansion key at index 0 must not be a blank string",
			},
			"EmptyKey": {
				params: map[string]interface{}{
					"updates": []map[string]interface{}{
						{
							"key":   "",
							"value": "value",
						},
					},
				},
				err: "expansion key at index 0 must not be a blank string",
			},
			"ConcatAndValuePresent": {
				params: map[string]interface{}{
					"updates": []map[string]interface{}{
						{
							"key":    "key",
							"value":  "value",
							"concat": "concat",
						},
					},
				},
				err: "expansion 'key' at index 0 must not have both a value and a concat",
			},
			"InvalidRedactFileWithNoFile": {
				params: map[string]interface{}{
					"redact_file_expansions": true,
				},
				err: "redact_file_expansions is true but no file was provided",
			},
			"ValidParams": {
				params: map[string]interface{}{
					"updates": []map[string]interface{}{
						{
							"key":   "key-1",
							"value": "value",
						},
						{
							"key":    "key-2",
							"concat": "concat",
						},
						{
							"key":    "key-3",
							"value":  "value-2",
							"redact": true,
						},
						{
							"key":    "key-4",
							"concat": "concat-2",
							"redact": true,
						},
					},
				},
				update: []updateParams{
					{
						Key:   "key-1",
						Value: "value",
					},
					{
						Key:    "key-2",
						Concat: "concat",
					},
					{
						Key:    "key-3",
						Value:  "value-2",
						Redact: true,
					},
					{
						Key:    "key-4",
						Concat: "concat-2",
						Redact: true,
					},
				},
			},
			"ValidFile": {
				params: map[string]interface{}{
					"file": "file.yaml",
				},
			},
			"ValidFileWithRedact": {
				params: map[string]interface{}{
					"file":                   "file.yaml",
					"redact_file_expansions": true,
				},
			},
		} {
			t.Run(tName, func(t *testing.T) {
				updateCommand := update{}
				err := updateCommand.ParseParams(tCase.params)
				if tCase.err == "" {
					require.NoError(t, err)
					assert.Equal(t, tCase.update, updateCommand.Updates)
				} else {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tCase.err)
				}
			})
		}
	})

	t.Run("ExecuteUpdates", func(t *testing.T) {
		for tName, tCase := range map[string]struct {
			updates    []updateParams
			expansions util.Expansions
			expected   util.Expansions
			redact     []agentutil.RedactInfo
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
				redact:     []agentutil.RedactInfo{{Key: "base", Value: "not eggs"}, {Key: "toppings", Value: ", chicken"}},
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
				redact:     []agentutil.RedactInfo{{Key: "base", Value: "not eggs"}, {Key: "toppings", Value: "bacon, chicken"}},
			},
			"RedactionReplaceValuesAndConcatAfterExtraUpdate": {
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
						Key:   "base",
						Value: "truly eggs",
					},
					{
						Key:    "toppings",
						Concat: ", another bacon",
					},
					{
						Key:   "not-redacted",
						Value: "other",
					},
				},
				expansions: util.Expansions{"base": "eggs", "toppings": "bacon"},
				expected:   util.Expansions{"base": "truly eggs", "toppings": "bacon, chicken, another bacon", "not-redacted": "other"},
				redact:     []agentutil.RedactInfo{{Key: "base", Value: "not eggs"}, {Key: "toppings", Value: "bacon, chicken"}},
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
	})
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
	expansions := util.Expansions{
		"foo":                      "bar",
		"baz":                      "qux",
		"password":                 "hunter2",
		evergreen.GithubAppToken:   "app_token",
		globals.AWSAccessKeyId:     "aws_key_id",
		globals.AWSSecretAccessKey: "aws_secret_key",
		globals.AWSSessionToken:    "aws_token",
	}
	tc := &internal.TaskConfig{
		Expansions:    expansions,
		NewExpansions: agentutil.NewDynamicExpansions(expansions),
		Redacted:      []string{"password"},
	}
	// This should be redacted and not written to the output.
	tc.NewExpansions.PutAndRedact("a_secret_password", "a_not_so_secret_value")
	f, err := os.CreateTemp("", t.Name())
	require.NoError(t, err)
	defer os.Remove(f.Name())

	writer := &expansionsWriter{File: f.Name()}
	err = writer.Execute(ctx, comm, logger, tc)
	require.NoError(t, err)
	out, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, "baz: qux\nfoo: bar\n", string(out))

	writer = &expansionsWriter{File: f.Name(), Redacted: true}
	err = writer.Execute(ctx, comm, logger, tc)
	require.NoError(t, err)
	out, err = os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, "baz: qux\nfoo: bar\npassword: hunter2\n", string(out))
}
