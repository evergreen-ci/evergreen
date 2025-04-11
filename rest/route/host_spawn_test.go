package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostPostHandler(t *testing.T) {
	checkUnexpirableHostSchedule := func(t *testing.T, h *host.Host, opts host.SleepScheduleOptions) {
		assert.True(t, h.NoExpiration)
		assert.Equal(t, opts.WholeWeekdaysOff, h.SleepSchedule.WholeWeekdaysOff)
		assert.Equal(t, opts.DailyStartTime, h.SleepSchedule.DailyStartTime)
		assert.Equal(t, opts.DailyStopTime, h.SleepSchedule.DailyStopTime)
		assert.Equal(t, opts.TimeZone, h.SleepSchedule.TimeZone)
		assert.NotZero(t, h.SleepSchedule.NextStartTime)
		assert.NotZero(t, h.SleepSchedule.NextStopTime)
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro){
		"SpawnsHostForTask": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.TaskID = "task"
			rh.options.KeyName = "ssh-rsa YWJjZDEyMzQK"

			resp := rh.Run(ctx)
			require.NotNil(t, t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			dbHost := resp.Data().(*model.APIHost)
			dbDistro, err := distro.FindOneId(ctx, d.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbDistro)
			userdata, ok := dbDistro.ProviderSettingsList[0].Lookup("user_data").StringValueOK()
			assert.False(t, ok)
			assert.Empty(t, userdata)
			assert.Empty(t, dbHost.InstanceTags)
			assert.Empty(t, dbHost.InstanceType)
		},
		"SpawnsHostWithUserData": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			doc := birch.NewDocument(
				birch.EC.String("ami", "ami-123"),
				birch.EC.String("user_data", "#!/bin/bash\necho my script"),
				birch.EC.String("region", evergreen.DefaultEC2Region),
			)
			d.ProviderSettingsList = []*birch.Document{doc}
			assert.NoError(t, d.ReplaceOne(ctx))

			resp := rh.Run(ctx)
			require.NotNil(t, t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			dbHost := resp.Data().(*model.APIHost)
			dbDistro, err := distro.FindOneId(ctx, "distro")
			assert.NoError(t, err)
			userdata, ok := dbDistro.ProviderSettingsList[0].Lookup("user_data").StringValueOK()
			assert.True(t, ok)
			assert.Equal(t, "#!/bin/bash\necho my script", userdata)
			assert.Empty(t, dbHost.InstanceTags)
			assert.Empty(t, dbHost.InstanceType)
		},
		"SpawnsHostWithInstanceTags": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.InstanceTags = []host.Tag{
				{
					Key:           "ssh-rsa YWJjZDEyMzQK",
					Value:         "value",
					CanBeModified: true,
				},
			}
			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			dbHost := resp.Data().(*model.APIHost)
			assert.Equal(t, []host.Tag{{Key: "ssh-rsa YWJjZDEyMzQK", Value: "value", CanBeModified: true}}, dbHost.InstanceTags)
			assert.Empty(t, dbHost.InstanceType)
		},
		"SpawnsHostWithExplicitInstanceType": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			d.Provider = evergreen.ProviderNameMock
			require.NoError(t, d.ReplaceOne(ctx))

			env.EvergreenSettings.Providers.AWS.AllowedInstanceTypes = append(env.EvergreenSettings.Providers.AWS.AllowedInstanceTypes, "test_instance_type")

			rh.options.InstanceType = "test_instance_type"
			rh.options.UserData = ""
			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			dbHost, ok := resp.Data().(*model.APIHost)
			require.True(t, ok)
			assert.Equal(t, rh.options.InstanceType, *dbHost.InstanceType)
		},
		"AdminOnlyDistroCannotBeSpawnedByNonAdmin": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			assert.False(t, u.HasDistroCreatePermission())

			d.AdminOnly = true
			require.NoError(t, d.ReplaceOne(ctx))

			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusForbidden, resp.Status(), resp.Data())
		},
		"AdminOnlyDistroCanBeSpawnedByDistroAdmin": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			createDistroRole := gimlet.Role{
				ID:          "create_distro",
				Name:        "create_distro",
				Scope:       "superuser_scope",
				Permissions: map[string]int{evergreen.PermissionDistroCreate: evergreen.DistroCreate.Value},
			}
			require.NoError(t, env.RoleManager().UpdateRole(createDistroRole))
			require.NoError(t, u.AddRole(ctx, createDistroRole.ID))

			superuserScope := gimlet.Scope{
				ID:        "superuser_scope",
				Name:      "superuser scope",
				Type:      evergreen.SuperUserResourceType,
				Resources: []string{evergreen.SuperUserPermissionsID},
			}
			require.NoError(t, env.RoleManager().AddScope(superuserScope))

			assert.True(t, u.HasDistroCreatePermission())

			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())
		},
		"UnexpirableHostSetsDefaultSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.NoExpiration = true
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			apiHost, ok := resp.Data().(*model.APIHost)
			require.True(t, ok)

			dbHost, err := host.FindOneId(ctx, utility.FromStringPtr(apiHost.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)

			var defaultSchedule host.SleepScheduleOptions
			defaultSchedule.SetDefaultSchedule()
			defaultSchedule.SetDefaultTimeZone(u.Settings.Timezone)
			checkUnexpirableHostSchedule(t, dbHost, defaultSchedule)
		},
		"UnexpirableHostSetsExplicitScheduleWithUserDefaultTimeZone": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.NoExpiration = true
			expectedOpts := host.SleepScheduleOptions{
				WholeWeekdaysOff: []time.Weekday{time.Monday, time.Tuesday},
				DailyStartTime:   "01:00",
				DailyStopTime:    "05:00",
			}
			rh.options.SleepScheduleOptions = expectedOpts
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			apiHost, ok := resp.Data().(*model.APIHost)
			require.True(t, ok)

			dbHost, err := host.FindOneId(ctx, utility.FromStringPtr(apiHost.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)

			expectedOpts.TimeZone = u.Settings.Timezone
			checkUnexpirableHostSchedule(t, dbHost, expectedOpts)
		},
		"UnexpirableHostSetsExplicitTimeZoneWithDefaultSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.NoExpiration = true
			rh.options.SleepScheduleOptions = host.SleepScheduleOptions{
				TimeZone: "Asia/Seoul",
			}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			apiHost, ok := resp.Data().(*model.APIHost)
			require.True(t, ok)

			dbHost, err := host.FindOneId(ctx, utility.FromStringPtr(apiHost.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)

			var defaultSchedule host.SleepScheduleOptions
			defaultSchedule.SetDefaultSchedule()
			defaultSchedule.SetDefaultTimeZone("Asia/Seoul")
			checkUnexpirableHostSchedule(t, dbHost, defaultSchedule)
		},
		"UnexpirableHostSetsExplicitScheduleAndExplicitTimeZone": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.NoExpiration = true
			expectedOpts := host.SleepScheduleOptions{
				WholeWeekdaysOff: []time.Weekday{time.Monday, time.Tuesday},
				DailyStartTime:   "01:00",
				DailyStopTime:    "05:00",
				TimeZone:         "Asia/Macau",
			}
			rh.options.SleepScheduleOptions = expectedOpts
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())

			apiHost, ok := resp.Data().(*model.APIHost)
			require.True(t, ok)

			dbHost, err := host.FindOneId(ctx, utility.FromStringPtr(apiHost.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)

			checkUnexpirableHostSchedule(t, dbHost, expectedOpts)
		},
		"UnexpirableHostSetsOnlyTimeZoneAndUsesDefaultSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.NoExpiration = true
			rh.options.SleepScheduleOptions = host.SleepScheduleOptions{
				TimeZone: "Asia/Macau",
			}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status(), resp.Data())
		},
		"ExpirableHostCannotSetSleepSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, rh *hostPostHandler, u *user.DBUser, d *distro.Distro) {
			rh.options.NoExpiration = false
			var defaultSchedule host.SleepScheduleOptions
			defaultSchedule.SetDefaultSchedule()
			rh.options.SleepScheduleOptions = defaultSchedule
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.NotEqual(t, http.StatusOK, resp.Status(), resp.Data())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx := t.Context()

			require.NoError(t, db.ClearCollections(distro.Collection, host.Collection))
			env := &mock.Environment{}
			assert.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.Spawnhost.SpawnHostsPerUser = 10
			env.EvergreenSettings.Spawnhost.UnexpirableHostsPerUser = 5
			var err error
			env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
				DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
					return queue.NewLocalLimitedSize(2, 1048), nil
				}}})
			require.NoError(t, err)

			doc := birch.NewDocument(
				birch.EC.String("ami", "ami-123"),
				birch.EC.String("region", evergreen.DefaultEC2Region),
			)
			d := &distro.Distro{
				Id:                   "distro",
				SpawnAllowed:         true,
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: []*birch.Document{doc},
			}
			require.NoError(t, d.Insert(ctx))
			assert.NoError(t, err)
			rh := &hostPostHandler{
				env: env,
				options: &model.HostRequestOptions{
					DistroID: d.Id,
					KeyName:  "ssh-rsa YWJjZDEyMzQK",
				},
			}
			u := &user.DBUser{
				Id:       "user",
				Settings: user.UserSettings{Timezone: "Asia/Macau"},
			}
			ctx = gimlet.AttachUser(ctx, u)

			tCase(ctx, t, env, rh, u, d)
		})
	}
}

func TestHostModifyHandlers(t *testing.T) {
	testutil.DisablePermissionsForTests()
	defer testutil.EnablePermissionsForTests()

	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.SubscriptionsCollection))
	}()

	checkSubscriptions := func(t *testing.T, userID string, numSubs int) {
		subscriptions, err := data.GetSubscriptions(t.Context(), userID, event.OwnerTypePerson)
		assert.NoError(t, err)
		assert.Len(t, subscriptions, numSubs)
	}
	checkSpawnHostModifyQueueGroup := func(t *testing.T, env *mock.Environment, numQueues int) {
		qg := env.RemoteQueueGroup()
		assert.Equal(t, numQueues, qg.Len())
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host){
		"StopHandlerEnqueuesStopJobForRunningHost": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStopHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[3].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			checkSubscriptions(t, "user", 1)
			checkSpawnHostModifyQueueGroup(t, env, 1)
		},
		"StopHandlerEnqueuesStopJobForStoppingHost": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStopHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[1].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			checkSubscriptions(t, "user", 1)
			checkSpawnHostModifyQueueGroup(t, env, 1)
		},
		"StopHandlerEnqueuesStopJobForAlreadyStoppedHost": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStopHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[0].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			assert.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			checkSubscriptions(t, "user", 1)
			checkSpawnHostModifyQueueGroup(t, env, 1)
		},
		"StopHandlerErrorsForNonstoppableHostStatus": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStopHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[2].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusBadRequest, resp.Status())

			checkSubscriptions(t, "user", 0)
			checkSpawnHostModifyQueueGroup(t, env, 0)
		},
		"StartHandlerEnqueuesStartJobForStoppedHost": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStartHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[0].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			checkSubscriptions(t, "user", 1)
			checkSpawnHostModifyQueueGroup(t, env, 1)
		},
		"StartHandlerEnqueuesStartJobForStoppingHost": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStartHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[1].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			checkSubscriptions(t, "user", 1)
			checkSpawnHostModifyQueueGroup(t, env, 1)
		},
		"StartHandlerErrorsForNonstartableHostStatus": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStartHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[2].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusBadRequest, resp.Status())

			checkSubscriptions(t, "user", 0)
			checkSpawnHostModifyQueueGroup(t, env, 0)
		},
		"StartHandlerEnqueuesJobForAlreadyRunningHost": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostStartHandler{
				env:              env,
				subscriptionType: event.SlackSubscriberType,
			}
			hostID := hosts[3].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			checkSubscriptions(t, "user", 1)
			checkSpawnHostModifyQueueGroup(t, env, 1)
		},
		"ModifyHandlerModifiesHost": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostModifyHandler{
				env: env,
				options: &host.HostModifyOptions{
					AddHours: 10 * time.Hour,
				},
			}
			hostID := hosts[3].Id

			rh.hostID = hostID
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			checkSpawnHostModifyQueueGroup(t, env, 1)
		},
		"ModifyHandlerFailsWithVeryLongTemporaryExemption": func(ctx context.Context, t *testing.T, env *mock.Environment, hosts []host.Host) {
			rh := &hostModifyHandler{
				env: env,
				options: &host.HostModifyOptions{
					AddTemporaryExemptionHours: 1000000,
				},
				hostID: hosts[0].Id,
			}

			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusBadRequest, resp.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.SubscriptionsCollection))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			hosts := []host.Host{
				{
					Id:       "host-stopped",
					Status:   evergreen.HostStopped,
					Provider: evergreen.ProviderNameMock,
					Distro:   distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock},
				},
				{
					Id:       "host-stopping",
					Status:   evergreen.HostStopping,
					Provider: evergreen.ProviderNameMock,
					Distro:   distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock},
				},
				{
					Id:       "host-provisioning",
					Status:   evergreen.HostProvisioning,
					Provider: evergreen.ProviderNameMock,
					Distro:   distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock},
				},
				{
					Id:       "host-running",
					Status:   evergreen.HostRunning,
					Provider: evergreen.ProviderNameMock,
					Distro:   distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock},
				},
			}
			for _, hostToAdd := range hosts {
				assert.NoError(t, hostToAdd.Insert(ctx))
			}

			tCase(ctx, t, env, hosts)
		})
	}

	t.Run("SleepScheduleOptions", func(t *testing.T) {
		for tName, tCase := range map[string]func(t *testing.T, h *host.Host, u *user.DBUser){
			"SetsDefaultScheduleForHostSwitchingToUnexpirable": func(t *testing.T, h *host.Host, u *user.DBUser) {
				rh := &hostModifyHandler{
					options: &host.HostModifyOptions{
						NoExpiration: utility.TruePtr(),
					},
				}
				opts := rh.getDefaultedSleepScheduleOpts(h, u)
				var defaultSchedule host.SleepScheduleOptions
				defaultSchedule.SetDefaultSchedule()
				defaultSchedule.SetDefaultTimeZone(u.Settings.Timezone)
				assert.Equal(t, defaultSchedule, opts)
			},
			"IgnoresExpirableHost": func(t *testing.T, h *host.Host, u *user.DBUser) {
				rh := &hostModifyHandler{options: &host.HostModifyOptions{}}
				opts := rh.getDefaultedSleepScheduleOpts(h, u)
				assert.Zero(t, opts)
			},
			"SetsDefaultScheduleForAlreadyUnexpirableHostMissingOne": func(t *testing.T, h *host.Host, u *user.DBUser) {
				h.NoExpiration = true
				rh := &hostModifyHandler{options: &host.HostModifyOptions{}}
				opts := rh.getDefaultedSleepScheduleOpts(h, u)
				var defaultSchedule host.SleepScheduleOptions
				defaultSchedule.SetDefaultSchedule()
				defaultSchedule.SetDefaultTimeZone(u.Settings.Timezone)
				assert.Equal(t, defaultSchedule, opts)
			},
			"SetsDefaultTimeZoneWhenSettingSchedule": func(t *testing.T, h *host.Host, u *user.DBUser) {
				rh := &hostModifyHandler{options: &host.HostModifyOptions{
					NoExpiration: utility.TruePtr(),
					SleepScheduleOptions: host.SleepScheduleOptions{
						DailyStartTime: "02:00",
						DailyStopTime:  "08:00",
					},
				}}
				defaultedOpts := rh.getDefaultedSleepScheduleOpts(h, u)
				expectedOpts := rh.options.SleepScheduleOptions
				expectedOpts.TimeZone = u.Settings.Timezone
				assert.Equal(t, expectedOpts, defaultedOpts)
			},
			"RetainsExistingSleepScheduleIfOnlySettingTimeZone": func(t *testing.T, h *host.Host, u *user.DBUser) {
				h.NoExpiration = true
				h.SleepSchedule.DailyStartTime = "02:00"
				h.SleepSchedule.DailyStopTime = "08:00"
				rh := &hostModifyHandler{options: &host.HostModifyOptions{
					SleepScheduleOptions: host.SleepScheduleOptions{
						TimeZone: "Asia/Seoul",
					},
				}}
				defaultedOpts := rh.getDefaultedSleepScheduleOpts(h, u)
				expectedOpts := rh.options.SleepScheduleOptions
				expectedOpts.DailyStartTime = h.SleepSchedule.DailyStartTime
				expectedOpts.DailyStopTime = h.SleepSchedule.DailyStopTime
				assert.Equal(t, expectedOpts, defaultedOpts)
			},
			"SetsDefaultScheduleForHostSwitchingToUnexpirableAndOnlySettingScheduleTimeZone": func(t *testing.T, h *host.Host, u *user.DBUser) {
				rh := &hostModifyHandler{
					options: &host.HostModifyOptions{
						NoExpiration: utility.TruePtr(),
						SleepScheduleOptions: host.SleepScheduleOptions{
							TimeZone: "Asia/Seoul",
						},
					},
				}
				defaultedOpts := rh.getDefaultedSleepScheduleOpts(h, u)
				var defaultSchedule host.SleepScheduleOptions
				defaultSchedule.SetDefaultSchedule()
				defaultSchedule.SetDefaultTimeZone(rh.options.TimeZone)
				assert.Equal(t, defaultSchedule, defaultedOpts)
			},
		} {
			t.Run(tName, func(t *testing.T) {
				u := user.DBUser{
					Id: "user",
					Settings: user.UserSettings{
						Timezone: "Asia/Macau",
					},
				}
				h := host.Host{
					Id:           "host_id",
					NoExpiration: false,
				}
				tCase(t, &h, &u)
			})
		}
	})
}

func TestCreateVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &createVolumeHandler{
		env:      testutil.NewEnvironment(ctx, t),
		provider: evergreen.ProviderNameMock,
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	v := host.Volume{ID: "volume1", Size: 15, CreatedBy: "user"}
	assert.NoError(t, v.Insert(t.Context()))
	v = host.Volume{ID: "volume2", Size: 35, CreatedBy: "user"}
	assert.NoError(t, v.Insert(t.Context()))
	v = host.Volume{ID: "not-relevant", Size: 400, CreatedBy: "someone-else"}
	assert.NoError(t, v.Insert(t.Context()))

	h.env.Settings().Providers.AWS.MaxVolumeSizePerUser = 100
	h.env.Settings().Providers.AWS.Subnets = []evergreen.Subnet{
		{AZ: "us-east-1a", SubnetID: "123"},
	}
	v = host.Volume{ID: "volume-new", Size: 80, CreatedBy: "user"}
	h.volume = &v

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	v.Size = 50
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
}

func TestDeleteVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &deleteVolumeHandler{
		env:      testutil.NewEnvironment(ctx, t),
		provider: evergreen.ProviderNameMock,
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	volumes := []host.Volume{
		{
			ID:               "my-volume",
			CreatedBy:        "user",
			AvailabilityZone: "us-east-1a",
		},
	}
	hosts := []host.Host{
		{
			Id:        "my-host",
			UserHost:  true,
			StartedBy: "user",
			Status:    evergreen.HostRunning,
			Volumes: []host.VolumeAttachment{
				{
					VolumeID:   "my-volume",
					DeviceName: "my-device",
				},
			},
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert(ctx))
	}
	for _, volumeToAdd := range volumes {
		assert.NoError(t, volumeToAdd.Insert(t.Context()))
	}
	h.VolumeID = "my-volume"
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
}

func TestAttachVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &attachVolumeHandler{
		env: testutil.NewEnvironment(ctx, t),
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	hosts := []host.Host{
		{
			Id:        "my-host",
			Status:    evergreen.HostRunning,
			StartedBy: "user",
			Zone:      "us-east-1c",
		},
		{
			Id: "different-host",
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert(ctx))
	}

	// no volume
	v := &host.VolumeAttachment{DeviceName: "my-device"}
	jsonBody, err := json.Marshal(v)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err := http.NewRequest(http.MethodGet, "/hosts/my-host/attach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.Error(t, h.Parse(ctx, r))

	// wrong availability zone
	v.VolumeID = "my-volume"
	volume := host.Volume{
		ID: v.VolumeID,
	}
	assert.NoError(t, volume.Insert(t.Context()))

	jsonBody, err = json.Marshal(v)
	assert.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodGet, "/hosts/my-host/attach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.NoError(t, h.Parse(ctx, r))

	require.NotNil(t, h.attachment)
	assert.Equal(t, "my-volume", h.attachment.VolumeID)
	assert.Equal(t, "my-device", h.attachment.DeviceName)

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
}

func TestDetachVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &detachVolumeHandler{
		env: testutil.NewEnvironment(ctx, t),
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	hosts := []host.Host{
		{
			Id:        "my-host",
			StartedBy: "user",
			Status:    evergreen.HostRunning,
			Volumes: []host.VolumeAttachment{
				{
					VolumeID:   "my-volume",
					DeviceName: "my-device",
				},
			},
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert(ctx))
	}

	v := host.VolumeAttachment{VolumeID: "not-a-volume"}
	jsonBody, err := json.Marshal(v)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err := http.NewRequest(http.MethodGet, "/hosts/my-host/detach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.NoError(t, h.Parse(ctx, r))
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusNotFound, resp.Status())
}

func TestModifyVolumeHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &modifyVolumeHandler{
		env:  testutil.NewEnvironment(ctx, t),
		opts: &model.VolumeModifyOptions{},
	}
	h.env.Settings().Providers.AWS.MaxVolumeSizePerUser = 200
	h.env.Settings().Spawnhost.UnexpirableVolumesPerUser = 1
	volume := host.Volume{
		ID:               "volume1",
		CreatedBy:        "user",
		Size:             64,
		AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
	}
	assert.NoError(t, volume.Insert(t.Context()))

	// parse request
	opts := &model.VolumeModifyOptions{Size: 20, NewName: "my-favorite-volume"}
	jsonBody, err := json.Marshal(opts)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)
	r, err := http.NewRequest("", "", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"volume_id": "volume1"})
	assert.NoError(t, h.Parse(context.Background(), r))
	assert.Equal(t, "volume1", h.volumeID)
	assert.EqualValues(t, 20, h.opts.Size)
	assert.Equal(t, "my-favorite-volume", h.opts.NewName)

	h.provider = evergreen.ProviderNameMock
	h.opts = &model.VolumeModifyOptions{}
	// another user
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "different-user"})
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.Status())

	// volume's owner
	ctx = gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// resize
	h.opts.Size = 200
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// resize, exceeding max size
	h.opts = &model.VolumeModifyOptions{Size: 500}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	// set expiration
	h.opts = &model.VolumeModifyOptions{Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// no expiration
	h.opts = &model.VolumeModifyOptions{NoExpiration: true}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// has expiration
	h.opts = &model.VolumeModifyOptions{HasExpiration: true}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
}

func TestGetVolumesHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	h := &getVolumesHandler{}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	h1 := host.Host{
		Id:        "has-a-volume",
		StartedBy: "user",
		Volumes: []host.VolumeAttachment{
			{VolumeID: "volume1", DeviceName: "/dev/sdf4"},
		},
	}

	volumesToAdd := []host.Volume{
		{
			ID:               "volume1",
			Host:             "has-a-volume",
			CreatedBy:        "user",
			Type:             evergreen.DefaultEBSType,
			Size:             64,
			AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
		},
		{
			ID:               "volume2",
			CreatedBy:        "user",
			Type:             evergreen.DefaultEBSType,
			Size:             36,
			AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
		},
		{
			ID:        "volume3",
			CreatedBy: "different-user",
		},
	}
	for _, volumeToAdd := range volumesToAdd {
		assert.NoError(t, volumeToAdd.Insert(t.Context()))
	}
	assert.NoError(t, h1.Insert(ctx))
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	volumes, ok := resp.Data().([]model.APIVolume)
	assert.True(t, ok)
	require.Len(t, volumes, 2)

	for _, v := range volumes {
		assert.Equal(t, "user", utility.FromStringPtr(v.CreatedBy))
		assert.Equal(t, evergreen.DefaultEBSType, utility.FromStringPtr(v.Type))
		assert.Equal(t, evergreen.DefaultEBSAvailabilityZone, utility.FromStringPtr(v.AvailabilityZone))
		if utility.FromStringPtr(v.ID) == "volume1" {
			assert.Equal(t, h1.Id, utility.FromStringPtr(v.HostID))
			assert.Equal(t, h1.Volumes[0].DeviceName, utility.FromStringPtr(v.DeviceName))
			assert.Equal(t, 64, v.Size)
		} else {
			assert.Empty(t, utility.FromStringPtr(v.HostID))
			assert.Empty(t, utility.FromStringPtr(v.DeviceName))
			assert.Equal(t, 36, v.Size)
		}
	}
}

func TestGetVolumeByIDHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	h := &getVolumeByIDHandler{}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	h1 := host.Host{
		Id:        "has-a-volume",
		StartedBy: "user",
		Volumes: []host.VolumeAttachment{
			{VolumeID: "volume1", DeviceName: "/dev/sdf4"},
		},
	}

	volume := host.Volume{
		ID:               "volume1",
		Host:             "has-a-volume",
		CreatedBy:        "user",
		Type:             evergreen.DefaultEBSType,
		Size:             64,
		AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
	}
	assert.NoError(t, volume.Insert(t.Context()))
	assert.NoError(t, h1.Insert(ctx))
	r, err := http.NewRequest(http.MethodGet, "/volumes/volume1", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"volume_id": "volume1"})
	assert.NoError(t, h.Parse(ctx, r))

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	v, ok := resp.Data().(*model.APIVolume)
	assert.True(t, ok)
	require.NotNil(t, v)
	assert.Equal(t, "user", utility.FromStringPtr(v.CreatedBy))
	assert.Equal(t, evergreen.DefaultEBSType, utility.FromStringPtr(v.Type))
	assert.Equal(t, evergreen.DefaultEBSAvailabilityZone, utility.FromStringPtr(v.AvailabilityZone))
	assert.Equal(t, h1.Id, utility.FromStringPtr(v.HostID))
	assert.Equal(t, h1.Volumes[0].DeviceName, utility.FromStringPtr(v.DeviceName))
	assert.Equal(t, 64, v.Size)
}

func TestMakeSpawnHostSubscription(t *testing.T) {
	user := &user.DBUser{
		EmailAddress: "evergreen@mongodb.com",
		Settings: user.UserSettings{
			SlackUsername: "mci",
		},
	}
	_, err := makeSpawnHostSubscription("id", "non-existent", user)
	assert.Error(t, err)

	sub, err := makeSpawnHostSubscription("id", event.SlackSubscriberType, user)
	assert.NoError(t, err)
	assert.Equal(t, event.ResourceTypeHost, utility.FromStringPtr(sub.ResourceType))
	assert.Len(t, sub.Selectors, 1)
	assert.Equal(t, event.SlackSubscriberType, utility.FromStringPtr(sub.Subscriber.Type))
	assert.Equal(t, "@mci", sub.Subscriber.Target)
}
