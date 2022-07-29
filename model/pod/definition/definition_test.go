package definition

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodDefinitionUpdateLastAccessed(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, pd PodDefinition){
		"SetsLastAccessedTime": func(t *testing.T, pd PodDefinition) {
			require.NoError(t, pd.Insert())

			require.NoError(t, pd.UpdateLastAccessed())
		},
		"FailsForNonexistentPodDefinition": func(t *testing.T, pd PodDefinition) {
			assert.Error(t, pd.UpdateLastAccessed())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			pd := PodDefinition{
				ID:         "id",
				ExternalID: "external_id",
				Family:     "family",
			}
			tCase(t, pd)
		})
	}
}

func TestPodDefinitionCache(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPodDefinitionCache)(nil), PodDefinitionCache{})

	var pdc PodDefinitionCache

	t.Run("Put", func(t *testing.T) {
		defer func() {
			assert.NoError(t, db.ClearCollections(Collection))
		}()

		for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
			"SucceedsWithNewItem": func(ctx context.Context, t *testing.T) {
				externalID := "external_id"
				defOpts := cocoa.NewECSPodDefinitionOptions().SetName("family")
				require.NoError(t, pdc.Put(ctx, cocoa.ECSPodDefinitionItem{
					ID:             externalID,
					DefinitionOpts: *defOpts,
				}))
				pd, err := FindOneByExternalID(externalID)
				require.NoError(t, err)
				require.NotZero(t, pd)
				assert.NotZero(t, pd.ID)
				assert.Equal(t, externalID, pd.ExternalID)
				assert.Equal(t, utility.FromStringPtr(defOpts.Name), pd.Family)
				assert.NotZero(t, pd.LastAccessed)
			},
			"IsIdempotentForIdenticalItem": func(ctx context.Context, t *testing.T) {
				externalID := "external_id"
				defOpts := cocoa.NewECSPodDefinitionOptions().SetName("name")
				require.NoError(t, pdc.Put(ctx, cocoa.ECSPodDefinitionItem{
					ID:             externalID,
					DefinitionOpts: *defOpts,
				}))

				pd, err := FindOneByExternalID(externalID)
				require.NoError(t, err)
				require.NotZero(t, pd)

				originalID := pd.ID
				assert.NotZero(t, pd.ID)
				assert.Equal(t, externalID, pd.ExternalID)
				assert.Equal(t, utility.FromStringPtr(defOpts.Name), pd.Family)
				assert.NotZero(t, pd.LastAccessed)

				require.NoError(t, pdc.Put(ctx, cocoa.ECSPodDefinitionItem{
					ID:             externalID,
					DefinitionOpts: *defOpts,
				}))

				pds, err := Find(db.Query(ByExternalID(externalID)))
				require.NoError(t, err)
				require.Len(t, pds, 1, "putting identical item should not have created any new pod definitions")

				assert.Equal(t, originalID, pds[0].ID)
				assert.Equal(t, externalID, pds[0].ExternalID)
				assert.Equal(t, utility.FromStringPtr(defOpts.Name), pds[0].Family)
				assert.NotZero(t, pd.LastAccessed)
			},
			"AddsMultipleDifferentItems": func(ctx context.Context, t *testing.T) {
				const numPodDefs = 10
				for i := 0; i < numPodDefs; i++ {
					defOpts := cocoa.NewECSPodDefinitionOptions().SetName(utility.RandomString())
					require.NoError(t, pdc.Put(ctx, cocoa.ECSPodDefinitionItem{
						ID:             utility.RandomString(),
						DefinitionOpts: *defOpts,
					}))
				}
				pds, err := Find(db.Query(bson.M{}))
				require.NoError(t, err)
				require.Len(t, pds, numPodDefs)
			},
		} {
			t.Run(tName, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				require.NoError(t, db.ClearCollections(Collection))
				tCase(ctx, t)
			})
		}
	})
}
