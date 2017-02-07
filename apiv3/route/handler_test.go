package route

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/util"
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
			auth.err = apiv3.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Not Authenticated",
			}
			checkResultMatches(mockMethod, auth.err, nil, http.StatusBadRequest, t)
		})
		Convey("And parse errors should cause response to "+
			"contain error information", func() {
			requestHandler.parseErr = apiv3.APIError{
				StatusCode: http.StatusRequestEntityTooLarge,
				Message:    "Request too large to parse",
			}
			checkResultMatches(mockMethod, requestHandler.parseErr,
				nil, http.StatusRequestEntityTooLarge, t)
		})
		Convey("And validate errors should cause response to "+
			"contain error information", func() {
			requestHandler.validateErr = apiv3.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Request did not contain all fields",
			}
			checkResultMatches(mockMethod, requestHandler.validateErr,
				nil, http.StatusBadRequest, t)
		})
		Convey("And execute errors should cause response to "+
			"contain error information", func() {
			requestHandler.executeErr = apiv3.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Not found in DB",
			}
			checkResultMatches(mockMethod, requestHandler.executeErr,
				nil, http.StatusNotFound, t)
		})
		Convey("And validate and execute errors should cause response to "+
			"contain validate error information", func() {
			requestHandler.executeErr = apiv3.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Not found in DB",
			}
			requestHandler.validateErr = apiv3.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Request did not contain all fields",
			}
			checkResultMatches(mockMethod, requestHandler.validateErr,
				nil, http.StatusBadRequest, t)
		})
		Convey("And authenticate and execute errors should cause response to "+
			"contain authenticate error information", func() {
			auth.err = apiv3.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Not Authenticated",
			}
			requestHandler.executeErr = apiv3.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Not found in DB",
			}
			checkResultMatches(mockMethod, auth.err,
				nil, http.StatusBadRequest, t)
		})
		Convey("And non-API errors should be surfaced as an internal "+
			"server error", func() {
			requestHandler.executeErr = errors.New("The DB is broken")
			checkResultMatches(mockMethod, requestHandler.executeErr,
				nil, http.StatusInternalServerError, t)
		})
		Convey("And api model should be returned successfully ", func() {
			requestHandler.storedModel = &model.MockModel{
				FieldId:   "model_id",
				FieldInt1: 1234,
				FieldInt2: 4567,
				FieldMap: map[string]string{
					"mapkey1": "mapval1",
					"mapkey2": "mapval2",
				},
				FieldStruct: &model.MockSubStruct{1},
			}
			checkResultMatches(mockMethod, nil,
				requestHandler.storedModel, http.StatusOK, t)
		})
	})
}

func checkResultMatches(m MethodHandler, expectedErr error,
	expectedModel model.Model, expectedStatusCode int, t *testing.T) {

	handler := makeHandler(m, &servicecontext.MockServiceContext{})
	req := &http.Request{}
	resp := httptest.NewRecorder()
	handler(resp, req)

	if expectedErr != nil {
		errResult := apiv3.APIError{}
		wrappedBody := ioutil.NopCloser(resp.Body)

		err := util.ReadJSONInto(wrappedBody, &errResult)
		So(err, ShouldBeNil)

		So(errResult.StatusCode, ShouldEqual, expectedStatusCode)
		if apiErr, ok := expectedErr.(apiv3.APIError); ok {
			So(apiErr.Message, ShouldEqual, errResult.Message)
		} else {
			So(expectedErr.Error(), ShouldEqual, errResult.Message)
		}
		So(resp.Code, ShouldEqual, expectedStatusCode)
	} else if expectedModel != nil {
		wrappedBody := ioutil.NopCloser(resp.Body)
		respModel := &model.MockModel{}
		err := util.ReadJSONInto(wrappedBody, respModel)
		So(err, ShouldBeNil)

		So(respModel, ShouldResemble, expectedModel)
	} else {
		t.Errorf("Expected-error and expected-model cannot both be nil")
	}
}
