package util

import (
	"reflect"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestRecursivelySetUndefinedFields(t *testing.T) {
	assert := assert.New(t)

	type inner struct {
		Foo string
	}

	type deep struct {
		Inner *inner
	}

	type shape struct {
		A string
		B *int
		C bool
		D *deep
	}

	a := shape{
		A: "a",
		B: utility.ToIntPtr(1),
		C: true,
		D: &deep{
			Inner: &inner{
				Foo: "foo",
			},
		},
	}
	b := shape{
		A: "b",
		D: &deep{
			Inner: &inner{
				Foo: "bar",
			},
		},
	}

	aPtr := &a
	bPtr := &b

	reflectedA := reflect.ValueOf(aPtr).Elem()
	reflectedB := reflect.ValueOf(bPtr).Elem()
	RecursivelySetUndefinedFields(reflectedB, reflectedA)
	assert.Equal("b", bPtr.A)
	assert.Equal(1, utility.FromIntPtr(bPtr.B))
	assert.Equal(true, bPtr.C)
	assert.Equal("bar", bPtr.D.Inner.Foo)

	c := shape{
		A: "c",
	}
	cPtr := &c
	reflectedC := reflect.ValueOf(cPtr).Elem()
	RecursivelySetUndefinedFields(reflectedC, reflectedA)
	assert.Equal("c", cPtr.A)
	assert.Equal(1, utility.FromIntPtr(cPtr.B))
	assert.Equal(true, cPtr.C)
	assert.Equal("foo", cPtr.D.Inner.Foo)

}
