package cloud

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFleet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(host.Collection))
	}()

	for name, test := range map[string]func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host){
		"SpawnHost": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			h, err := m.SpawnHost(ctx, h)
			assert.NoError(t, err)
			assert.Equal(t, "i-12345", h.Id)
		},
		"GetInstanceStatusesReturnsMultipleHostStatusesAndCachesData": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			h1 := h
			h2 := host.Host{
				Id:  "h2",
				Tag: "ht_2",
				Distro: distro.Distro{
					ProviderSettingsList: []*birch.Document{birch.NewDocument(
						birch.EC.String("ami", "ami"),
						birch.EC.String("instance_type", "instance"),
						birch.EC.String("key_name", "key"),
						birch.EC.String("aws_access_key_id", "key_id"),
						birch.EC.Double("bid_price", 0.001),
						birch.EC.SliceString("security_group_ids", []string{"abcdef"}),
					)},
					Provider: evergreen.ProviderNameEc2Fleet,
				},
			}
			require.NoError(t, h2.Insert(ctx))

			hosts := []host.Host{*h1, h2}

			startedAt := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
			client.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{
				Reservations: []types.Reservation{
					{
						Instances: []types.Instance{
							{
								// h2
								InstanceId:   aws.String(h2.Id),
								InstanceType: "c4.4xlarge",
								State: &types.InstanceState{
									Name: types.InstanceStateNameRunning,
								},
								LaunchTime: aws.Time(startedAt),
								Placement: &types.Placement{
									AvailabilityZone: aws.String("us-east-1b"),
								},
								PublicDnsName:    aws.String("h2_public_dns_name"),
								PrivateIpAddress: aws.String("127.0.0.1"),
								PublicIpAddress:  aws.String("127.0.0.2"),
								NetworkInterfaces: []types.InstanceNetworkInterface{
									{Ipv6Addresses: []types.InstanceIpv6Address{{Ipv6Address: aws.String("::1")}}},
								},
								BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
									{
										DeviceName: aws.String("/dev/sda1"),
										Ebs: &types.EbsInstanceBlockDevice{
											VolumeId: aws.String("vol-12345"),
										},
									},
								},
							},
						},
					},
					{
						Instances: []types.Instance{
							{
								// h1
								InstanceId:   aws.String(h1.Id),
								InstanceType: "m4.xlarge",
								State: &types.InstanceState{
									Name: types.InstanceStateNameRunning,
								},
								LaunchTime: aws.Time(startedAt),
								Placement: &types.Placement{
									AvailabilityZone: aws.String("us-east-1c"),
								},
								PublicDnsName:    aws.String("h1_public_dns_name"),
								PrivateIpAddress: aws.String("127.0.0.3"),
								PublicIpAddress:  aws.String("127.0.0.4"),
								NetworkInterfaces: []types.InstanceNetworkInterface{
									{Ipv6Addresses: []types.InstanceIpv6Address{{Ipv6Address: aws.String("::1")}}},
								},
								BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
									{
										DeviceName: aws.String("/dev/sdb1"),
										Ebs: &types.EbsInstanceBlockDevice{
											VolumeId: aws.String("vol-67890"),
										},
									},
								},
							},
						},
					},
				},
			}

			statuses, err := m.GetInstanceStatuses(ctx, hosts)
			assert.NoError(t, err)
			assert.Len(t, statuses, len(hosts), "should return same number of statuses as there are hosts")
			for hostID, status := range statuses {
				assert.Equal(t, StatusRunning, status)
				assert.Contains(t, []string{h1.Id, h2.Id}, hostID, "should return status for requested host")
			}

			dbHost1, err := host.FindOneId(ctx, h1.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost1)
			assert.Equal(t, "us-east-1c", dbHost1.Zone)
			assert.WithinDuration(t, startedAt, dbHost1.StartTime, 0)
			assert.Equal(t, "127.0.0.3", dbHost1.IPv4)
			assert.Equal(t, "127.0.0.4", dbHost1.PublicIPv4)
			assert.Equal(t, "::1", dbHost1.IP)
			assert.Equal(t, "h1_public_dns_name", dbHost1.Host)
			require.Len(t, dbHost1.Volumes, 1)
			assert.Equal(t, "vol-67890", dbHost1.Volumes[0].VolumeID)
			assert.Equal(t, "/dev/sdb1", dbHost1.Volumes[0].DeviceName)

			dbHost2, err := host.FindOneId(ctx, h2.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost2)
			assert.Equal(t, "us-east-1b", dbHost2.Zone)
			assert.WithinDuration(t, startedAt, dbHost2.StartTime, 0)
			assert.Equal(t, "127.0.0.1", dbHost2.IPv4)
			assert.Equal(t, "127.0.0.2", dbHost2.PublicIPv4)
			assert.Equal(t, "::1", dbHost2.IP)
			assert.Equal(t, "h2_public_dns_name", dbHost2.Host)
			require.Len(t, dbHost2.Volumes, 1)
			assert.Equal(t, "vol-12345", dbHost2.Volumes[0].VolumeID)
			assert.Equal(t, "/dev/sda1", dbHost2.Volumes[0].DeviceName)
		},
		"GetInstanceStatusesReturnsSingleHostStatusAndCachesData": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			hosts := []host.Host{*h}

			startedAt := time.Now().Round(time.Hour)
			client.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{
				Reservations: []types.Reservation{
					{
						Instances: []types.Instance{
							{
								InstanceId:   aws.String(h.Id),
								InstanceType: "m4.xlarge",
								State: &types.InstanceState{
									Name: types.InstanceStateNameRunning,
								},
								LaunchTime: aws.Time(startedAt),
								Placement: &types.Placement{
									AvailabilityZone: aws.String("us-east-1c"),
								},
								PublicDnsName:    aws.String("public_dns_name"),
								PrivateIpAddress: aws.String("127.0.0.1"),
								PublicIpAddress:  aws.String("127.0.0.2"),
								NetworkInterfaces: []types.InstanceNetworkInterface{
									{Ipv6Addresses: []types.InstanceIpv6Address{{Ipv6Address: aws.String("::1")}}},
								},
								BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
									{
										DeviceName: aws.String("/dev/sda1"),
										Ebs: &types.EbsInstanceBlockDevice{
											VolumeId: aws.String("vol-12345"),
										},
									},
								},
							},
						},
					},
				},
			}

			statuses, err := m.GetInstanceStatuses(ctx, hosts)
			assert.NoError(t, err)
			assert.Len(t, statuses, len(hosts), "should return same number of statuses as there are hosts")
			for _, status := range statuses {
				assert.Equal(t, StatusRunning, status)
			}

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			assert.Equal(t, "us-east-1c", dbHost.Zone)
			assert.Equal(t, startedAt, dbHost.StartTime)
			assert.Equal(t, "::1", dbHost.IP)
			assert.Equal(t, "127.0.0.1", dbHost.IPv4)
			assert.Equal(t, "127.0.0.2", dbHost.PublicIPv4)
			assert.Equal(t, "public_dns_name", dbHost.Host)
			require.Len(t, dbHost.Volumes, 1)
			assert.Equal(t, "vol-12345", dbHost.Volumes[0].VolumeID)
			assert.Equal(t, "/dev/sda1", dbHost.Volumes[0].DeviceName)
		},
		"GetInstanceInformation": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			info, err := m.GetInstanceState(ctx, h)
			assert.NoError(t, err)
			assert.Equal(t, StatusRunning, info.Status)

			assert.Equal(t, "us-east-1a", h.Zone)
			hDb, err := host.FindOneId(ctx, "h1")
			assert.NoError(t, err)
			assert.Equal(t, "us-east-1a", hDb.Zone)
		},
		"GetInstanceInformationNonExistentInstance": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			apiErr := &smithy.GenericAPIError{
				Code:    EC2ErrorNotFound,
				Message: "The instance ID 'test-id' does not exist",
			}
			wrappedAPIError := errors.Wrap(apiErr, "EC2 API returned error for DescribeInstances")
			client.RequestGetInstanceInfoError = wrappedAPIError
			info, err := m.GetInstanceState(ctx, h)
			assert.NoError(t, err)
			assert.Equal(t, StatusNonExistent, info.Status)
		},
		"GetInstanceInformationNonExistentReservation": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			client.RequestGetInstanceInfoError = noReservationError
			info, err := m.GetInstanceState(ctx, h)
			assert.NoError(t, err)
			assert.Equal(t, StatusNonExistent, info.Status)
		},
		"TerminateInstance": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			assert.NoError(t, m.TerminateInstance(ctx, h, "evergreen", ""))

			assert.Len(t, client.TerminateInstancesInput.InstanceIds, 1)
			assert.Equal(t, "h1", client.TerminateInstancesInput.InstanceIds[0])

			hDb, err := host.FindOneId(ctx, "h1")
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, hDb.Status)
		},
		"GetDNSName": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			dnsName, err := m.GetDNSName(ctx, h)
			assert.NoError(t, err)
			assert.Equal(t, "public_dns_name", dnsName)
		},
		"SpawnFleetHost": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			assert.NoError(t, m.spawnFleetHost(ctx, &host.Host{Tag: "ht_1"}, &EC2ProviderSettings{}))

			mockClient := m.client.(*awsClientMock)
			assert.Equal(t, "ht_1", *mockClient.DeleteLaunchTemplateInput.LaunchTemplateName)
		},
		"RequestFleet": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			ec2Settings := &EC2ProviderSettings{VpcName: "my_vpc", InstanceType: "instanceType0", IAMInstanceProfileARN: "my-profile"}

			instanceID, err := m.requestFleet(ctx, h, ec2Settings)
			assert.NoError(t, err)
			assert.Equal(t, "i-12345", instanceID)

			assert.Len(t, client.CreateFleetInput.LaunchTemplateConfigs, 1)
			assert.Equal(t, "ht_1", *client.CreateFleetInput.LaunchTemplateConfigs[0].LaunchTemplateSpecification.LaunchTemplateName)
		},
		"MakeOverrides": func(ctx context.Context, t *testing.T, m *ec2FleetManager, client *awsClientMock, h *host.Host) {
			ec2Settings := &EC2ProviderSettings{
				InstanceType:          "instanceType0",
				IAMInstanceProfileARN: "my-profile",
			}
			overrides, err := m.makeOverrides(ctx, ec2Settings)
			assert.NoError(t, err)
			require.Len(t, overrides, 1)
			assert.Equal(t, "subnet-654321", *overrides[0].SubnetId)

			ec2Settings = &EC2ProviderSettings{
				InstanceType: "not_supported",
			}
			overrides, err = m.makeOverrides(ctx, ec2Settings)
			assert.NoError(t, err)
			assert.Nil(t, overrides)

			ec2Settings = &EC2ProviderSettings{
				InstanceType:          "instanceType0",
				IAMInstanceProfileARN: "my-profile",
				SubnetId:              "subnet-654321",
			}
			overrides, err = m.makeOverrides(ctx, ec2Settings)
			assert.NoError(t, err)
			assert.Nil(t, overrides)
		},
	} {
		t.Run(name, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			h := &host.Host{
				Id:  "h1",
				Tag: "ht_1",
				Distro: distro.Distro{
					ProviderSettingsList: []*birch.Document{birch.NewDocument(
						birch.EC.String("ami", "ami"),
						birch.EC.String("instance_type", "instance"),
						birch.EC.String("key_name", "key"),
						birch.EC.String("aws_access_key_id", "key_id"),
						birch.EC.Double("bid_price", 0.001),
						birch.EC.SliceString("security_group_ids", []string{"abcdef"}),
					)},
					Provider: evergreen.ProviderNameEc2Fleet,
				},
			}
			require.NoError(t, db.Clear(host.Collection))
			require.NoError(t, h.Insert(ctx))

			typeCache[instanceRegionPair{instanceType: "instanceType0", region: evergreen.DefaultEC2Region}] = []evergreen.Subnet{{SubnetID: "subnet-654321"}}
			typeCache[instanceRegionPair{instanceType: "not_supported", region: evergreen.DefaultEC2Region}] = []evergreen.Subnet{}

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			client := &awsClientMock{}
			m := &ec2FleetManager{
				EC2FleetManagerOptions: &EC2FleetManagerOptions{
					client: client,
					region: "test-region",
				},
				settings: &evergreen.Settings{
					Providers: evergreen.CloudProviders{
						AWS: evergreen.AWSConfig{
							DefaultSecurityGroup: "sg-default",
							Subnets:              []evergreen.Subnet{{AZ: evergreen.DefaultEC2Region + "a", SubnetID: "subnet-654321"}},
						},
					},
				},
				env: env,
			}

			test(tctx, t, m, client, h)
		})
	}
}

func TestUploadLaunchTemplate(t *testing.T) {
	t.Run("UploadNew", func(t *testing.T) {
		m := &ec2FleetManager{
			EC2FleetManagerOptions: &EC2FleetManagerOptions{
				client: &awsClientMock{},
			},
			settings: &evergreen.Settings{},
		}
		assert.NoError(t, m.uploadLaunchTemplate(context.Background(), &host.Host{Tag: "ht_0"}, &EC2ProviderSettings{AMI: "ami"}))

		mockClient := m.client.(*awsClientMock)
		assert.Equal(t, "ami", *mockClient.CreateLaunchTemplateInput.LaunchTemplateData.ImageId)
		assert.Equal(t, "ht_0", *mockClient.CreateLaunchTemplateInput.LaunchTemplateName)
	})

	t.Run("DuplicateTemplate", func(t *testing.T) {
		m := &ec2FleetManager{
			EC2FleetManagerOptions: &EC2FleetManagerOptions{
				client: &awsClientMock{},
			},
			settings: &evergreen.Settings{},
		}
		assert.NoError(t, m.uploadLaunchTemplate(context.Background(), &host.Host{Tag: "ht_0"}, &EC2ProviderSettings{}))
		assert.NoError(t, m.uploadLaunchTemplate(context.Background(), &host.Host{Tag: "ht_0"}, &EC2ProviderSettings{}))
	})

	t.Run("InvalidName", func(t *testing.T) {
		m := &ec2FleetManager{
			EC2FleetManagerOptions: &EC2FleetManagerOptions{
				client: &awsClientMock{},
			},
			settings: &evergreen.Settings{},
		}
		assert.NoError(t, m.uploadLaunchTemplate(context.Background(), &host.Host{Tag: "ht*1"}, &EC2ProviderSettings{}))
		mockClient := m.client.(*awsClientMock)
		assert.Equal(t, "ht1", *mockClient.CreateLaunchTemplateInput.LaunchTemplateName)
	})
}

func TestCleanup(t *testing.T) {
	client := &awsClientMock{
		DescribeAddressesOutput: &ec2.DescribeAddressesOutput{},
	}
	m := &ec2FleetManager{
		EC2FleetManagerOptions: &EC2FleetManagerOptions{client: client},
	}

	t.Run("NotYetExpiredLaunchTemplates", func(t *testing.T) {
		client.launchTemplates = []types.LaunchTemplate{
			{
				LaunchTemplateId: aws.String("lt0"),
				CreateTime:       aws.Time(time.Now()),
			},
			{
				LaunchTemplateId: aws.String("lt1"),
				CreateTime:       aws.Time(time.Now()),
			},
		}

		assert.NoError(t, m.Cleanup(t.Context()))
		require.Len(t, client.launchTemplates, 2)
		assert.Equal(t, "lt0", *client.launchTemplates[0].LaunchTemplateId)
		assert.Equal(t, "lt1", *client.launchTemplates[1].LaunchTemplateId)
	})

	t.Run("AllExpiredLaunchTemplates", func(t *testing.T) {
		client.launchTemplates = []types.LaunchTemplate{
			{
				LaunchTemplateId: aws.String("lt0"),
				CreateTime:       aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
			},
			{
				LaunchTemplateId: aws.String("lt1"),
				CreateTime:       aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
			},
		}

		assert.NoError(t, m.Cleanup(t.Context()))
		assert.Empty(t, client.launchTemplates)
	})

	t.Run("SomeExpiredLaunchTemplates", func(t *testing.T) {
		client.launchTemplates = []types.LaunchTemplate{
			{
				LaunchTemplateId: aws.String("lt0"),
				CreateTime:       aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
			},
			{
				LaunchTemplateId: aws.String("lt1"),
				CreateTime:       aws.Time(time.Now()),
			},
		}

		assert.NoError(t, m.Cleanup(t.Context()))
		require.Len(t, client.launchTemplates, 1)
		assert.Equal(t, "lt1", *client.launchTemplates[0].LaunchTemplateId)
	})

	t.Run("FreesIPAddressFromTerminatedHost", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(host.Collection, host.IPAddressCollection))
		defer func() {
			assert.NoError(t, db.ClearCollections(host.Collection, host.IPAddressCollection))
		}()
		h1 := &host.Host{
			Id:              "h1",
			Tag:             "ht_1",
			Status:          evergreen.HostTerminated,
			IPAllocationID:  "eipalloc-12345",
			IPAssociationID: "eipassoc-12345",
		}
		h2 := &host.Host{
			Id:              "h2",
			Tag:             "ht_2",
			Status:          evergreen.HostRunning,
			IPAllocationID:  "eipalloc-67890",
			IPAssociationID: "eipassoc-67890",
		}
		require.NoError(t, h1.Insert(t.Context()))
		require.NoError(t, h2.Insert(t.Context()))

		ipAddr1 := &host.IPAddress{
			ID:           "ip1",
			AllocationID: "eipalloc-12345",
			HostTag:      h1.Tag,
		}
		require.NoError(t, ipAddr1.Insert(t.Context()))
		ipAddr2 := &host.IPAddress{
			ID:           "ip2",
			AllocationID: "eipalloc-67890",
			HostTag:      h2.Tag,
		}
		require.NoError(t, ipAddr2.Insert(t.Context()))

		require.NoError(t, m.Cleanup(t.Context()))

		dbIPAddr1, err := host.FindIPAddressByAllocationID(t.Context(), ipAddr1.AllocationID)
		require.NoError(t, err)
		require.NotZero(t, dbIPAddr1)
		assert.Empty(t, dbIPAddr1.HostTag, "IP address should be freed from terminated host")

		dbIPAddr2, err := host.FindIPAddressByAllocationID(t.Context(), ipAddr2.AllocationID)
		require.NoError(t, err)
		require.NotZero(t, dbIPAddr2)
		assert.Equal(t, h2.Tag, dbIPAddr2.HostTag, "IP address should remain associated with running host")
	})
}

func TestInstanceTypeAZCache(t *testing.T) {
	cache := instanceTypeSubnetCache{}
	defaultRegionClient := &awsClientMock{
		DescribeInstanceTypeOfferingsOutput: &ec2.DescribeInstanceTypeOfferingsOutput{
			InstanceTypeOfferings: []types.InstanceTypeOffering{
				{
					InstanceType: "instanceType0",
					Location:     aws.String(evergreen.DefaultEBSAvailabilityZone),
				},
			},
		},
	}
	settings := &evergreen.Settings{}
	settings.Providers.AWS.Subnets = []evergreen.Subnet{{SubnetID: "sn0", AZ: evergreen.DefaultEBSAvailabilityZone}}

	azsWithInstanceType, err := cache.subnetsWithInstanceType(context.Background(), settings, defaultRegionClient, instanceRegionPair{instanceType: "instanceType0", region: evergreen.DefaultEC2Region})
	assert.NoError(t, err)
	assert.Len(t, azsWithInstanceType, 1)
	assert.Equal(t, "sn0", azsWithInstanceType[0].SubnetID)

	// cache is populated
	subnets, ok := cache[instanceRegionPair{instanceType: "instanceType0", region: evergreen.DefaultEC2Region}]
	assert.True(t, ok)
	assert.Len(t, subnets, 1)

	// unsupported instance type
	defaultRegionClient.DescribeInstanceTypeOfferingsOutput = &ec2.DescribeInstanceTypeOfferingsOutput{}
	azsWithInstanceType, err = cache.subnetsWithInstanceType(context.Background(), settings, defaultRegionClient, instanceRegionPair{instanceType: "not_supported", region: evergreen.DefaultEC2Region})
	assert.NoError(t, err)
	assert.Empty(t, azsWithInstanceType)
}
