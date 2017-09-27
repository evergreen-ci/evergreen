package route

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestPrefetchUser(t *testing.T) {
	Convey("When there are users to fetch and a request", t, func() {
		serviceContext := &data.MockConnector{}
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
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		Convey("When examining users", func() {

			ctx := context.Background()

			Convey("When no header is set, should no-op", func() {
				ctx, err = PrefetchUser(ctx, serviceContext, req)
				So(err, ShouldBeNil)

				So(ctx.Value(evergreen.RequestUser), ShouldBeNil)
			})

			Convey("When just API-Key is set, should not set anything", func() {
				for i := 0; i < numUsers; i++ {
					req.Header = http.Header{}
					userId := fmt.Sprintf("user_%d", i)
					req.Header.Set("Api-User", userId)
					ctx, err = PrefetchUser(ctx, serviceContext, req)
					So(err, ShouldBeNil)

					So(ctx.Value(evergreen.RequestUser), ShouldBeNil)
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
					ctx, err = PrefetchUser(ctx, serviceContext, req)
					So(err, ShouldBeNil)
					u := user.DBUser{
						Id:     userId,
						APIKey: apiKey,
					}

					So(ctx.Value(evergreen.RequestUser), ShouldResemble, &u)
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

					ctx, err = PrefetchUser(ctx, serviceContext, req)

					errToResemble := rest.APIError{
						StatusCode: http.StatusUnauthorized,
						Message:    "Invalid API key",
					}

					So(err, ShouldResemble, errToResemble)
					So(ctx.Value(evergreen.RequestUser), ShouldBeNil)
				}
			})
		})
	})
}

func TestPrefetchProject(t *testing.T) {
	Convey("When there is a data and a request", t, func() {
		serviceContext := &data.MockConnector{}
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		Convey("When fetching the project context", func() {
			ctx := context.Background()
			Convey("should error if project is private and no user is set", func() {
				opCtx := model.Context{}
				opCtx.ProjectRef = &model.ProjectRef{
					Private: true,
				}
				serviceContext.MockContextConnector.CachedContext = opCtx
				ctx, err = PrefetchProjectContext(ctx, serviceContext, req)
				So(ctx.Value(RequestContext), ShouldBeNil)

				errToResemble := rest.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Project not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should error if patch exists and no user is set", func() {
				opCtx := model.Context{}
				opCtx.Patch = &patch.Patch{}
				serviceContext.MockContextConnector.CachedContext = opCtx
				ctx, err = PrefetchProjectContext(ctx, serviceContext, req)
				So(ctx.Value(RequestContext), ShouldBeNil)

				errToResemble := rest.APIError{
					StatusCode: http.StatusNotFound,
					Message:    "Not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should succeed if project ref exists and user is set", func() {
				opCtx := model.Context{}
				opCtx.ProjectRef = &model.ProjectRef{
					Private: true,
				}
				ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "test_user"})
				serviceContext.MockContextConnector.CachedContext = opCtx
				ctx, err = PrefetchProjectContext(ctx, serviceContext, req)
				So(err, ShouldBeNil)

				So(ctx.Value(RequestContext), ShouldResemble, &opCtx)
			})
		})
	})
}
