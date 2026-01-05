package cloud

import (
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRDPPasswordValidation(t *testing.T) {
	assert := assert.New(t)

	goodPasswords := []string{
		"地火風水心CP!",
		"V3ryStr0ng!",
		`Aaa\aa\`,
	}
	badPasswords := []string{"", "weak", "stilltooweak1", "火火火1"}

	for _, password := range goodPasswords {
		assert.True(host.ValidateRDPPassword(password), password)
	}

	for _, password := range badPasswords {
		assert.False(host.ValidateRDPPassword(password), password)
	}
}

func TestMakeExtendedHostExpiration(t *testing.T) {
	assert := assert.New(t)

	h := host.Host{
		CreationTime:   time.Now(),
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}

	expTime, err := MakeExtendedSpawnHostExpiration(&h, time.Hour)
	assert.NotZero(expTime)
	assert.NoError(err, expTime.Format(time.RFC3339))
}

func TestMakeExtendedHostExpirationFailsPastMax(t *testing.T) {
	assert := assert.New(t)

	h := host.Host{
		CreationTime:   time.Now().Add(-50 * time.Hour * 24), // 50 days in the past
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}

	expTime, err := MakeExtendedSpawnHostExpiration(&h, time.Hour)
	assert.Zero(expTime)
	assert.Error(err)
	assert.Contains(err.Error(), "cannot be extended more than")

	h.CreationTime = time.Now().Add(-25 * time.Hour * 24)

	expTime, err = MakeExtendedSpawnHostExpiration(&h, 7*24*time.Hour) // extend one week
	assert.Zero(expTime)
	assert.Error(err)
	assert.Contains(err.Error(), "cannot be extended more than")
}

func TestModifySpawnHostProviderSettings(t *testing.T) {
	require.NoError(t, db.Clear(host.VolumesCollection))

	vol := host.Volume{
		ID:               "v0",
		AvailabilityZone: "us-east-1a",
	}
	require.NoError(t, vol.Insert(t.Context()))

	config := evergreen.Settings{}
	config.Providers.AWS.Subnets = []evergreen.Subnet{{AZ: "us-east-1a", SubnetID: "new_id"}}

	d := distro.Distro{
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("subnet_id", "old_id"),
		)},
	}

	settingsList, err := modifySpawnHostProviderSettings(t.Context(), d, &config, "", vol.ID)
	assert.NoError(t, err)
	assert.Equal(t, "new_id", settingsList[0].LookupElement("subnet_id").Value().StringValue())
}

func TestValidateSSHKey(t *testing.T) {
	rsaKey := "ssh-rsa randomKeyname"
	dssKey := "ssh-dss randomKeyname"
	ed25519Key := "ssh-ed25519 randomKeyname"
	ecdsaKey := "ecdsa-sha2-nistp256 randomKeyname"
	invalidKey := "notThat randomKeyname"

	require.NoError(t, evergreen.ValidateSSHKey(rsaKey))
	require.NoError(t, evergreen.ValidateSSHKey(dssKey))
	require.NoError(t, evergreen.ValidateSSHKey(ed25519Key))
	require.NoError(t, evergreen.ValidateSSHKey(ecdsaKey))
	require.Error(t, evergreen.ValidateSSHKey(invalidKey))
}

// TestTerminateSpawnHostIntentHostSetsTerminationTime verifies termination_time
// is set when terminating intent hosts.
func TestTerminateSpawnHostIntentHostSetsTerminationTime(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.Clear(host.Collection))

	intentHost := &host.Host{Id: "intent-test", Status: evergreen.HostUninitialized, Provider: evergreen.ProviderNameMock}
	require.NoError(t, intentHost.Insert(ctx))

	err := TerminateSpawnHost(ctx, evergreen.GetEnvironment(), intentHost, "test-user", "test")
	require.NoError(t, err)

	dbHost, err := host.FindOneId(ctx, "intent-test")
	require.NoError(t, err)
	assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
	assert.False(t, dbHost.TerminationTime.IsZero(), "termination_time must be set")
}
