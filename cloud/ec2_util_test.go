package cloud

import (
	"context"
	"strings"
	"testing"

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
	const ipv4Addr = "127.0.0.1"
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, client *awsClientMock){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, client *awsClientMock) {
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, setHostPersistentDNSName(ctx, env, h, ipv4Addr, client))

			assert.NotZero(t, h.PersistentDNSName)
			assert.True(t, strings.HasSuffix(h.PersistentDNSName, env.Settings().Providers.AWS.PersistentDNS.Domain))
			assert.Equal(t, ipv4Addr, h.PublicIPv4)

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
		"FailsForEmptyIPv4Address": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, client *awsClientMock) {
			require.NoError(t, h.Insert(ctx))
			assert.Error(t, setHostPersistentDNSName(ctx, env, h, "", client))

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

			tCase(ctx, t, env, h, &awsClientMock{})
		})
	}
}

func TestAllocateIPAddressForHost(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, host.IPAddressCollection))
	}()

	serviceFlags, err := evergreen.GetServiceFlags(t.Context())
	require.NoError(t, err)
	originalFlags := *serviceFlags
	serviceFlags.ElasticIPsDisabled = false
	require.NoError(t, evergreen.SetServiceFlags(t.Context(), *serviceFlags))
	defer func() {
		assert.NoError(t, evergreen.SetServiceFlags(t.Context(), originalFlags))
	}()

	checkHostAssociatedWithIPAddr := func(t *testing.T, hostTag string, allocationID string) {
		dbIPAddr, err := host.FindIPAddressByAllocationID(t.Context(), allocationID)
		require.NoError(t, err)
		require.NotZero(t, dbIPAddr)
		assert.Equal(t, hostTag, dbIPAddr.HostTag)

		dbHost, err := host.FindOneByIdOrTag(t.Context(), hostTag)
		require.NoError(t, err)
		require.NotNil(t, dbHost)
		assert.Equal(t, allocationID, dbHost.IPAllocationID)
	}

	for tName, tCase := range map[string]func(t *testing.T, h *host.Host){
		"SucceedsWithElasticIPsEnabled": func(t *testing.T, h *host.Host) {
			ipAddr := &host.IPAddress{
				ID:           "ip_addr",
				AllocationID: "eipalloc-123456789",
			}
			require.NoError(t, ipAddr.Insert(t.Context()))

			require.NoError(t, allocateIPAddressForHost(t.Context(), h))

			assert.Equal(t, ipAddr.AllocationID, h.IPAllocationID)

			checkHostAssociatedWithIPAddr(t, h.Tag, ipAddr.AllocationID)
		},
		"ErrorsWithNoAvailableIPAddresses": func(t *testing.T, h *host.Host) {
			assert.Error(t, allocateIPAddressForHost(t.Context(), h))

			assert.Empty(t, h.IPAllocationID)

			dbHost, err := host.FindOneId(t.Context(), h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.Empty(t, dbHost.IPAllocationID)
		},
		"AssignsOnlyFreeIPAddresses": func(t *testing.T, h *host.Host) {
			ipAddr1 := &host.IPAddress{
				ID:           "ip_addr1",
				AllocationID: "eipalloc-111111111",
				HostTag:      "some_other_host1",
			}
			ipAddr2 := &host.IPAddress{
				ID:           "ip_addr2",
				AllocationID: "eipalloc-222222222",
			}
			ipAddr3 := &host.IPAddress{
				ID:           "ip_addr3",
				AllocationID: "eipalloc-333333333",
				HostTag:      "some_other_host2",
			}
			require.NoError(t, ipAddr1.Insert(t.Context()))
			require.NoError(t, ipAddr2.Insert(t.Context()))
			require.NoError(t, ipAddr3.Insert(t.Context()))

			require.NoError(t, allocateIPAddressForHost(t.Context(), h))

			assert.NotEmpty(t, h.IPAllocationID)
			assert.Equal(t, ipAddr2.AllocationID, h.IPAllocationID)

			checkHostAssociatedWithIPAddr(t, h.Tag, ipAddr2.AllocationID)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, host.IPAddressCollection))

			h := &host.Host{
				Id:  "host_id",
				Tag: "host_tag",
			}
			require.NoError(t, h.Insert(t.Context()))

			tCase(t, h)
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
