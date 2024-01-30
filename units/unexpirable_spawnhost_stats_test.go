package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUnexpirableSpawnHostStatsJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.TestSpan(ctx, t)

	j, ok := NewUnexpirableSpawnHostStatsJob(utility.RoundPartOfMinute(0).Format(TSFormat)).(*unexpirableSpawnHostStatsJob)
	require.True(t, ok)

	assert.NotZero(t, j.ID())
}

func TestUnexpirableSpawnHostStatsJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *unexpirableSpawnHostStatsJob){
		"ReturnsZeroStatsForNoHosts": func(ctx context.Context, t *testing.T, j *unexpirableSpawnHostStatsJob) {
			stats := j.getStats(nil)
			assert.Zero(t, stats.totalUptime)
			assert.Empty(t, stats.uptimeSecsByDistro)
			assert.Empty(t, stats.uptimeSecsByInstanceType)
		},
		"ReturnsStatsForHosts": func(ctx context.Context, t *testing.T, j *unexpirableSpawnHostStatsJob) {
			hosts := []host.Host{
				{
					Id: "h0",
					Distro: distro.Distro{
						Id:       "distro0",
						Provider: evergreen.ProviderNameEc2OnDemand,
						ProviderSettingsList: []*birch.Document{
							birch.NewDocument(birch.EC.String("instance_type", "m5.xlarge")),
						},
					},
					InstanceType: "m5.xlarge",
				},
				{
					Id: "h1",
					Distro: distro.Distro{
						Id:       "distro0",
						Provider: evergreen.ProviderNameEc2OnDemand,
						ProviderSettingsList: []*birch.Document{
							birch.NewDocument(birch.EC.String("instance_type", "m5.xlarge")),
						},
					},
					InstanceType: "m5.xlarge",
				},
				{
					Id: "h2",
					Distro: distro.Distro{
						Id:       "distro1",
						Provider: evergreen.ProviderNameEc2OnDemand,
						ProviderSettingsList: []*birch.Document{
							birch.NewDocument(birch.EC.String("instance_type", "m5.xlarge")),
						},
					},
					InstanceType: "m5.xlarge",
				},
				{
					Id: "h3",
					Distro: distro.Distro{
						Id:       "distro2",
						Provider: evergreen.ProviderNameEc2OnDemand,
						ProviderSettingsList: []*birch.Document{
							birch.NewDocument(birch.EC.String("instance_type", "c5.xlarge")),
						},
					},
					InstanceType: "c5.xlarge",
				},
				{
					Id: "h4",
					Distro: distro.Distro{
						Id:       "distro0",
						Provider: evergreen.ProviderNameEc2OnDemand,
						ProviderSettingsList: []*birch.Document{
							birch.NewDocument(birch.EC.String("instance_type", "m5.xlarge")),
						},
					},
					InstanceType: "m5.xlarge",
				},
				{
					Id: "h5",
					Distro: distro.Distro{
						Id:       "distro0",
						Provider: evergreen.ProviderNameEc2OnDemand,
						ProviderSettingsList: []*birch.Document{
							birch.NewDocument(birch.EC.String("instance_type", "m5.xlarge")),
						},
					},
					InstanceType: "m5.xlarge",
				},
			}

			stats := j.getStats(hosts)

			const day = 24 * time.Hour
			daySecs := int(day.Seconds())
			assert.Equal(t, time.Duration(len(hosts))*24*time.Hour, stats.totalUptime)

			assert.Len(t, stats.uptimeSecsByDistro, 3)
			assert.Equal(t, 4*daySecs, stats.uptimeSecsByDistro["distro0"])
			assert.Equal(t, daySecs, stats.uptimeSecsByDistro["distro1"])
			assert.Equal(t, daySecs, stats.uptimeSecsByDistro["distro2"])

			assert.Len(t, stats.uptimeSecsByInstanceType, 2)
			assert.Equal(t, 5*daySecs, stats.uptimeSecsByInstanceType["m5.xlarge"])
			assert.Equal(t, daySecs, stats.uptimeSecsByInstanceType["c5.xlarge"])
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, cancel := context.WithCancel(ctx)
			defer cancel()
			j, ok := NewUnexpirableSpawnHostStatsJob("").(*unexpirableSpawnHostStatsJob)
			require.True(t, ok)
			tCase(tctx, t, j)
		})
	}
}
