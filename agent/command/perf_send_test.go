package command

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/poplar"
	"github.com/stretchr/testify/assert"
)

func TestPerfSendParseParams(t *testing.T) {
	for _, test := range []struct {
		name   string
		params map[string]interface{}
		hasErr bool
	}{
		{
			name:   "MissingFile",
			params: map[string]interface{}{},
			hasErr: true,
		},
		{
			name:   "FileOnly",
			params: map[string]interface{}{"file": "fn"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := &perfSend{}
			err := cmd.ParseParams(test.params)

			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerfSendAddEvgData(t *testing.T) {
	cmd := &perfSend{
		AWSKey:    "key",
		AWSSecret: "secret",
		Region:    "region",
		Bucket:    "bucket",
		Prefix:    "prefix",
	}
	conf := &internal.TaskConfig{
		Task: task.Task{
			Id:                  "id",
			Project:             "project",
			Version:             "version",
			RevisionOrderNumber: 100,
			BuildVariant:        "variant",
			DisplayName:         "name",
			Execution:           1,
			Requester:           evergreen.RepotrackerVersionRequester,
		},
	}
	report := &poplar.Report{}
	expectedReport := &poplar.Report{
		Project:   conf.Task.Project,
		Version:   conf.Task.Version,
		Order:     conf.Task.RevisionOrderNumber,
		Variant:   conf.Task.BuildVariant,
		TaskName:  conf.Task.DisplayName,
		TaskID:    conf.Task.Id,
		Execution: conf.Task.Execution,
		Mainline:  true,
		Requester: evergreen.RepotrackerVersionRequester,
		BucketConf: poplar.BucketConfiguration{
			APIKey:    cmd.AWSKey,
			APISecret: cmd.AWSSecret,
			Region:    cmd.Region,
			Name:      cmd.Bucket,
			Prefix:    cmd.Prefix,
		},
	}
	cmd.addEvgData(report, conf)
	assert.Equal(t, expectedReport, report)
}
