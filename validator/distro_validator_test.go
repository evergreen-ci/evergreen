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
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
				CloneMethod: distro.CloneMethodLegacySSH,
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
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
				CloneMethod: distro.CloneMethodLegacySSH,
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
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
				CloneMethod: distro.CloneMethodLegacySSH,
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

func TestEnsureValidAliases(t *testing.T) {

	Convey("When validating a distros' aliases...", t, func() {
		d := distro.Distro{Id: "c", Aliases: []string{"c"}}
		distroIds := []string{"a", "b", "c"}
		Convey("if a distro is declared as an alias of itself, an error should be returned", func() {
			vErrors := ensureValidAliases(&d, distroIds)
			So(vErrors, ShouldNotResemble, ValidationErrors{})
			So(len(vErrors), ShouldEqual, 1)
			So(vErrors[0].Message, ShouldEqual, "'c' cannot be an distro alias of itself")
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

func nonLegacyBootstrapSettings() distro.BootstrapSettings {
	return distro.BootstrapSettings{
		ClientDir:             "/client_dir",
		JasperBinaryDir:       "/jasper_binary_dir",
		JasperCredentialsPath: "/jasper_credentials_path",
		ServiceUser:           "service_user",
		ShellPath:             "/shell_path",
	}
}

func TestEnsureValidBootstrapSettings(t *testing.T) {
	ctx := context.Background()
	for _, bootstrap := range []string{
		distro.BootstrapMethodLegacySSH,
		distro.BootstrapMethodSSH,
		distro.BootstrapMethodUserData,
		distro.BootstrapMethodPreconfiguredImage,
	} {
		for _, communication := range []string{
			distro.CommunicationMethodLegacySSH,
			distro.CommunicationMethodSSH,
			distro.CommunicationMethodRPC,
		} {
			d := &distro.Distro{BootstrapSettings: nonLegacyBootstrapSettings()}
			d.BootstrapSettings.Method = bootstrap
			d.BootstrapSettings.Communication = communication
			switch bootstrap {
			case distro.BootstrapMethodLegacySSH:
				if communication == distro.CommunicationMethodLegacySSH {
					assert.Nil(t, ensureValidBootstrapSettings(ctx, d, &evergreen.Settings{}))
				} else {
					assert.NotNil(t, ensureValidBootstrapSettings(ctx, d, &evergreen.Settings{}))
				}
			default:
				if communication == distro.CommunicationMethodLegacySSH {
					assert.NotNil(t, ensureValidBootstrapSettings(ctx, d, &evergreen.Settings{}))
				} else {
					assert.Nil(t, ensureValidBootstrapSettings(ctx, d, &evergreen.Settings{}))
				}
			}
		}
	}

	for testName, testCase := range map[string]func(t *testing.T, s distro.BootstrapSettings){
		"InvalidBootstrapMethod": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = "foobar"
			s.Communication = distro.CommunicationMethodLegacySSH
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"InvalidCommunication": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodLegacySSH
			s.Communication = "foobar"
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"UnspecifiedBootstrapMethodLegacyCommunication": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = ""
			s.Communication = distro.CommunicationMethodLegacySSH
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"UnspecifiedCommunicationLegacyBootstrapMethod": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodLegacySSH
			s.Communication = ""
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"UnspecifiedCommunicationNonLegacyBootstrapMethod": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodSSH
			s.Communication = ""
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"UnspecifiedBootstrapMethodNonLegacyCommunication": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = ""
			s.Communication = distro.CommunicationMethodSSH
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"WindowsNoShellPath": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodSSH
			s.Communication = distro.CommunicationMethodSSH
			s.ShellPath = ""
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: distro.ArchWindowsAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"WindowsNoServiceUser": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodSSH
			s.Communication = distro.CommunicationMethodSSH
			s.ServiceUser = ""
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: distro.ArchWindowsAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"ResourceLimits": func(t *testing.T, s distro.BootstrapSettings) {
			for resourceTestName, resourceTestCase := range map[string]func(t *testing.T, s distro.BootstrapSettings){
				"PositiveValues": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = 1
					s.ResourceLimits.NumProcesses = 2
					s.ResourceLimits.LockedMemoryKB = 3
					s.ResourceLimits.VirtualMemoryKB = 4
					assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: distro.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
				},
				"ZeroValues": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = 0
					s.ResourceLimits.NumProcesses = 0
					s.ResourceLimits.LockedMemoryKB = 0
					s.ResourceLimits.VirtualMemoryKB = 0
					assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: distro.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
				},
				"UnlimitedResources": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = -1
					s.ResourceLimits.NumProcesses = -1
					s.ResourceLimits.LockedMemoryKB = -1
					s.ResourceLimits.VirtualMemoryKB = -1
					assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: distro.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
				},
				"InvalidResourceLimits": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = -50
					s.ResourceLimits.NumProcesses = -50
					s.ResourceLimits.LockedMemoryKB = -50
					s.ResourceLimits.VirtualMemoryKB = -50
					assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: distro.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
				},
			} {
				t.Run(resourceTestName, func(t *testing.T) {
					s.Method = distro.BootstrapMethodSSH
					s.Communication = distro.CommunicationMethodSSH
					resourceTestCase(t, s)
				})
			}
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t, nonLegacyBootstrapSettings())
		})
	}
}

func TestEnsureValidCloneMethod(t *testing.T) {
	ctx := context.Background()
	assert.NotNil(t, ensureValidCloneMethod(ctx, &distro.Distro{}, &evergreen.Settings{}))
	assert.Nil(t, ensureValidCloneMethod(ctx, &distro.Distro{CloneMethod: distro.CloneMethodLegacySSH}, &evergreen.Settings{}))
	assert.Nil(t, ensureValidCloneMethod(ctx, &distro.Distro{CloneMethod: distro.CloneMethodOAuth}, &evergreen.Settings{}))
}
