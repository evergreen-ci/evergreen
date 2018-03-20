package util

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type KvPairSuite struct {
	kvSlice       KeyValuePairSlice
	kvSliceNested KeyValuePairSlice
	testMap       map[string]string
	testMapNested map[string]map[string]string
	suite.Suite
}

func TestKvPairSuite(t *testing.T) {
	suite.Run(t, new(KvPairSuite))
}

func (s *KvPairSuite) SetupSuite() {
	s.kvSlice = KeyValuePairSlice{
		{Key: "key1", Value: "value1"},
		{Key: "key2", Value: "value2"},
	}
	s.kvSliceNested = KeyValuePairSlice{
		{Key: "key3", Value: KeyValuePairSlice{
			{Key: "key4", Value: "value4"},
		}},
	}
	s.testMap = map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	s.testMapNested = map[string]map[string]string{
		"key3": map[string]string{
			"key4": "value4",
		},
	}
}

func (s *KvPairSuite) TestKvSliceToMap() {
	out, err := s.kvSlice.KvSliceToMap()
	s.NoError(err)
	s.Equal(s.testMap, out)
}

func (s *KvPairSuite) TestKvSliceToMapNested() {
	out, err := s.kvSliceNested.KvSliceToMapNested()
	s.NoError(err)
	s.Equal(s.testMapNested, out)
}

func (s *KvPairSuite) TestMapToKvSlice() {
	out := MapToKvSlice(s.testMap)
	s.EqualValues(s.kvSlice, out)
}

func (s *KvPairSuite) TestMapToKvSliceNested() {
	out := MapToKvSliceNested(s.testMapNested)
	pair1 := out[0]
	s.Equal("key3", pair1.Key)
	s.EqualValues(s.kvSliceNested[0].Value, pair1.Value)
}
