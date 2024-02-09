package host

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindUnexpirableRunning(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"ReturnsUnexpirableRunningHost": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"DoesNotReturnExpirableHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.NoExpiration = false
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"DoesNotReturnNonRunningHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.Status = evergreen.HostStopped
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"DoesNotReturnEvergreenOwnedHosts": func(ctx context.Context, t *testing.T, h *Host) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(Collection))
			h := Host{
				Id:           "host_id",
				Status:       evergreen.HostRunning,
				StartedBy:    "myself",
				NoExpiration: true,
			}
			tCase(ctx, t, &h)
		})
	}
}
