package main

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestTestifyPass(t *testing.T) {
	t.Parallel()
	t.Log("start")
	assert := assert.New(t)
	assert.True(true)
	time.Sleep(10 * time.Second)
	t.Log("end")
}

func TestTestifyFail(t *testing.T) {
	t.Parallel()
	t.Log("start")
	assert := assert.New(t)
	assert.True(false)
	time.Sleep(10 * time.Second)
	t.Log("end")
}

func TestConveyPass(t *testing.T) {
	t.Parallel()
	t.Log("start")
	Convey("asinine test", t, func() {
		So(true, ShouldEqual, true)
	})
	time.Sleep(10 * time.Second)
	t.Log("end")
}

func TestConveyFail(t *testing.T) {
	t.Parallel()
	t.Log("start")
	Convey("asinine test", t, func() {
		So(false, ShouldEqual, true)
	})
	time.Sleep(10 * time.Second)
	t.Log("end")
}

func TestNativeTestPass(t *testing.T) {
	t.Parallel()
	t.Log("start")
	if true != true {
		t.Fail()
		t.Error("true is not equal to true")
	}
	time.Sleep(10 * time.Second)
	t.Log("end")
}

func TestNativeTestFail(t *testing.T) {
	t.Parallel()
	t.Log("start")
	if true != false {
		t.Fail()
		t.Error("true is not equal to false")
		t.Fail()
	}
	time.Sleep(10 * time.Second)
	t.Log("end")
}

func TestSkippedTestFail(t *testing.T) {
	t.Parallel()
	t.Log("start")
	time.Sleep(10 * time.Second)
	t.Skip("skipping because reasons")
	t.FailNow()
	t.Log("end")
}

type testifySuite struct {
	pass bool
	suite.Suite
}

func (s *testifySuite) TestThings() {
	s.T().Log("start")
	s.True(s.pass)
	time.Sleep(10 * time.Second)
	s.T().Log("end")
}

func TestTestifySuiteFail(t *testing.T) {
	suite.Run(t, &testifySuite{pass: false})
}

func TestTestifySuite(t *testing.T) {
	suite.Run(t, &testifySuite{pass: true})
}
