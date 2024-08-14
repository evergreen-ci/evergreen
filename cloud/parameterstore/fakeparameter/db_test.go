package fakeparameter

import (
	"context"
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
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, p *FakeParameter){
		"FindsExistingParameter": func(ctx context.Context, t *testing.T, p *FakeParameter) {
			require.NoError(t, p.Insert(ctx))

			dbParam, err := FindOneID(ctx, p.Name)
			require.NoError(t, err)
			require.NotZero(t, dbParam)
			assert.Equal(t, p, dbParam)
		},
		"ReturnsNilForNonexistentParameter": func(ctx context.Context, t *testing.T, p *FakeParameter) {
			dbParam, err := FindOneID(ctx, "nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, dbParam)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(Collection))

			p := FakeParameter{
				Name:        "id",
				Value:       "value",
				LastUpdated: utility.BSONTime(time.Now()),
			}

			tCase(ctx, t, &p)
		})
	}
}
