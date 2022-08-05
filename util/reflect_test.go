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

	// Test Deep copy of structs
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

	// Test deep copy with zero and nil default values
	d := shape{
		A: "d",
		D: &deep{
			Inner: &inner{
				Foo: "foo",
			},
		},
	}
	dPtr := &d
	reflectedD := reflect.ValueOf(dPtr).Elem()

	e := shape{
		A: "e",
	}
	ePtr := &e
	reflectedE := reflect.ValueOf(ePtr).Elem()
	RecursivelySetUndefinedFields(reflectedE, reflectedD)
	assert.Equal("e", ePtr.A)
	assert.Nil(ePtr.B)
	assert.Equal(false, ePtr.C)
	assert.Equal("foo", ePtr.D.Inner.Foo)
}
