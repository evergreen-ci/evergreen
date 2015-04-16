package validator

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers/ec2"
	"10gen.com/mci/db"
	"10gen.com/mci/model/distro"
	_ "10gen.com/mci/plugin/config"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	conf = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(conf))
}

func TestCheckDistro(t *testing.T) {
	Convey("When validating a distro", t, func() {

		Convey("if a new distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a",
				Provider: ec2.OnDemandProviderName,
				ProviderSettings: &map[string]interface{}{
					"ami":            "a",
					"key_name":       "a",
					"instance_type":  "a",
					"security_group": "a",
					"mount_points":   nil,
				},
			}

			So(CheckDistro(d, conf, true), ShouldResemble, []ValidationError{})
		})

		Convey("if a new distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a",
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
			So(CheckDistro(d, conf, true), ShouldNotResemble, []ValidationError{})
		})

		Convey("if an existing distro passes all of the validation tests, no errors should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a",
				Provider: ec2.OnDemandProviderName,
				ProviderSettings: &map[string]interface{}{
					"ami":            "a",
					"key_name":       "a",
					"instance_type":  "a",
					"security_group": "a",
					"mount_points":   nil,
				},
			}

			So(CheckDistro(d, conf, false), ShouldResemble, []ValidationError{})
		})

		Convey("if an existing distro fails a validation test, an error should be returned", func() {
			d := &distro.Distro{Id: "a", Arch: "a", User: "a", SSHKey: "a",
				Provider: ec2.OnDemandProviderName,
				ProviderSettings: &map[string]interface{}{
					"ami":            "",
					"key_name":       "a",
					"instance_type":  "a",
					"security_group": "a",
					"mount_points":   nil,
				},
			}
			// empty ami for provider
			So(CheckDistro(d, conf, false), ShouldNotResemble, []ValidationError{})
		})

		Reset(func() {
			db.Clear(distro.Collection)
		})
	})
}

func TestEnsureUniqueId(t *testing.T) {
	Convey("When validating a distros' ids...", t, func() {
		Convey("if a distro has a duplicate id, an error should be returned", func() {
			distroIds = []string{"a", "b", "c"}
			err := ensureUniqueId(&distro.Distro{Id: "c"}, conf)
			So(err, ShouldNotResemble, []ValidationError{})
			So(len(err), ShouldEqual, 1)
		})
		Convey("if a distro doesn't have a duplicate id, no error should be returned", func() {
			distroIds = []string{"a", "b", "c"}
			err := ensureUniqueId(&distro.Distro{Id: "d"}, conf)
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
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", Provider: "a"},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", Provider: ec2.OnDemandProviderName},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"instance_type":  "a",
				"security_group": "a",
				"key_name":       "a",
				"mount_points":   nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"ami":            "a",
				"security_group": "a",
				"key_name":       "a",
				"mount_points":   nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"ami":           "a",
				"instance_type": "a",
				"key_name":      "a",
				"mount_points":  nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
				"ami":            "a",
				"instance_type":  "a",
				"security_group": "a",
				"mount_points":   nil,
			}},
			{Id: "a", Arch: "a", User: "a", SSHKey: "a", Provider: ec2.OnDemandProviderName, ProviderSettings: &map[string]interface{}{
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
