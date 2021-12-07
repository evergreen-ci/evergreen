package command

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestMacSignValidateParams(t *testing.T) {

	Convey("With a mac signing and notarization command", t, func() {

		var cmd *macSign

		Convey("when validating signing command params", func() {
			cmd = &macSign{}

			Convey("a missing key id should cause an error", func() {

				params := map[string]interface{}{
					"secret":          "secret",
					"local_zip_file":  "local",
					"output_zip_file": "zip",
					"service_url":     "url",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
			})
			Convey("a missing secret should cause an error", func() {

				params := map[string]interface{}{
					"key_id":          "keyId",
					"local_zip_file":  "local",
					"output_zip_file": "zip",
					"service_url":     "url",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
			})
			Convey("a missing service url should cause an error", func() {

				params := map[string]interface{}{
					"key_id":          "keyId",
					"secret":          "secret",
					"local_zip_file":  "local",
					"output_zip_file": "zip",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
			})
			Convey("a missing output_zip_file should cause an error", func() {

				params := map[string]interface{}{
					"key_id":         "keyId",
					"secret":         "secret",
					"local_zip_file": "local",
					"service_url":    "url",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
			})
			Convey("a missing local_zip_file should cause an error", func() {

				params := map[string]interface{}{
					"key_id":          "keyId",
					"secret":          "secret",
					"service_url":     "url",
					"output_zip_file": "zip",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
			})
			Convey("valid set of params should not cause an error", func() {

				params := map[string]interface{}{
					"key_id":          "keyId",
					"secret":          "secret",
					"service_url":     "url",
					"output_zip_file": "outputzip",
					"local_zip_file":  "inputzip",
				}
				So(cmd.ParseParams(params), ShouldBeNil)
				So(cmd.KeyId, ShouldEqual, params["key_id"])
				So(cmd.Secret, ShouldEqual, params["secret"])
				So(cmd.ServiceUrl, ShouldEqual, params["service_url"])
				So(cmd.OutputZipFile, ShouldEqual, params["output_zip_file"])
				So(cmd.LocalZipFile, ShouldEqual, params["local_zip_file"])
			})
		})

		Convey("when validating notarization command params", func() {
			cmd = &macSign{}

			Convey("notarizing requested bundleId not given should cause an error", func() {

				params := map[string]interface{}{
					"key_id":          "keyId",
					"secret":          "secret",
					"service_url":     "url",
					"output_zip_file": "outputzip",
					"local_zip_file":  "inputzip",
					"notarize":        true,
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
			})
			Convey("valid set of notarizing params should not cause an error", func() {

				params := map[string]interface{}{
					"key_id":          "keyId",
					"secret":          "secret",
					"service_url":     "url",
					"output_zip_file": "outputzip",
					"local_zip_file":  "inputzip",
					"notarize":        true,
					"bundle_id":       "bundleId",
				}
				So(cmd.ParseParams(params), ShouldBeNil)
				So(cmd.KeyId, ShouldEqual, params["key_id"])
				So(cmd.Secret, ShouldEqual, params["secret"])
				So(cmd.ServiceUrl, ShouldEqual, params["service_url"])
				So(cmd.OutputZipFile, ShouldEqual, params["output_zip_file"])
				So(cmd.LocalZipFile, ShouldEqual, params["local_zip_file"])
				So(cmd.Notarize, ShouldEqual, params["notarize"])
				So(cmd.BundleId, ShouldEqual, params["bundle_id"])
			})
		})
	})
}

func TestMacSignExecute(t *testing.T) {

	Convey("With a mac signing and notarization command", t, func() {

		Convey("when valid params given", func() {

			Convey("failed macnotary command should cause an error", func() {
				ctx := context.TODO()
				mockClientBinary := filepath.Join(t.TempDir(), "client")
				require.NoError(t,
					ioutil.WriteFile(mockClientBinary, []byte("#!/bin/sh \nexit 1"), 0777),
				)

				cmd := &macSign{
					KeyId:         "keyId",
					Secret:        "secret",
					ServiceUrl:    "url",
					OutputZipFile: "output",
					LocalZipFile:  "input",
					Notarize:      true,
					BundleId:      "bundleId",
					ClientBinary:  mockClientBinary,
				}

				comm := client.NewMock("http://localhost.com")
				conf := &internal.TaskConfig{
					Expansions:   &util.Expansions{},
					Task:         &task.Task{Id: "mock_id", Secret: "mock_secret"},
					Project:      &model.Project{},
					BuildVariant: &model.BuildVariant{},
				}
				logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
				require.NoError(t, err)

				require.Error(t, cmd.Execute(ctx, comm, logger, conf))
			})
			Convey("successful macnotary command should produce output file", func() {
				ctx := context.TODO()

				cmd := &macSign{
					KeyId:         "keyId",
					Secret:        "secret",
					ServiceUrl:    "url",
					OutputZipFile: filepath.Join(t.TempDir(), "output.zip"),
					LocalZipFile:  "input",
					Notarize:      true,
					BundleId:      "bundleId",
					ClientBinary:  filepath.Join(t.TempDir(), "client"),
				}

				require.NoError(t,
					ioutil.WriteFile(cmd.ClientBinary, []byte(fmt.Sprintf("#!/bin/sh \ntouch %s && exit 0", cmd.OutputZipFile)), 0777),
				)

				comm := client.NewMock("http://localhost.com")
				conf := &internal.TaskConfig{
					Expansions:   &util.Expansions{},
					Task:         &task.Task{Id: "mock_id", Secret: "mock_secret"},
					Project:      &model.Project{},
					BuildVariant: &model.BuildVariant{},
				}
				logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
				require.NoError(t, err)

				require.NoError(t, cmd.Execute(ctx, comm, logger, conf))
				require.FileExists(t, cmd.OutputZipFile)
			})
		})
	})
}
