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

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	p1 := &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
	}
	require.NoError(t, p1.Insert())

	p2 := &Pod{
		ID:     utility.RandomString(),
		Status: StatusStarting,
	}
	require.NoError(t, p2.Insert())

	p3 := &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
	}
	require.NoError(t, p3.Insert())

	pods, err := FindByInitializing()
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, StatusInitializing, pods[0].Status)
	assert.Equal(t, StatusInitializing, pods[1].Status)

	ids := map[string]struct{}{p1.ID: {}, p3.ID: {}}
	assert.Contains(t, ids, pods[0].ID)
	assert.Contains(t, ids, pods[1].ID)
}
