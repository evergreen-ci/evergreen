package util

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecursivelySetUndefinedFields(t *testing.T) {
	assert := assert.New(t)

	type inner struct {
		Foo string
	}

	type shape struct {
		A string
		B int
		C bool
		D inner
	}

	a := shape{
		A: "a",
		B: 1,
		C: true,
		D: inner{
			Foo: "foo",
		},
	}
	b := shape{
		A: "b",
		D: inner{
			Foo: "bar",
		},
	}

	aPtr := &a
	bPtr := &b

	reflectedA := reflect.ValueOf(aPtr).Elem()
	reflectedB := reflect.ValueOf(bPtr).Elem()
	RecursivelySetUndefinedFields(reflectedB, reflectedA)
	assert.Equal("b", bPtr.A)
	assert.Equal(1, bPtr.B)
	assert.Equal(true, bPtr.C)
	assert.Equal("bar", bPtr.D.Foo)

}
