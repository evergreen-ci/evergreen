package validator

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	_ "github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
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
	conf.Providers.AWS.EC2Keys = []evergreen.EC2Key{{Key: "key", Secret: "secret"}}
	conf.SSHKeyPairs = []evergreen.SSHKeyPair{{Name: "a"}}
	defer func() {
		conf.SSHKeyPairs = nil
	}()

	Convey("When validating a distro", t, func() {

		Convey("if a new distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(
					birch.EC.String("ami", "a"),
					birch.EC.String("key_name", "a"),
					birch.EC.String("instance_type", "a"),
					birch.EC.SliceString("security_group_ids", []string{"a"}),
				)},
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
				CloneMethod: evergreen.CloneMethodLegacySSH,
				FinderSettings: distro.FinderSettings{
					Version: evergreen.FinderVersionLegacy,
				},
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
				},
				HostAllocatorSettings: distro.HostAllocatorSettings{
					Version:      evergreen.HostAllocatorUtilization,
					MinimumHosts: 10,
					MaximumHosts: 20,
				},
			}
			verrs, err := CheckDistro(ctx, d, conf, true)
			So(err, ShouldBeNil)
			So(verrs, ShouldResemble, ValidationErrors{})
		})

		Convey("if a new distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(
					birch.EC.String("ami", "a"),
					birch.EC.String("key_name", "a"),
					birch.EC.String("instance_type", "a"),
					birch.EC.SliceString("security_group_ids", []string{"a"}),
				)},
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
				CloneMethod: evergreen.CloneMethodLegacySSH,
			}
			// simulate duplicate id
			dupe := distro.Distro{Id: "a"}
			So(dupe.Insert(ctx), ShouldBeNil)
			verrs, err := CheckDistro(ctx, d, conf, true)
			So(err, ShouldBeNil)
			So(verrs, ShouldNotResemble, ValidationErrors{})
		})

		Convey("if an existing distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(
					birch.EC.String("ami", "a"),
					birch.EC.String("key_name", "a"),
					birch.EC.String("instance_type", "a"),
					birch.EC.SliceString("security_group_ids", []string{"a"}),
				)},
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
				CloneMethod: evergreen.CloneMethodLegacySSH,
				FinderSettings: distro.FinderSettings{
					Version: evergreen.FinderVersionLegacy,
				},
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
				},
				HostAllocatorSettings: distro.HostAllocatorSettings{
					Version:      evergreen.HostAllocatorUtilization,
					MinimumHosts: 10,
					MaximumHosts: 20,
				},
			}
			verrs, err := CheckDistro(ctx, d, conf, false)
			So(err, ShouldBeNil)
			So(verrs, ShouldResemble, ValidationErrors{})
		})

		Convey("if an existing distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(
					birch.EC.String("ami", "a"),
					birch.EC.String("key_name", "a"),
					birch.EC.String("instance_type", "a"),
					birch.EC.SliceString("security_group_ids", []string{"a"}),
				)},
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
	Convey("When validating a distro's ids...", t, func() {
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
	Convey("When validating a distro's aliases...", t, func() {
		d := distro.Distro{Id: "c", Aliases: []string{"c"}}
		Convey("if a distro is declared as an alias of itself, an error should be returned", func() {
			vErrors := ensureValidAliases(&d)
			So(vErrors, ShouldNotResemble, ValidationErrors{})
			So(len(vErrors), ShouldEqual, 1)
			So(vErrors[0].Message, ShouldEqual, "'c' cannot be an distro alias of itself")
		})

	})
}

func TestEnsureNoAliases(t *testing.T) {
	for testName, testParams := range map[string]struct {
		distro     distro.Distro
		aliases    []string
		shouldPass bool
	}{
		"PassesWithNoAliasesAndNoConflictingAliases": {
			distro:     distro.Distro{Id: "id"},
			aliases:    []string{"some_other_alias", "another_alias"},
			shouldPass: true,
		},
		"FailsWithAliases": {
			distro:     distro.Distro{Id: "id", Aliases: []string{"alias"}},
			shouldPass: false,
		},
		"FailsWithConflictingAliases": {
			distro:     distro.Distro{Id: "conflicting_distro_id"},
			aliases:    []string{"other_aliase", "conflicting_distro_id"},
			shouldPass: false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			errs := ensureNoAliases(&testParams.distro, testParams.aliases)
			if testParams.shouldPass {
				assert.Empty(t, errs)
			} else {
				assert.NotEmpty(t, errs)
			}
		})
	}
}

func TestEnsureHasRequiredFields(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := testutil.NewEnvironment(ctx, t)
	conf := env.Settings()

	i := -1
	Convey("When validating a distro...", t, func() {
		So(db.ClearCollections(distro.Collection), ShouldBeNil)
		d := []distro.Distro{
			{},
			{Id: "a"},
			{Id: "a", Arch: "linux_amd64"},
			{Id: "a", Arch: "linux_amd64", User: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: "a"},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("key_name", "a"),
				birch.EC.String("instance_type", "a"),
				birch.EC.SliceString("security_group_ids", []string{"a"}),
			)}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("ami", "a"),
				birch.EC.String("key_name", "a"),
				birch.EC.SliceString("security_group_ids", []string{"a"}),
			)}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("ami", "a"),
				birch.EC.String("key_name", "a"),
				birch.EC.String("instance_type", "a"),
			)}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("ami", "a"),
				birch.EC.String("instance_type", "a"),
				birch.EC.SliceString("security_group_ids", []string{"a"}),
			)}},
			{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("ami", "a"),
				birch.EC.String("key_name", "a"),
				birch.EC.String("instance_type", "a"),
				birch.EC.SliceString("security_group_ids", []string{"a"}),
			)}},
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

func TestEnsureHasRequiredFieldsWithProviderList(t *testing.T) {
	ctx := context.Background()
	validSettings := cloud.EC2ProviderSettings{
		AMI:              "a",
		KeyName:          "a",
		InstanceType:     "a",
		SecurityGroupIDs: []string{"a"},
		MountPoints:      nil,
		Region:           evergreen.DefaultEC2Region,
	}
	invalidSettings := cloud.EC2ProviderSettings{
		KeyName:          "a",
		InstanceType:     "a",
		SecurityGroupIDs: []string{"a"},
		MountPoints:      nil,
		Region:           "us-west-1",
	}
	invalidSettings2 := cloud.EC2ProviderSettings{
		AMI:              "b",
		KeyName:          "b",
		SecurityGroupIDs: []string{"b"},
		MountPoints:      nil,
		Region:           "us-west-2",
	}

	validDoc := &birch.Document{}
	validDoc2 := &birch.Document{}
	invalidDoc := &birch.Document{}
	invalidDoc2 := &birch.Document{}

	bytes, err := bson.Marshal(validSettings)
	assert.NoError(t, err)
	assert.NoError(t, validDoc.UnmarshalBSON(bytes))
	validSettings.Region = "us-west-1"
	bytes, err = bson.Marshal(validSettings)
	assert.NoError(t, err)
	assert.NoError(t, validDoc2.UnmarshalBSON(bytes))
	bytes, err = bson.Marshal(invalidSettings)
	assert.NoError(t, err)
	assert.NoError(t, invalidDoc.UnmarshalBSON(bytes))
	bytes, err = bson.Marshal(invalidSettings2)
	assert.NoError(t, err)
	assert.NoError(t, invalidDoc2.UnmarshalBSON(bytes))

	validList := []*birch.Document{validDoc, validDoc2}
	invalidList1 := []*birch.Document{invalidDoc, validDoc, invalidDoc2}
	invalidList2 := []*birch.Document{validDoc, validDoc}

	d1 := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: invalidList1}
	d2 := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: invalidList2}
	d3 := &distro.Distro{Id: "a", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a", Provider: evergreen.ProviderNameEc2OnDemand, ProviderSettingsList: validList}

	for name, test := range map[string]func(*testing.T){
		"ListWithErrors": func(t *testing.T) {
			assert.Len(t, ensureHasRequiredFields(ctx, d1, nil), 2)
		},
		"ListWithRepeatRegions": func(t *testing.T) {
			assert.Len(t, ensureHasRequiredFields(ctx, d2, nil), 1)
		},
		"ValidList": func(t *testing.T) {
			assert.Len(t, ensureHasRequiredFields(ctx, d3, nil), 0)
		},
	} {
		t.Run(name, test)
	}
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
				{
					Distro: "d4",
					Id:     "test-pool-valid",
				},
				{
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
	assert.NoError(d1.Insert(ctx))
	assert.NoError(d2.Insert(ctx))
	assert.NoError(d3.Insert(ctx))
	assert.NoError(d4.Insert(ctx))

	err := ensureValidContainerPool(ctx, d1, conf)
	assert.Equal(err, ValidationErrors{{Error,
		"error in container pool settings: container pool 'test-pool-invalid' has invalid distro 'd1'"}})
	err = ensureValidContainerPool(ctx, d2, conf)
	assert.Equal(err, ValidationErrors{{Error,
		"error in container pool settings: container pool 'test-pool-invalid' has invalid distro 'd1'"}})
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

	assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: distro.BootstrapSettings{Method: distro.BootstrapMethodNone}}, &evergreen.Settings{}))

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
		"UnspecifiedShellPath": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodSSH
			s.Communication = distro.CommunicationMethodSSH
			s.ShellPath = ""
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"WindowsNoServiceUser": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodSSH
			s.Communication = distro.CommunicationMethodSSH
			s.ServiceUser = ""
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: evergreen.ArchWindowsAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"FormattedEnvironmentVariables": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodUserData
			s.Communication = distro.CommunicationMethodRPC
			s.Env = append(s.Env, distro.EnvVar{Key: "foo", Value: "bar"})
			s.Env = append(s.Env, distro.EnvVar{Key: "bat", Value: "baz"})
			assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"EnvironmentVariableWithoutKey": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodUserData
			s.Communication = distro.CommunicationMethodRPC
			s.Env = append(s.Env, distro.EnvVar{Value: "foo"})
			assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"EnvironmentVariableWithoutValue": func(t *testing.T, s distro.BootstrapSettings) {
			s.Method = distro.BootstrapMethodUserData
			s.Communication = distro.CommunicationMethodRPC
			s.Env = append(s.Env, distro.EnvVar{Key: "foo"})
			assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{BootstrapSettings: s}, &evergreen.Settings{}))
		},
		"ResourceLimits": func(t *testing.T, s distro.BootstrapSettings) {
			for resourceTestName, resourceTestCase := range map[string]func(t *testing.T, s distro.BootstrapSettings){
				"PositiveValues": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = 1
					s.ResourceLimits.NumProcesses = 2
					s.ResourceLimits.NumTasks = 3
					s.ResourceLimits.LockedMemoryKB = 4
					s.ResourceLimits.VirtualMemoryKB = 5
					assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: evergreen.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
				},
				"ZeroValues": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = 0
					s.ResourceLimits.NumProcesses = 0
					s.ResourceLimits.NumTasks = 0
					s.ResourceLimits.LockedMemoryKB = 0
					s.ResourceLimits.VirtualMemoryKB = 0
					assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: evergreen.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
				},
				"UnlimitedResources": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = -1
					s.ResourceLimits.NumProcesses = -1
					s.ResourceLimits.NumTasks = -1
					s.ResourceLimits.LockedMemoryKB = -1
					s.ResourceLimits.VirtualMemoryKB = -1
					assert.Nil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: evergreen.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
				},
				"InvalidResourceLimits": func(t *testing.T, s distro.BootstrapSettings) {
					s.ResourceLimits.NumFiles = -50
					s.ResourceLimits.NumProcesses = -50
					s.ResourceLimits.NumTasks = -50
					s.ResourceLimits.LockedMemoryKB = -50
					s.ResourceLimits.VirtualMemoryKB = -50
					assert.NotNil(t, ensureValidBootstrapSettings(ctx, &distro.Distro{Arch: evergreen.ArchLinuxAmd64, BootstrapSettings: s}, &evergreen.Settings{}))
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

func TestEnsureValidStaticBootstrapSettings(t *testing.T) {
	ctx := context.Background()
	d := distro.Distro{
		Provider: evergreen.ProviderNameStatic,
	}
	for _, method := range []string{
		distro.BootstrapMethodLegacySSH,
		distro.BootstrapMethodSSH,
	} {
		d.BootstrapSettings.Method = method
		assert.Nil(t, ensureValidStaticBootstrapSettings(ctx, &d, &evergreen.Settings{}))
	}

	d.BootstrapSettings.Method = distro.BootstrapMethodUserData
	assert.NotNil(t, ensureValidStaticBootstrapSettings(ctx, &d, &evergreen.Settings{}))
}

func TestEnsureStaticHasAuthorizedKeysFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{
		SSHKeyPairs: []evergreen.SSHKeyPair{
			{
				Name: "ssh_key_pair1",
			},
		},
	}
	assert.Nil(t, ensureStaticHasAuthorizedKeysFile(ctx, &distro.Distro{Provider: evergreen.ProviderNameStatic, AuthorizedKeysFile: "~/.ssh/authorized_keys"}, settings))
	assert.NotNil(t, ensureStaticHasAuthorizedKeysFile(ctx, &distro.Distro{Provider: evergreen.ProviderNameStatic}, settings))
	assert.Nil(t, ensureStaticHasAuthorizedKeysFile(ctx, &distro.Distro{Provider: evergreen.ProviderNameEc2Fleet}, settings))
}

func TestEnsureHasValidVirtualWorkstationSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{}
	assert.Nil(t, ensureHasValidVirtualWorkstationSettings(ctx, &distro.Distro{
		IsVirtualWorkstation: true,
		Arch:                 evergreen.ArchLinuxArm64,
		HomeVolumeSettings: distro.HomeVolumeSettings{
			FormatCommand: "format_command",
		},
	}, settings))
	assert.NotNil(t, ensureHasValidVirtualWorkstationSettings(ctx, &distro.Distro{
		IsVirtualWorkstation: true,
	}, settings))
	assert.NotNil(t, ensureHasValidVirtualWorkstationSettings(ctx, &distro.Distro{
		IsVirtualWorkstation: true,
		Arch:                 evergreen.ArchWindowsAmd64,
		HomeVolumeSettings: distro.HomeVolumeSettings{
			FormatCommand: "format_command",
		},
	}, settings))
}

func TestValidateAliases(t *testing.T) {
	assert.NotNil(t, validateAliases(&distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameDocker,
		ContainerPool: "",
		Aliases:       []string{"alias_1", "alias_2"},
	}, []string{}))

	assert.NotNil(t, validateAliases(&distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameStatic,
		ContainerPool: "container_pool",
		Aliases:       []string{"alias_1", "alias_2"},
	}, []string{}))

	assert.NotNil(t, validateAliases(&distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameStatic,
		ContainerPool: "container_pool",
		Aliases:       []string{},
	}, []string{"distro"}))

	assert.NotNil(t, validateAliases(&distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameStatic,
		ContainerPool: "",
		Aliases:       []string{"distro"},
	}, []string{}))

	assert.Nil(t, validateAliases(&distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameDocker,
		ContainerPool: "container_pool",
		Aliases:       []string{},
	}, []string{"something_else"}))

	assert.Nil(t, validateAliases(&distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameStatic,
		ContainerPool: "",
		Aliases:       []string{"alias_1", "alias_2"},
	}, []string{}))
}
