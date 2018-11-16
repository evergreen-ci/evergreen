package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	yaml "gopkg.in/yaml.v2"
)

type StringOrBoolSuite struct {
	f stringOrBoolContainer
	suite.Suite
}

type stringOrBoolContainer struct {
	Foo StringOrBool `json:"foo" yaml:"foo"`
}

func TestStringOrBoolSuite(t *testing.T) {
	suite.Run(t, new(StringOrBoolSuite))
}

func (s *StringOrBoolSuite) SetupTest() {
	s.f = stringOrBoolContainer{}
}

func (s *StringOrBoolSuite) TestBool() {
	var (
		val bool
		err error
	)

	for _, v := range []StringOrBool{"", "false", "False", "0", "F", "f", "no", "n"} {
		val, err = v.Bool()
		s.False(val)
		s.NoError(err)
	}

	for _, v := range []StringOrBool{"true", "True", "1", "T", "t", "yes", "y"} {
		val, err = v.Bool()
		s.True(val)
		s.NoError(err)
	}

	for _, v := range []StringOrBool{"NOPE", "NONE", "EMPTY", "01", "100"} {
		val, err = v.Bool()
		s.False(val)
		s.Error(err)

	}

	// make sure the nil form works too
	var vptr *StringOrBool
	s.Nil(vptr)

	s.NotPanics(func() {
		val, err = vptr.Bool()
	})
	s.False(val)
	s.NoError(err)

}

func (s *StringOrBoolSuite) TestUnmarshalJSON() {
	j := []byte("{\"foo\": \"true\"}")
	err := json.Unmarshal(j, &s.f)
	s.NoError(err)
	s.Equal(StringOrBool("true"), s.f.Foo)
}

func (s *StringOrBoolSuite) TestMarshalJSON() {
	s.f = stringOrBoolContainer{Foo: "true"}
	out, err := json.Marshal(&s.f)
	s.NoError(err)
	s.Equal([]byte("{\"foo\":\"true\"}"), out)
}

func (s *StringOrBoolSuite) TestUnmarshalYAML() {
	y := []byte("foo: \"true\"")
	err := yaml.Unmarshal(y, &s.f)
	s.NoError(err)
	s.Equal(StringOrBool("true"), s.f.Foo)
}

func (s *StringOrBoolSuite) TestMarshalYAML() {
	s.f = stringOrBoolContainer{Foo: "true"}
	out, err := yaml.Marshal(&s.f)
	s.NoError(err)
	s.Equal([]byte("foo: \"true\"\n"), out)
}
