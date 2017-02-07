package route

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/gorilla/context"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAdminAuthenticator(t *testing.T) {
	Convey("When there is an http request, "+
		"a project ref, authenticator, and a service context", t, func() {
		req, err := http.NewRequest(evergreen.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		projectRef := model.ProjectRef{}
		serviceContext := &servicecontext.MockServiceContext{}
		auther := ProjectAdminAuthenticator{}
		Convey("When authenticating", func() {

			Reset(func() {
				context.Clear(req)
			})

			Convey("if user is in the admins, should succeed", func() {
				projectRef.Admins = []string{"test_user"}
				ctx := model.Context{
					ProjectRef: &projectRef,
				}

				u := user.DBUser{
					Id: "test_user",
				}
				context.Set(req, RequestUser, &u)
				context.Set(req, RequestContext, &ctx)
				So(auther.Authenticate(serviceContext, req), ShouldBeNil)
			})
			Convey("if user is in the super users, should succeed", func() {
				superUsers := []string{"test_user"}
				projectRef.Admins = []string{"other_user"}
				ctx := model.Context{
					ProjectRef: &projectRef,
				}
				serviceContext.SetSuperUsers(superUsers)

				u := user.DBUser{
					Id: "test_user",
				}
				context.Set(req, RequestUser, &u)
				context.Set(req, RequestContext, &ctx)
				So(auther.Authenticate(serviceContext, req), ShouldBeNil)
			})
			Convey("if user is not in the admin and not a super user, should error", func() {
				superUsers := []string{"other_user"}
				serviceContext.SetSuperUsers(superUsers)

				projectRef.Admins = []string{"other_user"}
				ctx := model.Context{
					ProjectRef: &projectRef,
				}

				u := user.DBUser{
					Id: "test_user",
				}
				context.Set(req, RequestUser, &u)
				context.Set(req, RequestContext, &ctx)
				err := auther.Authenticate(serviceContext, req)

				errToResemble := apiv3.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Not Found",
				}
				So(err, ShouldResemble, errToResemble)
			})
		})
	})

}
func TestSuperUserAuthenticator(t *testing.T) {
	Convey("When there is an http request, "+
		"an authenticator, and a service context", t, func() {
		req, err := http.NewRequest(evergreen.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		serviceContext := &servicecontext.MockServiceContext{}
		auther := SuperUserAuthenticator{}
		Convey("When authenticating", func() {

			Reset(func() {
				context.Clear(req)
			})

			Convey("if user is in the superusers, should succeed", func() {
				superUsers := []string{"test_user"}
				serviceContext.SetSuperUsers(superUsers)

				u := user.DBUser{
					Id: "test_user",
				}
				context.Set(req, RequestUser, &u)
				So(auther.Authenticate(serviceContext, req), ShouldBeNil)
			})
			Convey("if user is not in the superusers, should error", func() {
				superUsers := []string{"other_user"}
				serviceContext.SetSuperUsers(superUsers)

				u := user.DBUser{
					Id: "test_user",
				}
				context.Set(req, RequestUser, &u)
				err := auther.Authenticate(serviceContext, req)

				errToResemble := apiv3.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Not Found",
				}
				So(err, ShouldResemble, errToResemble)

			})
		})
	})

}
