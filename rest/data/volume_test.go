package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateVolume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, env *mock.Environment, h *host.Host, v *host.Volume, options *restModel.HostRequestOptions, user *user.DBUser){
		"StartsJob": func(t *testing.T, ctx context.Context, env *mock.Environment, h *host.Host, v *host.Volume, options *restModel.HostRequestOptions, user *user.DBUser) {
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, v.Insert(t.Context()))

			jobStarted, err := MigrateVolume(ctx, v.ID, options, user, env)
			assert.NoError(t, err)
			assert.True(t, jobStarted)
			assert.Equal(t, 1, env.RemoteQueue().Stats(ctx).Running)
		},
		"FailsWithoutKey": func(t *testing.T, ctx context.Context, env *mock.Environment, h *host.Host, v *host.Volume, options *restModel.HostRequestOptions, user *user.DBUser) {
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, v.Insert(t.Context()))

			options.KeyName = ""
			jobStarted, err := MigrateVolume(ctx, v.ID, options, user, env)
			assert.Error(t, err)
			assert.False(t, jobStarted)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.EventCollection, distro.Collection, user.Collection))
			const testPublicKey = "ssh-rsa 1234567890abcdef"
			const testPublicKeyName = "testPubKey"

			d := &distro.Distro{
				Id:                   "d",
				SpawnAllowed:         true,
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("region", evergreen.DefaultEC2Region))},
			}
			require.NoError(t, d.Insert(ctx))
			testUser := &user.DBUser{
				Id:     "u",
				APIKey: "testApiKey",
			}
			testUser.PubKeys = append(testUser.PubKeys, user.PubKey{
				Name: testPublicKeyName,
				Key:  testPublicKey,
			})
			require.NoError(t, testUser.Insert(t.Context()))

			options := &restmodel.HostRequestOptions{
				DistroID:     d.Id,
				TaskID:       "",
				KeyName:      testPublicKeyName,
				UserData:     "testUserData",
				InstanceTags: nil,
			}

			h := &host.Host{
				Id:     "h",
				Status: evergreen.HostRunning,
			}
			v := &host.Volume{
				ID: "v",
			}

			ctx := gimlet.AttachUser(context.Background(), testUser)
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			tCase(t, ctx, env, h, v, options, testUser)
		})
	}
}
