package gimlet

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// ResponderSuite provides a basic battery of tests for each responder
// implementation included in the gimlet package.
type ResponderSuite struct {
	factory func() Responder
	resp    Responder
	suite.Suite
}

func TestResponderImplSuite(t *testing.T) {
	s := new(ResponderSuite)
	s.factory = func() Responder { return &responderImpl{} }
	suite.Run(t, s)
}

func TestResponderBuilderSuite(t *testing.T) {
	s := new(ResponderSuite)
	s.factory = func() Responder { return &responseBuilder{} }
	suite.Run(t, s)
}

func (s *ResponderSuite) SetupTest() {
	s.resp = s.factory()
}

func (s *ResponderSuite) TestDefaultValues() {
	s.Nil(s.resp.Pages())
	s.Zero(s.resp.Status())
	s.Zero(s.resp.Data())
	s.Zero(s.resp.Format())
}

func (s *ResponderSuite) TesteStatusSetter() {
	for _, good := range []int{200, 500, 418, 502, 409,
		http.StatusOK,
		http.StatusInternalServerError,
		http.StatusTeapot,
		http.StatusServiceUnavailable,
		http.StatusConflict,
	} {
		s.NoError(s.resp.SetStatus(good))
		s.Equal(good, s.resp.Status())
	}

	for _, bad := range []int{42, 99, 103, 150, 199, 227, 299, 309, 399, 419, 452, 420, 499, 512, 9001} {
		s.Error(s.resp.SetStatus(bad))
		s.NotEqual(bad, s.resp.Status())
		s.NotZero(s.resp.Status())
	}
}

func (s *ResponderSuite) TestSetFormat() {
	for _, good := range []OutputFormat{JSON, YAML, BINARY, TEXT, HTML} {
		s.NoError(s.resp.SetFormat(good))
		s.Equal(good, s.resp.Format())
	}

	for _, bad := range []OutputFormat{100, 42, -1, -100, -42, 9001} {
		s.Error(s.resp.SetFormat(bad))
		s.NotEqual(bad, s.resp.Format())
		s.NotZero(s.resp.Format())
	}
}

func (s *ResponderSuite) TestSetData() {
	// nil shold error, but it will return nil because the object it zero'd
	s.Error(s.resp.AddData(nil))

	// setting should work
	s.NoError(s.resp.AddData(1))
	s.NotNil(s.resp.Data())

	switch val := s.resp.Data().(type) {
	case int:
		s.True(val == 1)
	case []interface{}:
		s.Len(val, 1)
		s.Equal(1, val[0])
	default:
		// should never get here
		s.True(false)
	}

	// nil should error and not not rest error
	s.Error(s.resp.AddData(nil))

	switch val := s.resp.Data().(type) {
	case int:
		s.True(val == 1)
	case []interface{}:
		s.Len(val, 1)
		s.Equal(1, val[0])
	default:
		// should never get here
		s.True(false)
	}
}

func (s *ResponderSuite) TestMutabilityOfData() {
	s.NoError(s.resp.AddData(1))

	switch r := s.resp.(type) {
	case *responderImpl:
		s.Error(r.AddData(2))
		s.Equal(1, r.Data())
	case *responseBuilder:
		s.NoError(r.AddData(2))
		s.Equal([]interface{}{1, 2}, r.Data())
	}
}

func (s *ResponderSuite) TestPages() {
	s.Nil(s.resp.Pages())

	for _, bad := range []*ResponsePages{
		{Next: &Page{}, Prev: &Page{}},
	} {
		s.Error(s.resp.SetPages(bad))
		s.Nil(s.resp.Pages())
	}

	for _, good := range []*ResponsePages{
		{},
		{nil, nil},
	} {
		s.NoError(s.resp.SetPages(good))
		s.NotNil(s.resp.Pages())
	}
}

func TestResponseBuilderConstructor(t *testing.T) {
	var (
		err  error
		resp Responder
	)

	assert := assert.New(t)

	resp, err = NewBasicResponder(http.StatusOK, TEXT, "foo")
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Implements((*Responder)(nil), resp)

	resp, err = NewBasicResponder(42, TEXT, "foo")
	assert.Error(err)
	assert.Nil(resp)

	resp, err = NewBasicResponder(http.StatusAccepted, TEXT, nil)
	assert.Error(err)
	assert.Nil(resp)

	resp, err = NewBasicResponder(http.StatusAccepted, OutputFormat(100), 1)
	assert.Error(err)
	assert.Nil(resp)

	resp, err = NewBasicResponder(42, OutputFormat(100), nil)
	assert.Error(err)
	assert.Nil(resp)

	resp = NewResponseBuilder()
	assert.NotNil(resp)
	assert.NoError(resp.Validate())

	resp = &responseBuilder{format: 11}
	assert.Error(resp.Validate())

	resp = &responseBuilder{pages: &ResponsePages{Next: &Page{}}}
	resp.SetFormat(TEXT)
	resp.SetStatus(http.StatusTeapot)
	assert.Error(resp.Validate())

	resp = &responseBuilder{}
	assert.Error(resp.Validate())
	resp.SetFormat(TEXT)
	resp.SetStatus(http.StatusTeapot)
	assert.NoError(resp.Validate())

}

func TestSimpleResponseBuilder(t *testing.T) {
	f := map[string]interface{}{"foo": "bar"}
	err := errors.New("foo")

	for idx, er := range []error{
		ErrorResponse{0, "coffee"},
		&ErrorResponse{0, "coffee"},
		ErrorResponse{400, "coffee"},
		&ErrorResponse{400, "coffee"},
		ErrorResponse{501, "coffee"},
		&ErrorResponse{501, "coffee"},
	} {
		t.Run(fmt.Sprintf("TextConstructorCase%d", idx), func(t *testing.T) {
			for idx, resp := range []Responder{
				NewTextResponse("foo"),
				NewTextErrorResponse("foo"),
				NewTextInternalErrorResponse("foo"),
				MakeTextErrorResponder(err),
				MakeTextErrorResponder(er),
				MakeTextInternalErrorResponder(err),
			} {
				assert.Equal(t, TEXT, resp.Format(), "%d", idx)
				assert.NoError(t, resp.Validate())
			}
		})
		t.Run(fmt.Sprintf("HTMLConstructorCase%d", idx), func(t *testing.T) {
			for idx, resp := range []Responder{
				NewHTMLResponse("foo"),
				NewHTMLErrorResponse("foo"),
				NewHTMLInternalErrorResponse("foo"),
			} {
				assert.Equal(t, HTML, resp.Format(), "%d", idx)
				assert.NoError(t, resp.Validate())
			}
		})
		t.Run(fmt.Sprintf("BinaryConstructorCase%d", idx), func(t *testing.T) {
			for idx, resp := range []Responder{
				NewBinaryResponse(f),
				NewBinaryErrorResponse(f),
				NewBinaryInternalErrorResponse(f),
			} {
				assert.Equal(t, BINARY, resp.Format(), "%d", idx)
				assert.NoError(t, resp.Validate())
			}
		})
		t.Run(fmt.Sprintf("JSONConstructorValidCase%d", idx), func(t *testing.T) {
			for idx, resp := range []Responder{
				NewJSONResponse(f),
				NewJSONErrorResponse(f),
				NewJSONInternalErrorResponse(f),
				MakeJSONErrorResponder(err),
				MakeJSONErrorResponder(er),
				MakeJSONInternalErrorResponder(err),
			} {
				assert.Equal(t, JSON, resp.Format(), "%d", idx)
				assert.NoError(t, resp.Validate())
			}
		})
		t.Run(fmt.Sprintf("YAMLConstructorValid%d", idx), func(t *testing.T) {
			for idx, resp := range []Responder{
				NewYAMLResponse(f),
				NewYAMLErrorResponse(f),
				NewYAMLInternalErrorResponse(f),
				MakeYAMLErrorResponder(err),
				MakeYAMLInternalErrorResponder(err),
				MakeYAMLErrorResponder(er),
			} {
				assert.Equal(t, YAML, resp.Format(), "%d", idx)
				assert.NoError(t, resp.Validate())
			}
		})
	}
	t.Run("ErrorConstructorGeneric", func(t *testing.T) {
		er := ErrorResponse{418, "coffee"}
		for idx, resp := range []Responder{
			MakeTextErrorResponder(er),
			MakeJSONErrorResponder(er),
			MakeYAMLErrorResponder(er),
			MakeJSONInternalErrorResponder(er),
			MakeYAMLInternalErrorResponder(er),
			MakeTextInternalErrorResponder(er),
		} {
			assert.Equal(t, resp.Status(), 418, "%d", idx)
			assert.NoError(t, resp.Validate())
		}
	})
}
