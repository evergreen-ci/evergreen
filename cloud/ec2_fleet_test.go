package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestSpawnHost(t *testing.T) {
	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

	h := &host.Host{
		Distro: distro.Distro{
			Provider: evergreen.ProviderNameEc2Fleet,
			ProviderSettings: &map[string]interface{}{
				"ami":                "AMI-ubuntu",
				"instance_type":      "m1.small",
				"security_group_ids": []string{"sg1"},
				"mount_points":       []MountPoint{},
				"key_name":           "keyName",
			},
		},
	}

	var err error
	h, err = m.SpawnHost(context.Background(), h)
	assert.NoError(t, err)
	assert.Equal(t, "i-12345", h.Id)
}

func TestGetInstanceStatuses(t *testing.T) {
	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

	hosts := []host.Host{{Id: "instance_id"}}

	statuses, err := m.GetInstanceStatuses(context.Background(), hosts)
	assert.NoError(t, err)
	for _, status := range statuses {
		assert.Equal(t, StatusRunning, status)
	}

	mockClient := m.client.(*awsClientMock)
	assert.Len(t, mockClient.DescribeInstancesInput.InstanceIds, 1)
	assert.Equal(t, "instance_id", *mockClient.DescribeInstancesInput.InstanceIds[0])
}

func TestGetInstanceStatus(t *testing.T) {
	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}
	status, err := m.GetInstanceStatus(context.Background(), &host.Host{Id: "h1"})
	assert.NoError(t, err)
	assert.Equal(t, StatusRunning, status)
}

func TestTerminateInstance(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection))

	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

	h := &host.Host{Id: "h1"}
	assert.NoError(t, h.Insert())

	assert.NoError(t, m.TerminateInstance(context.Background(), h, "evergreen"))

	mockClient := m.client.(*awsClientMock)
	assert.Len(t, mockClient.TerminateInstancesInput.InstanceIds, 1)
	assert.Equal(t, "h1", *mockClient.TerminateInstancesInput.InstanceIds[0])

	hDb, err := host.FindOneId("h1")
	assert.NoError(t, err)
	assert.Equal(t, evergreen.HostTerminated, hDb.Status)
}

func TestGetDNSName(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection))

	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

	h := &host.Host{Id: "h1"}
	assert.NoError(t, h.Insert())

	dnsName, err := m.GetDNSName(context.Background(), h)
	assert.NoError(t, err)
	assert.Equal(t, "public_dns_name", dnsName)

	assert.Equal(t, "us-east-1a", h.Zone)
	hDb, err := host.FindOneId("h1")
	assert.NoError(t, err)
	assert.Equal(t, "us-east-1a", hDb.Zone)
}

func TestSpawnFleetSpotHost(t *testing.T) {
	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

	resources, err := m.spawnFleetSpotHost(context.Background(), &host.Host{}, &EC2ProviderSettings{}, []*ec2.LaunchTemplateBlockDeviceMappingRequest{})
	assert.NoError(t, err)
	assert.Len(t, resources, 1)
	assert.Equal(t, "i-12345", resources[0])

	mockClient := m.client.(*awsClientMock)
	assert.Equal(t, "templateID", *mockClient.DeleteLaunchTemplateInput.LaunchTemplateId)
}

func TestUploadLaunchTemplate(t *testing.T) {
	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

	ec2Settings := &EC2ProviderSettings{AMI: "ami"}
	templateID, templateVersion, err := m.uploadLaunchTemplate(context.Background(), &host.Host{}, ec2Settings, []*ec2.LaunchTemplateBlockDeviceMappingRequest{})
	assert.NoError(t, err)
	assert.Equal(t, "templateID", *templateID)
	assert.Equal(t, int64(1), *templateVersion)

	mockClient := m.client.(*awsClientMock)
	assert.Equal(t, "ami", *mockClient.CreateLaunchTemplateInput.LaunchTemplateData.ImageId)
}

func TestRequestFleet(t *testing.T) {
	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

	ec2Settings := &EC2ProviderSettings{VpcName: "my_vpc"}

	instanceID, err := m.requestFleet(context.Background(), ec2Settings, aws.String("templateID"), aws.Int64(1))
	assert.NoError(t, err)
	assert.Equal(t, "i-12345", *instanceID)

	mockClient := m.client.(*awsClientMock)
	assert.Len(t, mockClient.CreateFleetInput.LaunchTemplateConfigs, 1)
	assert.Equal(t, "templateID", *mockClient.CreateFleetInput.LaunchTemplateConfigs[0].LaunchTemplateSpecification.LaunchTemplateId)
}

func TestMakeOverrides(t *testing.T) {
	m := &ec2FleetManager{
		client: &awsClientMock{},
		credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		}),
	}

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
}

func TestSubnetMatchesAz(t *testing.T) {
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
}
