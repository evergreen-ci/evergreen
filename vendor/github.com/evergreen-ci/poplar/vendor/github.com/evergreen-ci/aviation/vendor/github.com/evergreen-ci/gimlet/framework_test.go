package gimlet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type mockHandler struct {
	shouldParseError  bool
	shouldNilResponse bool
	responder         Responder
}

func (m *mockHandler) Factory() RouteHandler {
	return &mockHandler{
		shouldParseError:  m.shouldParseError,
		shouldNilResponse: m.shouldNilResponse,
		responder:         m.responder,
	}
}
func (m *mockHandler) Parse(ctx context.Context, r *http.Request) error {
	if m.shouldParseError {
		return errors.New("should parse error")
	}

	return nil
}

func (m *mockHandler) Run(ctx context.Context) Responder {
	if m.shouldNilResponse {
		return nil

	}

	if m.responder != nil {
		return m.responder
	}

	return &responderImpl{}
}

type mockResponder struct {
	responderImpl

	shouldFailValidation bool
}

func (m *mockResponder) Validate() error {
	if m.shouldFailValidation {
		return errors.New("validation error")
	}
	return nil
}

type HandlerSuite struct {
	rw *httptest.ResponseRecorder
	r  *http.Request
	suite.Suite
}

func TestHandlerSuite(t *testing.T) { suite.Run(t, new(HandlerSuite)) }

func (s *HandlerSuite) SetupTest() {
	url, err := url.Parse("http://example.net/test-route/")
	s.Require().NoError(err)
	s.Require().NotNil(url)
	s.rw = httptest.NewRecorder()
	s.r = &http.Request{
		URL: url,
	}
}

func (s *HandlerSuite) TestParseError() {
	handler := handleHandler(&mockHandler{
		shouldParseError: true,
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Contains(s.rw.Body.String(), "parse error")
	s.Equal(http.StatusBadRequest, s.rw.Code)
}

func (s *HandlerSuite) TestNilResponseReturnsError() {
	handler := handleHandler(&mockHandler{
		shouldNilResponse: true,
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Contains(s.rw.Body.String(), "undefined response")
	s.Equal(http.StatusInternalServerError, s.rw.Code)
}

func (s *HandlerSuite) TestValidationReturnsErrors() {
	handler := handleHandler(&mockHandler{
		responder: &mockResponder{
			shouldFailValidation: true,
		},
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Contains(s.rw.Body.String(), "validation error")
	s.Equal(http.StatusBadRequest, s.rw.Code)
}

func (s *HandlerSuite) TestPaginationPopulatesHeaders() {
	handler := handleHandler(&mockHandler{
		responder: &mockResponder{
			responderImpl: responderImpl{
				status: http.StatusTeapot,
				pages: &ResponsePages{
					Next: &Page{},
					Prev: &Page{},
				},
			},
		},
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Equal(http.StatusTeapot, s.rw.Code)
	links, ok := s.rw.Header()["Link"]
	s.True(ok)
	for _, l := range links {
		s.Contains(l, "test-route")
	}

}

func (s *HandlerSuite) TestJSONOutputFormatCorrectlyPropogated() {
	handler := handleHandler(&mockHandler{
		responder: &mockResponder{
			responderImpl: responderImpl{
				format: JSON,
				status: http.StatusTeapot,
			},
		},
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Equal(http.StatusTeapot, s.rw.Code)
	s.Contains(s.rw.HeaderMap["Content-Type"][0], JSON.ContentType())
}

func (s *HandlerSuite) TestYAMLOutputFormatCorrectlyPropogated() {
	handler := handleHandler(&mockHandler{
		responder: &mockResponder{
			responderImpl: responderImpl{
				format: YAML,
				status: http.StatusTeapot,
			},
		},
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Equal(http.StatusTeapot, s.rw.Code)
	s.Contains(s.rw.HeaderMap["Content-Type"][0], YAML.ContentType())
}

func (s *HandlerSuite) TestTextOutputFormatCorrectlyPropogated() {
	handler := handleHandler(&mockHandler{
		responder: &mockResponder{
			responderImpl: responderImpl{
				format: TEXT,
				status: http.StatusTeapot,
			},
		},
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Equal(http.StatusTeapot, s.rw.Code)
	s.Contains(s.rw.HeaderMap["Content-Type"][0], TEXT.ContentType())
}

func (s *HandlerSuite) TestHTMLOutputFormatCorrectlyPropogated() {
	handler := handleHandler(&mockHandler{
		responder: &mockResponder{
			responderImpl: responderImpl{
				format: HTML,
				status: http.StatusTeapot,
			},
		},
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Equal(http.StatusTeapot, s.rw.Code)
	s.Contains(s.rw.HeaderMap["Content-Type"][0], HTML.ContentType())
}

func (s *HandlerSuite) TestBinOutputFormatCorrectlyPropogated() {
	handler := handleHandler(&mockHandler{
		responder: &mockResponder{
			responderImpl: responderImpl{
				format: BINARY,
				status: http.StatusTeapot,
			},
		},
	})

	s.NotPanics(func() { handler(s.rw, s.r) })
	s.Equal(http.StatusTeapot, s.rw.Code)
	s.Contains(s.rw.HeaderMap["Content-Type"][0], BINARY.ContentType())
}

func (s *HandlerSuite) TestGetErrorHelper() {
	in := ErrorResponse{100, "foo"}
	out := getError(in, 8998)
	s.Equal(100, out.StatusCode)
	s.Equal("foo", out.Message)

	in2 := &ErrorResponse{101, "foo0"}
	out = getError(in2, 8999)
	s.Equal(101, out.StatusCode)
	s.Equal("foo0", out.Message)

	in3 := errors.WithStack(in2)
	out = getError(in3, 9000)
	s.Equal(101, out.StatusCode)
	s.Equal("foo0", out.Message)

	in4 := errors.New("wtf")
	out = getError(in4, 9001)
	s.Equal(9001, out.StatusCode)
	s.Equal("wtf", out.Message)
}
