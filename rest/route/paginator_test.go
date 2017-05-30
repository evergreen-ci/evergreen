package route

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/context"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMakePaginationHeader(t *testing.T) {
	testURL := "http://evergreenurl.com"
	testRoute := "test/route"

	testPrevKey := "prevkey"
	testNextKey := "nextkey"
	testNextLimit := 100
	testPrevLimit := 105

	testLimitQueryParam := "limit"
	testKeyQueryParam := "key"

	Convey("When there is an http response writer", t, func() {
		w := httptest.NewRecorder()
		Convey("creating a PaginationMetadata with both a NextKey and PrevKey"+
			" should create 2 links in the header", func() {
			testPages := &PageResult{
				Next: &Page{
					Key:      testNextKey,
					Relation: "next",
					Limit:    testNextLimit,
				},
				Prev: &Page{
					Key:      testPrevKey,
					Relation: "prev",
					Limit:    testPrevLimit,
				},
			}

			pm := &PaginationMetadata{
				Pages:           testPages,
				KeyQueryParam:   testKeyQueryParam,
				LimitQueryParam: testLimitQueryParam,
			}

			err := pm.MakeHeader(w, testURL, testRoute)
			So(err, ShouldBeNil)
			linksHeader, ok := w.Header()["Link"]
			So(ok, ShouldBeTrue)
			So(len(linksHeader), ShouldEqual, 1)
			pmRes, err := ParsePaginationHeader(linksHeader[0], testKeyQueryParam, testLimitQueryParam)
			So(err, ShouldBeNil)

			So(pmRes.Pages, ShouldNotBeNil)
			So(pmRes.Pages.Next, ShouldResemble, testPages.Next)
			So(pmRes.Pages.Prev, ShouldResemble, testPages.Prev)

		})
		Convey("creating a PaginationMetadata with just a NextKey"+
			" should creating 1 links in the header", func() {
			testPages := &PageResult{
				Next: &Page{
					Key:      testNextKey,
					Relation: "next",
					Limit:    testNextLimit,
				},
			}

			pm := &PaginationMetadata{
				Pages: testPages,

				KeyQueryParam:   testKeyQueryParam,
				LimitQueryParam: testLimitQueryParam,
			}

			err := pm.MakeHeader(w, testURL, testRoute)
			So(err, ShouldBeNil)
			linksHeader, ok := w.Header()["Link"]
			So(ok, ShouldBeTrue)
			So(len(linksHeader), ShouldEqual, 1)
			pmRes, err := ParsePaginationHeader(linksHeader[0], testKeyQueryParam, testLimitQueryParam)
			So(err, ShouldBeNil)

			So(pmRes.Pages, ShouldNotBeNil)
			So(pmRes.Pages.Next, ShouldResemble, testPages.Next)

		})
		Convey("creating a page with a limit of 0 should parse as 0", func() {
			testPages := &PageResult{
				Next: &Page{
					Key:      testNextKey,
					Relation: "next",
				},
			}

			pm := &PaginationMetadata{
				Pages: testPages,

				KeyQueryParam:   testKeyQueryParam,
				LimitQueryParam: testLimitQueryParam,
			}

			err := pm.MakeHeader(w, testURL, testRoute)
			So(err, ShouldBeNil)
			linksHeader, ok := w.Header()["Link"]
			So(ok, ShouldBeTrue)
			So(len(linksHeader), ShouldEqual, 1)
			pmRes, err := ParsePaginationHeader(linksHeader[0], testKeyQueryParam, testLimitQueryParam)
			So(err, ShouldBeNil)

			So(pmRes.Pages, ShouldNotBeNil)
			So(pmRes.Pages.Next, ShouldResemble, testPages.Next)
		})
	})
}

type testPaginationRequestHandler struct {
	*PaginationExecutor
}

func (prh *testPaginationRequestHandler) Handler() RequestHandler {
	return prh
}

func TestPaginationExecutor(t *testing.T) {
	curPage := Page{
		Limit:    10,
		Key:      "cur_key",
		Relation: "current",
	}
	nextPage := Page{
		Limit:    10,
		Key:      "next_key",
		Relation: "next",
	}
	prevPage := Page{
		Limit:    7,
		Key:      "prev_key",
		Relation: "prev",
	}
	numModels := 10
	testKeyQueryParam := "key"
	testLimitQueryParam := "limit"
	testModels := []model.Model{}
	for i := 0; i < numModels; i++ {
		nextModel := &model.MockModel{
			FieldId:   fmt.Sprintf("model_%d", i),
			FieldInt1: i,
		}
		testModels = append(testModels, nextModel)
	}
	Convey("When paginating with a paginator func and executor", t, func() {
		testPaginatorFunc := mockPaginatorFuncGenerator(testModels, nextPage.Key, prevPage.Key,
			nextPage.Limit, prevPage.Limit, nil)

		executor := &PaginationExecutor{
			KeyQueryParam:   testKeyQueryParam,
			LimitQueryParam: testLimitQueryParam,

			Paginator: testPaginatorFunc,
		}

		prh := &testPaginationRequestHandler{
			executor,
		}

		mh := MethodHandler{
			MethodType:     "GET",
			Authenticator:  &NoAuthAuthenticator{},
			RequestHandler: prh,
		}
		Convey("A request with key and limit should return a correctly formed result",
			func() {

				queryParams := fmt.Sprintf("%s=%s&%s=%d", testKeyQueryParam, curPage.Key,
					testLimitQueryParam, curPage.Limit)

				expectedPages := PageResult{&nextPage, &prevPage}

				hcf := func(header map[string][]string, t *testing.T) {
					linkHeader := header["Link"]
					So(len(linkHeader), ShouldEqual, 1)
					pmRes, err := ParsePaginationHeader(linkHeader[0], testKeyQueryParam, testLimitQueryParam)
					So(err, ShouldBeNil)

					So(pmRes.Pages, ShouldNotBeNil)
					So(pmRes.Pages.Next, ShouldResemble, expectedPages.Next)
					So(pmRes.Pages.Prev, ShouldResemble, expectedPages.Prev)
				}

				checkResultMatches(mh, nil,
					testModels, hcf, queryParams, http.StatusOK, t)
			})
		Convey("A request without a non number limit should return an error",
			func() {

				queryParams := fmt.Sprintf("%s=%s", testLimitQueryParam, "garbage")

				expectedError := rest.APIError{
					StatusCode: http.StatusBadRequest,
					Message: fmt.Sprintf("Value '%v' provided for '%v' must be integer",
						"garbage", testLimitQueryParam),
				}

				checkResultMatches(mh, expectedError,
					nil, nil, queryParams, http.StatusBadRequest, t)
			})
		Convey("an errored executor should return an error",
			func() {
				errToReturn := fmt.Errorf("not found in DB")

				executor.Paginator = mockPaginatorFuncGenerator(nil, "", "", 0, 0, errToReturn)
				checkResultMatches(mh, errToReturn,
					nil, nil, "", http.StatusInternalServerError, t)
			})
	})
}

type testAdditionalArgsPaginator struct {
	*PaginationExecutor
}

func (tap *testAdditionalArgsPaginator) Handler() RequestHandler {
	return tap
}

type ArgHolder struct {
	Arg1 int
	Arg2 string
}

const (
	testArg1Holder = "arg1"
	testArg2Holder = "arg2"
)

func (tap *testAdditionalArgsPaginator) ParseAndValidate(r *http.Request) error {
	arg1 := context.Get(r, testArg1Holder)
	castArg1, ok := arg1.(int)
	if !ok {
		return fmt.Errorf("incorrect type for arg1. found %v", arg1)
	}
	arg2 := context.Get(r, testArg2Holder)
	castArg2, ok := arg2.(string)
	if !ok {
		return fmt.Errorf("incorrect type for arg2. found %v", arg2)
	}

	ah := ArgHolder{
		Arg1: castArg1,
		Arg2: castArg2,
	}
	tap.Args = &ah
	return tap.PaginationExecutor.ParseAndValidate(r)
}

func TestPaginatorAdditionalArgs(t *testing.T) {
	Convey("When paginating with a additional args paginator func and args", t, func() {
		arg1 := 7
		arg2 := "testArg1"

		sc := data.MockConnector{}

		r := &http.Request{
			URL: &url.URL{
				RawQuery: "key=key&limit=100",
			},
		}

		context.Set(r, testArg1Holder, arg1)
		context.Set(r, testArg2Holder, arg2)

		pf := func(key string, limit int, args interface{},
			sc data.Connector) ([]model.Model, *PageResult, error) {
			castArgs, ok := args.(*ArgHolder)
			So(ok, ShouldBeTrue)
			So(castArgs.Arg1, ShouldEqual, arg1)
			So(castArgs.Arg2, ShouldEqual, arg2)
			So(key, ShouldEqual, "key")
			So(limit, ShouldEqual, 100)
			return []model.Model{}, nil, nil
		}

		executor := &PaginationExecutor{
			KeyQueryParam:   "key",
			LimitQueryParam: "limit",

			Paginator: pf,
		}
		tap := testAdditionalArgsPaginator{executor}

		Convey("parsing should succeed and args should be set", func() {
			err := tap.ParseAndValidate(r)
			So(err, ShouldBeNil)
			_, err = tap.Execute(&sc)
			So(err, ShouldBeNil)
		})
	})
}
