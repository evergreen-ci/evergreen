package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PrefetchProjectContext gets the information related to the project that the request contains
// and fetches the associated project context and attaches that to the request context.
func PrefetchProjectContext(ctx context.Context, r *http.Request, input map[string]string) (context.Context, error) {
	r = r.WithContext(ctx)
	if input != nil {
		r = gimlet.SetURLVars(r, input)
	}
	rw := httptest.NewRecorder()
	NewProjectContextMiddleware().ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	})

	if rw.Code != http.StatusOK {
		return ctx, gimlet.ErrorResponse{
			StatusCode: rw.Code,
			Message:    "not found",
		}
	}

	return ctx, nil
}

func TestPrefetchProject(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, patch.Collection, user.Collection))
	doc := &model.ProjectRef{
		Id:      "mci",
		Private: utility.TruePtr(),
	}
	require.NoError(t, doc.Insert())
	patchDoc := &patch.Patch{
		Id: bson.ObjectIdHex("aabbccddeeff112233445566"),
	}
	require.NoError(t, patchDoc.Insert())
	testUser := &user.DBUser{Id: "test_user"}
	require.NoError(t, testUser.Insert())
	Convey("When there is a data and a request", t, func() {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		Convey("When fetching the project context", func() {
			ctx := context.Background()
			Convey("should error if project is private and no user is set", func() {
				ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"project_id": "mci"})
				So(ctx.Value(RequestContext), ShouldBeNil)

				errToResemble := gimlet.ErrorResponse{
					StatusCode: http.StatusNotFound,
					Message:    "not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should error if patch exists and no user is set", func() {
				opCtx := model.Context{}
				opCtx.Patch = &patch.Patch{}
				ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"patch_id": "aabbccddeeff112233445566"})
				So(ctx.Value(RequestContext), ShouldBeNil)

				errToResemble := gimlet.ErrorResponse{
					StatusCode: http.StatusNotFound,
					Message:    "not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should succeed if project ref exists and user is set", func() {
				opCtx := model.Context{}
				opCtx.ProjectRef = &model.ProjectRef{
					Private: utility.TruePtr(),
					Id:      "mci",
				}
				ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})
				ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"project_id": "mci"})
				So(err, ShouldBeNil)

				So(ctx.Value(RequestContext), ShouldResemble, &opCtx)
			})
		})
	})
}

func TestNewProjectAdminMiddleware(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private: utility.TruePtr(),
		Id:      "orchard",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		Admins:  []string{"johnny.appleseed"},
	}
	adminRole := gimlet.Role{
		ID:          "r1",
		Scope:       "orchard",
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(adminRole))
	adminScope := gimlet.Scope{ID: "orchard", Resources: []string{"orchard"}, Type: evergreen.ProjectResourceType}
	assert.NoError(env.RoleManager().AddScope(adminScope))

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "not.admin"})
	r, err := http.NewRequest(http.MethodGet, "/projects/orchard", nil)
	assert.NoError(err)
	assert.NotNil(r)

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	mw := NewProjectAdminMiddleware()
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "johnny.appleseed", SystemRoles: []string{"r1"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	rw = httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestNewCanCreateMiddleware(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(evergreen.RoleCollection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	adminRole := gimlet.Role{
		ID:          "r1",
		Scope:       "anything",
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(adminRole))

	opCtx := model.Context{}

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "not.admin"})
	r, err := http.NewRequest(http.MethodPut, "/projects/makeFromRoute", nil)
	assert.NoError(err)
	assert.NotNil(r)
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	mw := NewCanCreateMiddleware()
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "johnny.appleseed", SystemRoles: []string{"r1"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	rw = httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewarePROwner(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.ProjectRefCollection, commitqueue.Collection))

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private: utility.TruePtr(),
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}

	assert.NoError(opCtx.ProjectRef.Insert())
	cq := commitqueue.CommitQueue{
		ProjectID: opCtx.ProjectRef.Id,
		Queue: []commitqueue.CommitQueueItem{
			{Issue: "1234", Source: commitqueue.SourcePullRequest},
		},
	}
	assert.NoError(commitqueue.InsertQueue(&cq))

	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID: 1234,
			},
		},
	})

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       "1234",
	})

	mw := NewMockCommitQueueItemOwnerMiddleware()
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewareUnauthorizedUserGitHub(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.ProjectRefCollection, commitqueue.Collection))

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private: utility.TruePtr(),
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	assert.NoError(opCtx.ProjectRef.Insert())
	cq := commitqueue.CommitQueue{
		ProjectID: opCtx.ProjectRef.Id,
		Queue: []commitqueue.CommitQueueItem{
			{Issue: "1234", Source: commitqueue.SourcePullRequest},
		},
	}
	assert.NoError(commitqueue.InsertQueue(&cq))

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID: 4321,
			},
		},
	})

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       "1234",
	})

	mw := NewMockCommitQueueItemOwnerMiddleware()
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewareUserPatch(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(patch.Collection, model.ProjectRefCollection, commitqueue.Collection, user.Collection))

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private: utility.TruePtr(),
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	assert.NoError(opCtx.ProjectRef.Insert())

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	patchUsr := &user.DBUser{Id: "octocat", OnlyAPI: false}
	require.NoError(t, patchUsr.Insert())

	patchId := bson.NewObjectId()
	p := &patch.Patch{
		Id:     patchId,
		Author: patchUsr.Id,
	}
	assert.NoError(p.Insert())

	cq := commitqueue.CommitQueue{
		ProjectID: opCtx.ProjectRef.Id,
		Queue: []commitqueue.CommitQueueItem{
			{Issue: patchId.Hex(), Source: commitqueue.SourceDiff},
		},
	}
	assert.NoError(commitqueue.InsertQueue(&cq))

	p, err = patch.FindOne(patch.ByUserAndCommitQueue("octocat", false))
	assert.NoError(err)
	assert.NotNil(p)

	// not authorized
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       p.Id.Hex(),
	})

	mw := NewCommitQueueItemOwnerMiddleware()

	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	// authorized
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "octocat"})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       p.Id.Hex(),
	})
	rw = httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewarePatchAdmin(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(patch.Collection, model.ProjectRefCollection, commitqueue.Collection,
		evergreen.ScopeCollection, evergreen.RoleCollection))

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private: utility.TruePtr(),
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	assert.NoError(opCtx.ProjectRef.Insert())

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	patchId := bson.NewObjectId()
	p := &patch.Patch{
		Id:     patchId,
		Author: "octocat",
	}
	assert.NoError(p.Insert())
	cq := commitqueue.CommitQueue{
		ProjectID: opCtx.ProjectRef.Id,
		Queue: []commitqueue.CommitQueueItem{
			{Issue: patchId.Hex(), Source: commitqueue.SourceDiff},
		},
	}
	assert.NoError(commitqueue.InsertQueue(&cq))

	p, err = patch.FindOne(patch.ByUserAndCommitQueue("octocat", false))
	assert.NoError(err)
	assert.NotNil(p)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Id:          "me",
		SystemRoles: []string{"patch_admin"},
	})
	rm := evergreen.GetEnvironment().RoleManager()
	assert.NoError(rm.AddScope(gimlet.Scope{
		ID:        "projects",
		Resources: []string{"mci"},
		Type:      evergreen.ProjectResourceType,
	}))
	assert.NoError(rm.UpdateRole(gimlet.Role{
		ID:    "patch_admin",
		Scope: "projects",
		Permissions: gimlet.Permissions{
			evergreen.PermissionPatches: evergreen.PatchSubmitAdmin.Value,
		},
	}))

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       p.Id.Hex(),
	})
	rw := httptest.NewRecorder()

	mw := NewCommitQueueItemOwnerMiddleware()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestTaskAuthMiddleware(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	assert.NoError(db.ClearCollections(host.Collection, task.Collection))
	task1 := task.Task{
		Id:     "task1",
		Secret: "abcdef",
	}
	host1 := &host.Host{
		Id:     "host1",
		Secret: "abcdef",
	}
	assert.NoError(task1.Insert())
	assert.NoError(host1.Insert(ctx))
	m := NewTaskAuthMiddleware()
	r := &http.Request{
		Header: http.Header{
			evergreen.HostHeader:       []string{"host1"},
			evergreen.HostSecretHeader: []string{"abcdef"},
			evergreen.TaskHeader:       []string{"task1"},
		},
	}

	rw := httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusConflict, rw.Code)

	r.Header.Set(evergreen.TaskSecretHeader, "abcdef")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestHostAuthMiddleware(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewHostAuthMiddleware()
	for testName, testCase := range map[string]func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder){
		"Succeeds": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.Equal(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidSecret": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{"foo"},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutHostID": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidHostID": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{"foo"},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		// "": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(host.Collection))
			defer func() {
				assert.NoError(t, db.Clear(host.Collection))
			}()
			h := &host.Host{
				Id:     "id",
				Secret: "secret",
			}
			require.NoError(t, h.Insert(ctx))

			testCase(t, h, httptest.NewRecorder())
		})
	}
}

func TestPodAuthMiddleware(t *testing.T) {
	m := NewPodAuthMiddleware()
	for testName, testCase := range map[string]func(t *testing.T, p *pod.Pod, rw *httptest.ResponseRecorder){
		"Succeeds": func(t *testing.T, p *pod.Pod, rw *httptest.ResponseRecorder) {
			s, err := p.GetSecret()
			require.NoError(t, err)
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader:       []string{p.ID},
					evergreen.PodSecretHeader: []string{s.Value},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.Equal(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutSecret": func(t *testing.T, p *pod.Pod, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader: []string{p.ID},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidSecret": func(t *testing.T, p *pod.Pod, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader:       []string{p.ID},
					evergreen.PodSecretHeader: []string{"foo"},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutPodID": func(t *testing.T, p *pod.Pod, rw *httptest.ResponseRecorder) {
			s, err := p.GetSecret()
			require.NoError(t, err)
			r := &http.Request{
				Header: http.Header{
					evergreen.PodSecretHeader: []string{s.Value},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidPodID": func(t *testing.T, p *pod.Pod, rw *httptest.ResponseRecorder) {
			s, err := p.GetSecret()
			require.NoError(t, err)
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader:       []string{"foo"},
					evergreen.PodSecretHeader: []string{s.Value},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(pod.Collection))
			defer func() {
				assert.NoError(t, db.Clear(pod.Collection))
			}()
			p := &pod.Pod{
				ID: "id",
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					EnvSecrets: map[string]pod.Secret{
						pod.PodSecretEnvVar: {
							ExternalID: "external_id",
							Value:      "value",
						},
					},
				},
			}
			require.NoError(t, p.Insert())

			testCase(t, p, httptest.NewRecorder())
		})
	}
}

func TestPodOrHostAuthMiddleware(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewPodOrHostAuthMiddleWare()
	for testName, testCase := range map[string]func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder){
		"SucceedsWithPod": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			s, err := p.GetSecret()
			require.NoError(t, err)
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader:       []string{p.ID},
					evergreen.PodSecretHeader: []string{s.Value},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.Equal(t, http.StatusOK, rw.Code)
		},
		"SucceedsWithHost": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.Equal(t, http.StatusOK, rw.Code)
		},
		"FailsWithPodAndHostHeaders": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			s, err := p.GetSecret()
			require.NoError(t, err)
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader:        []string{p.ID},
					evergreen.PodSecretHeader:  []string{s.Value},
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithNoHeaders": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutPodSecret": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader: []string{p.ID},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutHostSecret": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader: []string{h.Id},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidPodSecret": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader:       []string{p.ID},
					evergreen.PodSecretHeader: []string{"foo"},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidHostSecret": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{"foo"},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutPodID": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			s, err := p.GetSecret()
			require.NoError(t, err)
			r := &http.Request{
				Header: http.Header{
					evergreen.PodSecretHeader: []string{s.Value},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutHostID": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidPodID": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			s, err := p.GetSecret()
			require.NoError(t, err)
			r := &http.Request{
				Header: http.Header{
					evergreen.PodHeader:       []string{"foo"},
					evergreen.PodSecretHeader: []string{s.Value},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidHostID": func(t *testing.T, p *pod.Pod, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{"foo"},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(pod.Collection, host.Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(pod.Collection, host.Collection))
			}()
			p := &pod.Pod{
				ID: "id",
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					EnvSecrets: map[string]pod.Secret{
						pod.PodSecretEnvVar: {
							ExternalID: "external_id",
							Value:      "value",
						},
					},
				},
			}
			h := &host.Host{
				Id:     "id",
				Secret: "secret",
			}
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, p.Insert())

			testCase(t, p, h, httptest.NewRecorder())
		})
	}
}

func TestProjectViewPermission(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require := require.New(t)
	counter := 0
	counterFunc := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, model.ProjectRefCollection))
	require.NoError(db.CreateCollections(evergreen.ScopeCollection))
	role1 := gimlet.Role{
		ID:          "r1",
		Scope:       "proj1",
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(role1))
	defaultRole := gimlet.Role{
		ID:          evergreen.UnauthedUserRoles[0],
		Scope:       "all",
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(defaultRole))
	scope1 := gimlet.Scope{
		ID:        "proj1",
		Resources: []string{"proj1"},
		Type:      "project",
	}
	assert.NoError(env.RoleManager().AddScope(scope1))
	scopeAll := gimlet.Scope{
		ID:        "all",
		Resources: []string{"proj1", "proj2"},
		Type:      "project",
	}
	assert.NoError(env.RoleManager().AddScope(scopeAll))
	proj1 := model.ProjectRef{
		Id: "proj1",
	}
	proj2 := model.ProjectRef{
		Id:      "proj2",
		Private: utility.FalsePtr(),
	}
	assert.NoError(proj1.Insert())
	assert.NoError(proj2.Insert())
	permissionMiddleware := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView)
	checkPermission := func(rw http.ResponseWriter, r *http.Request) {
		permissionMiddleware.ServeHTTP(rw, r, counterFunc)
	}
	authenticator := gimlet.NewBasicAuthenticator(nil, nil)
	opts, err := gimlet.NewBasicUserOptions("user")
	require.NoError(err)
	user := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").RoleManager(env.RoleManager()))
	um, err := gimlet.NewBasicUserManager([]gimlet.BasicUser{*user}, env.RoleManager())
	assert.NoError(err)
	authHandler := gimlet.NewAuthenticationHandler(authenticator, um)
	req := httptest.NewRequest(http.MethodGet, "http://foo.com/bar", nil)

	// no project should 404
	rw := httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusNotFound, rw.Code)
	assert.Equal(0, counter)

	// public project should return 200 even with no user
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "proj2"})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)

	// private project with no user attached should 401
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "proj1"})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(1, counter)

	// attach a user, but with no permissions yet
	ctx = gimlet.AttachUser(req.Context(), user)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(1, counter)

	// give user the right permissions
	opts, err = gimlet.NewBasicUserOptions("user")
	require.NoError(err)
	user = gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").Roles(role1.ID).RoleManager(env.RoleManager()))
	_, err = um.GetOrCreateUser(user)
	assert.NoError(err)
	ctx = gimlet.AttachUser(req.Context(), user)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(2, counter)
}

func TestEventLogPermission(t *testing.T) {
	//setup
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require := require.New(t)
	counter := 0
	counterFunc := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, model.ProjectRefCollection, distro.Collection))
	require.NoError(db.CreateCollections(evergreen.ScopeCollection))
	projRole := gimlet.Role{
		ID:          "proj",
		Scope:       "proj1",
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(projRole))
	distroRole := gimlet.Role{
		ID:          "distro",
		Scope:       "distro1",
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(distroRole))
	superuserRole := gimlet.Role{
		ID:          "superuser",
		Scope:       "superuser",
		Permissions: map[string]int{evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(superuserRole))
	scope1 := gimlet.Scope{
		ID:        "proj1",
		Resources: []string{"proj1"},
		Type:      evergreen.ProjectResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(scope1))
	scope2 := gimlet.Scope{
		ID:        "distro1",
		Resources: []string{"distro1"},
		Type:      evergreen.DistroResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(scope2))
	scope3 := gimlet.Scope{
		ID:        "superuser",
		Resources: []string{evergreen.SuperUserPermissionsID},
		Type:      evergreen.SuperUserResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(scope3))
	proj1 := model.ProjectRef{
		Id:      "proj1",
		Private: utility.TruePtr(),
	}
	assert.NoError(proj1.Insert())
	distro1 := distro.Distro{
		Id: "distro1",
	}
	assert.NoError(distro1.Insert(ctx))
	permissionMiddleware := EventLogPermissionsMiddleware{}
	checkPermission := func(rw http.ResponseWriter, r *http.Request) {
		permissionMiddleware.ServeHTTP(rw, r, counterFunc)
	}
	authenticator := gimlet.NewBasicAuthenticator(nil, nil)
	opts, err := gimlet.NewBasicUserOptions("user")
	require.NoError(err)
	user := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").Roles(projRole.ID, distroRole.ID, superuserRole.ID).RoleManager(env.RoleManager()))
	um, err := gimlet.NewBasicUserManager([]gimlet.BasicUser{*user}, env.RoleManager())
	assert.NoError(err)
	authHandler := gimlet.NewAuthenticationHandler(authenticator, um)
	req := httptest.NewRequest(http.MethodGet, "http://foo.com/bar", nil)

	// no user + private project should 401
	rw := httptest.NewRecorder()
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.EventResourceTypeProject, "resource_id": proj1.Id})
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// have user, project event
	req = req.WithContext(gimlet.AttachUser(req.Context(), user))
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.EventResourceTypeProject, "resource_id": proj1.Id})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)

	// distro event
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.ResourceTypeDistro, "resource_id": distro1.Id})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(2, counter)

	// superuser event
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.ResourceTypeAdmin, "resource_id": evergreen.SuperUserPermissionsID})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(3, counter)
}
