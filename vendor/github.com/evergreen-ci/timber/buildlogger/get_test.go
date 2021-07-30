package buildlogger

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("NoBaseURL", func(t *testing.T) {
		opts := BuildloggerGetOptions{TaskID: "task"}
		_, err := opts.parse()
		assert.Error(t, err)
	})
	t.Run("NoIDAndNoTaskID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
		}
		_, err := opts.parse()
		assert.Error(t, err)
	})
	t.Run("IDAndTaskID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			ID:     "id",
			TaskID: "task",
		}
		_, err := opts.parse()
		assert.Error(t, err)
	})
	t.Run("TestNameAndNoTaskID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			ID:       "id",
			TestName: "test",
		}
		_, err := opts.parse()
		assert.Error(t, err)
	})
	t.Run("GroupIDAndNoTaskID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			ID:      "id",
			GroupID: "group",
		}
		_, err := opts.parse()
		assert.Error(t, err)
	})
	t.Run("GroupIDAndMeta", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:  "task",
			GroupID: "group",
			Meta:    true,
		}
		_, err := opts.parse()
		assert.Error(t, err)
	})
	t.Run("ID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			ID: "id/1",
		}
		urlString, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/%s%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.ID), getParams(opts)), urlString)

		// meta
		opts.Meta = true
		urlString, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/%s/meta%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.ID), getParams(opts)), urlString)
	})
	t.Run("TaskID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID: "task/1",
		}
		urlString, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), getParams(opts)), urlString)

		// meta
		opts.Meta = true
		urlString, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s/meta%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), getParams(opts)), urlString)
	})
	t.Run("TestName", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:   "task?",
			TestName: "test/1",
		}
		urlString, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/test_name/%s/%s%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), url.PathEscape(opts.TestName), getParams(opts)), urlString)

		// meta
		opts.Meta = true
		urlString, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/test_name/%s/%s/meta%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), url.PathEscape(opts.TestName), getParams(opts)), urlString)
	})
	t.Run("TaskIDAndGroupID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:  "task?",
			GroupID: "group/group/group",
		}
		urlString, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s/group/%s%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), url.PathEscape(opts.GroupID), getParams(opts)), urlString)
	})
	t.Run("TestNameAndGroupID", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:   "task?",
			TestName: "test/?1",
			GroupID:  "group/group/group",
		}
		urlString, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/test_name/%s/%s/group/%s%s", opts.CedarOpts.BaseURL, url.PathEscape(opts.TaskID), url.PathEscape(opts.TestName), url.PathEscape(opts.GroupID), getParams(opts)), urlString)
	})
	t.Run("Parameters", func(t *testing.T) {
		opts := BuildloggerGetOptions{
			CedarOpts: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:        "task",
			Start:         time.Now().Add(-time.Hour),
			End:           time.Now(),
			ProcessName:   "proc",
			Tags:          []string{"tag1", "tag2", "tag3"},
			PrintTime:     true,
			PrintPriority: true,
			Tail:          100,
			Limit:         1000,
		}
		urlString, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s%s", opts.CedarOpts.BaseURL, opts.TaskID, getParams(opts)), urlString)

		// meta
		opts.Meta = true
		urlString, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s/meta%s", opts.CedarOpts.BaseURL, opts.TaskID, getParams(opts)), urlString)
	})
}

func TestPaginatedReadCloser(t *testing.T) {
	t.Run("PaginatedRoute", func(t *testing.T) {
		handler := &mockHandler{pages: 3}
		server := httptest.NewServer(handler)
		handler.baseURL = server.URL

		opts := timber.GetOptions{}
		resp, err := opts.DoReq(context.TODO(), server.URL)
		require.NoError(t, err)

		var r io.ReadCloser
		r = &paginatedReadCloser{
			ctx:        context.TODO(),
			header:     resp.Header,
			ReadCloser: resp.Body,
		}

		data, err := ioutil.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "PAGINATED BODY PAGE 1\nPAGINATED BODY PAGE 2\nPAGINATED BODY PAGE 3\n", string(data))
		assert.NoError(t, r.Close())
	})
	t.Run("NonPaginatedRoute", func(t *testing.T) {
		handler := &mockHandler{}
		server := httptest.NewServer(handler)
		handler.baseURL = server.URL

		opts := timber.GetOptions{}
		resp, err := opts.DoReq(context.TODO(), server.URL)
		require.NoError(t, err)

		var r io.ReadCloser
		r = &paginatedReadCloser{
			ctx:        context.TODO(),
			header:     resp.Header,
			ReadCloser: resp.Body,
		}

		data, err := ioutil.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "NON-PAGINATED BODY PAGE", string(data))
		assert.NoError(t, r.Close())
	})
	t.Run("SplitPageByteSlice", func(t *testing.T) {
		handler := &mockHandler{pages: 2}
		server := httptest.NewServer(handler)
		handler.baseURL = server.URL

		opts := timber.GetOptions{}
		resp, err := opts.DoReq(context.TODO(), server.URL)
		require.NoError(t, err)

		var r io.ReadCloser
		r = &paginatedReadCloser{
			ctx:        context.TODO(),
			header:     resp.Header,
			ReadCloser: resp.Body,
		}

		p := make([]byte, 33) // 1.5X len of each page
		n, err := r.Read(p)
		require.NoError(t, err)
		assert.Equal(t, len(p), n)
		assert.Equal(t, "PAGINATED BODY PAGE 1\nPAGINATED B", string(p))
		p = make([]byte, 33)
		n, err = r.Read(p)
		require.Equal(t, io.EOF, err)
		assert.Equal(t, 11, n)
		assert.Equal(t, "ODY PAGE 2\n", string(p[:11]))
		assert.NoError(t, r.Close())
	})
}

type mockHandler struct {
	baseURL string
	pages   int
	count   int
}

func (h *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.pages > 0 {
		if h.count <= h.pages-1 {
			w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"%s\"", h.baseURL, "next"))
			_, _ = w.Write([]byte(fmt.Sprintf("PAGINATED BODY PAGE %d\n", h.count+1)))
		}
		h.count++
	} else {
		_, _ = w.Write([]byte("NON-PAGINATED BODY PAGE"))
	}
}

func getParams(opts BuildloggerGetOptions) string {
	params := fmt.Sprintf(
		"?execution=%d&proc_name=%s&print_time=%v&print_priority=%v&n=%d&limit=%d&paginate=true",
		opts.Execution,
		url.QueryEscape(opts.ProcessName),
		opts.PrintTime,
		opts.PrintPriority,
		opts.Tail,
		opts.Limit,
	)
	if !opts.Start.IsZero() {
		params += fmt.Sprintf("&start=%s", opts.Start.Format(time.RFC3339))
	}
	if !opts.End.IsZero() {
		params += fmt.Sprintf("&end=%s", opts.End.Format(time.RFC3339))
	}
	for _, tag := range opts.Tags {
		params += fmt.Sprintf("&tags=%s", tag)
	}

	return params
}
