package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestLoadUserManager(t *testing.T) {
	Convey("When Loading a UserManager from an AuthConfig", t, func() {
		c := evergreen.CrowdConfig{}
		g := evergreen.GithubAuthConfig{}
		n := evergreen.NaiveAuthConfig{}
		Convey("a UserManager should not be able to be created in an empty AuthConfig", func() {
			a := evergreen.AuthConfig{}
			_, err := LoadUserManager(a)
			So(err, ShouldNotBeNil)
		})
		Convey("a UserManager should not be able to be created if there are more than one AuthConfig type", func() {
			a := evergreen.AuthConfig{
				Crowd:  &c,
				Naive:  &n,
				Github: nil}
			_, err := LoadUserManager(a)
			So(err, ShouldNotBeNil)
		})
		Convey("a UserManager should not be able to be created if one AuthConfig type is Github", func() {
			a := evergreen.AuthConfig{
				Crowd:  nil,
				Naive:  nil,
				Github: &g}
			_, err := LoadUserManager(a)
			So(err, ShouldBeNil)
		})

		Convey("a UserManager should not be able to be created if one AuthConfig type is Crowd", func() {
			a := evergreen.AuthConfig{
				Crowd:  &c,
				Naive:  nil,
				Github: nil}
			_, err := LoadUserManager(a)
			So(err, ShouldBeNil)
		})

		Convey("a UserManager should not be able to be created if one AuthConfig type is Naive", func() {
			a := evergreen.AuthConfig{
				Crowd:  nil,
				Naive:  &n,
				Github: nil}
			_, err := LoadUserManager(a)
			So(err, ShouldBeNil)
		})
	})
}

func TestSuperUserValidation(t *testing.T) {
	assert := assert.New(t)
	superUsers := []string{"super"}
	su := &simpleUser{
		UserId: "super",
	}
	ru := &simpleUser{
		UserId: "regular",
	}
	assert.True(IsSuperUser(superUsers, su))
	assert.False(IsSuperUser(superUsers, ru))
	assert.False(IsSuperUser(superUsers, nil))
}
