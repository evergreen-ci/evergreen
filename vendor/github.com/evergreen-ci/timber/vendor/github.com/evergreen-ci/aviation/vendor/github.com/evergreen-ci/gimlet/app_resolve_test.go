package gimlet

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteResolutionHelpers(t *testing.T) {
	hndlr := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	logger := MakeRecoveryLogger()
	app := &APIApp{}
	app.SetPrefix("baz")

	for _, tc := range []struct {
		route     *APIRoute
		expected  string
		addPrefix bool
	}{
		{
			expected:  "/v1/foo/bar",
			addPrefix: false,
			route: &APIRoute{
				prefix:  "/foo",
				version: 1,
				route:   "/bar",
				handler: hndlr,
			},
		},
		{
			expected:  "/baz/v1/foo/bar",
			addPrefix: true,
			route: &APIRoute{
				prefix:  "/foo",
				version: 1,
				handler: hndlr,
				route:   "/bar",
			},
		},
		{
			expected:  "/baz/v1/foo/bar",
			addPrefix: true,
			route: &APIRoute{
				handler: hndlr,
				version: 1,
				route:   "/foo/bar",
			},
		},
		{
			expected:  "/v1/foo/bar",
			addPrefix: false,
			route: &APIRoute{
				prefix:  "/foo",
				handler: hndlr,
				version: 1,
				route:   "/bar",
			},
		},
		{
			expected:  "/foo/v1/bar",
			addPrefix: true,
			route: &APIRoute{
				prefix:            "/foo",
				handler:           hndlr,
				version:           1,
				route:             "/bar",
				overrideAppPrefix: true,
			},
		},
		{
			expected:  "/v1/foo/bar",
			addPrefix: false,
			route: &APIRoute{
				prefix:            "/foo",
				handler:           hndlr,
				version:           1,
				route:             "/bar",
				overrideAppPrefix: true,
			},
		},
	} {
		// by default there's no middleware and everything's
		// the same
		assert.Equal(t, tc.expected, tc.route.resolveVersionedRoute(app, tc.addPrefix))
		h := tc.route.getHandlerWithMiddlware(nil)
		assert.Equal(t, fmt.Sprint(hndlr), fmt.Sprint(h))

		// if there's global middleware, we're different
		h = tc.route.getHandlerWithMiddlware([]Middleware{logger})
		assert.NotEqual(t, fmt.Sprint(hndlr), fmt.Sprint(h))

		// if you add wrapper middleware we're different differently
		tc.route.wrappers = append(tc.route.wrappers, logger)
		h = tc.route.getHandlerWithMiddlware(nil)
		assert.NotEqual(t, fmt.Sprint(hndlr), fmt.Sprint(h))
	}
}
