package patch

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigChanged(t *testing.T) {

	Convey("With calling ConfigChanged with a remote configuration "+
		"path", t, func() {

		Convey("the function should return true if the config has changed", func() {
			remoteConfigPath := "config/evergreen.yml"
			p := &patch.Patch{
				Patches: []patch.ModulePatch{{
					PatchSet: patch.PatchSet{
						Summary: []thirdparty.Summary{{
							Name:      remoteConfigPath,
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			}
			So(p.ConfigChanged(remoteConfigPath), ShouldBeTrue)
		})

		Convey("the function should not return true if the config has not changed", func() {
			remoteConfigPath := "config/evergreen.yml"
			p := &patch.Patch{
				Patches: []patch.ModulePatch{{
					PatchSet: patch.PatchSet{
						Summary: []thirdparty.Summary{{
							Name:      "dakar",
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			}
			So(p.ConfigChanged(remoteConfigPath), ShouldBeFalse)
		})
	})
}
