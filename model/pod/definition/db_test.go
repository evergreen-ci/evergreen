package definition

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
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
			require.NoError(t, db.Insert(Collection, pd))

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
			require.NoError(t, db.Insert(Collection, pd))

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

func TestFindOneByDigest(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			pd := PodDefinition{
				ID:         "id",
				ExternalID: "external_id",
				Digest:     "abcdef123456",
			}
			require.NoError(t, db.Insert(Collection, pd))

			dbPodDef, err := FindOneByDigest(pd.Digest)
			require.NoError(t, err)
			require.NotZero(t, dbPodDef)
			assert.Equal(t, pd.ID, dbPodDef.ID)
			assert.Equal(t, pd.ExternalID, dbPodDef.ExternalID)
			assert.Equal(t, pd.Digest, dbPodDef.Digest)
		},
		"ReturnsNilWithNonexistentPodDefinition": func(t *testing.T) {
			pd, err := FindOneByDigest("nonexistent")
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
