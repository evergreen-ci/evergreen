package shrub

import (
	"errors"
	"fmt"
	"testing"
)

type panicContent struct {
	msg string
}

func (p panicContent) String() string { return p.msg }

func TestSafeBuilder(t *testing.T) {
	msgs := []interface{}{
		"foo",
		errors.New("foo"),
		panicContent{"foo"},
		struct{ msg string }{msg: "foo"},
	}

	for _, val := range msgs {
		t.Run(fmt.Sprintf("%T", val), func(t *testing.T) {
			out, err := BuildConfiguration(func(c *Configuration) {
				panic(val)
			})
			assert(t, err != nil)
			assert(t, out == nil)
		})

	}

}
