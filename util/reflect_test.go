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
	assert.True(bPtr.C)
	assert.Equal("bar", bPtr.D.Inner.Foo)

	c := shape{
		A: "c",
	}
	cPtr := &c
	reflectedC := reflect.ValueOf(cPtr).Elem()
	RecursivelySetUndefinedFields(reflectedC, reflectedA)
	assert.Equal("c", cPtr.A)
	assert.Equal(1, utility.FromIntPtr(cPtr.B))
	assert.True(cPtr.C)
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
	assert.False(ePtr.C)
	assert.Equal("foo", ePtr.D.Inner.Foo)
}

func TestIsFieldUndefinedSlicesAndMaps(t *testing.T) {
	t.Run("NilSlice", func(t *testing.T) {
		var s []string
		assert.True(t, IsFieldUndefined(reflect.ValueOf(&s).Elem()))
	})
	t.Run("EmptySlice", func(t *testing.T) {
		s := []string{}
		assert.True(t, IsFieldUndefined(reflect.ValueOf(&s).Elem()))
	})
	t.Run("NonEmptySlice", func(t *testing.T) {
		s := []string{"a"}
		assert.False(t, IsFieldUndefined(reflect.ValueOf(&s).Elem()))
	})
	t.Run("NilMap", func(t *testing.T) {
		var m map[string]string
		assert.True(t, IsFieldUndefined(reflect.ValueOf(&m).Elem()))
	})
	t.Run("EmptyMap", func(t *testing.T) {
		m := map[string]string{}
		assert.True(t, IsFieldUndefined(reflect.ValueOf(&m).Elem()))
	})
	t.Run("NonEmptyMap", func(t *testing.T) {
		m := map[string]string{"k": "v"}
		assert.False(t, IsFieldUndefined(reflect.ValueOf(&m).Elem()))
	})
}

func TestRecursivelySetUndefinedFieldsEmptySlice(t *testing.T) {
	type config struct {
		Tags     []string
		Labels   map[string]string
		Name     string
	}

	t.Run("EmptySliceOverriddenByDefault", func(t *testing.T) {
		target := config{Tags: []string{}, Name: "target"}
		defaults := config{Tags: []string{"default-tag"}, Labels: map[string]string{"env": "prod"}}

		tVal := reflect.ValueOf(&target).Elem()
		dVal := reflect.ValueOf(&defaults).Elem()
		RecursivelySetUndefinedFields(tVal, dVal)

		assert.Equal(t, []string{"default-tag"}, target.Tags)
		assert.Equal(t, map[string]string{"env": "prod"}, target.Labels)
		assert.Equal(t, "target", target.Name)
	})

	t.Run("NilSliceOverriddenByDefault", func(t *testing.T) {
		target := config{Name: "target"}
		defaults := config{Tags: []string{"default-tag"}}

		tVal := reflect.ValueOf(&target).Elem()
		dVal := reflect.ValueOf(&defaults).Elem()
		RecursivelySetUndefinedFields(tVal, dVal)

		assert.Equal(t, []string{"default-tag"}, target.Tags)
		assert.Equal(t, "target", target.Name)
	})

	t.Run("NonEmptySliceNotOverridden", func(t *testing.T) {
		target := config{Tags: []string{"my-tag"}, Name: "target"}
		defaults := config{Tags: []string{"default-tag"}}

		tVal := reflect.ValueOf(&target).Elem()
		dVal := reflect.ValueOf(&defaults).Elem()
		RecursivelySetUndefinedFields(tVal, dVal)

		assert.Equal(t, []string{"my-tag"}, target.Tags)
		assert.Equal(t, "target", target.Name)
	})

	t.Run("EmptyMapOverriddenByDefault", func(t *testing.T) {
		target := config{Labels: map[string]string{}}
		defaults := config{Labels: map[string]string{"env": "prod"}}

		tVal := reflect.ValueOf(&target).Elem()
		dVal := reflect.ValueOf(&defaults).Elem()
		RecursivelySetUndefinedFields(tVal, dVal)

		assert.Equal(t, map[string]string{"env": "prod"}, target.Labels)
	})

	t.Run("NonEmptyMapNotOverridden", func(t *testing.T) {
		target := config{Labels: map[string]string{"env": "staging"}}
		defaults := config{Labels: map[string]string{"env": "prod"}}

		tVal := reflect.ValueOf(&target).Elem()
		dVal := reflect.ValueOf(&defaults).Elem()
		RecursivelySetUndefinedFields(tVal, dVal)

		assert.Equal(t, map[string]string{"env": "staging"}, target.Labels)
	})
}
