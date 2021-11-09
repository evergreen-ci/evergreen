package timber

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaginatedReadCloser(t *testing.T) {
	t.Run("PaginatedRoute", func(t *testing.T) {
		handler := &mockHandler{pages: 3}
		server := httptest.NewServer(handler)
		handler.baseURL = server.URL

		opts := GetOptions{}
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

		opts := GetOptions{}
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

		opts := GetOptions{}
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
