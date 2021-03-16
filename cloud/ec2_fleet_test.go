package cloud

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFleet(t *testing.T) {
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
			for _, status := range statuses {
				assert.Equal(t, StatusRunning, status)
			}

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.DescribeInstancesInput.InstanceIds, 1)
			assert.Equal(t, "h1", *mockClient.DescribeInstancesInput.InstanceIds[0])

			hDb, err := host.FindOneId("h1")
			assert.NoError(t, err)
			assert.Equal(t, "us-east-1a", hDb.Zone)
		},
		"GetInstanceStatus": func(*testing.T) {
			status, err := m.GetInstanceStatus(context.Background(), h)
			assert.NoError(t, err)
			assert.Equal(t, StatusRunning, status)

			assert.Equal(t, "us-east-1a", h.Zone)
			hDb, err := host.FindOneId("h1")
			assert.NoError(t, err)
			assert.Equal(t, "us-east-1a", hDb.Zone)
		},
		"TerminateInstance": func(*testing.T) {
			assert.NoError(t, m.TerminateInstance(context.Background(), h, "evergreen", ""))

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.TerminateInstancesInput.InstanceIds, 1)
			assert.Equal(t, "h1", *mockClient.TerminateInstancesInput.InstanceIds[0])

			hDb, err := host.FindOneId("h1")
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, hDb.Status)
		},
		"GetDNSName": func(*testing.T) {
			dnsName, err := m.GetDNSName(context.Background(), h)
			assert.NoError(t, err)
			assert.Equal(t, "public_dns_name", dnsName)
		},
		"SpawnFleetSpotHost": func(*testing.T) {
			assert.NoError(t, m.spawnFleetSpotHost(context.Background(), &host.Host{}, &EC2ProviderSettings{}))

			mockClient := m.client.(*awsClientMock)
			assert.Equal(t, "templateID", *mockClient.DeleteLaunchTemplateInput.LaunchTemplateId)
		},
		"UploadLaunchTemplate": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{AMI: "ami"}
			templateID, templateVersion, err := m.uploadLaunchTemplate(context.Background(), &host.Host{}, ec2Settings)
			assert.NoError(t, err)
			assert.Equal(t, "templateID", *templateID)
			assert.Equal(t, int64(1), *templateVersion)

			mockClient := m.client.(*awsClientMock)
			assert.Equal(t, "ami", *mockClient.CreateLaunchTemplateInput.LaunchTemplateData.ImageId)
		},
		"RequestFleet": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{VpcName: "my_vpc", InstanceType: "instanceType0"}

			instanceID, err := m.requestFleet(context.Background(), ec2Settings, aws.String("templateID"), aws.Int64(1))
			assert.NoError(t, err)
			assert.Equal(t, "i-12345", *instanceID)

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.CreateFleetInput.LaunchTemplateConfigs, 1)
			assert.Equal(t, "templateID", *mockClient.CreateFleetInput.LaunchTemplateConfigs[0].LaunchTemplateSpecification.LaunchTemplateId)
		},
		"MakeOverrides": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{
				InstanceType: "instanceType0",
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
				InstanceType: "instanceType0",
				SubnetId:     "subnet-654321",
			}
			overrides, err = m.makeOverrides(context.Background(), ec2Settings)
			assert.NoError(t, err)
			assert.Nil(t, overrides)
		},
		"SubnetMatchesAz": func(*testing.T) {
			subnet := &ec2.Subnet{
				Tags: []*ec2.Tag{
					{Key: aws.String("key1"), Value: aws.String("value1")},
					{Key: aws.String("Name"), Value: aws.String("mysubnet_us-east-extra")},
				},
				AvailabilityZone: aws.String("us-east-1a"),
			}
			assert.False(t, subnetMatchesAz(subnet))

			subnet = &ec2.Subnet{
				Tags: []*ec2.Tag{
					{Key: aws.String("key1"), Value: aws.String("value1")},
					{Key: aws.String("Name"), Value: aws.String("mysubnet_us-east-1a")},
				},
				AvailabilityZone: aws.String("us-east-1a"),
			}
			assert.True(t, subnetMatchesAz(subnet))
		},
		"CostForDuration": func(*testing.T) {
			start := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
			end := start.Add(time.Hour * 24)
			h = &host.Host{
				ComputeCostPerHour: float64(1),
				VolumeTotalSize:    int64(30),
				Zone:               "us-east-1a",
			}
			pkgCachingPriceFetcher.ebsPrices = make(map[string]float64)
			pkgCachingPriceFetcher.ebsPrices["us-east-1"] = float64(1)

			cost, err := m.CostForDuration(context.Background(), h, start, end)
			assert.NoError(t, err)
			assert.EqualValues(t, 25, cost)
		},
	} {
		h = &host.Host{
			Id: "h1",
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
			credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
				AccessKeyID:     "key",
				SecretAccessKey: "secret",
			}),
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

func TestInstanceTypeAZCache(t *testing.T) {
	cache := instanceTypeSubnetCache{}
	defaultRegionClient := &awsClientMock{
		DescribeInstanceTypeOfferingsOutput: &ec2.DescribeInstanceTypeOfferingsOutput{
			InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
				{
					InstanceType: aws.String("instanceType0"),
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
