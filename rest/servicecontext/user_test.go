package servicecontext

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFindUserById(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindUserById")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	serviceContext := &DBServiceContext{}
	numUsers := 10

	Convey("When there are user documents in the database", t, func() {
		testutil.HandleTestingErr(db.Clear(user.Collection), t, "Error clearing"+
			" '%v' collection", user.Collection)
		for i := 0; i < numUsers; i++ {
			testUser := &user.DBUser{
				Id:     fmt.Sprintf("user_%d", i),
				APIKey: fmt.Sprintf("apikey_%d", i),
			}
			So(testUser.Insert(), ShouldBeNil)
		}

		Convey("then properly finding each user should succeed", func() {
			for i := 0; i < numUsers; i++ {
				found, err := serviceContext.FindUserById(fmt.Sprintf("user_%d", i))
				So(err, ShouldBeNil)
				So(found.GetAPIKey(), ShouldEqual, fmt.Sprintf("apikey_%d", i))
			}
		})
		Convey("then searching for user that doesn't exist should return nil",
			func() {
				found, err := serviceContext.FindUserById("fake_user")
				So(err, ShouldBeNil)
				So(found, ShouldBeNil)
			})
	})
}
