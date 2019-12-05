package cloud

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFleet(t *testing.T) {
	var h *host.Host
	m := &ec2FleetManager{
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
				},
			},
		},
	}

	for name, test := range map[string]func(*testing.T){
		"SpawnHost": func(*testing.T) {
			h, err := m.SpawnHost(context.Background(), h)
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
			assert.NoError(t, m.spawnFleetSpotHost(context.Background(), &host.Host{}, &EC2ProviderSettings{}, []*ec2.LaunchTemplateBlockDeviceMappingRequest{}))

			mockClient := m.client.(*awsClientMock)
			assert.Equal(t, "templateID", *mockClient.DeleteLaunchTemplateInput.LaunchTemplateId)
		},
		"UploadLaunchTemplate": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{AMI: "ami"}
			templateID, templateVersion, err := m.uploadLaunchTemplate(context.Background(), &host.Host{}, ec2Settings, []*ec2.LaunchTemplateBlockDeviceMappingRequest{})
			assert.NoError(t, err)
			assert.Equal(t, "templateID", *templateID)
			assert.Equal(t, int64(1), *templateVersion)

			mockClient := m.client.(*awsClientMock)
			assert.Equal(t, "ami", *mockClient.CreateLaunchTemplateInput.LaunchTemplateData.ImageId)
		},
		"RequestFleet": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{VpcName: "my_vpc"}

			instanceID, err := m.requestFleet(context.Background(), ec2Settings, aws.String("templateID"), aws.Int64(1))
			assert.NoError(t, err)
			assert.Equal(t, "i-12345", *instanceID)

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.CreateFleetInput.LaunchTemplateConfigs, 1)
			assert.Equal(t, "templateID", *mockClient.CreateFleetInput.LaunchTemplateConfigs[0].LaunchTemplateSpecification.LaunchTemplateId)
		},
		"MakeOverrides": func(*testing.T) {
			ec2Settings := &EC2ProviderSettings{VpcName: "vpc-123456"}

			overrides, err := m.makeOverrides(context.Background(), ec2Settings)
			assert.NoError(t, err)
			assert.Len(t, overrides, 1)
			assert.Equal(t, "subnet-654321", *overrides[0].SubnetId)

			mockClient := m.client.(*awsClientMock)
			assert.Len(t, mockClient.DescribeVpcsInput.Filters, 1)
			assert.Equal(t, "tag:Name", *mockClient.DescribeVpcsInput.Filters[0].Name)
			assert.Len(t, mockClient.DescribeVpcsInput.Filters[0].Values, 1)
			assert.Equal(t, ec2Settings.VpcName, *mockClient.DescribeVpcsInput.Filters[0].Values[0])

			assert.Len(t, mockClient.DescribeSubnetsInput.Filters, 1)
			assert.Equal(t, "vpc-id", *mockClient.DescribeSubnetsInput.Filters[0].Name)
			assert.Len(t, mockClient.DescribeSubnetsInput.Filters[0].Values, 1)
			assert.Equal(t, "vpc-123456", *mockClient.DescribeSubnetsInput.Filters[0].Values[0])
		},
		"SubnetMatchesAz": func(*testing.T) {
			subnet := &ec2.Subnet{
				Tags: []*ec2.Tag{
					&ec2.Tag{Key: aws.String("key1"), Value: aws.String("value1")},
					&ec2.Tag{Key: aws.String("Name"), Value: aws.String("mysubnet_us-east-extra")},
				},
				AvailabilityZone: aws.String("us-east-1a"),
			}
			assert.False(t, subnetMatchesAz(subnet))

			subnet = &ec2.Subnet{
				Tags: []*ec2.Tag{
					&ec2.Tag{Key: aws.String("key1"), Value: aws.String("value1")},
					&ec2.Tag{Key: aws.String("Name"), Value: aws.String("mysubnet_us-east-1a")},
				},
				AvailabilityZone: aws.String("us-east-1a"),
			}
			assert.True(t, subnetMatchesAz(subnet))
		},
	} {
		h = &host.Host{
			Id: "h1",
			Distro: distro.Distro{
				ProviderSettings: &map[string]interface{}{
					"key_name":           "key",
					"aws_access_key_id":  "key_id",
					"ami":                "ami",
					"instance_type":      "instance",
					"security_group_ids": []string{"abcdef"},
					"bid_price":          float64(0.001),
				},
				Provider: evergreen.ProviderNameEc2Fleet,
			},
		}
		require.NoError(t, db.Clear(host.Collection))
		require.NoError(t, h.Insert())
		t.Run(name, test)
	}
}
