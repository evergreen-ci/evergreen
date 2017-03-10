package validator

import (
	"testing"

	"github.com/evergreen-ci/evergreen/cloud/providers/ec2"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

var conf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(conf))
}

func TestCheckDistro(t *testing.T) {
	Convey("When validating a distro", t, func() {

		Convey("if a new distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: ec2.OnDemandProviderName,
				ProviderSettings: &map[string]interface{}{
					"ami":            "a",
					"key_name":       "a",
					"instance_type":  "a",
					"security_group": "a",
					"mount_points":   nil,
				},
			}
			verrs, err := CheckDistro(d, conf, true)
			So(err, ShouldBeNil)
			So(verrs, ShouldResemble, []ValidationError{})
		})

		Convey("if a new distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: ec2.OnDemandProviderName,
				ProviderSettings: &map[string]interface{}{
					"ami":            "a",
					"key_name":       "a",
					"instance_type":  "a",
					"security_group": "a",
					"mount_points":   nil,
				},
			}
			// simulate duplicate id
			dupe := distro.Distro{Id: "a"}
			So(dupe.Insert(), ShouldBeNil)
			verrs, err := CheckDistro(d, conf, true)
			So(err, ShouldBeNil)
			So(verrs, ShouldNotResemble, []ValidationError{})
		})

		Convey("if an existing distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: ec2.OnDemandProviderName,
				ProviderSettings: &map[string]interface{}{
					"ami":            "a",
					"key_name":       "a",
					"instance_type":  "a",
					"security_group": "a",
					"mount_points":   nil,
				},
			}
			verrs, err := CheckDistro(d, conf, false)
			So(err, ShouldBeNil)
			So(verrs, ShouldResemble, []ValidationError{})
		})

		Convey("if an existing distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a",
				Provider: ec2.OnDemandProviderName,
				ProviderSettings: &map[string]interface{}{
					"ami":            "",
					"key_name":       "a",
					"instance_type":  "a",
					"security_group": "a",
					"mount_points":   nil,
				},
			}
			verrs, err := CheckDistro(d, conf, false)
			So(err, ShouldBeNil)
			So(verrs, ShouldNotResemble, []ValidationError{})
			// empty ami for provider
		})

		Reset(func() {
			db.Clear(distro.Collection)
		})
	})
}

func TestEnsureUniqueId(t *testing.T) {

	Convey("When validating a distros' ids...", t, func() {
		distroIds := []string{"a", "b", "c"}
		Convey("if a distro has a duplicate id, an error should be returned", func() {
			err := ensureUniqueId(&distro.Distro{Id: "c"}, conf, distroIds)
			So(err, ShouldNotResemble, []ValidationError{})
			So(len(err), ShouldEqual, 1)
		})
		Convey("if a distro doesn't have a duplicate id, no error should be returned", func() {
			err := ensureUniqueId(&distro.Distro{Id: "d"}, conf, distroIds)
			So(err, ShouldBeNil)
		})
	})
}

func TestEnsureHasRequiredFields(t *testing.T) {
	i := -1
	Convey("When validating a distro...", t, func() {
		d := []distro.Distro{
			{},
			{Id: "a"},
			{Id: "a", Arch: "a"},
			{Id: "a", Arch: "a", User: "a"},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a"},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a"},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a", Provider: "a"},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a", Provider: ec2.OnDemandProviderName},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"instance_type":  "a",
				"security_group": "a",
				"key_name":       "a",
				"mount_points":   nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"ami":            "a",
				"security_group": "a",
				"key_name":       "a",
				"mount_points":   nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"ami":           "a",
				"instance_type": "a",
				"key_name":      "a",
				"mount_points":  nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"ami":            "a",
				"instance_type":  "a",
				"security_group": "a",
				"mount_points":   nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", WorkDir: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"ami":            "a",
				"key_name":       "a",
				"instance_type":  "a",
				"security_group": "a",
				"mount_points":   nil,
			}},
		}
		i++
		Convey("an error should be returned if the distro does not contain an id", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain an architecture", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain a user", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain an ssh key", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain a working directory", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain a provider", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain a valid provider", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain any provider settings", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
		})
		Convey("an error should be returned if the distro does not contain all required provider settings", func() {
			Convey("for ec2, it must have the ami", func() {
				So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
			})
			Convey("for ec2, it must have the instance_type", func() {
				So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
			})
			Convey("for ec2, it must have the security group", func() {
				So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
			})
			Convey("for ec2, it must have the key name", func() {
				So(ensureHasRequiredFields(&d[i], conf), ShouldNotResemble, []ValidationError{})
			})
		})
		Convey("no error should be returned if the distro contains all required provider settings", func() {
			So(ensureHasRequiredFields(&d[i], conf), ShouldResemble, []ValidationError{})
		})
	})
}

func TestEnsureValidExpansions(t *testing.T) {
	Convey("When validating a distro's expansions...", t, func() {
		Convey("if any key is blank, an error should be returned", func() {
			d := &distro.Distro{
				Expansions: []distro.Expansion{{"", "b"}, {"c", "d"}},
			}
			err := ensureValidExpansions(d, conf)
			So(err, ShouldNotResemble, []ValidationError{})
			So(len(err), ShouldEqual, 1)
		})
		Convey("if no expansion key is blank, no error should be returned", func() {
			d := &distro.Distro{
				Expansions: []distro.Expansion{{"a", "b"}, {"c", "d"}},
			}
			err := ensureValidExpansions(d, conf)
			So(err, ShouldBeNil)
		})
	})
}

func TestEnsureValidSSHOptions(t *testing.T) {
	Convey("When validating a distro's SSH options...", t, func() {
		Convey("if any option is blank, an error should be returned", func() {
			d := &distro.Distro{
				SSHOptions: []string{"", "b", "", "d"},
			}
			err := ensureValidSSHOptions(d, conf)
			So(err, ShouldNotResemble, []ValidationError{})
			So(len(err), ShouldEqual, 1)
		})
		Convey("if no option is blank, no error should be returned", func() {
			d := &distro.Distro{
				SSHOptions: []string{"a", "b"},
			}
			err := ensureValidSSHOptions(d, conf)
			So(err, ShouldBeNil)
		})
	})
}
