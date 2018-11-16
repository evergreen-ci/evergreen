package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	yaml "gopkg.in/yaml.v2"
)

type StringOrIntSuite struct {
	f stringOrIntContainer
	suite.Suite
}

type stringOrIntContainer struct {
	Foo StringOrInt `json:"foo" yaml:"foo"`
}

func TestStringOrIntSuite(t *testing.T) {
	suite.Run(t, new(StringOrIntSuite))
}

func (s *StringOrIntSuite) SetupTest() {
	s.f = stringOrIntContainer{}
}

func (s *StringOrIntSuite) TestInt() {
	var (
		i   int
		err error
	)

	for k, v := range map[StringOrInt]int{
		"-1000000": -1000000,
		"-1":       -1,
		"0":        0,
		"1":        1,
		"2":        2,
		"27017":    27017,
	} {
		i, err = k.Int()
		s.NoError(err)
		s.Equal(i, v)
	}
}

func (s *StringOrIntSuite) TestUnmarshalJSON() {
	j := []byte("{\"foo\": \"8\"}")
	err := json.Unmarshal(j, &s.f)
	s.NoError(err)
	s.Equal(StringOrInt("8"), s.f.Foo)
}

func (s *StringOrIntSuite) TestMarshalJSON() {
	s.f = stringOrIntContainer{Foo: "8"}
	out, err := json.Marshal(&s.f)
	s.NoError(err)
	s.Equal([]byte("{\"foo\":\"8\"}"), out)
}

func (s *StringOrIntSuite) TestUnmarshalYAML() {
	y := []byte("foo: \"8\"")
	err := yaml.Unmarshal(y, &s.f)
	s.NoError(err)
	s.Equal(StringOrInt("8"), s.f.Foo)
}

func (s *StringOrIntSuite) TestMarshalYAML() {
	s.f = stringOrIntContainer{Foo: "8"}
	out, err := yaml.Marshal(&s.f)
	s.NoError(err)
	s.Equal([]byte("foo: \"8\"\n"), out)
}
