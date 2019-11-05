package route

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestGetPermissions(t *testing.T) {
	result := GetAllPermissions()

	assert.Equal(t, len(evergreen.ProjectPermissions), len(result.ProjectPermissions))
	assert.Equal(t, len(evergreen.DistroPermissions), len(result.DistroPermissions))
}
