package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchListModulesEndPoints(t *testing.T) {
	ctx := t.Context()

	testutil.DisablePermissionsForTests()
	defer testutil.EnablePermissionsForTests()
	testDirectory := testutil.GetDirectoryOfFile()
	testConfig := testutil.TestConfig()
	testApiServer, err := CreateTestServer(ctx, testConfig, nil, false)
	require.NoError(t, err, "failed to create new API server")
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
			require.NoError(t, err, "problem making request")
			defer resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, 404)
		})

		Convey("with a patch", func() {
			testData, err := modelUtil.SetupAPITestData(testConfig, "compile", "linux-64",
				filepath.Join(testDirectory, "testdata/base_project.yaml"), modelUtil.ExternalPatch)
			require.NoError(t, err, "problem setting up test server")

			_, err = modelUtil.SetupPatches(t.Context(), modelUtil.ExternalPatch, testData.Build,
				modelUtil.PatchRequest{
					ModuleName: "recursive",
					FilePath:   filepath.Join(testDirectory, "testdata/testmodule.patch"),
					Githash:    githash,
				})
			require.NoError(t, err, "problem setting up patch")

			request, err := http.NewRequest("GET", fmt.Sprintf(url, modelUtil.PatchId, testData.Build.Id), nil)
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			So(err, ShouldBeNil)
			resp, err := http.DefaultClient.Do(request)
			require.NoError(t, err, "problem making request")
			defer resp.Body.Close()
			data := struct {
				Project string   `json:"project"`
				Modules []string `json:"modules"`
			}{}

			err = utility.ReadJSON(resp.Body, &data)
			So(err, ShouldBeNil)
			So(len(data.Modules), ShouldEqual, 1)
			So(data.Project, ShouldEqual, testData.Build.Id)
		})

		Convey("with a patch that adds a module", func() {
			testData, err := modelUtil.SetupAPITestData(testConfig, "compile", "linux-64",
				filepath.Join(testDirectory, "testdata/base_project.yaml"), modelUtil.ExternalPatch)
			require.NoError(t, err, "problem setting up test server")
			_, err = modelUtil.SetupPatches(t.Context(), modelUtil.InlinePatch, testData.Build,
				modelUtil.PatchRequest{
					ModuleName: "evgHome",
					FilePath:   filepath.Join(testDirectory, "testdata/testaddsmodule.patch"),
					Githash:    githash,
				})
			require.NoError(t, err, "problem setting up patch")

			request, err := http.NewRequest("GET", fmt.Sprintf(url, modelUtil.PatchId, testData.Build.Id), nil)
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			So(err, ShouldBeNil)
			resp, err := http.DefaultClient.Do(request)
			require.NoError(t, err, "problem making request")
			defer resp.Body.Close()
			data := struct {
				Project string   `json:"project"`
				Modules []string `json:"modules"`
			}{}
			err = utility.ReadJSON(resp.Body, &data)
			So(err, ShouldBeNil)
			So(len(data.Modules), ShouldEqual, 2)
			So(data.Project, ShouldEqual, testData.Build.Id)
		})
	})
}

func TestPatchOwnershipChecks(t *testing.T) {
	const (
		owner   = "owner"
		other   = "other"
		project = "proj"
	)
	as := &APIServer{}

	type routeCase struct {
		handler http.HandlerFunc
		method  string
		path    string
		body    string
	}

	setup := func(t *testing.T) (unfinalized, finalized patch.Patch) {
		require.NoError(t, db.ClearCollections(patch.Collection, model.VersionCollection, task.Collection, build.Collection))
		unfinalized = patch.Patch{
			Id:          mgobson.NewObjectId(),
			Author:      owner,
			Project:     project,
			Description: "original",
			Patches:     []patch.ModulePatch{{ModuleName: "mod", Githash: "original"}},
		}
		require.NoError(t, unfinalized.Insert(t.Context()))
		finalized = patch.Patch{
			Id:        mgobson.NewObjectId(),
			Author:    owner,
			Project:   project,
			Version:   "v1",
			Activated: true,
		}
		require.NoError(t, finalized.Insert(t.Context()))
		v := model.Version{Id: "v1"}
		require.NoError(t, v.Insert(t.Context()))
		return unfinalized, finalized
	}

	do := func(t *testing.T, username string, p patch.Patch, rc routeCase) *httptest.ResponseRecorder {
		r := httptest.NewRequest(rc.method, rc.path, strings.NewReader(rc.body))
		r = r.WithContext(gimlet.AttachUser(r.Context(), &user.DBUser{Id: username}))
		r = gimlet.SetURLVars(r, map[string]string{"patchId": p.Id.Hex()})
		w := httptest.NewRecorder()
		rc.handler(w, r)
		return w
	}

	updateModule := routeCase{handler: as.updatePatchModule, method: http.MethodPost, path: "/", body: `{"module": "mod", "githash": "injected", "patch_bytes": ""}`}
	deleteModule := routeCase{handler: as.deletePatchModule, method: http.MethodDelete, path: "/?module=mod"}
	updateDescription := routeCase{handler: as.existingPatchRequest, method: http.MethodPost, path: "/", body: `{"action": "update", "description": "injected"}`}
	finalize := routeCase{handler: as.existingPatchRequest, method: http.MethodPost, path: "/", body: `{"action": "finalize"}`}
	cancel := routeCase{handler: as.existingPatchRequest, method: http.MethodPost, path: "/", body: `{"action": "cancel"}`}

	t.Run("UpdateModuleByNonOwnerShouldBeUnauthorized", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, other, p, updateModule)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		require.NotNil(t, dbPatch)
		require.Len(t, dbPatch.Patches, 1)
		assert.Equal(t, "original", dbPatch.Patches[0].Githash)
	})

	t.Run("UpdateModuleByOwnerShouldSucceed", func(t *testing.T) {
		require.NoError(t, db.ClearGridCollections(patch.GridFSPrefix))
		p, _ := setup(t)
		w := do(t, owner, p, updateModule)
		assert.Equal(t, http.StatusOK, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		require.NotNil(t, dbPatch)
		require.Len(t, dbPatch.Patches, 1)
		assert.Equal(t, "injected", dbPatch.Patches[0].Githash)
	})

	t.Run("DeleteModuleByNonOwnerShouldBeUnauthorized", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, other, p, deleteModule)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		require.NotNil(t, dbPatch)
		assert.Len(t, dbPatch.Patches, 1)
	})

	t.Run("DeleteModuleByOwnerShouldSucceed", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, owner, p, deleteModule)
		assert.Equal(t, http.StatusOK, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		require.NotNil(t, dbPatch)
		assert.Empty(t, dbPatch.Patches)
	})

	t.Run("UpdateDescriptionByNonOwnerShouldBeUnauthorized", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, other, p, updateDescription)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		require.NotNil(t, dbPatch)
		assert.Equal(t, "original", dbPatch.Description)
	})

	t.Run("UpdateDescriptionByOwnerShouldSucceed", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, owner, p, updateDescription)
		assert.Equal(t, http.StatusOK, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		require.NotNil(t, dbPatch)
		assert.Equal(t, "injected", dbPatch.Description)
	})

	t.Run("FinalizeByNonOwnerShouldBeUnauthorized", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, other, p, finalize)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		require.NotNil(t, dbPatch)
		assert.False(t, dbPatch.Activated)
		assert.Empty(t, dbPatch.Version)
	})

	t.Run("CancelUnfinalizedPatchByNonOwnerShouldBeUnauthorized", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, other, p, cancel)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		assert.NotNil(t, dbPatch)
	})

	t.Run("CancelUnfinalizedPatchByOwnerShouldDeletePatch", func(t *testing.T) {
		p, _ := setup(t)
		w := do(t, owner, p, cancel)
		assert.Equal(t, http.StatusOK, w.Code)

		dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		assert.Nil(t, dbPatch)
	})

	t.Run("CancelFinalizedPatchByNonOwnerShouldSucceed", func(t *testing.T) {
		_, p := setup(t)
		w := do(t, other, p, cancel)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestValidatePatchConfigPath(t *testing.T) {
	t.Run("ValidPathsSucceed", func(t *testing.T) {
		assert.NoError(t, validatePatchConfigPath("config/evergreen.yml"))
	})
	t.Run("AbsolutePathRejected", func(t *testing.T) {
		assert.Error(t, validatePatchConfigPath("/etc/passwd"))
	})
	t.Run("DirectoryTraversalRejected", func(t *testing.T) {
		assert.Error(t, validatePatchConfigPath("../etc/passwd"))
	})
	t.Run("ShellMetacharactersRejected", func(t *testing.T) {
		assert.Error(t, validatePatchConfigPath("$(touch /tmp/pwned)"))
		assert.Error(t, validatePatchConfigPath("config;rm -rf /"))
		assert.Error(t, validatePatchConfigPath("path`whoami`"))
		assert.Error(t, validatePatchConfigPath("a&b"))
		assert.Error(t, validatePatchConfigPath("file\nmalicious"))
	})
}
