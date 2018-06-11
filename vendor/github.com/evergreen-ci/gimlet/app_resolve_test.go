package gimlet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteResolutionHelpers(t *testing.T) {
	app := &APIApp{}
	for _, tc := range []struct {
		route     *APIRoute
		expected  string
		addPrefix bool
	}{
		{
			expected:  "/v1/bar",
			addPrefix: false,
			route: &APIRoute{
				prefix:  "/foo",
				version: 1,
				route:   "/bar",
			},
		},
		{
			expected:  "/foo/v1/bar",
			addPrefix: true,
			route: &APIRoute{
				prefix:  "/foo",
				version: 1,
				route:   "/bar",
			},
		},
		{
			expected:  "/foo/v1/bar",
			addPrefix: true,
			route: &APIRoute{
				prefix:  "/foo",
				version: 1,
				route:   "/foo/bar",
			},
		},
	} {
		assert.Equal(t, tc.expected, tc.route.resolveVersionedRoute(app, tc.addPrefix))
	}

	handler, err := NewApp().Handler()
	assert.NoError(t, err)
	h := getRouteHandlerWithMiddlware(nil, handler)
	assert.Equal(t, handler, h)
	h = getRouteHandlerWithMiddlware([]Middleware{NewRecoveryLogger()}, handler)
	assert.NotEqual(t, handler, h)
}
