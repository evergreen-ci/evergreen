package route

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupVariantQuarantineMockTSS(t *testing.T) {
	original := evergreen.GetEnvironment().Settings().TestSelection.URL
	var quarantined atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "transition_variant") {
			quarantined.Store(r.URL.Query().Get("is_manually_quarantined") == "true")
			_, _ = w.Write([]byte("null"))
			return
		}
		if strings.Contains(r.URL.Path, "get_variant_state") {
			state := "stable"
			if quarantined.Load() {
				state = "manually_quarantined"
			}
			_, _ = fmt.Fprintf(w, `{"task_a":{"task_name":"task_a","test_stats":{"test_1":{"state":"%s"}}}}`, state)
			return
		}
		_, _ = w.Write([]byte("null"))
	}))
	evergreen.GetEnvironment().Settings().TestSelection.URL = srv.URL
	t.Cleanup(func() {
		srv.Close()
		evergreen.GetEnvironment().Settings().TestSelection.URL = original
	})
}

func newRequestWithVars(vars map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r = gimlet.SetURLVars(r, vars)
	return r
}

func ctxWithProjectRef(ref *serviceModel.ProjectRef) context.Context {
	ctx := context.WithValue(context.Background(), RequestContext, &serviceModel.Context{ProjectRef: ref})
	return gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})
}

func TestVariantQuarantineSucceeds(t *testing.T) {
	setupVariantQuarantineMockTSS(t)

	h := makeVariantQuarantineHandler()
	r := newRequestWithVars(map[string]string{"project_id": "sandbox", "variant_name": "ubuntu"})
	ctx := ctxWithProjectRef(&serviceModel.ProjectRef{Id: "sandbox_project_id", Identifier: "sandbox"})
	require.NoError(t, h.Parse(ctx, r))
	res := h.Run(ctx)

	assert.Equal(t, http.StatusOK, res.Status())
	apiStatus, ok := res.Data().(*model.APIVariantQuarantineStatus)
	require.True(t, ok)
	require.Len(t, apiStatus.Tasks, 1)
	require.Len(t, apiStatus.Tasks[0].Tests, 1)
	assert.True(t, apiStatus.Tasks[0].Tests[0].IsManuallyQuarantined)
}

func TestVariantUnquarantineSucceeds(t *testing.T) {
	setupVariantQuarantineMockTSS(t)

	h := makeVariantUnquarantineHandler()
	r := newRequestWithVars(map[string]string{"project_id": "sandbox", "variant_name": "ubuntu"})
	ctx := ctxWithProjectRef(&serviceModel.ProjectRef{Id: "sandbox_project_id", Identifier: "sandbox"})
	require.NoError(t, h.Parse(ctx, r))
	res := h.Run(ctx)

	assert.Equal(t, http.StatusOK, res.Status())
	apiStatus, ok := res.Data().(*model.APIVariantQuarantineStatus)
	require.True(t, ok)
	require.Len(t, apiStatus.Tasks, 1)
	require.Len(t, apiStatus.Tasks[0].Tests, 1)
	assert.False(t, apiStatus.Tasks[0].Tests[0].IsManuallyQuarantined)
}

func TestVariantQuarantineMissingProjectRefReturns404(t *testing.T) {
	setupVariantQuarantineMockTSS(t)

	h := makeVariantQuarantineHandler()
	r := newRequestWithVars(map[string]string{"project_id": "sandbox", "variant_name": "ubuntu"})
	ctx := context.WithValue(context.Background(), RequestContext, &serviceModel.Context{ProjectRef: nil})
	require.NoError(t, h.Parse(ctx, r))
	res := h.Run(ctx)
	assert.Equal(t, http.StatusNotFound, res.Status())
}

func TestVariantQuarantineStatusReadSucceeds(t *testing.T) {
	setupVariantQuarantineMockTSS(t)

	h := makeVariantQuarantineStatusHandler()
	r := newRequestWithVars(map[string]string{"project_id": "sandbox", "variant_name": "ubuntu"})
	ctx := ctxWithProjectRef(&serviceModel.ProjectRef{Id: "sandbox_project_id", Identifier: "sandbox"})
	require.NoError(t, h.Parse(ctx, r))
	res := h.Run(ctx)

	assert.Equal(t, http.StatusOK, res.Status())
	apiStatus, ok := res.Data().(*model.APIVariantQuarantineStatus)
	require.True(t, ok)
	assert.NotEmpty(t, apiStatus.Tasks)
}
