package hoststat

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestFindByDistroSince(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsOnlyThoseMatchingDistro": func(t *testing.T) {
			stat1 := NewHostStat("distro1", 100)
			stat1.Timestamp = utility.BSONTime(stat1.Timestamp)
			require.NoError(t, stat1.Insert(t.Context()))
			stat2 := NewHostStat("distro2", 200)
			stat2.Timestamp = utility.BSONTime(stat2.Timestamp)
			require.NoError(t, stat2.Insert(t.Context()))

			dbStats, err := FindByDistroSince(t.Context(), "distro1", stat1.Timestamp.Add(-time.Hour))
			require.NoError(t, err)
			require.Len(t, dbStats, 1)
			assert.Equal(t, *stat1, dbStats[0])
		},
		"ReturnsOnlyThoseInTimeRange": func(t *testing.T) {
			stat1 := NewHostStat("distro1", 100)
			stat1.Timestamp = utility.BSONTime(stat1.Timestamp)
			require.NoError(t, stat1.Insert(t.Context()))
			stat2 := NewHostStat("distro1", 200)
			stat2.Timestamp = utility.BSONTime(time.Now().Add(-100 * utility.Day))
			require.NoError(t, stat2.Insert(t.Context()))

			dbStats, err := FindByDistroSince(t.Context(), "distro1", stat1.Timestamp.Add(-time.Hour))
			require.NoError(t, err)
			require.Len(t, dbStats, 1)
			assert.Equal(t, *stat1, dbStats[0])
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			tCase(t)
		})
	}
}
