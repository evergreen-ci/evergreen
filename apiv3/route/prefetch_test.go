package route

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/gorilla/context"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPrefetchUser(t *testing.T) {
	Convey("When there are users to fetch and a request", t, func() {
		serviceContext := &servicecontext.MockServiceContext{}
		users := map[string]*user.DBUser{}
		numUsers := 10
		for i := 0; i < numUsers; i++ {
			userId := fmt.Sprintf("user_%d", i)
			apiKey := fmt.Sprintf("apiKey_%d", i)
			users[userId] = &user.DBUser{
				Id:     userId,
				APIKey: apiKey,
			}
		}
		serviceContext.MockUserConnector.CachedUsers = users
		req, err := http.NewRequest(evergreen.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		Convey("When examining users", func() {

			Reset(func() {
				context.Clear(req)
			})

			Convey("When no header is set, should no-op", func() {
				err := PrefetchUser(req, serviceContext)
				So(err, ShouldBeNil)

				So(context.Get(req, RequestUser), ShouldBeNil)
			})

			Convey("When just API-Key is set, should not set anything", func() {
				for i := 0; i < numUsers; i++ {
					req.Header = http.Header{}
					userId := fmt.Sprintf("user_%d", i)
					req.Header.Set("Api-User", userId)
					err := PrefetchUser(req, serviceContext)
					So(err, ShouldBeNil)

					So(context.Get(req, RequestUser), ShouldBeNil)
				}
			})
			Convey("When API-User and API-Key is set,"+
				" should set user in the context", func() {
				for i := 0; i < numUsers; i++ {
					req.Header = http.Header{}
					userId := fmt.Sprintf("user_%d", i)
					apiKey := fmt.Sprintf("apiKey_%d", i)
					req.Header.Set("Api-Key", apiKey)
					req.Header.Set("Api-User", userId)
					err := PrefetchUser(req, serviceContext)
					So(err, ShouldBeNil)
					u := user.DBUser{
						Id:     userId,
						APIKey: apiKey,
					}

					So(context.Get(req, RequestUser), ShouldResemble, &u)
				}
			})
			Convey("When API-User and API-Key is set incorrectly,"+
				" should return an error", func() {
				for i := 0; i < numUsers; i++ {
					req.Header = http.Header{}
					userId := fmt.Sprintf("user_%d", i)
					apiKey := fmt.Sprintf("apiKey_%d", i+1)
					req.Header.Set("Api-Key", apiKey)
					req.Header.Set("Api-User", userId)

					err := PrefetchUser(req, serviceContext)

					errToResemble := apiv3.APIError{
						StatusCode: http.StatusUnauthorized,
						Message:    "Invalid API key",
					}

					So(err, ShouldResemble, errToResemble)
					So(context.Get(req, RequestUser), ShouldBeNil)
				}
			})
		})
	})
}

func TestPrefetchProject(t *testing.T) {
	Convey("When there is a servicecontext and a request", t, func() {
		serviceContext := &servicecontext.MockServiceContext{}
		req, err := http.NewRequest(evergreen.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		Convey("When fetching the project context", func() {
			Reset(func() {
				context.Clear(req)
			})
			Convey("should error if project is private and no user is set", func() {
				ctx := model.Context{}
				ctx.ProjectRef = &model.ProjectRef{
					Private: true,
				}
				serviceContext.MockContextConnector.CachedContext = ctx
				err := PrefetchProjectContext(req, serviceContext)
				So(context.Get(req, RequestContext), ShouldBeNil)

				errToResemble := apiv3.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Project Not Found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should error if patch exists and no user is set", func() {
				ctx := model.Context{}
				ctx.Patch = &patch.Patch{}
				serviceContext.MockContextConnector.CachedContext = ctx
				err := PrefetchProjectContext(req, serviceContext)
				So(context.Get(req, RequestContext), ShouldBeNil)

				errToResemble := apiv3.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Not Found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should succeed if project ref exists and user is set", func() {
				ctx := model.Context{}
				ctx.ProjectRef = &model.ProjectRef{
					Private: true,
				}
				context.Set(req, RequestUser, &user.DBUser{Id: "test_user"})
				serviceContext.MockContextConnector.CachedContext = ctx
				err := PrefetchProjectContext(req, serviceContext)
				So(err, ShouldBeNil)

				So(context.Get(req, RequestContext), ShouldResemble, &ctx)
			})
		})
	})
}
