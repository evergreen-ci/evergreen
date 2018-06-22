package uglytests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestTestifyPass(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	assert.True(true)
}

func TestTestifyFail(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	assert.True(false)
}

func TestConveyPass(t *testing.T) {
	t.Parallel()
	Convey("asinine test", t, func() {
		So(true, ShouldEqual, true)
	})
}

func TestConveyFail(t *testing.T) {
	t.Parallel()
	Convey("asinine test", t, func() {
		So(false, ShouldEqual, true)
	})
}

func TestNativeTestPass(t *testing.T) {
	t.Parallel()
	if true != true {
		t.Fail()
		t.Error("true is not equal to true")
	}
}

func TestNativeTestFail(t *testing.T) {
	t.Parallel()
	if true != false {
		t.Fail()
		t.Error("true is not equal to false")
	}
}
