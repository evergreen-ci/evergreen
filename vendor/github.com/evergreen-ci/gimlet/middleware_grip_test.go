package gimlet

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/negroni"
)

func TestReqestLogger(t *testing.T) {
	assert := assert.New(t)

	sender, err := send.NewInternalLogger("test", grip.GetSender().Level())
	assert.NoError(err)
	middlewear := NewAppLogger().(*appLogging)
	middlewear.Journaler = logging.MakeGrip(sender)

	next := func(w http.ResponseWriter, r *http.Request) {
		middlewear.Journaler.Info("hello")
	}
	assert.False(sender.HasMessage())
	req := &http.Request{
		URL: &url.URL{},
	}
	rw := negroni.NewResponseWriter(nil)

	startAt := getNumber()
	middlewear.ServeHTTP(rw, req, next)
	assert.Equal(startAt+2, getNumber())
	assert.True(sender.HasMessage())
	assert.Equal(sender.Len(), 3)
}

func TestReqestPanicLogger(t *testing.T) {
	assert := assert.New(t)

	sender, err := send.NewInternalLogger("test", grip.GetSender().Level())
	assert.NoError(err)
	middlewear := NewRecoveryLogger(logging.MakeGrip(sender)).(*appRecoveryLogger)

	next := func(w http.ResponseWriter, r *http.Request) {
		middlewear.Journaler.Info("hello")
	}
	assert.False(sender.HasMessage())
	req := &http.Request{
		URL: &url.URL{},
	}
	rw := negroni.NewResponseWriter(nil)

	startAt := getNumber()
	middlewear.ServeHTTP(rw, req, next)
	assert.Equal(startAt+2, getNumber())
	assert.True(sender.HasMessage())
	assert.Equal(sender.Len(), 3)
}

func TestReqestPanicLoggerWithPanic(t *testing.T) {
	assert := assert.New(t)

	sender, err := send.NewInternalLogger("test", grip.GetSender().Level())
	assert.NoError(err)
	middlewear := NewRecoveryLogger(logging.MakeGrip(sender))

	next := func(w http.ResponseWriter, r *http.Request) {
		panic("oops")
	}
	assert.False(sender.HasMessage())
	req := &http.Request{
		URL:    &url.URL{},
		Header: http.Header{},
	}
	req = AddLoggingAnnotation(req, "annie", "hall")
	testrw := httptest.NewRecorder()
	rw := negroni.NewResponseWriter(testrw)

	startAt := getNumber()
	middlewear.ServeHTTP(rw, req, next)

	assert.Equal(startAt+2, getNumber())
	assert.True(sender.HasMessage())
	assert.Equal(sender.Len(), 2)

	m, ok := sender.GetMessageSafe()
	assert.True(ok)
	assert.NotNil(m)
	fields, ok := m.Message.Raw().(message.Fields)
	assert.True(ok)
	fmt.Println(fields)
}

func TestDefaultGripMiddlwareSetters(t *testing.T) {
	assert := assert.New(t)
	r := &http.Request{
		URL: &url.URL{Path: "foo"},
	}
	r = r.WithContext(context.Background())
	ctx := r.Context()

	var l grip.Journaler
	assert.NotPanics(func() { l = GetLogger(ctx) })
	assert.NotNil(l)
	assert.Equal(l.GetSender(), grip.GetSender())

	now := time.Now()
	logger := logging.MakeGrip(send.MakeInternalLogger())

	assert.NotEqual(logger, GetLogger(ctx))
	assert.Zero(getRequestStartAt(ctx))

	r = setupLogger(l, r)
	ctx = r.Context()

	assert.Equal(l, GetLogger(ctx))

	id := GetRequestID(ctx)

	assert.True(id > 0, "%d", id)
	assert.NotZero(getRequestStartAt(ctx))
	assert.True(now.Before(getRequestStartAt(ctx)))

}

func TestLoggingAnnotationRetreive(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	ctx = context.WithValue(ctx, loggingAnnotationsKey, logAnnotation{"k", "v"})

	la := getLogAnnotation(ctx)
	assert.Equal("k", la.key)
	assert.Equal("v", la.value)

	ctx = context.WithValue(ctx, loggingAnnotationsKey, 1)
	la = getLogAnnotation(ctx)
	assert.Nil(la)

	ctx = context.WithValue(ctx, loggingAnnotationsKey, &logAnnotation{"key", "val"})
	la = getLogAnnotation(ctx)
	assert.Equal("key", la.key)
	assert.Equal("val", la.value)
}

func TestLoggingAnnotation(t *testing.T) {
	assert := assert.New(t)

	sender, err := send.NewInternalLogger("test", grip.GetSender().Level())
	assert.NoError(err)
	middlewear := NewRecoveryLogger(logging.MakeGrip(sender))

	var called bool
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
	}

	assert.False(sender.HasMessage())
	req := &http.Request{
		URL:    &url.URL{},
		Header: http.Header{},
	}
	req = AddLoggingAnnotation(req, "annie", "hall")
	testrw := httptest.NewRecorder()
	rw := negroni.NewResponseWriter(testrw)

	startAt := getNumber()
	middlewear.ServeHTTP(rw, req, next)
	assert.True(called)

	assert.Equal(startAt+2, getNumber())
	assert.True(sender.HasMessage())
	assert.Equal(sender.Len(), 2)

	// first message is the started one
	m, ok := sender.GetMessageSafe()
	assert.True(ok)
	assert.NotNil(m)

	// get the second message for the completed
	m, ok = sender.GetMessageSafe()
	assert.True(ok)
	assert.NotNil(m)

	fields, ok := m.Message.Raw().(message.Fields)
	assert.True(ok)
	fmt.Println(fields)
	val, ok := fields["annie"]
	assert.True(ok)
	assert.Equal("hall", val)
}
