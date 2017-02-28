package route

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/gorilla/mux"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMakeRoute(t *testing.T) {

	// With a route with many methods
	Convey("With mocked methods", t, func() {
		getAuth := &mockAuthenticator{}
		getHandler := &mockRequestHandler{
			storedModels: []model.Model{
				&model.MockModel{
					FieldId: "GET_method",
				},
			},
		}
		mockGet := MethodHandler{
			Authenticator:  getAuth,
			RequestHandler: getHandler,
			MethodType:     evergreen.MethodGet,
		}

		postAuth := &mockAuthenticator{}
		postHandler := &mockRequestHandler{
			storedModels: []model.Model{
				&model.MockModel{
					FieldId: "POST_method",
				},
			},
		}
		mockPost := MethodHandler{
			Authenticator:  postAuth,
			RequestHandler: postHandler,
			MethodType:     evergreen.MethodPost,
		}

		deleteAuth := &mockAuthenticator{}
		deleteHandler := &mockRequestHandler{
			storedModels: []model.Model{
				&model.MockModel{
					FieldId: "DELETE_method",
				},
			},
		}
		mockDelete := MethodHandler{
			Authenticator:  deleteAuth,
			RequestHandler: deleteHandler,
			MethodType:     evergreen.MethodDelete,
		}
		Convey("then adding and registering should result in a correct route", func() {
			sc := &servicecontext.MockServiceContext{}
			sc.SetPrefix("rest")
			r := mux.NewRouter()
			route := &RouteManager{
				Version: 2,
				Route:   "/test_path",
				Methods: []MethodHandler{mockGet, mockPost, mockDelete},
			}
			route.Register(r, sc)

			for _, method := range route.Methods {

				req, err := http.NewRequest(method.MethodType, "/rest/v2/test_path", nil)

				So(err, ShouldBeNil)
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)
				So(rr.Code, ShouldEqual, http.StatusOK)

				res := model.MockModel{}
				err = json.Unmarshal(rr.Body.Bytes(), &res)

				So(err, ShouldBeNil)
				So(res.FieldId, ShouldEqual, fmt.Sprintf("%s_method", method.MethodType))
			}
		})
	})
}
