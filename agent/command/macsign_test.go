package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
)

func TestMacSignValidateParams(t *testing.T) {
	for _, tc := range []struct {
		name      string
		params    map[string]interface{}
		shouldErr bool
	}{
		{
			name: "can detect missing key_id",
			params: map[string]interface{}{
				"secret":          "secret",
				"local_zip_file":  "local",
				"output_zip_file": "zip",
				"service_url":     "url",
			},
			shouldErr: true,
		},
		{
			name: "can detect missing secret",
			params: map[string]interface{}{
				"key_id":          "keyId",
				"local_zip_file":  "local",
				"output_zip_file": "zip",
				"service_url":     "url",
			},
			shouldErr: true,
		},
		{
			name: "can detect missing service_url",
			params: map[string]interface{}{
				"key_id":          "keyId",
				"secret":          "secret",
				"local_zip_file":  "local",
				"output_zip_file": "zip",
			},
			shouldErr: true,
		},
		{
			name: "can detect missing output_zip_file",
			params: map[string]interface{}{
				"key_id":         "keyId",
				"secret":         "secret",
				"local_zip_file": "local",
				"service_url":    "url",
			},
			shouldErr: true,
		},
		{
			name: "can detect missing local_zip_file",
			params: map[string]interface{}{
				"key_id":          "keyId",
				"secret":          "secret",
				"service_url":     "url",
				"output_zip_file": "zip",
			},
			shouldErr: true,
		},
		{
			name: "valid set of signing params produces no error",
			params: map[string]interface{}{
				"key_id":          "keyId",
				"secret":          "secret",
				"service_url":     "url",
				"output_zip_file": "outputzip",
				"local_zip_file":  "inputzip",
			},
			shouldErr: false,
		},
		// notary specific tests
		{
			name: "can detect missing bundleId on notarization",
			params: map[string]interface{}{
				"key_id":          "keyId",
				"secret":          "secret",
				"service_url":     "url",
				"output_zip_file": "outputzip",
				"local_zip_file":  "inputzip",
				"notarize":        true,
			},
			shouldErr: true,
		},
		{
			name: "valid set of notarization params produces no error",
			params: map[string]interface{}{
				"key_id":          "keyId",
				"secret":          "secret",
				"service_url":     "url",
				"output_zip_file": "outputzip",
				"local_zip_file":  "inputzip",
				"notarize":        true,
				"bundle_id":       "bundleId",
			},
			shouldErr: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("command supported only on darwin and linux")
			}
			tc := tc
			t.Parallel()
			cmd := &macSign{}
			err := cmd.ParseParams(tc.params)

			if tc.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMacSignExecute(t *testing.T) {

	for _, tc := range []struct {
		name              string
		params            map[string]interface{}
		executableContent string
		shouldErr         bool
	}{
		{
			name: "can detect failure from macnotary binary",
			params: map[string]interface{}{
				"key_id":            "keyId",
				"secret":            "secret",
				"local_zip_file":    "local",
				"output_zip_file":   "zip",
				"service_url":       "url",
				"notarize":          true,
				"bundle_id":         "bundleId",
				"working_directory": "shouldbecreated",
				"verify":            true,
			},
			executableContent: "#!/bin/sh \nexit 1",
			shouldErr:         true,
		},
		{
			name: "can create file on successful macnotary execution",
			params: map[string]interface{}{
				"key_id":          "keyId",
				"secret":          "secret",
				"local_zip_file":  "local",
				"output_zip_file": filepath.Join(t.TempDir(), "output.zip"),
				"service_url":     "url",
				"notarize":        true,
				"bundle_id":       "bundleId",
				"verify":          true,
			},
			executableContent: fmt.Sprintf("#!/bin/sh \ntouch %s && exit 0", filepath.Join(t.TempDir(), "output.zip")),
			shouldErr:         false,
		},
	} {
		{
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("command supported only on darwin and linux")
				}
				t.Parallel()
				ctx := context.TODO()
				mockClientBinary := filepath.Join(t.TempDir(), "client")
				assert.NoError(t,
					os.WriteFile(mockClientBinary, []byte(tc.executableContent), 0777),
				)
				cmd := &macSign{}
				tc.params["client_binary"] = mockClientBinary
				err := cmd.ParseParams(tc.params)
				assert.NoError(t, err)

				assert.NoError(t, cmd.validate())

				comm := client.NewMock("http://localhost.com")
				conf := &internal.TaskConfig{
					Expansions:   util.Expansions{},
					Task:         task.Task{Id: "mock_id", Secret: "mock_secret"},
					Project:      model.Project{},
					BuildVariant: model.BuildVariant{},
				}
				logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
				assert.NoError(t, err)

				if tc.shouldErr {
					assert.Error(t, cmd.Execute(ctx, comm, logger, conf))
				} else {
					assert.NoError(t, cmd.Execute(ctx, comm, logger, conf))
				}
			})

		}
	}
}
