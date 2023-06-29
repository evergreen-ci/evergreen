package cloud

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFleet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var h *host.Host
	var m *ec2FleetManager
	for name, test := range map[string]func(*testing.T){
		"SpawnHost": func(*testing.T) {
			var err error
			h, err = m.SpawnHost(context.Background(), h)
			assert.NoError(t, err)
			assert.Equal(t, "i-12345", h.Id)
		},
		"GetInstanceStatuses": func(*testing.T) {
			hosts := []host.Host{*h}

			statuses, err := m.GetInstanceStatuses(context.Background(), hosts)
			assert.NoError(t, err)
			assert.Len(t, statuses, len(hosts), "should return same number of statuses as there are hosts")
			for _, status := range statuses {
				assert.Equal(t, StatusRunning, status)
			}

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.DescribeInstancesInput.InstanceIds, 1)
			assert.Equal(t, "h1", mockClient.DescribeInstancesInput.InstanceIds[0])

			hDb, err := host.FindOneId(ctx, "h1")
			assert.NoError(t, err)
			assert.Equal(t, "us-east-1a", hDb.Zone)
		},
		"GetInstanceStatus": func(*testing.T) {
			status, err := m.GetInstanceStatus(context.Background(), h)
			assert.NoError(t, err)
			assert.Equal(t, StatusRunning, status)

			assert.Equal(t, "us-east-1a", h.Zone)
			hDb, err := host.FindOneId(ctx, "h1")
			assert.NoError(t, err)
			assert.Equal(t, "us-east-1a", hDb.Zone)
		},
		"GetInstanceStatusNonExistentInstance": func(*testing.T) {
			apiErr := &smithy.GenericAPIError{
				Code:    EC2ErrorNotFound,
				Message: "The instance ID 'test-id' does not exist",
			}
			wrappedAPIError := errors.Wrap(apiErr, "EC2 API returned error for DescribeInstances")
			mockClient := m.client.(*awsClientMock)
			mockClient.RequestGetInstanceInfoError = wrappedAPIError
			status, err := m.GetInstanceStatus(context.Background(), h)
			assert.NoError(t, err)
			assert.Equal(t, StatusNonExistent, status)
		},
		"GetInstanceStatusNonExistentReservation": func(*testing.T) {
			mockClient := m.client.(*awsClientMock)
			mockClient.RequestGetInstanceInfoError = noReservationError
			status, err := m.GetInstanceStatus(context.Background(), h)
			assert.NoError(t, err)
			assert.Equal(t, StatusNonExistent, status)
		},
		"TerminateInstance": func(*testing.T) {
			assert.NoError(t, m.TerminateInstance(context.Background(), h, "evergreen", ""))

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.TerminateInstancesInput.InstanceIds, 1)
			assert.Equal(t, "h1", mockClient.TerminateInstancesInput.InstanceIds[0])

			hDb, err := host.FindOneId(ctx, "h1")
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, hDb.Status)
		},
		"GetDNSName": func(*testing.T) {
			dnsName, err := m.GetDNSName(context.Background(), h)
			assert.NoError(t, err)
			assert.Equal(t, "public_dns_name", dnsName)
		},
		"SpawnFleetSpotHost": func(*testing.T) {
			assert.NoError(t, m.spawnFleetSpotHost(context.Background(), &host.Host{Tag: "ht_1"}, &EC2ProviderSettings{}))

			mockClient := m.client.(*awsClientMock)
			assert.Equal(t, "ht_1", *mockClient.DeleteLaunchTemplateInput.LaunchTemplateName)
		},
		"RequestFleet": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{VpcName: "my_vpc", InstanceType: "instanceType0", IAMInstanceProfileARN: "my-profile"}

			instanceID, err := m.requestFleet(context.Background(), h, ec2Settings)
			assert.NoError(t, err)
			assert.Equal(t, "i-12345", instanceID)

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.CreateFleetInput.LaunchTemplateConfigs, 1)
			assert.Equal(t, "ht_1", *mockClient.CreateFleetInput.LaunchTemplateConfigs[0].LaunchTemplateSpecification.LaunchTemplateName)
		},
		"MakeOverrides": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{
				InstanceType:          "instanceType0",
				IAMInstanceProfileARN: "my-profile",
			}
			overrides, err := m.makeOverrides(context.Background(), ec2Settings)
			assert.NoError(t, err)
			require.Len(t, overrides, 1)
			assert.Equal(t, "subnet-654321", *overrides[0].SubnetId)

			ec2Settings = &EC2ProviderSettings{
				InstanceType: "not_supported",
			}
			overrides, err = m.makeOverrides(context.Background(), ec2Settings)
			assert.NoError(t, err)
			assert.Nil(t, overrides)

			ec2Settings = &EC2ProviderSettings{
				InstanceType:          "instanceType0",
				IAMInstanceProfileARN: "my-profile",
				SubnetId:              "subnet-654321",
			}
			overrides, err = m.makeOverrides(context.Background(), ec2Settings)
			assert.NoError(t, err)
			assert.Nil(t, overrides)
		},
	} {
		h = &host.Host{
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
		typeCache[instanceRegionPair{instanceType: "instanceType0", region: evergreen.DefaultEC2Region}] = []evergreen.Subnet{{SubnetID: "subnet-654321"}}
		typeCache[instanceRegionPair{instanceType: "not_supported", region: evergreen.DefaultEC2Region}] = []evergreen.Subnet{}
		m = &ec2FleetManager{
			EC2FleetManagerOptions: &EC2FleetManagerOptions{
				client: &awsClientMock{},
				region: "test-region",
			},
			credentials: credentials.NewStaticCredentialsProvider("key", "secret", ""),
			settings: &evergreen.Settings{
				Providers: evergreen.CloudProviders{
					AWS: evergreen.AWSConfig{
						DefaultSecurityGroup: "sg-default",
						Subnets:              []evergreen.Subnet{{AZ: evergreen.DefaultEC2Region + "a", SubnetID: "subnet-654321"}},
					},
				},
			},
		}

		require.NoError(t, db.Clear(host.Collection))
		require.NoError(t, h.Insert())
		t.Run(name, test)
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
	client := &awsClientMock{}
	m := &ec2FleetManager{
		EC2FleetManagerOptions: &EC2FleetManagerOptions{client: client},
	}

	t.Run("NotYetExpired", func(t *testing.T) {
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

		assert.NoError(t, m.Cleanup(context.Background()))
		require.Len(t, client.launchTemplates, 2)
		assert.Equal(t, "lt0", *client.launchTemplates[0].LaunchTemplateId)
		assert.Equal(t, "lt1", *client.launchTemplates[1].LaunchTemplateId)
	})

	t.Run("AllExpired", func(t *testing.T) {
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

		assert.NoError(t, m.Cleanup(context.Background()))
		assert.Empty(t, client.launchTemplates)
	})

	t.Run("SomeExpired", func(t *testing.T) {
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

		assert.NoError(t, m.Cleanup(context.Background()))
		require.Len(t, client.launchTemplates, 1)
		assert.Equal(t, "lt1", *client.launchTemplates[0].LaunchTemplateId)
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
