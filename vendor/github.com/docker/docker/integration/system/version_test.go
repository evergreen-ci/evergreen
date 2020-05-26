package system // import "github.com/docker/docker/integration/system"

import (
	"context"
	"testing"

	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

func TestVersion(t *testing.T) {
	defer setupTest(t)()
	client := testEnv.APIClient()

	version, err := client.ServerVersion(context.Background())
	assert.NilError(t, err)

	assert.Check(t, version.APIVersion != "")
	assert.Check(t, version.Version != "")
	assert.Check(t, version.MinAPIVersion != "")
	assert.Check(t, is.Equal(testEnv.DaemonInfo.ExperimentalBuild, version.Experimental))
	assert.Check(t, is.Equal(testEnv.OSType, version.Os))
}
