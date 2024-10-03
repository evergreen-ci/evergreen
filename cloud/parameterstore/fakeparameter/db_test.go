package fakeparameter

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func init() {
	// This is identical to testutil.Setup(), but that can't be called directly
	// in this package without causing an import cycle.

	const (
		testDir      = "config_test"
		testSettings = "evg_settings.yml"
	)

	if evergreen.GetEnvironment() == nil {
		ctx := context.Background()

		path := filepath.Join(evergreen.FindEvergreenHome(), testDir, testSettings)
		env, err := evergreen.NewEnvironment(ctx, path, "", "", nil, noop.NewTracerProvider())

		grip.EmergencyPanic(message.WrapError(err, message.Fields{
			"message": "could not initialize test environment",
			"path":    path,
		}))

		evergreen.SetEnvironment(env)
	}
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
