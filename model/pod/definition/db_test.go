package definition

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

func TestFindOneID(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			pd := PodDefinition{
				ID: "id",
			}
			require.NoError(t, pd.Insert())

			dbPodDef, err := FindOneID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPodDef)
			assert.Equal(t, pd.ID, dbPodDef.ID)
			assert.Equal(t, pd.ExternalID, dbPodDef.ExternalID)
		},
		"ReturnsNilWithNonexistentPodDefinition": func(t *testing.T) {
			pd, err := FindOneID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, pd)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			tCase(t)
		})
	}
}

func TestFindOneByExternalID(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			pd := PodDefinition{
				ID:         "id",
				ExternalID: "external_id",
			}
			require.NoError(t, pd.Insert())

			dbPodDef, err := FindOneByExternalID(pd.ExternalID)
			require.NoError(t, err)
			require.NotZero(t, dbPodDef)
			assert.Equal(t, pd.ID, dbPodDef.ID)
			assert.Equal(t, pd.ExternalID, dbPodDef.ExternalID)
		},
		"ReturnsNilWithNonexistentPodDefinition": func(t *testing.T) {
			pd, err := FindOneByExternalID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, pd)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			tCase(t)
		})
	}
}

func TestFindOneByFamily(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			pd := PodDefinition{
				ID:         "id",
				ExternalID: "external_id",
				Family:     "family",
			}
			require.NoError(t, pd.Insert())

			dbPodDef, err := FindOneByFamily(pd.Family)
			require.NoError(t, err)
			require.NotZero(t, dbPodDef)
			assert.Equal(t, pd.ID, dbPodDef.ID)
			assert.Equal(t, pd.ExternalID, dbPodDef.ExternalID)
			assert.Equal(t, pd.Family, dbPodDef.Family)
		},
		"ReturnsNilWithNonexistentPodDefinition": func(t *testing.T) {
			pd, err := FindOneByFamily("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, pd)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			tCase(t)
		})
	}
}

func TestFindOneByLastAccessedBefore(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			podDefs := []PodDefinition{
				{
					ID:           "pod_def0",
					LastAccessed: time.Now().Add(-time.Hour),
				},
				{
					ID:           "pod_def1",
					LastAccessed: time.Now(),
				},
				{
					ID:           "pod_def2",
					LastAccessed: time.Now().Add(-5 * time.Hour),
				},
			}
			for _, podDef := range podDefs {
				require.NoError(t, podDef.Insert())
			}
			found, err := FindByLastAccessedBefore(time.Minute, -1)
			require.NoError(t, err)
			require.Len(t, found, 2)
			for _, podDef := range found {
				assert.True(t, utility.StringSliceContains([]string{
					podDefs[0].ID,
					podDefs[2].ID,
				}, podDef.ID), "unexpected pod definition '%s'", podDef.ID)
			}
		},
		"LimitsResults": func(t *testing.T) {
			podDefs := []PodDefinition{
				{
					ID:           "pod_def0",
					LastAccessed: time.Now().Add(-time.Hour),
				},
				{
					ID:           "pod_def1",
					LastAccessed: time.Now().Add(-5 * time.Hour),
				},
			}
			for _, podDef := range podDefs {
				require.NoError(t, podDef.Insert())
			}
			found, err := FindByLastAccessedBefore(time.Minute, 1)
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.True(t, utility.StringSliceContains([]string{
				podDefs[0].ID,
				podDefs[1].ID,
			}, found[0].ID), "unexpected pod definition '%s'", found[0].ID)
		},
		"IgnoresPodDefinitionAccessedWithinTTL": func(t *testing.T) {
			podDef := PodDefinition{
				ID:           "pod_def",
				LastAccessed: time.Now(),
			}
			require.NoError(t, podDef.Insert())

			found, err := FindByLastAccessedBefore(time.Minute, -1)
			require.NoError(t, err)
			assert.Empty(t, found)
		},
		"ReturnsPodDefinitionsWithZeroLastAccessedTime": func(t *testing.T) {
			podDef := PodDefinition{
				ID: "pod_def",
			}
			require.NoError(t, podDef.Insert())

			found, err := FindByLastAccessedBefore(time.Minute, -1)
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, podDef.ID, found[0].ID)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			tCase(t)
		})
	}
}
