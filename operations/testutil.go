package operations

import (
	"github.com/evergreen-ci/evergreen/rest/client"
)

// This is only set during tests.
var mockClient *client.Mock
