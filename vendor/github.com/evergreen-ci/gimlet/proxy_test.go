package gimlet

import (
	"net/http"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyService(t *testing.T) {
	t.Run("Slashes", func(t *testing.T) {
		assert.Equal(t, "foo/bar", singleJoiningSlash("foo", "bar"))
		assert.Equal(t, "foo/bar", singleJoiningSlash("foo", "/bar"))
		assert.Equal(t, "foo/bar", singleJoiningSlash("foo/", "/bar"))
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("Default", func(t *testing.T) {
			opts := &ProxyOptions{}
			assert.Error(t, opts.Validate())
		})
		t.Run("HostPool", func(t *testing.T) {
			opts := &ProxyOptions{}
			opts.TargetPool = []string{"a", "b"}
			assert.NoError(t, opts.Validate())
		})
		t.Run("HostPoolDeclared", func(t *testing.T) {
			opts := &ProxyOptions{}
			opts.TargetPool = []string{}
			assert.Error(t, opts.Validate())
		})
		t.Run("FindFunction", func(t *testing.T) {
			opts := &ProxyOptions{}
			opts.FindTarget = func(r *http.Request) ([]string, error) { return nil, nil }
			assert.NoError(t, opts.Validate())
		})
		t.Run("ErrorWhenAllResolversSpecified", func(t *testing.T) {
			opts := &ProxyOptions{
				TargetPool: []string{"a"},
				FindTarget: func(r *http.Request) ([]string, error) { return nil, nil },
			}
			assert.Error(t, opts.Validate())
		})
	})

	t.Run("DirectorHeaders", func(t *testing.T) {
		opts := &ProxyOptions{
			TargetPool:      []string{"localhost:8080"},
			HeadersToAdd:    map[string]string{"Key": "value"},
			HeadersToDelete: []string{"baz"},
		}

		req, err := http.NewRequest(http.MethodGet, "http://example.com/target/path", nil)
		require.NoError(t, err)
		assert.Len(t, req.Header, 0)
		req.Header["Foo"] = []string{"bar"}
		req.Header["Baz"] = []string{"bot"}
		opts.director(req)
		assert.Len(t, req.Header, 3)
		assert.Contains(t, req.Header, "User-Agent")
		assert.Contains(t, req.Header, "Key")
		assert.Contains(t, req.Header, "Foo")
	})
	t.Run("DirectorPath", func(t *testing.T) {
		opts := &ProxyOptions{
			TargetPool: []string{"localhost:8080"},
		}
		req, err := http.NewRequest(http.MethodGet, "http://example.com/target/path", nil)
		require.NoError(t, err)
		assert.Equal(t, "example.com", req.URL.Host)
		opts.director(req)
		assert.Equal(t, "localhost:8080", req.URL.Host)
		assert.Equal(t, "/target/path", req.URL.Path)
	})
	t.Run("PanicWithNoHosts", func(t *testing.T) {
		opts := &ProxyOptions{
			TargetPool: []string{},
			FindTarget: func(r *http.Request) ([]string, error) { return nil, nil },
		}

		req, err := http.NewRequest(http.MethodGet, "http://example.com/target/path", nil)
		require.NoError(t, err)
		assert.Panics(t, func() { opts.director(req) })
	})

	t.Run("AddPrefix", func(t *testing.T) {
		opts := &ProxyOptions{
			TargetPool:   []string{"localhost:8080"},
			RemotePrefix: "/proxy/add/",
		}

		req, err := http.NewRequest(http.MethodGet, "http://example.com/target/path", nil)
		require.NoError(t, err)
		opts.director(req)
		assert.Equal(t, "/proxy/add/target/path", req.URL.Path)
	})
	t.Run("StripPrefix", func(t *testing.T) {
		tp := &testProxy{
			director: (&ProxyOptions{
				TargetPool:        []string{"localhost:8080"},
				StripSourcePrefix: true,
			}).director,
		}

		router := mux.NewRouter()
		router.Handle("/target/path", tp)
		req, err := http.NewRequest(http.MethodGet, "http://example.com/target/path", nil)
		require.NoError(t, err)
		router.ServeHTTP(nil, req)
		assert.Equal(t, "/", tp.finalRequest.URL.Path)
	})
	t.Run("ReplacePrefix", func(t *testing.T) {
		tp := &testProxy{
			director: (&ProxyOptions{
				TargetPool:        []string{"localhost:8080"},
				RemotePrefix:      "/proxy/add/",
				StripSourcePrefix: true,
			}).director,
		}

		router := mux.NewRouter()
		router.Handle("/target/path", tp)
		req, err := http.NewRequest(http.MethodGet, "http://example.com/target/path", nil)
		require.NoError(t, err)
		router.ServeHTTP(nil, req)
		assert.Equal(t, "/proxy/add/", tp.finalRequest.URL.Path)
	})
}

type testProxy struct {
	director     func(*http.Request)
	finalRequest http.Request
}

func (p *testProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.director(r)
	p.finalRequest = *r
}
