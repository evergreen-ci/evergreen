package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
)

func init() {
	reporting.QuietMode()
}

func TestAdminAuthenticator(t *testing.T) {
	Convey("When there is an http request, "+
		"a project ref, authenticator, and a service context", t, func() {

		projectRef := model.ProjectRef{}
		serviceContext := &data.MockConnector{}
		author := ProjectAdminAuthenticator{}
		Convey("When authenticating", func() {
			ctx := context.Background()

			Convey("if user is in the admins, should succeed", func() {
				projectRef.Admins = []string{"test_user"}
				opCtx := model.Context{
					ProjectRef: &projectRef,
				}

				u := user.DBUser{
					Id: "test_user",
				}
				ctx = context.WithValue(ctx, evergreen.RequestUser, &u)
				ctx = context.WithValue(ctx, RequestContext, &opCtx)
				So(author.Authenticate(ctx, serviceContext), ShouldBeNil)
			})
			Convey("if user is in the super users, should succeed", func() {
				superUsers := []string{"test_user"}
				projectRef.Admins = []string{"other_user"}
				opCtx := model.Context{
					ProjectRef: &projectRef,
				}
				serviceContext.SetSuperUsers(superUsers)

				u := user.DBUser{
					Id: "test_user",
				}
				ctx = context.WithValue(ctx, evergreen.RequestUser, &u)
				ctx = context.WithValue(ctx, RequestContext, &opCtx)
				So(author.Authenticate(ctx, serviceContext), ShouldBeNil)
			})
			Convey("if user is not in the admin and not a super user, should error", func() {
				superUsers := []string{"other_user"}
				serviceContext.SetSuperUsers(superUsers)

				projectRef.Admins = []string{"other_user"}
				opCtx := model.Context{
					ProjectRef: &projectRef,
				}

				u := user.DBUser{
					Id: "test_user",
				}
				ctx = context.WithValue(ctx, evergreen.RequestUser, &u)
				ctx = context.WithValue(ctx, RequestContext, &opCtx)
				err := author.Authenticate(ctx, serviceContext)

				errToResemble := rest.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
		})
	})

}
func TestSuperUserAuthenticator(t *testing.T) {
	Convey("When there is an http request, an authenticator, and a service context", t, func() {
		serviceContext := &data.MockConnector{}
		author := SuperUserAuthenticator{}
		Convey("When authenticating", func() {

			ctx := context.Background()

			Convey("if user is in the superusers, should succeed", func() {
				superUsers := []string{"test_user"}
				serviceContext.SetSuperUsers(superUsers)

				u := user.DBUser{
					Id: "test_user",
				}
				ctx = context.WithValue(ctx, evergreen.RequestUser, &u)
				So(author.Authenticate(ctx, serviceContext), ShouldBeNil)
			})
			Convey("if user is not in the superusers, should error", func() {
				superUsers := []string{"other_user"}
				serviceContext.SetSuperUsers(superUsers)

				u := user.DBUser{
					Id: "test_user",
				}
				ctx = context.WithValue(ctx, evergreen.RequestUser, &u)
				err := author.Authenticate(ctx, serviceContext)

				errToResemble := rest.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Not found",
				}
				So(err, ShouldResemble, errToResemble)

			})
		})
	})

}
