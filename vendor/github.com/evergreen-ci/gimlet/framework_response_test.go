package gimlet

import (
	"net/http"
	"testing"

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
	s.factory = func() Responder { return NewResponseBuilder() }
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
	s.Equal(1, s.resp.Data())

	// nil should error and not not rest error
	s.Error(s.resp.AddData(nil))
	s.Equal(1, s.resp.Data())
}

func (s *ResponderSuite) TestValidatior() {
	// this is a placeholder; these implementations should in the
	// future provide some validation, but this would require more
	// changes to the test.
	s.Nil(s.resp.Validate())
}

func (s *ResponderSuite) TestMutabilityOfData() {
	s.NoError(s.resp.AddData(1))
	s.Equal(1, s.resp.Data())

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
}
