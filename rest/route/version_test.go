package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// VersionSuite enables testing for version related routes.
type VersionSuite struct {
	bv, bi []string            // build variants and build indices for testing
	btc    [][]build.TaskCache // build task caches for testing
	env    evergreen.Environment
	ctx    context.Context
	cancel context.CancelFunc

	suite.Suite
}

func TestVersionSuite(t *testing.T) {
	suite.Run(t, new(VersionSuite))
}

// Declare field variables to use across routes related to version.
var versionId, revision, author, authorEmail, msg, status, repo, branch, project string

// SetupSuite sets up the test suite for routes related to version.
// More version-related routes will be implemented later.
func (s *VersionSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	env := &mock.Environment{}
	s.Require().NoError(env.Configure(s.ctx))
	s.env = env

	// Initialize values for version field variables.
	versionId = "versionId"
	revision = "revision"
	author = "author"
	authorEmail = "author_email"
	msg = "message"
	status = "status"
	repo = "repo"
	branch = "branch"
	project = "project"

	s.NoError(db.ClearCollections(task.Collection, serviceModel.VersionCollection, build.Collection, manifest.Collection))
	s.bv = append(s.bv, "buildvariant1", "buildvariant2")
	s.bi = append(s.bi, "buildId1", "buildId2")
	s.btc = [][]build.TaskCache{
		{{Id: "task1"}, {Id: "task3"}},
		{{Id: "task2"}},
	}

	// Initialize fields for a test model.Version
	buildVariants := []serviceModel.VersionBuildStatus{
		{
			BuildVariant: s.bv[0],
			BuildId:      s.bi[0],
		},
		{
			BuildVariant: s.bv[1],
			BuildId:      s.bi[1],
		},
	}
	testVersion1 := serviceModel.Version{
		Id:            versionId,
		Revision:      revision,
		Author:        author,
		AuthorEmail:   authorEmail,
		Message:       msg,
		Status:        status,
		Repo:          repo,
		Branch:        branch,
		BuildVariants: buildVariants,
		Identifier:    project,
	}

	testBuild1 := build.Build{
		Id:           s.bi[0],
		Version:      versionId,
		BuildVariant: s.bv[0],
		Tasks:        s.btc[0],
	}
	testBuild2 := build.Build{
		Id:           s.bi[1],
		Version:      versionId,
		BuildVariant: s.bv[1],
		Tasks:        s.btc[1],
	}
	versions := []serviceModel.Version{testVersion1}

	tasks := []task.Task{
		{Id: "task1", Version: versionId, Aborted: false, Status: evergreen.TaskStarted, BuildId: testBuild1.Id},
		{Id: "task2", Version: versionId, Aborted: false, Status: evergreen.TaskDispatched, BuildId: testBuild2.Id},
		{Id: "task3", Version: versionId, Aborted: false, Status: evergreen.TaskUndispatched, BuildId: testBuild1.Id},
	}

	builds := []build.Build{testBuild1, testBuild2}

	for _, item := range versions {
		s.Require().NoError(item.Insert(s.ctx))
	}
	for _, item := range tasks {
		s.Require().NoError(item.Insert(s.ctx))
	}
	for _, item := range builds {
		s.Require().NoError(item.Insert(s.ctx))
	}
}

func (s *VersionSuite) TearDownSuite() {
	s.cancel()
}

// TestFindByVersionId tests the route for finding version by its ID.
func (s *VersionSuite) TestFindByVersionId() {
	handler := &versionHandler{versionId: "versionId"}
	res := handler.Run(s.ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := res.Data().(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)
	s.Equal(utility.ToStringPtr(revision), h.Revision)
	s.Equal(utility.ToStringPtr(author), h.Author)
	s.Equal(utility.ToStringPtr(authorEmail), h.AuthorEmail)
	s.Equal(utility.ToStringPtr(msg), h.Message)
	// Status may change during other tests, so check if it's either the original status or started
	s.True(utility.FromStringPtr(h.Status) == status || utility.FromStringPtr(h.Status) == evergreen.VersionStarted)
	s.Equal(utility.ToStringPtr(repo), h.Repo)
	s.Equal(utility.ToStringPtr(branch), h.Branch)
	s.Equal(utility.ToStringPtr(project), h.Project)
}

func (s *VersionSuite) TestPatchVersionVersion() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})
	handler := &versionPatchHandler{
		versionId: versionId,
		Activated: utility.TruePtr(),
	}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	v, err := serviceModel.VersionFindOneId(s.ctx, versionId)
	s.NoError(err)
	for _, b := range v.BuildIds {
		build, err := build.FindOneId(s.ctx, b)
		s.NoError(err)
		s.True(build.Activated)
	}
	s.Equal(versionId, v.Id)
	s.Equal(revision, v.Revision)
	s.Equal(author, v.Author)
	s.Equal(authorEmail, v.AuthorEmail)
	s.Equal(msg, v.Message)
	s.Equal(evergreen.VersionStarted, v.Status)
	s.Equal(repo, v.Repo)
	s.Equal(branch, v.Branch)
	s.Equal(utility.TruePtr(), v.Activated)
}

// TestFindAllBuildsForVersion tests the route for finding all builds for a version.
func (s *VersionSuite) TestFindAllBuildsForVersion() {
	handler := &buildsForVersionHandler{
		versionId: "versionId",
		env:       s.env,
	}
	res := handler.Run(s.ctx)
	s.Equal(http.StatusOK, res.Status())
	s.Require().NotNil(res)

	builds, ok := res.Data().([]model.APIBuild)
	s.Require().True(ok)

	s.Require().Len(builds, 2)

	for idx, b := range builds {
		s.Equal(utility.ToStringPtr(s.bi[idx]), b.Id)
		s.Equal(utility.ToStringPtr(versionId), b.Version)
		s.Equal(utility.ToStringPtr(s.bv[idx]), b.BuildVariant)
		s.Zero(b.TaskCache, "task info should not be populated by default")
		s.Zero(b.StatusCounts, "task info should not be populated by default")
	}
}

func (s *VersionSuite) TestFindAllBuildsForVersionWithTaskStatuses() {
	handler := &buildsForVersionHandler{
		versionId:       "versionId",
		env:             s.env,
		includeTaskInfo: true,
	}
	res := handler.Run(s.ctx)
	s.Equal(http.StatusOK, res.Status())
	s.Require().NotNil(res)

	builds, ok := res.Data().([]model.APIBuild)
	s.Require().True(ok)

	s.Require().Len(builds, 2)

	for idx, b := range builds {
		s.Equal(utility.ToStringPtr(s.bi[idx]), b.Id)
		s.Equal(utility.ToStringPtr(versionId), b.Version)
		s.Equal(utility.ToStringPtr(s.bv[idx]), b.BuildVariant)
		s.Require().Len(b.TaskCache, len(s.btc[idx]))
	}
	s.Equal(task.TaskStatusCount{Started: 1, Undispatched: 1}, builds[0].StatusCounts)
	s.Equal(task.TaskStatusCount{Started: 1}, builds[1].StatusCounts)
}

func (s *VersionSuite) TestFindBuildsForVersionByVariant() {
	handler := &buildsForVersionHandler{
		versionId: "versionId",
		env:       s.env,
	}

	for i, variant := range s.bv {
		handler.variant = variant
		res := handler.Run(s.ctx)
		s.Equal(http.StatusOK, res.Status())
		s.NotNil(res)

		builds, ok := res.Data().([]model.APIBuild)
		s.True(ok)

		s.Require().Len(builds, 1)
		s.Equal(versionId, utility.FromStringPtr(builds[0].Version))
		s.Equal(s.bi[i], utility.FromStringPtr(builds[0].Id))
		s.Equal(variant, utility.FromStringPtr(builds[0].BuildVariant))
	}
}

// TestAbortVersion tests the route for aborting a version.
func (s *VersionSuite) TestAbortVersion() {
	handler := &versionAbortHandler{versionId: "versionId"}

	// Check that Execute runs without error and returns
	// the correct Version.
	res := handler.Run(s.ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())
	version := res.Data()
	h, ok := (version).(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)

	// Check that all tasks have been aborted.
	tasks, err := task.FindAllTaskIDsFromVersion(s.ctx, "versionId")
	s.NoError(err)
	for _, t := range tasks {
		foundTask, err := task.FindOneId(s.ctx, t)
		s.NoError(err)
		if utility.StringSliceContains(evergreen.TaskInProgressStatuses, foundTask.Status) {
			s.True(foundTask.Aborted)
		}
	}
}

// TestRestartVersion tests the route for restarting a version.
func (s *VersionSuite) TestRestartVersion() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionRestartHandler{versionId: "versionId"}

	// Check that Execute runs without error and returns
	// the correct Version.
	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	version := res.Data()
	h, ok := (version).(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)
	v, err := serviceModel.VersionFindOneId(s.ctx, "versionId")
	s.NoError(err)
	s.Equal(evergreen.VersionStarted, v.Status)
}

// TestActivateVersionTasks tests the route for activating specific tasks in a version.
func (s *VersionSuite) TestActivateVersionTasks() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	testTasks := []task.Task{
		{
			Id:           "inactive_task1",
			Version:      versionId,
			BuildVariant: s.bv[0],
			DisplayName:  "test_task_1",
			Activated:    false,
			Status:       evergreen.TaskUndispatched,
			BuildId:      s.bi[0],
		},
		{
			Id:           "inactive_task2",
			Version:      versionId,
			BuildVariant: s.bv[0],
			DisplayName:  "test_task_2",
			Activated:    false,
			Status:       evergreen.TaskUndispatched,
			BuildId:      s.bi[0],
		},
		{
			Id:           "inactive_task3",
			Version:      versionId,
			BuildVariant: s.bv[1],
			DisplayName:  "test_task_3",
			Activated:    false,
			Status:       evergreen.TaskUndispatched,
			BuildId:      s.bi[1],
		},
	}

	for _, task := range testTasks {
		s.Require().NoError(task.Insert(s.ctx))
	}

	handler := &versionActivateTasksHandler{
		versionId: versionId,
		Variants: []Variant{
			{
				Name:  s.bv[0],
				Tasks: []string{"test_task_1", "test_task_2"},
			},
			{
				Name:  s.bv[1],
				Tasks: []string{"test_task_3"},
			},
		},
	}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	// Test successful activation
	// Verify tasks were activated
	activatedTask1, err := task.FindOneId(s.ctx, "inactive_task1")
	s.NoError(err)
	s.True(activatedTask1.Activated)

	activatedTask2, err := task.FindOneId(s.ctx, "inactive_task2")
	s.NoError(err)
	s.True(activatedTask2.Activated)

	activatedTask3, err := task.FindOneId(s.ctx, "inactive_task3")
	s.NoError(err)
	s.True(activatedTask3.Activated)
}

// TestActivateVersionTasksInvalidVariant tests error handling for invalid variant/task combinations.
func (s *VersionSuite) TestActivateVersionTasksInvalidVariant() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionActivateTasksHandler{
		versionId: versionId,
		Variants: []Variant{
			{
				Name:  s.bv[0],
				Tasks: []string{"test_task_1", "test_task_2"},
			},
		},
	}

	// Test with non-existent variant/tasks
	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusBadRequest, res.Status())
}

func (s *VersionSuite) TestGetManifestForVersion() {
	mfst := manifest.Manifest{
		Id:          versionId,
		Revision:    revision,
		ProjectName: project,
		Branch:      branch,
		Modules: map[string]*manifest.Module{
			"module1": {
				Branch:   "module1_branch",
				Repo:     "module1_repo",
				Revision: "module1_revision",
				Owner:    "module1_owner",
				URL:      "module1_url",
			},
		},
	}
	exists, err := mfst.TryInsert(s.ctx)
	s.Require().NoError(err)
	s.False(exists)

	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionManifestGetHandler{versionId: "versionId"}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	apiMfst, ok := res.Data().(*model.APIManifest)
	s.Require().True(ok)
	s.Equal(versionId, utility.FromStringPtr(apiMfst.Id))
	s.Equal(revision, utility.FromStringPtr(apiMfst.Revision))
	s.Equal(project, utility.FromStringPtr(apiMfst.ProjectName))
	s.Equal(branch, utility.FromStringPtr(apiMfst.Branch))
	s.NotEmpty(apiMfst.Modules)
	s.Len(apiMfst.Modules, 1)
	expectedModule := mfst.Modules["module1"]
	s.Equal("module1", utility.FromStringPtr(apiMfst.Modules[0].Name))
	s.Equal(expectedModule.Branch, utility.FromStringPtr(apiMfst.Modules[0].Branch))
	s.Equal(expectedModule.Repo, utility.FromStringPtr(apiMfst.Modules[0].Repo))
	s.Equal(expectedModule.Revision, utility.FromStringPtr(apiMfst.Modules[0].Revision))
	s.Equal(expectedModule.Owner, utility.FromStringPtr(apiMfst.Modules[0].Owner))
	s.Equal(expectedModule.URL, utility.FromStringPtr(apiMfst.Modules[0].URL))
}

func (s *VersionSuite) TestGetManifestForVersionErrorsForNonexistentVersion() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionManifestGetHandler{versionId: "nonexistent"}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusNotFound, res.Status())
}

func TestGetVersionManifestProof(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"WithAdjacentVersions": func(t *testing.T) {
			projectID := "proof-adjacent-project"
			previousPrevious := insertProofVersion(t, projectID, "proof-adjacent-previous-previous", "project-revision-1", 1)
			previous := insertProofVersion(t, projectID, "proof-adjacent-previous", "project-revision-2", 2)
			current := insertProofVersion(t, projectID, "proof-adjacent-current", "project-revision-2", 3)
			next := insertProofVersion(t, projectID, "proof-adjacent-next", "project-revision-2", 4)
			insertProofManifest(t, previousPrevious, "module-revision-1")
			insertProofManifest(t, previous, "module-revision-1")
			insertProofManifest(t, current, "module-revision-2")
			insertProofManifest(t, next, "module-revision-3")

			handler := &versionManifestProofGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			proof, ok := res.Data().(*versionManifestProofResponse)
			require.True(t, ok)
			require.NotNil(t, proof.BeforePrevious)
			require.NotNil(t, proof.Previous)
			require.NotNil(t, proof.Current)
			require.NotNil(t, proof.Next)

			assert.Equal(t, previousPrevious.Id, proof.BeforePrevious.VersionID)
			assert.Equal(t, previousPrevious.Requester, proof.BeforePrevious.Requester)
			assert.Equal(t, previousPrevious.Revision, proof.BeforePrevious.ProjectRevision)
			assert.Equal(t, previousPrevious.RevisionOrderNumber, proof.BeforePrevious.Order)

			assert.Equal(t, previous.Id, proof.Previous.VersionID)
			assert.True(t, proof.Previous.HasComparison)
			assert.True(t, proof.Previous.ManifestFound)
			assert.True(t, proof.Previous.ProjectRevisionChanged)
			require.Len(t, proof.Previous.Modules, 1)
			assert.False(t, proof.Previous.Modules[0].Changed)

			assert.Equal(t, current.Id, proof.Current.VersionID)
			assert.True(t, proof.Current.HasComparison)
			assert.True(t, proof.Current.ManifestFound)
			assert.False(t, proof.Current.ProjectRevisionChanged)
			currentModule := proofModuleByName(proof.Current.Modules, "module1")
			require.NotNil(t, currentModule)
			assert.True(t, currentModule.Changed)
			assert.Equal(t, "module-revision-2", utility.FromStringPtr(currentModule.Revision))

			assert.Equal(t, next.Id, proof.Next.VersionID)
			assert.True(t, proof.Next.HasComparison)
			assert.True(t, proof.Next.ManifestFound)
			assert.False(t, proof.Next.ProjectRevisionChanged)
			nextModule := proofModuleByName(proof.Next.Modules, "module1")
			require.NotNil(t, nextModule)
			assert.True(t, nextModule.Changed)
			assert.Equal(t, "module-revision-3", utility.FromStringPtr(nextModule.Revision))
		},
		"WithOnlyCurrentVersion": func(t *testing.T) {
			projectID := "proof-current-only-project"
			current := insertProofVersion(t, projectID, "proof-current-only-current", "project-revision-1", 1)
			insertProofManifest(t, current, "module-revision-1")

			handler := &versionManifestProofGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			proof, ok := res.Data().(*versionManifestProofResponse)
			require.True(t, ok)
			assert.Nil(t, proof.BeforePrevious)
			assert.Nil(t, proof.Previous)
			assert.Nil(t, proof.Next)
			require.NotNil(t, proof.Current)
			assert.False(t, proof.Current.HasComparison)
			assert.True(t, proof.Current.ManifestFound)
			assert.False(t, proof.Current.ProjectRevisionChanged)
			require.Len(t, proof.Current.Modules, 1)
			assert.False(t, proof.Current.Modules[0].Changed)
		},
		"WithMissingPreviousPreviousAndNext": func(t *testing.T) {
			projectID := "proof-missing-previous-previous-project"
			previous := insertProofVersion(t, projectID, "proof-missing-previous-previous-previous", "project-revision-1", 1)
			current := insertProofVersion(t, projectID, "proof-missing-previous-previous-current", "project-revision-2", 2)
			insertProofManifest(t, previous, "module-revision-1")
			insertProofManifest(t, current, "module-revision-2")

			handler := &versionManifestProofGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			proof, ok := res.Data().(*versionManifestProofResponse)
			require.True(t, ok)
			assert.Nil(t, proof.BeforePrevious)
			require.NotNil(t, proof.Previous)
			require.NotNil(t, proof.Current)
			assert.Nil(t, proof.Next)

			assert.False(t, proof.Previous.HasComparison)
			assert.False(t, proof.Previous.ProjectRevisionChanged)
			require.Len(t, proof.Previous.Modules, 1)
			assert.False(t, proof.Previous.Modules[0].Changed)

			assert.True(t, proof.Current.HasComparison)
			assert.True(t, proof.Current.ProjectRevisionChanged)
			currentModule := proofModuleByName(proof.Current.Modules, "module1")
			require.NotNil(t, currentModule)
			assert.True(t, currentModule.Changed)
		},
		"ReturnsProjectDataWithoutCurrentManifest": func(t *testing.T) {
			projectID := "proof-missing-current-manifest-project"
			current := insertProofVersion(t, projectID, "proof-missing-current-manifest-current", "project-revision-1", 1)

			handler := &versionManifestProofGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			proof, ok := res.Data().(*versionManifestProofResponse)
			require.True(t, ok)
			assert.Nil(t, proof.BeforePrevious)
			assert.Nil(t, proof.Previous)
			assert.Nil(t, proof.Next)
			require.NotNil(t, proof.Current)
			assert.Equal(t, current.Id, proof.Current.VersionID)
			assert.Equal(t, current.Revision, proof.Current.ProjectRevision)
			assert.False(t, proof.Current.ManifestFound)
			assert.Empty(t, proof.Current.Modules)
		},
		"HandlesMissingNeighborManifest": func(t *testing.T) {
			projectID := "proof-missing-neighbor-manifest-project"
			previous := insertProofVersion(t, projectID, "proof-missing-neighbor-manifest-previous", "project-revision-1", 1)
			current := insertProofVersion(t, projectID, "proof-missing-neighbor-manifest-current", "project-revision-2", 2)
			insertProofManifest(t, current, "module-revision-1")

			handler := &versionManifestProofGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			proof, ok := res.Data().(*versionManifestProofResponse)
			require.True(t, ok)
			require.NotNil(t, proof.Previous)
			require.NotNil(t, proof.Current)
			assert.Equal(t, previous.Id, proof.Previous.VersionID)
			assert.False(t, proof.Previous.ManifestFound)
			assert.Empty(t, proof.Previous.Modules)
			assert.True(t, proof.Current.HasComparison)
			assert.True(t, proof.Current.ProjectRevisionChanged)
			assert.True(t, proof.Current.ManifestFound)
			require.Len(t, proof.Current.Modules, 1)
			assert.False(t, proof.Current.Modules[0].Changed)
		},
		"ErrorsForNonexistentVersion": func(t *testing.T) {
			handler := &versionManifestProofGetHandler{versionId: "proof-nonexistent-version"}

			res := handler.Run(t.Context())
			require.NotNil(t, res)
			assert.Equal(t, http.StatusNotFound, res.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(serviceModel.VersionCollection, manifest.Collection))
			tCase(t)
		})
	}
}

func TestGetVersionManifestProofHistory(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsVersionsNewestToOldestLimitedToTwenty": func(t *testing.T) {
			projectID := "proof-history-limit-project"
			versions := make([]serviceModel.Version, 0, 22)
			for i := 1; i <= 22; i++ {
				v := insertProofVersion(t, projectID, fmt.Sprintf("proof-history-limit-%d", i), fmt.Sprintf("project-revision-%d", i), i)
				insertProofManifest(t, v, "module-revision")
				versions = append(versions, v)
			}

			handler := &versionManifestProofHistoryGetHandler{versionId: versions[21].Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			history, ok := res.Data().(*versionManifestProofHistoryResponse)
			require.True(t, ok)
			require.Len(t, history.Versions, versionManifestProofHistoryLimit)
			assert.Equal(t, versions[21].Id, history.Versions[0].Version.VersionID)
			assert.Equal(t, versions[2].Id, history.Versions[19].Version.VersionID)
			assert.True(t, history.Versions[19].Version.HasComparison)
			assert.True(t, history.Versions[19].OnlyOneRevisionChanged)
			assert.Equal(t, 1, history.Versions[19].ChangedRevisionCount)
		},
		"CalculatesExactOneRevisionChanged": func(t *testing.T) {
			projectID := "proof-history-changed-project"
			base := insertProofVersion(t, projectID, "proof-history-changed-base", "project-revision-1", 1)
			projectChanged := insertProofVersion(t, projectID, "proof-history-changed-project", "project-revision-2", 2)
			moduleChanged := insertProofVersion(t, projectID, "proof-history-changed-module", "project-revision-2", 3)
			projectAndModuleChanged := insertProofVersion(t, projectID, "proof-history-changed-project-and-module", "project-revision-3", 4)
			unchanged := insertProofVersion(t, projectID, "proof-history-changed-unchanged", "project-revision-3", 5)
			insertProofManifest(t, base, "module-revision-1")
			insertProofManifest(t, projectChanged, "module-revision-1")
			insertProofManifest(t, moduleChanged, "module-revision-2")
			insertProofManifest(t, projectAndModuleChanged, "module-revision-3")
			insertProofManifest(t, unchanged, "module-revision-3")

			handler := &versionManifestProofHistoryGetHandler{versionId: unchanged.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			history, ok := res.Data().(*versionManifestProofHistoryResponse)
			require.True(t, ok)
			require.Len(t, history.Versions, 5)

			unchangedItem := proofHistoryItemByID(history.Versions, unchanged.Id)
			require.NotNil(t, unchangedItem)
			assert.False(t, unchangedItem.OnlyOneRevisionChanged)
			assert.Equal(t, 0, unchangedItem.ChangedRevisionCount)
			assert.False(t, unchangedItem.ModulesChanged)

			projectAndModuleChangedItem := proofHistoryItemByID(history.Versions, projectAndModuleChanged.Id)
			require.NotNil(t, projectAndModuleChangedItem)
			assert.False(t, projectAndModuleChangedItem.OnlyOneRevisionChanged)
			assert.Equal(t, 2, projectAndModuleChangedItem.ChangedRevisionCount)
			assert.False(t, projectAndModuleChangedItem.ModulesChanged)

			moduleChangedItem := proofHistoryItemByID(history.Versions, moduleChanged.Id)
			require.NotNil(t, moduleChangedItem)
			assert.True(t, moduleChangedItem.OnlyOneRevisionChanged)
			assert.Equal(t, 1, moduleChangedItem.ChangedRevisionCount)
			assert.False(t, moduleChangedItem.ModulesChanged)

			projectChangedItem := proofHistoryItemByID(history.Versions, projectChanged.Id)
			require.NotNil(t, projectChangedItem)
			assert.True(t, projectChangedItem.OnlyOneRevisionChanged)
			assert.Equal(t, 1, projectChangedItem.ChangedRevisionCount)
			assert.False(t, projectChangedItem.ModulesChanged)

			baseItem := proofHistoryItemByID(history.Versions, base.Id)
			require.NotNil(t, baseItem)
			assert.False(t, baseItem.OnlyOneRevisionChanged)
			assert.Equal(t, 0, baseItem.ChangedRevisionCount)
			assert.False(t, baseItem.ModulesChanged)
		},
		"ModuleAppearOrDisappearSetsModulesChangedWithoutCountingRevision": func(t *testing.T) {
			projectID := "proof-history-module-membership-project"
			base := insertProofVersion(t, projectID, "proof-history-module-membership-base", "project-revision-1", 1)
			moduleAdded := insertProofVersion(t, projectID, "proof-history-module-membership-added", "project-revision-1", 2)
			moduleRemoved := insertProofVersion(t, projectID, "proof-history-module-membership-removed", "project-revision-1", 3)
			moduleAddedWithProjectChange := insertProofVersion(t, projectID, "proof-history-module-membership-added-project", "project-revision-2", 4)

			insertProofManifestWithModules(t, base, map[string]*manifest.Module{
				"module1": {Branch: "main", Repo: "module-repo", Revision: "module-revision-1", Owner: "module-owner", URL: "module-url"},
			})
			insertProofManifestWithModules(t, moduleAdded, map[string]*manifest.Module{
				"module1": {Branch: "main", Repo: "module-repo", Revision: "module-revision-1", Owner: "module-owner", URL: "module-url"},
				"module2": {Branch: "main", Repo: "module-repo-2", Revision: "module-revision-2", Owner: "module-owner", URL: "module-url-2"},
			})
			insertProofManifestWithModules(t, moduleRemoved, map[string]*manifest.Module{
				"module1": {Branch: "main", Repo: "module-repo", Revision: "module-revision-1", Owner: "module-owner", URL: "module-url"},
			})
			insertProofManifestWithModules(t, moduleAddedWithProjectChange, map[string]*manifest.Module{
				"module1": {Branch: "main", Repo: "module-repo", Revision: "module-revision-1", Owner: "module-owner", URL: "module-url"},
				"module2": {Branch: "main", Repo: "module-repo-2", Revision: "module-revision-2", Owner: "module-owner", URL: "module-url-2"},
			})

			handler := &versionManifestProofHistoryGetHandler{versionId: moduleAddedWithProjectChange.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			history, ok := res.Data().(*versionManifestProofHistoryResponse)
			require.True(t, ok)
			require.Len(t, history.Versions, 4)

			addedWithProjectItem := proofHistoryItemByID(history.Versions, moduleAddedWithProjectChange.Id)
			require.NotNil(t, addedWithProjectItem)
			assert.True(t, addedWithProjectItem.OnlyOneRevisionChanged)
			assert.Equal(t, 1, addedWithProjectItem.ChangedRevisionCount)
			assert.True(t, addedWithProjectItem.ModulesChanged)

			removedItem := proofHistoryItemByID(history.Versions, moduleRemoved.Id)
			require.NotNil(t, removedItem)
			assert.False(t, removedItem.OnlyOneRevisionChanged)
			assert.Equal(t, 0, removedItem.ChangedRevisionCount)
			assert.True(t, removedItem.ModulesChanged)

			addedItem := proofHistoryItemByID(history.Versions, moduleAdded.Id)
			require.NotNil(t, addedItem)
			assert.False(t, addedItem.OnlyOneRevisionChanged)
			assert.Equal(t, 0, addedItem.ChangedRevisionCount)
			assert.True(t, addedItem.ModulesChanged)
		},
		"IgnoresNonSystemVersionsInHistory": func(t *testing.T) {
			projectID := "proof-history-ignore-patch-project"
			previous := insertProofVersion(t, projectID, "proof-history-ignore-patch-previous", "project-revision-1", 1)
			patchVersion := insertProofVersionWithRequester(t, projectID, "proof-history-ignore-patch-patch", evergreen.PatchVersionRequester, "project-revision-2", 2)
			current := insertProofVersion(t, projectID, "proof-history-ignore-patch-current", "project-revision-3", 3)
			insertProofManifest(t, previous, "module-revision-1")
			insertProofManifest(t, patchVersion, "module-revision-2")
			insertProofManifest(t, current, "module-revision-1")

			handler := &versionManifestProofHistoryGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			history, ok := res.Data().(*versionManifestProofHistoryResponse)
			require.True(t, ok)
			require.Len(t, history.Versions, 2)
			assert.Equal(t, current.Id, history.Versions[0].Version.VersionID)
			assert.Equal(t, previous.Id, history.Versions[1].Version.VersionID)
			assert.True(t, history.Versions[0].OnlyOneRevisionChanged)
			assert.Equal(t, 1, history.Versions[0].ChangedRevisionCount)
		},
		"HandlesMissingManifests": func(t *testing.T) {
			projectID := "proof-history-missing-manifest-project"
			previous := insertProofVersion(t, projectID, "proof-history-missing-manifest-previous", "project-revision-1", 1)
			current := insertProofVersion(t, projectID, "proof-history-missing-manifest-current", "project-revision-2", 2)
			insertProofManifest(t, previous, "module-revision-1")

			handler := &versionManifestProofHistoryGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			history, ok := res.Data().(*versionManifestProofHistoryResponse)
			require.True(t, ok)
			require.Len(t, history.Versions, 2)
			assert.False(t, history.Versions[0].Version.ManifestFound)
			assert.True(t, history.Versions[0].OnlyOneRevisionChanged)
			assert.Equal(t, 1, history.Versions[0].ChangedRevisionCount)
		},
		"HandlesMissingManifestsWithoutProjectRevisionChange": func(t *testing.T) {
			projectID := "proof-history-missing-manifest-no-change-project"
			insertProofVersion(t, projectID, "proof-history-missing-manifest-no-change-previous", "project-revision-1", 1)
			current := insertProofVersion(t, projectID, "proof-history-missing-manifest-no-change-current", "project-revision-1", 2)

			handler := &versionManifestProofHistoryGetHandler{versionId: current.Id}
			res := handler.Run(t.Context())
			require.NotNil(t, res)
			require.Equal(t, http.StatusOK, res.Status(), res.Data())

			history, ok := res.Data().(*versionManifestProofHistoryResponse)
			require.True(t, ok)
			require.Len(t, history.Versions, 2)
			assert.False(t, history.Versions[0].Version.ManifestFound)
			assert.False(t, history.Versions[0].OnlyOneRevisionChanged)
			assert.Equal(t, 0, history.Versions[0].ChangedRevisionCount)
		},
		"ErrorsForNonexistentVersion": func(t *testing.T) {
			handler := &versionManifestProofHistoryGetHandler{versionId: "proof-history-nonexistent-version"}

			res := handler.Run(t.Context())
			require.NotNil(t, res)
			assert.Equal(t, http.StatusNotFound, res.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(serviceModel.VersionCollection, manifest.Collection))
			tCase(t)
		})
	}
}

func insertProofVersion(t *testing.T, projectID, versionID, revision string, order int) serviceModel.Version {
	return insertProofVersionWithRequester(t, projectID, versionID, evergreen.RepotrackerVersionRequester, revision, order)
}

func insertProofVersionWithRequester(t *testing.T, projectID, versionID, requester, revision string, order int) serviceModel.Version {
	t.Helper()
	ts := time.Date(2026, time.June, 16, 12, order, 0, 0, time.UTC)
	v := serviceModel.Version{
		Id:                  versionID,
		Identifier:          projectID,
		Requester:           requester,
		Revision:            revision,
		RevisionOrderNumber: order,
		CreateTime:          ts,
		IngestTime:          ts.Add(time.Second),
	}
	require.NoError(t, v.Insert(t.Context()))
	return v
}

func insertProofManifest(t *testing.T, v serviceModel.Version, moduleRevision string) {
	t.Helper()
	insertProofManifestWithModules(t, v, map[string]*manifest.Module{
		"module1": {
			Branch:   "main",
			Repo:     "module-repo",
			Revision: moduleRevision,
			Owner:    "module-owner",
			URL:      "module-url",
		},
	})
}

func insertProofManifestWithModules(t *testing.T, v serviceModel.Version, modules map[string]*manifest.Module) {
	t.Helper()
	mfst := manifest.Manifest{
		Id:          v.Id,
		Revision:    v.Revision,
		ProjectName: v.Identifier,
		Branch:      "main",
		IsBase:      true,
		Modules:     modules,
	}
	exists, err := mfst.TryInsert(t.Context())
	require.NoError(t, err)
	assert.False(t, exists)
}

func proofModuleByName(modules []versionManifestProofModule, name string) *versionManifestProofModule {
	for i := range modules {
		if utility.FromStringPtr(modules[i].Name) == name {
			return &modules[i]
		}
	}
	return nil
}

func proofHistoryItemByID(items []versionManifestProofHistoryItem, versionID string) *versionManifestProofHistoryItem {
	for i := range items {
		if items[i].Version.VersionID == versionID {
			return &items[i]
		}
	}
	return nil
}
