package service

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPatchListModulesEndPoints(t *testing.T) {
	testDirectory := testutil.GetDirectoryOfFile()
	testConfig := testutil.TestConfig()
	testApiServer, err := CreateTestServer(testConfig, nil)
	testutil.HandleTestingErr(err, t, "failed to create new API server")
	defer testApiServer.Close()

	const (
		path    = "/api/patches/%s/%s/modules"
		githash = "1e5232709595db427893826ce19289461cba3f75"
	)

	url := testApiServer.URL + path

	Convey("list modules endpoint should function adequately", t, func() {
		Convey("without data there should be nothing found", func() {
			request, err := http.NewRequest("GET", fmt.Sprintf(url, "patchOne", "test"), bytes.NewBuffer([]byte{}))
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			So(err, ShouldBeNil)
			resp, err := http.DefaultClient.Do(request)
			testutil.HandleTestingErr(err, t, "problem making request")
			So(resp.StatusCode, ShouldEqual, 404)
		})

		Convey("with a patch", func() {
			testData, err := modelUtil.SetupAPITestData(testConfig, "compile", "linux-64",
				filepath.Join(testDirectory, "testdata/base_project.yaml"), modelUtil.ExternalPatch)
			testutil.HandleTestingErr(err, t, "problem setting up test server")

			_, err = modelUtil.SetupPatches(modelUtil.ExternalPatch, testData.Build,
				modelUtil.PatchRequest{"recursive", filepath.Join(testDirectory, "testdata/testmodule.patch"), githash})
			testutil.HandleTestingErr(err, t, "problem setting up patch")

			request, err := http.NewRequest("GET", fmt.Sprintf(url, modelUtil.PatchId, testData.Build.Id), nil)
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			So(err, ShouldBeNil)
			resp, err := http.DefaultClient.Do(request)
			testutil.HandleTestingErr(err, t, "problem making request")
			data := struct {
				Project string   `json:"project"`
				Modules []string `json:"modules"`
			}{}

			err = util.ReadJSONInto(resp.Body, &data)
			So(err, ShouldBeNil)
			So(len(data.Modules), ShouldEqual, 1)
			So(data.Project, ShouldEqual, testData.Build.Id)
		})

		Convey("with a patch that adds a module", func() {
			testData, err := modelUtil.SetupAPITestData(testConfig, "compile", "linux-64",
				filepath.Join(testDirectory, "testdata/base_project.yaml"), modelUtil.ExternalPatch)
			testutil.HandleTestingErr(err, t, "problem setting up test server")
			_, err = modelUtil.SetupPatches(modelUtil.InlinePatch, testData.Build,
				modelUtil.PatchRequest{"evgHome", filepath.Join(testDirectory, "testdata/testaddsmodule.patch"), githash})
			testutil.HandleTestingErr(err, t, "problem setting up patch")

			request, err := http.NewRequest("GET", fmt.Sprintf(url, modelUtil.PatchId, testData.Build.Id), nil)
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			So(err, ShouldBeNil)
			resp, err := http.DefaultClient.Do(request)
			testutil.HandleTestingErr(err, t, "problem making request")
			data := struct {
				Project string   `json:"project"`
				Modules []string `json:"modules"`
			}{}

			err = util.ReadJSONInto(resp.Body, &data)
			So(err, ShouldBeNil)
			So(len(data.Modules), ShouldEqual, 2)
			So(data.Project, ShouldEqual, testData.Build.Id)
		})
	})
}
