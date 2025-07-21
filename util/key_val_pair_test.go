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
		"key3": {
			"key4": "value4",
		},
	}
}

func (s *KvPairSuite) TestKvSliceToMap() {
	out, err := s.kvSlice.Map()
	s.NoError(err)
	s.Equal(s.testMap, out)
}

func (s *KvPairSuite) TestKvSliceToMapNested() {
	out, err := s.kvSliceNested.NestedMap()
	s.NoError(err)
	s.Equal(s.testMapNested, out)
}

func (s *KvPairSuite) TestMapToKvSlice() {
	out := MakeKeyValuePair(s.testMap)
	matches := 0
	for _, testPair := range out {
		for _, refPair := range s.kvSlice {
			if testPair.Key == refPair.Key && testPair.Value == refPair.Value {
				matches++
			}
		}
	}
	s.Equal(2, matches)
}

func (s *KvPairSuite) TestMapToKvSliceNested() {
	out := MakeNestedKeyValuePair(s.testMapNested)
	pair1 := out[0]
	s.Equal("key3", pair1.Key)
	s.EqualValues(s.kvSliceNested[0].Value, pair1.Value)

	nestedMap, err := out.NestedMap()
	s.NoError(err)
	s.EqualValues(s.testMapNested, nestedMap)
}

func (s *KvPairSuite) TestErrorsForInvalidInput() {
	invalidSlice := KeyValuePairSlice{
		{Key: "foo", Value: true},
	}
	_, err := invalidSlice.Map()
	s.Error(err)
	_, err = invalidSlice.NestedMap()
	s.Error(err)

	tooMuchNesting := KeyValuePairSlice{
		{Key: "level1", Value: KeyValuePairSlice{
			{Key: "level2", Value: KeyValuePairSlice{
				{Key: "level3", Value: "bar"},
			}},
		}},
	}
	_, err = tooMuchNesting.Map()
	s.Error(err)
	_, err = tooMuchNesting.NestedMap()
	s.Error(err)
}
