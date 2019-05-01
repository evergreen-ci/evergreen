package validator

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	_ "github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestCheckDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	conf := env.Settings()

	Convey("When validating a distro", t, func() {

		Convey("if a new distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettings: &map[string]interface{}{
					"ami":                "a",
					"key_name":           "a",
					"instance_type":      "a",
					"security_group_ids": []string{"a"},
					"mount_points":       nil,
				},
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
				BootstrapMethod:     distro.BootstrapMethodLegacySSH,
				CommunicationMethod: distro.CommunicationMethodLegacySSH,
				FinderSettings: distro.FinderSettings{
					Version: evergreen.FinderVersionLegacy,
				},
			}
			verrs, err := CheckDistro(ctx, d, conf, true)
			So(err, ShouldBeNil)
			So(verrs, ShouldResemble, ValidationErrors{})
		})

		Convey("if a new distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettings: &map[string]interface{}{
					"ami":                "a",
					"key_name":           "a",
					"instance_type":      "a",
					"security_group_ids": []string{"a"},
					"mount_points":       nil,
				},
				BootstrapMethod:     distro.BootstrapMethodLegacySSH,
				CommunicationMethod: distro.CommunicationMethodLegacySSH,
			}
			// simulate duplicate id
			dupe := distro.Distro{Id: "a"}
			So(dupe.Insert(), ShouldBeNil)
			verrs, err := CheckDistro(ctx, d, conf, true)
			So(err, ShouldBeNil)
			So(verrs, ShouldNotResemble, ValidationErrors{})
		})

		Convey("if an existing distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettings: &map[string]interface{}{
					"ami":                "a",
					"key_name":           "a",
					"instance_type":      "a",
					"security_group_ids": []string{"a"},
					"mount_points":       nil,
				},
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
				BootstrapMethod:     distro.BootstrapMethodLegacySSH,
				CommunicationMethod: distro.CommunicationMethodLegacySSH,
				FinderSettings: distro.FinderSettings{
					Version: evergreen.FinderVersionLegacy,
				},
			}
			verrs, err := CheckDistro(ctx, d, conf, false)
			So(err, ShouldBeNil)
			So(verrs, ShouldResemble, ValidationErrors{})
		})

		Convey("if an existing distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettings: &map[string]interface{}{
					"ami":                "",
					"key_name":           "a",
					"instance_type":      "a",
					"security_group_ids": []string{"a"},
					"mount_points":       nil,
				},
				BootstrapMethod:     distro.BootstrapMethodLegacySSH,
				CommunicationMethod: distro.CommunicationMethodLegacySSH,
			}
			verrs, err := CheckDistro(ctx, d, conf, false)
			So(err, ShouldBeNil)
			So(verrs, ShouldNotResemble, ValidationErrors{})
			// empty ami for provider
		})

		Reset(func() {
			So(db.Clear(distro.Collection), ShouldBeNil)
		})
	})
}

func TestEnsureUniqueId(t *testing.T) {

	Convey("When validating a distros' ids...", t, func() {
		distroIds := []string{"a", "b", "c"}
		Convey("if a distro has a duplicate id, an error should be returned", func() {
			err := ensureUniqueId(&distro.Distro{Id: "c"}, distroIds)
			So(err, ShouldNotResemble, ValidationErrors{})
			So(len(err), ShouldEqual, 1)
		})
		Convey("if a distro doesn't have a duplicate id, no error should be returned", func() {
			err := ensureUniqueId(&distro.Distro{Id: "d"}, distroIds)
			So(err, ShouldBeNil)
		})
	})
}

func TestEnsureHasRequiredFields(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := testutil.NewEnvironment(ctx, t)
	conf := env.Settings()

	i := -1
	Convey("When validating a distro...", t, func() {
		d := []distro.Distro{
			{},
			{Id: "a"},
			{Id: "a", Arch: "linux_amd64"},
			{Id: "a", Arch: "linux_amd64", User: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettings: &map[string]interface{}{
				"instance_type":      "a",
				"security_group_ids": []string{"a"},
				"key_name":           "a",
				"mount_points":       nil,
			}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettings: &map[string]interface{}{
				"ami":                "a",
				"security_group_ids": []string{"a"},
				"key_name":           "a",
				"mount_points":       nil,
			}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettings: &map[string]interface{}{
				"ami":           "a",
				"instance_type": "a",
				"key_name":      "a",
				"mount_points":  nil,
			}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettings: &map[string]interface{}{
				"ami":                "a",
				"instance_type":      "a",
				"security_group_ids": []string{"a"},
				"mount_points":       nil,
			}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettings: &map[string]interface{}{
				"ami":                "a",
				"key_name":           "a",
				"instance_type":      "a",
				"security_group_ids": []string{"a"},
				"mount_points":       nil,
			}},
		}
		i++
		Convey("an error should be returned if the distro does not contain an id", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain an architecture", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain a user", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain an ssh key", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain a working directory", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain a provider", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain a valid provider", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain any provider settings", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
		})
		Convey("an error should be returned if the distro does not contain all required provider settings", func() {
			Convey("for ec2, it must have the ami", func() {
				So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
			})
			Convey("for ec2, it must have the instance_type", func() {
				So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
			})
			Convey("for ec2, it must have the security group", func() {
				So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldNotResemble, ValidationErrors{})
			})
		})
		Convey("no error should be returned if the distro contains all required provider settings", func() {
			So(ensureHasRequiredFields(ctx, &d[i], conf), ShouldResemble, ValidationErrors{})
		})
	})
}

func TestEnsureValidExpansions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	conf := env.Settings()

	Convey("When validating a distro's expansions...", t, func() {
		Convey("if any key is blank, an error should be returned", func() {
			d := &distro.Distro{
				Expansions: []distro.Expansion{{Key: "", Value: "b"}, {Key: "c", Value: "d"}},
			}
			err := ensureValidExpansions(ctx, d, conf)
			So(err, ShouldNotResemble, ValidationErrors{})
			So(len(err), ShouldEqual, 1)
		})
		Convey("if no expansion key is blank, no error should be returned", func() {
			d := &distro.Distro{
				Expansions: []distro.Expansion{{Key: "a", Value: "b"}, {Key: "c", Value: "d"}},
			}
			err := ensureValidExpansions(ctx, d, conf)
			So(err, ShouldBeNil)
		})
	})
}

func TestEnsureValidSSHOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	conf := env.Settings()

	Convey("When validating a distro's SSH options...", t, func() {
		Convey("if any option is blank, an error should be returned", func() {
			d := &distro.Distro{
				SSHOptions: []string{"", "b", "", "d"},
			}
			err := ensureValidSSHOptions(ctx, d, conf)
			So(err, ShouldNotResemble, ValidationErrors{})
			So(len(err), ShouldEqual, 1)
		})
		Convey("if no option is blank, no error should be returned", func() {
			d := &distro.Distro{
				SSHOptions: []string{"a", "b"},
			}
			err := ensureValidSSHOptions(ctx, d, conf)
			So(err, ShouldBeNil)
		})
	})
}

func TestEnsureNonZeroID(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	conf := env.Settings()

	assert.NotNil(ensureHasNonZeroID(ctx, nil, conf))
	assert.NotNil(ensureHasNonZeroID(ctx, &distro.Distro{}, conf))
	assert.NotNil(ensureHasNonZeroID(ctx, &distro.Distro{Id: ""}, conf))

	assert.Nil(ensureHasNonZeroID(ctx, &distro.Distro{Id: "foo"}, conf))
	assert.Nil(ensureHasNonZeroID(ctx, &distro.Distro{Id: " "}, conf))
}

func TestEnsureNoUnauthorizedCharacters(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	conf := env.Settings()

	assert.NotNil(ensureHasNoUnauthorizedCharacters(ctx, &distro.Distro{Id: "|distro"}, conf))
	assert.NotNil(ensureHasNoUnauthorizedCharacters(ctx, &distro.Distro{Id: "distro|"}, conf))
	assert.NotNil(ensureHasNoUnauthorizedCharacters(ctx, &distro.Distro{Id: "dist|ro"}, conf))

	assert.Nil(ensureHasNonZeroID(ctx, &distro.Distro{Id: "distro"}, conf))
}

func TestEnsureValidContainerPool(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(db.Clear(distro.Collection))

	conf := &evergreen.Settings{
		ContainerPools: evergreen.ContainerPoolsConfig{
			Pools: []evergreen.ContainerPool{
				evergreen.ContainerPool{
					Distro: "d4",
					Id:     "test-pool-valid",
				},
				evergreen.ContainerPool{
					Distro: "d1",
					Id:     "test-pool-invalid",
				},
			},
		},
	}

	d1 := &distro.Distro{
		Id:            "d1",
		ContainerPool: "test-pool-valid",
	}
	d2 := &distro.Distro{
		Id:            "d2",
		ContainerPool: "test-pool-invalid",
	}
	d3 := &distro.Distro{
		Id:            "d3",
		ContainerPool: "test-pool-missing",
	}
	d4 := &distro.Distro{
		Id: "d4",
	}
	assert.NoError(d1.Insert())
	assert.NoError(d2.Insert())
	assert.NoError(d3.Insert())
	assert.NoError(d4.Insert())

	err := ensureValidContainerPool(ctx, d1, conf)
	assert.Equal(err, ValidationErrors{{Error,
		"error in container pool settings: container pool test-pool-invalid has invalid distro"}})
	err = ensureValidContainerPool(ctx, d2, conf)
	assert.Equal(err, ValidationErrors{{Error,
		"error in container pool settings: container pool test-pool-invalid has invalid distro"}})
	err = ensureValidContainerPool(ctx, d3, conf)
	assert.Equal(err, ValidationErrors{{Error,
		"distro container pool does not exist"}})
	err = ensureValidContainerPool(ctx, d4, conf)
	assert.Nil(err)
}

func TestEnsureValidBootstrapAndCommunicationMethods(t *testing.T) {
	ctx := context.Background()
	for _, bootstrapMethod := range distro.ValidBootstrapMethods {
		for _, communicationMethod := range distro.ValidCommunicationMethods {
			d := &distro.Distro{BootstrapMethod: bootstrapMethod, CommunicationMethod: communicationMethod}
			if bootstrapMethod == distro.BootstrapMethodLegacySSH && communicationMethod == distro.CommunicationMethodLegacySSH {
				assert.Nil(t, ensureValidBootstrapAndCommunicationMethods(ctx, d, &evergreen.Settings{}))
			} else if bootstrapMethod == distro.BootstrapMethodLegacySSH || communicationMethod == distro.CommunicationMethodLegacySSH {
				assert.NotNil(t, ensureValidBootstrapAndCommunicationMethods(ctx, d, &evergreen.Settings{}))
			} else {
				assert.Nil(t, ensureValidBootstrapAndCommunicationMethods(ctx, d, &evergreen.Settings{}))
			}
		}
	}
	assert.NotNil(t, ensureValidBootstrapAndCommunicationMethods(ctx, &distro.Distro{BootstrapMethod: "foobar", CommunicationMethod: distro.CommunicationMethodLegacySSH}, &evergreen.Settings{}))
	assert.NotNil(t, ensureValidBootstrapAndCommunicationMethods(ctx, &distro.Distro{BootstrapMethod: distro.BootstrapMethodLegacySSH}, &evergreen.Settings{}))
	assert.NotNil(t, ensureValidBootstrapAndCommunicationMethods(ctx, &distro.Distro{CommunicationMethod: distro.CommunicationMethodLegacySSH}, &evergreen.Settings{}))

	assert.NotNil(t, ensureValidBootstrapAndCommunicationMethods(ctx, &distro.Distro{BootstrapMethod: "foobar", CommunicationMethod: distro.CommunicationMethodSSH}, &evergreen.Settings{}))
	assert.NotNil(t, ensureValidBootstrapAndCommunicationMethods(ctx, &distro.Distro{BootstrapMethod: distro.BootstrapMethodSSH}, &evergreen.Settings{}))
	assert.NotNil(t, ensureValidBootstrapAndCommunicationMethods(ctx, &distro.Distro{CommunicationMethod: distro.CommunicationMethodSSH}, &evergreen.Settings{}))
}
