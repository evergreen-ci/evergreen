package slogger

import (
	"errors"
	"strings"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
)

func TestLoggerLogf(t *testing.T) {
	assert := assert.New(t) // nolint
	sink, err := send.NewInternalLogger("sink", send.LevelInfo{Default: level.Info, Threshold: level.Info})
	assert.NoError(err)
	defer sink.Close()
	logger := &Logger{Name: "sloggerTest", Appenders: []send.Sender{sink}}

	assert.NoError(err)

	l, errs := logger.Logf(INFO, "foo %s", "bar")
	assert.True(len(errs) == 0)
	assert.NotNil(l)
	assert.Equal(l, sink.GetMessage().Message)
}

func TestLoggerErrorf(t *testing.T) {
	assert := assert.New(t) // nolint
	sink, err := send.NewInternalLogger("sink", send.LevelInfo{Default: level.Info, Threshold: level.Info})
	assert.NoError(err)
	defer sink.Close()
	logger := &Logger{Name: "sloggerTest", Appenders: []send.Sender{sink}}

	err = logger.Errorf(INFO, "foo %s", "bar")
	assert.Error(err)
	assert.Equal("foo bar", err.Error())
	assert.True(strings.Contains(sink.GetMessage().Rendered, "foo bar"))
}

func TestLoggerStackf(t *testing.T) {
	assert := assert.New(t) // nolint
	sink, err := send.NewInternalLogger("sink", send.LevelInfo{Default: level.Info, Threshold: level.Info})
	assert.NoError(err)
	defer sink.Close()
	logger := &Logger{Name: "sloggerTest", Appenders: []send.Sender{sink}}

	assert.NoError(err)

	l, errs := logger.Stackf(INFO, errors.New("baz"), "foo %s", "bar")
	assert.True(len(errs) == 0)
	assert.NotNil(l)

	msg := sink.GetMessage().Rendered
	assert.True(strings.HasSuffix(msg, "foo bar: baz"), msg)

}
