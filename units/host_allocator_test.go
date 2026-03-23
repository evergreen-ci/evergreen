package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleTaskDistroHostAllocatorJob(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)

	assert := assert.New(t)
	require := require.New(t)

	env := &mock.Environment{}
	require.NoError(env.Configure(ctx))

	require.NoError(db.ClearCollections(distro.Collection, model.TaskQueuesCollection, host.Collection))
	defer func() {
		assert.NoError(db.ClearCollections(distro.Collection, model.TaskQueuesCollection, host.Collection))
	}()

	d := distro.Distro{
		Id:               "d",
		SingleTaskDistro: true,
	}
	require.NoError(d.Insert(ctx))

	tq := model.TaskQueue{
		Distro: d.Id,
		Queue: []model.TaskQueueItem{
			{
				Id: "t1",
			},
			{
				Id: "t2",
			},
			{
				Id: "t3",
			},
		},
		DistroQueueInfo: model.DistroQueueInfo{
			Length:                    3,
			LengthWithDependenciesMet: 2,
		},
	}
	require.NoError(tq.Save(t.Context()))

	h := host.Host{
		Id:     "h1",
		Distro: d,
		Status: evergreen.HostProvisioning,
	}
	require.NoError(h.Insert(ctx))

	j := NewHostAllocatorJob(env, d.Id, time.Now())

	j.Run(ctx)

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	hosts, err := host.AllActiveHosts(ctx, d.Id)
	require.NoError(err)
	require.Len(hosts, 2)
}

func TestApplyTaskHostOverrides(t *testing.T) {
	// makeProviderSettings creates a birch provider settings document from an
	// EC2ProviderSettings struct in the same way the running application does.
	makeProviderSettings := func(t *testing.T, s cloud.EC2ProviderSettings) *birch.Document {
		t.Helper()
		doc, err := s.ToDocument()
		require.NoError(t, err)
		return doc
	}
	// getProviderSettings reads EC2ProviderSettings back from a birch document.
	getProviderSettings := func(t *testing.T, doc *birch.Document) cloud.EC2ProviderSettings {
		t.Helper()
		var s cloud.EC2ProviderSettings
		require.NoError(t, s.FromDocument(doc))
		return s
	}

	nonZeroOverrides := &distro.TaskHostOverrides{
		ProviderAccount:              "override-account",
		IAMInstanceProfileARN:        "arn:aws:iam::123456789:instance-profile/override",
		SecurityGroupIDs:             []string{"sg-override1", "sg-override2"},
		SubnetID:                     "subnet-override",
		DoNotAssignPublicIPv4Address: true,
	}

	t.Run("NilOverridesFieldDoesNotOverrideAnything", func(t *testing.T) {
		original := cloud.EC2ProviderSettings{
			AMI:              "ami-original",
			InstanceType:     "m5.xlarge",
			SecurityGroupIDs: []string{"sg-original"},
			SubnetId:         "subnet-original",
			Region:           evergreen.DefaultEC2Region,
		}
		d := distro.Distro{
			Provider:             evergreen.ProviderNameEc2Fleet,
			ProviderAccount:      "original-account",
			ProviderSettingsList: []*birch.Document{makeProviderSettings(t, original)},
			TaskHostOverrides:    nil,
		}
		require.NoError(t, applyTaskHostOverrides(&d))

		assert.Equal(t, "original-account", d.ProviderAccount)
		ec2Settings := getProviderSettings(t, d.ProviderSettingsList[0])
		assert.Equal(t, original.SubnetId, ec2Settings.SubnetId)
		assert.Equal(t, original.SecurityGroupIDs, ec2Settings.SecurityGroupIDs)
	})

	t.Run("NonEC2ProviderIsNotOverridden", func(t *testing.T) {
		d := distro.Distro{
			Provider:          evergreen.ProviderNameDocker,
			ProviderAccount:   "original-account",
			TaskHostOverrides: nonZeroOverrides,
		}
		require.NoError(t, applyTaskHostOverrides(&d))
		assert.Equal(t, "original-account", d.ProviderAccount)
	})

	t.Run("AllFieldsOverriddenWithFullyPopulatedOverrides", func(t *testing.T) {
		original := cloud.EC2ProviderSettings{
			AMI:                          "ami-original",
			InstanceType:                 "m5.xlarge",
			IAMInstanceProfileARN:        "arn:original",
			SecurityGroupIDs:             []string{"sg-original"},
			SubnetId:                     "subnet-original",
			DoNotAssignPublicIPv4Address: false,
			Region:                       evergreen.DefaultEC2Region,
		}
		d := distro.Distro{
			Provider:             evergreen.ProviderNameEc2Fleet,
			ProviderAccount:      "original-account",
			ProviderSettingsList: []*birch.Document{makeProviderSettings(t, original)},
			TaskHostOverrides:    nonZeroOverrides,
		}
		require.NoError(t, applyTaskHostOverrides(&d))

		assert.Equal(t, nonZeroOverrides.ProviderAccount, d.ProviderAccount)

		ec2Settings := getProviderSettings(t, d.ProviderSettingsList[0])
		assert.Equal(t, nonZeroOverrides.IAMInstanceProfileARN, ec2Settings.IAMInstanceProfileARN)
		assert.Equal(t, nonZeroOverrides.SecurityGroupIDs, ec2Settings.SecurityGroupIDs)
		assert.Equal(t, nonZeroOverrides.SubnetID, ec2Settings.SubnetId)
		assert.Equal(t, nonZeroOverrides.DoNotAssignPublicIPv4Address, ec2Settings.DoNotAssignPublicIPv4Address)
		assert.Equal(t, original.AMI, ec2Settings.AMI)
		assert.Equal(t, original.InstanceType, ec2Settings.InstanceType)
	})

	t.Run("OnlyDefaultRegionSettingsIsOverridden", func(t *testing.T) {
		otherRegionSettings := cloud.EC2ProviderSettings{
			AMI:              "ami-west",
			InstanceType:     "m5.xlarge",
			SecurityGroupIDs: []string{"sg-west"},
			SubnetId:         "subnet-west",
			Region:           "us-west-2",
		}
		defaultRegionSettings := cloud.EC2ProviderSettings{
			AMI:              "ami-east",
			InstanceType:     "m5.xlarge",
			SecurityGroupIDs: []string{"sg-east"},
			SubnetId:         "subnet-east",
			Region:           evergreen.DefaultEC2Region,
		}
		d := distro.Distro{
			Provider: evergreen.ProviderNameEc2Fleet,
			ProviderSettingsList: []*birch.Document{
				makeProviderSettings(t, otherRegionSettings),
				makeProviderSettings(t, defaultRegionSettings),
			},
			TaskHostOverrides: nonZeroOverrides,
		}
		require.NoError(t, applyTaskHostOverrides(&d))

		otherEC2Settings := getProviderSettings(t, d.ProviderSettingsList[0])
		assert.Equal(t, otherRegionSettings.SubnetId, otherEC2Settings.SubnetId)
		assert.Equal(t, otherRegionSettings.SecurityGroupIDs, otherEC2Settings.SecurityGroupIDs)

		defaultEC2Settings := getProviderSettings(t, d.ProviderSettingsList[1])
		assert.Equal(t, nonZeroOverrides.SubnetID, defaultEC2Settings.SubnetId)
		assert.Equal(t, nonZeroOverrides.SecurityGroupIDs, defaultEC2Settings.SecurityGroupIDs)
	})

	t.Run("ZeroValueOverridesAreAppliedIfOverridesFieldIsNotNil", func(t *testing.T) {
		// Zero/empty override values replace existing non-zero values.
		zeroOverrides := &distro.TaskHostOverrides{
			ProviderAccount:              "",
			IAMInstanceProfileARN:        "",
			SecurityGroupIDs:             []string{"sg-required"},
			SubnetID:                     "",
			DoNotAssignPublicIPv4Address: false,
		}
		original := cloud.EC2ProviderSettings{
			IAMInstanceProfileARN:        "arn:original",
			SecurityGroupIDs:             []string{"sg-original"},
			SubnetId:                     "subnet-original",
			DoNotAssignPublicIPv4Address: true,
			Region:                       evergreen.DefaultEC2Region,
		}
		d := distro.Distro{
			Provider:             evergreen.ProviderNameEc2Fleet,
			ProviderAccount:      "original-account",
			ProviderSettingsList: []*birch.Document{makeProviderSettings(t, original)},
			TaskHostOverrides:    zeroOverrides,
		}
		require.NoError(t, applyTaskHostOverrides(&d))

		assert.Empty(t, d.ProviderAccount)

		ec2Settings := getProviderSettings(t, d.ProviderSettingsList[0])
		assert.Empty(t, ec2Settings.IAMInstanceProfileARN)
		assert.Empty(t, ec2Settings.SubnetId)
		assert.False(t, ec2Settings.DoNotAssignPublicIPv4Address)
	})

	t.Run("DoesNotApplyAnyOverridesForEmptyProviderSettingsList", func(t *testing.T) {
		d := distro.Distro{
			Provider:          evergreen.ProviderNameEc2Fleet,
			ProviderAccount:   "original-account",
			TaskHostOverrides: nonZeroOverrides,
		}
		require.NoError(t, applyTaskHostOverrides(&d))
		assert.Equal(t, nonZeroOverrides.ProviderAccount, d.ProviderAccount)
		assert.Empty(t, d.ProviderSettingsList)
	})
}

func TestHostAllocatorJobTaskHostOverrides(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	require.NoError(t, db.ClearCollections(distro.Collection, model.TaskQueuesCollection, host.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(distro.Collection, model.TaskQueuesCollection, host.Collection))
	}()

	overrides := distro.TaskHostOverrides{
		ProviderAccount:              "override-account",
		IAMInstanceProfileARN:        "arn:aws:iam::123456789:instance-profile/override",
		SecurityGroupIDs:             []string{"sg-override"},
		SubnetID:                     "subnet-override",
		DoNotAssignPublicIPv4Address: true,
	}

	originalSettings := cloud.EC2ProviderSettings{
		AMI:                          "ami-12345",
		InstanceType:                 "m5.xlarge",
		IAMInstanceProfileARN:        "arn:original",
		SecurityGroupIDs:             []string{"sg-original"},
		SubnetId:                     "subnet-original",
		DoNotAssignPublicIPv4Address: false,
		Region:                       evergreen.DefaultEC2Region,
	}
	settingsDoc, err := originalSettings.ToDocument()
	require.NoError(t, err)

	d := distro.Distro{
		Id:                   "test-distro",
		Provider:             evergreen.ProviderNameEc2Fleet,
		ProviderAccount:      "original-account",
		ProviderSettingsList: []*birch.Document{settingsDoc},
		TaskHostOverrides:    &overrides,
		SingleTaskDistro:     true,
	}
	require.NoError(t, d.Insert(ctx))

	tq := model.TaskQueue{
		Distro: d.Id,
		Queue:  []model.TaskQueueItem{{Id: "t1"}},
		DistroQueueInfo: model.DistroQueueInfo{
			Length:                    1,
			LengthWithDependenciesMet: 1,
		},
	}
	require.NoError(t, tq.Save(ctx))

	j := NewHostAllocatorJob(env, d.Id, time.Now())
	j.Run(ctx)
	require.NoError(t, j.Error())

	hosts, err := host.AllActiveHosts(ctx, d.Id)
	require.NoError(t, err)
	require.Len(t, hosts, 1)

	intentHost := hosts[0]
	assert.Equal(t, overrides.ProviderAccount, intentHost.Distro.ProviderAccount)

	require.Len(t, intentHost.Distro.ProviderSettingsList, 1)
	var ec2Settings cloud.EC2ProviderSettings
	require.NoError(t, ec2Settings.FromDocument(intentHost.Distro.ProviderSettingsList[0]))
	assert.Equal(t, overrides.IAMInstanceProfileARN, ec2Settings.IAMInstanceProfileARN)
	assert.Equal(t, overrides.SecurityGroupIDs, ec2Settings.SecurityGroupIDs)
	assert.Equal(t, overrides.SubnetID, ec2Settings.SubnetId)
	assert.Equal(t, overrides.DoNotAssignPublicIPv4Address, ec2Settings.DoNotAssignPublicIPv4Address)
	assert.Equal(t, originalSettings.AMI, ec2Settings.AMI)
	assert.Equal(t, originalSettings.InstanceType, ec2Settings.InstanceType)
}
