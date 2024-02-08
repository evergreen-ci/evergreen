package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	r53Types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanLaunchTemplateName(t *testing.T) {
	for name, params := range map[string]struct {
		input    string
		expected string
	}{
		"AlreadyClean": {
			input:    "abcdefghijklmnopqrstuvwxyz0123456789_()/-",
			expected: "abcdefghijklmnopqrstuvwxyz0123456789_()/-",
		},
		"IncludesInvalid": {
			input:    "abcdef*123456",
			expected: "abcdef123456",
		},
		"AllInvalid": {
			input:    "!@#$%^&*",
			expected: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expected, cleanLaunchTemplateName(params.input))
		})
	}
}

func TestUsesHourlyBilling(t *testing.T) {
	for name, testCase := range map[string]struct {
		distro distro.Distro
		hourly bool
	}{
		"ubuntu": {
			distro: distro.Distro{
				Id:   "ubuntu",
				Arch: "linux",
			},
			hourly: false,
		},
		"windows": {
			distro: distro.Distro{
				Id:   "windows-large",
				Arch: "windows",
			},
			hourly: false,
		},
		"suse": {
			distro: distro.Distro{
				Id:   "suse15-large",
				Arch: "linux",
			},
			hourly: true,
		},
		"mac": {
			distro: distro.Distro{
				Id:   "macos-1100",
				Arch: "osx",
			},
			hourly: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, UsesHourlyBilling(&testCase.distro), testCase.hourly)
		})
	}
}

func TestSetHostPersistentDNSName(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, instance *types.Instance, client *awsClientMock){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, instance *types.Instance, client *awsClientMock) {
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, setHostPersistentDNSName(ctx, env, h, instance, client))

			assert.NotZero(t, h.PersistentDNSName)
			assert.True(t, strings.HasSuffix(h.PersistentDNSName, env.Settings().Providers.AWS.PersistentDNS.Domain))
			assert.Equal(t, utility.FromStringPtr(instance.PublicIpAddress), h.PublicIPv4)

			require.NotZero(t, client.ChangeResourceRecordSetsInput)
			require.NotZero(t, client.ChangeResourceRecordSetsInput.ChangeBatch)
			require.Len(t, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes, 1)
			require.NotZero(t, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet)
			require.Len(t, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet.ResourceRecords, 1)
			assert.Equal(t, env.Settings().Providers.AWS.PersistentDNS.HostedZoneID, utility.FromStringPtr(client.ChangeResourceRecordSetsInput.HostedZoneId))
			assert.Equal(t, r53Types.ChangeActionUpsert, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].Action)
			assert.Equal(t, h.PersistentDNSName, utility.FromStringPtr(client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet.Name), "DNS record name should match host's persistent DNS name")
			assert.Equal(t, h.PublicIPv4, utility.FromStringPtr(client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet.ResourceRecords[0].Value), "DNS record IP address should match host's IP address")

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, h.PersistentDNSName, dbHost.PersistentDNSName)
			assert.Equal(t, h.PublicIPv4, dbHost.PublicIPv4)
		},
		"FailsForInstanceWithoutIPv4Address": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, instance *types.Instance, client *awsClientMock) {
			instance.PublicIpAddress = nil
			require.NoError(t, h.Insert(ctx))
			assert.Error(t, setHostPersistentDNSName(ctx, env, h, instance, client))

			assert.Zero(t, h.PersistentDNSName)
			assert.Zero(t, h.PublicIPv4)

			assert.Zero(t, client.ChangeResourceRecordSetsInput)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Zero(t, dbHost.PersistentDNSName)
			assert.Zero(t, dbHost.PublicIPv4)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(host.Collection))

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.Providers.AWS.PersistentDNS = evergreen.PersistentDNSConfig{
				HostedZoneID: "hosted_zone_id",
				Domain:       "example.com",
			}
			const hostID = "host_id"
			h := &host.Host{
				Id:        hostID,
				StartedBy: "some.user",
			}
			instance := &types.Instance{
				InstanceId:      aws.String(hostID),
				PublicIpAddress: aws.String("127.0.0.1"),
			}

			tCase(ctx, t, env, h, instance, &awsClientMock{})
		})
	}
}

func TestDeleteHostPersistentDNSName(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, client *awsClientMock){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, client *awsClientMock) {
			persistentDNS := h.PersistentDNSName
			ipv4Addr := h.PublicIPv4
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, deleteHostPersistentDNSName(ctx, env, h, client))

			assert.Zero(t, h.PersistentDNSName)
			assert.Zero(t, h.PublicIPv4)

			require.NotZero(t, client.ChangeResourceRecordSetsInput)
			require.NotZero(t, client.ChangeResourceRecordSetsInput.ChangeBatch)
			require.Len(t, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes, 1)
			require.NotZero(t, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet)
			require.Len(t, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet.ResourceRecords, 1)
			assert.Equal(t, env.Settings().Providers.AWS.PersistentDNS.HostedZoneID, utility.FromStringPtr(client.ChangeResourceRecordSetsInput.HostedZoneId))
			assert.Equal(t, r53Types.ChangeActionDelete, client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].Action)
			assert.Equal(t, persistentDNS, utility.FromStringPtr(client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet.Name), "DNS record to delete should match host's persistent DNS name")
			assert.Equal(t, ipv4Addr, utility.FromStringPtr(client.ChangeResourceRecordSetsInput.ChangeBatch.Changes[0].ResourceRecordSet.ResourceRecords[0].Value), "DNS record IP address to delete should match host's IP address")

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Zero(t, dbHost.PersistentDNSName)
			assert.Zero(t, dbHost.PublicIPv4)
		},
		"NoopsForInstanceWithoutIPv4AddressOrPersistentDNSName": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, client *awsClientMock) {
			h.PublicIPv4 = ""
			h.PersistentDNSName = ""
			require.NoError(t, h.Insert(ctx))
			assert.NoError(t, deleteHostPersistentDNSName(ctx, env, h, client))

			assert.NotZero(t, h.PersistentDNSName)
			assert.Zero(t, h.PublicIPv4)

			assert.Zero(t, client.ChangeResourceRecordSetsInput)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.NotZero(t, dbHost.PersistentDNSName)
			assert.Zero(t, dbHost.PublicIPv4)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(host.Collection))

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.Providers.AWS.PersistentDNS = evergreen.PersistentDNSConfig{
				HostedZoneID: "hosted_zone_id",
				Domain:       "example.com",
			}
			h := &host.Host{
				Id:                "host_id",
				StartedBy:         "some.user",
				PersistentDNSName: "somebody-abc12.example.com",
				PublicIPv4:        "127.0.0.1",
			}

			tCase(ctx, t, env, h, &awsClientMock{})
		})
	}
}
