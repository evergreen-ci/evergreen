package pod

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindByInitializing(t *testing.T) {
	require.NoError(t, db.Clear(Collection))

	p := &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
		Secret: "secret",
	}
	require.NoError(t, p.Insert())

	p = &Pod{
		ID:     utility.RandomString(),
		Status: StatusStarting,
		Secret: "secret",
	}
	require.NoError(t, p.Insert())

	p = &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
		Secret: "secret",
	}
	require.NoError(t, p.Insert())

	pods, err := FindByInitializing()
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, StatusInitializing, pods[0].Status)
	assert.Equal(t, StatusInitializing, pods[1].Status)
}
