package manifest

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestManifest(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
	testutil.HandleTestingErr(
		db.ClearCollections(manifest.Collection), t,
		"error clearing test collections")

	Convey("With a pre-existing manifest for a revision existing", t, func() {
		m := manifest.Manifest{
			Id:          "abc123",
			ProjectName: "mci_test",
			Modules:     map[string]*manifest.Module{},
		}
		m.Modules["sample"] = &manifest.Module{
			Branch:   "master",
			Revision: "xyz345",
			Repo:     "repo",
			Owner:    "sr527",
			URL:      "randomurl.com",
		}

		dup, err := m.TryInsert()
		So(dup, ShouldBeFalse)
		So(err, ShouldBeNil)

		Convey("insertion of another manifest should give a duplicate error", func() {
			badManifest := manifest.Manifest{
				Id:          "abc123",
				ProjectName: "this_shouldn't_insert",
				Modules:     map[string]*manifest.Module{},
			}
			dup, err = badManifest.TryInsert()
			So(dup, ShouldBeTrue)
			So(err, ShouldBeNil)
		})

	})

}
