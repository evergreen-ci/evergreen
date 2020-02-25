package fetcher

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("NoBaseURL", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL: "https://cedar.mongodb.com",
			TaskID:  "task",
		}
		_, err := opts.parse()
		require.NoError(t, err)
		opts.BaseURL = ""
		_, err = opts.parse()
		assert.Error(t, err)
	})
	t.Run("NoIDAndNoTaskID", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL: "https://cedar.mongodb.com",
			TaskID:  "task",
		}
		_, err := opts.parse()
		require.NoError(t, err)
		opts.TaskID = ""
		_, err = opts.parse()
		assert.Error(t, err)
	})
	t.Run("IDAndTaskID", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL: "https://cedar.mongodb.com",
			TaskID:  "task",
		}
		_, err := opts.parse()
		require.NoError(t, err)
		opts.ID = "id"
		_, err = opts.parse()
		assert.Error(t, err)
	})
	t.Run("ID", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL: "https://cedar.mongodb.com",
			ID:      "id",
		}
		url, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/%s%s", opts.BaseURL, opts.ID, getParams(opts)), url)

		// meta
		opts.Meta = true
		url, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/%s/meta%s", opts.BaseURL, opts.ID, getParams(opts)), url)
	})
	t.Run("TaskID", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL: "https://cedar.mongodb.com",
			TaskID:  "task",
		}
		url, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s%s", opts.BaseURL, opts.TaskID, getParams(opts)), url)

		// meta
		opts.Meta = true
		url, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s/meta%s", opts.BaseURL, opts.TaskID, getParams(opts)), url)
	})
	t.Run("TestName", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL:  "https://cedar.mongodb.com",
			TaskID:   "task",
			TestName: "test",
		}
		url, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/test_name/%s/%s%s", opts.BaseURL, opts.TaskID, opts.TestName, getParams(opts)), url)

		// meta
		opts.Meta = true
		url, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/test_name/%s/%s/meta%s", opts.BaseURL, opts.TaskID, opts.TestName, getParams(opts)), url)
	})
	t.Run("GroupID", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL:  "https://cedar.mongodb.com",
			TaskID:   "task",
			TestName: "test",
			GroupID:  "group",
		}
		url, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/test_name/%s/%s/group/%s%s", opts.BaseURL, opts.TaskID, opts.TestName, opts.GroupID, getParams(opts)), url)

		// meta
		opts.Meta = true
		url, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/test_name/%s/%s/group/%s/meta%s", opts.BaseURL, opts.TaskID, opts.TestName, opts.GroupID, getParams(opts)), url)
	})
	t.Run("Parameters", func(t *testing.T) {
		opts := FetchOptions{
			BaseURL:       "https://cedar.mongodb.com",
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
		url, err := opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s%s", opts.BaseURL, opts.TaskID, getParams(opts)), url)

		// meta
		opts.Meta = true
		url, err = opts.parse()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s/rest/v1/buildlogger/task_id/%s/meta%s", opts.BaseURL, opts.TaskID, getParams(opts)), url)
	})
}

func TestPaginatedReadCloser(t *testing.T) {
	t.Run("PaginatedRoute", func(t *testing.T) {
		handler := &mockHandler{paginate: true}
		server := httptest.NewServer(handler)
		handler.baseURL = server.URL

		resp, err := doReq(server.URL, nil)
		require.NoError(t, err)

		var r io.ReadCloser
		r = &paginatedReadCloser{
			header:     resp.Header,
			ReadCloser: resp.Body,
		}

		data, err := ioutil.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "PAGINATED BODY PAGE 1\nPAGINATED BODY PAGE 2\n", string(data))
		assert.NoError(t, r.Close())
	})
	t.Run("NonPaginatedRoute", func(t *testing.T) {
		handler := &mockHandler{}
		server := httptest.NewServer(handler)
		handler.baseURL = server.URL

		resp, err := doReq(server.URL, nil)
		require.NoError(t, err)

		var r io.ReadCloser
		r = &paginatedReadCloser{
			header:     resp.Header,
			ReadCloser: resp.Body,
		}

		data, err := ioutil.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "NON-PAGINATED BODY PAGE", string(data))
		assert.NoError(t, r.Close())
	})
	t.Run("SplitPageByteSlice", func(t *testing.T) {
		handler := &mockHandler{paginate: true}
		server := httptest.NewServer(handler)
		handler.baseURL = server.URL

		resp, err := doReq(server.URL, nil)
		require.NoError(t, err)

		var r io.ReadCloser
		r = &paginatedReadCloser{
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
	baseURL  string
	count    int
	paginate bool
}

func (h *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.paginate {
		w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"%s\"", h.baseURL, "next"))
		if h.count <= 1 {
			_, _ = w.Write([]byte(fmt.Sprintf("PAGINATED BODY PAGE %d\n", h.count+1)))
		}
		h.count++
	} else {
		_, _ = w.Write([]byte("NON-PAGINATED BODY PAGE"))
	}
}

func getParams(opts FetchOptions) string {
	params := fmt.Sprintf(
		"?execution=%d&proc_name=%s&print_time=%v&print_priority=%v&n=%d&limit=%d&start=%s&end=%s&paginate=true",
		opts.Execution,
		opts.ProcessName,
		opts.PrintTime,
		opts.PrintPriority,
		opts.Tail,
		opts.Limit,
		opts.Start.Format(time.RFC3339),
		opts.End.Format(time.RFC3339),
	)
	for _, tag := range opts.Tags {
		params += fmt.Sprintf("&tags=%s", tag)
	}

	return params
}
