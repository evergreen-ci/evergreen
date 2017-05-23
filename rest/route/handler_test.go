package route

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/servicecontext"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMakeHandler(t *testing.T) {

	Convey("When creating an http.HandlerFunc", t, func() {
		auth := &mockAuthenticator{}
		requestHandler := &mockRequestHandler{}
		mockMethod := MethodHandler{
			Authenticator:  auth,
			RequestHandler: requestHandler,
		}

		Convey("And authenticator errors should cause response to "+
			"contain error information", func() {
			auth.err = rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Not Authenticated",
			}
			checkResultMatchesBasic(mockMethod, auth.err, nil, http.StatusBadRequest, t)
		})
		Convey("And parseValidate errors should cause response to "+
			"contain error information", func() {
			requestHandler.parseValidateErr = rest.APIError{
				StatusCode: http.StatusRequestEntityTooLarge,
				Message:    "Request too large to parse",
			}
			checkResultMatchesBasic(mockMethod, requestHandler.parseValidateErr,
				nil, http.StatusRequestEntityTooLarge, t)
		})
		Convey("And execute errors should cause response to "+
			"contain error information", func() {
			requestHandler.executeErr = rest.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Not found in DB",
			}
			checkResultMatchesBasic(mockMethod, requestHandler.executeErr,
				nil, http.StatusNotFound, t)
		})
		Convey("And parseValidate and execute errors should cause response to "+
			"contain parseValidate error information", func() {
			requestHandler.executeErr = rest.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Not found in DB",
			}
			requestHandler.parseValidateErr = rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Request did not contain all fields",
			}
			checkResultMatchesBasic(mockMethod, requestHandler.parseValidateErr,
				nil, http.StatusBadRequest, t)
		})
		Convey("And authenticate and execute errors should cause response to "+
			"contain authenticate error information", func() {
			auth.err = rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Not Authenticated",
			}
			requestHandler.executeErr = rest.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Not found in DB",
			}
			checkResultMatchesBasic(mockMethod, auth.err,
				nil, http.StatusBadRequest, t)
		})
		Convey("And non-API errors should be surfaced as an internal "+
			"server error", func() {
			requestHandler.executeErr = errors.New("The DB is broken")
			checkResultMatchesBasic(mockMethod, requestHandler.executeErr,
				nil, http.StatusInternalServerError, t)
		})
		Convey("And api model should be returned successfully ", func() {
			requestHandler.storedModels = []model.Model{
				&model.MockModel{
					FieldId:   "model_id",
					FieldInt1: 1234,
					FieldInt2: 4567,
					FieldMap: map[string]string{
						"mapkey1": "mapval1",
						"mapkey2": "mapval2",
					},
					FieldStruct: &model.MockSubStruct{1},
				},
			}
			checkResultMatchesBasic(mockMethod, nil,
				requestHandler.storedModels, http.StatusOK, t)
		})
	})
}

type headerCheckFunc func(map[string][]string, *testing.T)

func checkResultMatchesBasic(m MethodHandler, expectedErr error,
	expectedModels []model.Model, expectedStatusCode int, t *testing.T) {
	checkResultMatches(m, expectedErr, expectedModels, nil, "", expectedStatusCode, t)
}

func checkResultMatches(m MethodHandler, expectedErr error,
	expectedModels []model.Model, headerCheck headerCheckFunc, queryParams string,
	expectedStatusCode int, t *testing.T) {

	testRoute := "/test/route"

	handler := makeHandler(m, &servicecontext.MockServiceContext{})

	u := &url.URL{
		RawQuery: queryParams,
		Path:     testRoute,
	}

	req, err := http.NewRequest(evergreen.MethodGet, u.String(), nil)
	So(err, ShouldBeNil)

	resp := httptest.NewRecorder()
	r := mux.NewRouter()
	r.HandleFunc(testRoute, handler)

	r.ServeHTTP(resp, req)

	if expectedErr != nil {
		errResult := rest.APIError{}
		wrappedBody := ioutil.NopCloser(resp.Body)

		err := util.ReadJSONInto(wrappedBody, &errResult)
		So(err, ShouldBeNil)

		So(errResult.StatusCode, ShouldEqual, expectedStatusCode)
		if apiErr, ok := expectedErr.(rest.APIError); ok {
			So(apiErr.Message, ShouldEqual, errResult.Message)
		} else {
			So(expectedErr.Error(), ShouldEqual, errResult.Message)
		}
		So(resp.Code, ShouldEqual, expectedStatusCode)
	} else if len(expectedModels) > 0 {
		wrappedBody := ioutil.NopCloser(resp.Body)

		var respResult []*model.MockModel
		if len(expectedModels) == 1 {
			singleResult := &model.MockModel{}
			err := util.ReadJSONInto(wrappedBody, singleResult)
			So(err, ShouldBeNil)
			respResult = append(respResult, singleResult)
		} else {
			err := util.ReadJSONInto(wrappedBody, &respResult)
			So(err, ShouldBeNil)
		}

		for ix, res := range respResult {
			var resModel model.Model = res
			So(resModel, ShouldResemble, expectedModels[ix])
		}

	} else {
		t.Errorf("Expected-error and expected-model cannot both be nil")
	}

	if headerCheck != nil {
		headerCheck(resp.Header(), t)
	}
}
