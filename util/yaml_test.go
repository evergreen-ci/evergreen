package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalYAMLStrictWithFallback(t *testing.T) {
	smallYml := `
top_level:
    field: first
    field: duplicated
`

	type TinyStruct struct {
		Something string `yaml:"something"`
	}
	type SmallStruct struct {
		TopLevel   map[string]string `yaml:"top_level"`
		SomeList   []string          `yaml:"some_list"`
		TinyPieces []TinyStruct      `yaml:"tiny_pieces"`
	}
	// duplicate map items should error
	var myStruct SmallStruct
	err := UnmarshalYAMLStrictWithFallback([]byte(smallYml), &myStruct)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")

	// duplicate struct items should error
	smallYml = `
top_level:
    field: yee
top_level:
    field: ohno
`
	err = UnmarshalYAMLStrictWithFallback([]byte(smallYml), &myStruct)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")

	// duplicate lists should error
	smallYml = `
some_list:
  - my item
some_list:
  - my other item
`
	err = UnmarshalYAMLStrictWithFallback([]byte(smallYml), &myStruct)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")

	// nested duplicate lists should error
	type LargerStruct struct {
		Pieces []SmallStruct `yaml:"pieces"`
	}
	var largeStruct LargerStruct
	smallYml = `
pieces:
- tiny_pieces:
  - something: old
  tiny_pieces:
  - something: blue
`
	err = UnmarshalYAMLStrictWithFallback([]byte(smallYml), &largeStruct)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")

}
