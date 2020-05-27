package rehttp

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aybabtme/iocontrol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

func assertNetTimeoutErr(t *testing.T, err error) {
	if assert.NotNil(t, err) {
		nerr, ok := err.(net.Error)
		require.True(t, ok)
		assert.True(t, nerr.Timeout())
		t.Logf("%#v", err)
	}
}

func assertURLTimeoutErr(t *testing.T, err error) {
	if assert.NotNil(t, err) {
		uerr, ok := err.(*url.Error)
		require.True(t, ok)
		nerr, ok := uerr.Err.(net.Error)
		require.True(t, ok)
		assert.True(t, nerr.Timeout())
		t.Logf("%#v", nerr)
	}
}

func TestContextCancelOnRetry(t *testing.T) {
	callCnt := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cnt := atomic.AddInt32(&callCnt, 1)
		switch cnt {
		case 1:
			w.WriteHeader(500)
		default:
			time.Sleep(2 * time.Second)
			fmt.Fprint(w, r.URL.Path)
		}
	}))
	defer srv.Close()

	// cancel while waiting on retry response
	tr := NewTransport(nil, RetryAll(RetryMaxRetries(1), RetryStatusInterval(500, 600)), ConstDelay(0))
	c := &http.Client{
		Transport: tr,
	}

	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancelFn()

	res, err := ctxhttp.Get(ctx, c, srv.URL+"/test")
	require.Nil(t, res)

	assert.Equal(t, context.DeadlineExceeded, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCnt))

	// cancel while waiting on delay
	atomic.StoreInt32(&callCnt, 0)
	tr = NewTransport(nil, RetryAll(RetryMaxRetries(1), RetryStatusInterval(500, 600)), ConstDelay(2*time.Second))
	c = &http.Client{
		Transport: tr,
	}

	ctx, cancelFn = context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancelFn()

	res, err = ctxhttp.Get(ctx, c, srv.URL+"/test")
	require.Nil(t, res)

	assert.Equal(t, context.DeadlineExceeded, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCnt))
}

func TestContextCancel(t *testing.T) {
	// server that doesn't reply before the timeout
	wg := sync.WaitGroup{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, r.URL.Path)
		wg.Done()
	}))
	defer srv.Close()

	tr := NewTransport(nil, RetryAll(RetryMaxRetries(1), RetryStatusInterval(500, 600)), ConstDelay(0))
	c := &http.Client{
		Transport: tr,
	}

	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancelFn()

	wg.Add(1)
	res, err := ctxhttp.Get(ctx, c, srv.URL+"/test")
	require.Nil(t, res)

	assert.Equal(t, context.DeadlineExceeded, err)
	wg.Wait()
}

func TestClientTimeoutSlowBody(t *testing.T) {
	// server that flushes the headers ASAP, but sends the body slowly
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		tw := iocontrol.ThrottledWriter(w, 2, time.Second)
		fmt.Fprint(tw, r.URL.Path)
	}))
	defer srv.Close()

	runWithClient := func(c *http.Client) {
		res, err := c.Get(srv.URL + "/testing")

		// should receive a response
		require.Nil(t, err)
		require.NotNil(t, res)

		// should fail with timeout while reading body
		_, err = io.Copy(ioutil.Discard, res.Body)
		res.Body.Close()
		assertNetTimeoutErr(t, err)
	}

	// test with retry transport
	tr := NewTransport(nil, RetryAll(RetryMaxRetries(2), RetryTemporaryErr()), ConstDelay(time.Second))

	c := &http.Client{
		Transport: tr,
		Timeout:   time.Second,
	}
	runWithClient(c)

	// test with default transport, make sure it behaves the same way
	c = &http.Client{Timeout: time.Second}
	runWithClient(c)
}

func TestClientTimeoutOnRetry(t *testing.T) {
	// server returns 500 on first call, sleeps 2s before reply on other calls
	callCnt := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cnt := atomic.AddInt32(&callCnt, 1)
		switch cnt {
		case 1:
			w.WriteHeader(500)
		default:
			time.Sleep(2 * time.Second)
			fmt.Fprint(w, r.URL.Path)
		}
	}))
	defer srv.Close()

	// timeout while waiting for retry request
	tr := NewTransport(nil, RetryAll(RetryMaxRetries(1), RetryStatusInterval(500, 600)), ConstDelay(0))
	c := &http.Client{
		Transport: tr,
		Timeout:   time.Second,
	}
	res, err := c.Get(srv.URL + "/test")
	require.Nil(t, res)
	assertURLTimeoutErr(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCnt))

	atomic.StoreInt32(&callCnt, 0)

	// timeout while waiting for retry delay
	tr = NewTransport(nil, RetryAll(RetryMaxRetries(1), RetryStatusInterval(500, 600)), ConstDelay(2*time.Second))
	c = &http.Client{
		Transport: tr,
		Timeout:   time.Second,
	}
	res, err = c.Get(srv.URL + "/test")
	require.Nil(t, res)
	assertURLTimeoutErr(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCnt))
}

func TestClientTimeout(t *testing.T) {
	// server that doesn't reply before the timeout
	wg := sync.WaitGroup{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, r.URL.Path)
		wg.Done()
	}))
	defer srv.Close()

	// test with retry transport
	tr := NewTransport(nil, RetryAll(RetryMaxRetries(2), RetryTemporaryErr()), ConstDelay(time.Second))
	c := &http.Client{
		Transport: tr,
		Timeout:   time.Second,
	}
	wg.Add(1)
	res, err := c.Get(srv.URL + "/test")
	require.Nil(t, res)
	assertURLTimeoutErr(t, err)

	// test with default transport, make sure it returns the same error
	c = &http.Client{Timeout: time.Second}
	wg.Add(1)
	res, err = c.Get(srv.URL + "/test")
	require.Nil(t, res)
	assertURLTimeoutErr(t, err)

	wg.Wait()
}

func TestTransportTimeout(t *testing.T) {
	// server that doesn't reply before the timeout
	wg := sync.WaitGroup{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, r.URL.Path)
		wg.Done()
	}))
	defer srv.Close()

	// test with retry transport
	httpTr := &http.Transport{
		ResponseHeaderTimeout: time.Second,
	}
	tr := NewTransport(httpTr, RetryAll(RetryMaxRetries(1), RetryTemporaryErr()), ConstDelay(time.Second))
	c := &http.Client{Transport: tr}
	// ResponseHeaderTimeout causes a TemporaryErr, so it will retry once
	wg.Add(2)
	res, err := c.Get(srv.URL + "/test")
	require.Nil(t, res)
	assertURLTimeoutErr(t, err)

	// test with HTTP transport, make sure it returns the same error
	c = &http.Client{Transport: httpTr}
	wg.Add(1)
	res, err = c.Get(srv.URL + "/test")
	require.Nil(t, res)
	assertURLTimeoutErr(t, err)

	wg.Wait()
}

func TestClientNoRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.Path)
	}))
	defer srv.Close()

	tr := NewTransport(nil, RetryAll(RetryMaxRetries(2), RetryTemporaryErr()), ConstDelay(time.Second))

	c := &http.Client{
		Transport: tr,
	}
	res, err := c.Get(srv.URL + "/test")
	require.Nil(t, err)
	defer res.Body.Close()

	assert.Equal(t, 200, res.StatusCode)
	var buf bytes.Buffer
	_, err = io.Copy(&buf, res.Body)
	require.Nil(t, err)
	assert.Equal(t, "/test", buf.String())
}

func TestClientRetryWithBody(t *testing.T) {
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(500)
			fail = false
			return
		}

		io.Copy(w, r.Body)
	}))
	defer srv.Close()

	tr := NewTransport(nil, RetryAll(RetryMaxRetries(1), RetryStatuses(500)), ConstDelay(0))

	c := &http.Client{
		Transport: tr,
	}
	res, err := c.Post(srv.URL+"/test", "", strings.NewReader("body"))
	require.Nil(t, err)
	defer res.Body.Close()

	assert.Equal(t, 200, res.StatusCode)
	var buf bytes.Buffer
	_, err = io.Copy(&buf, res.Body)
	require.Nil(t, err)
	assert.Equal(t, "body", buf.String())
	assert.False(t, fail)
}

func TestClientRetryWithHeaders(t *testing.T) {
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(500)
			fail = false
			return
		}

		for k, v := range r.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tr := NewTransport(nil, RetryAll(RetryMaxRetries(1), RetryStatuses(500)), ConstDelay(0))

	c := &http.Client{
		Transport: tr,
	}
	req, err := http.NewRequest("GET", srv.URL+"/test", nil)
	require.NoError(t, err)
	req.Header.Set("X-1", "a")
	req.Header.Set("X-2", "b")

	res, err := c.Do(req)
	require.Nil(t, err)
	defer res.Body.Close()

	assert.Equal(t, 200, res.StatusCode)
	assert.Equal(t, "a", res.Header.Get("X-1"))
	assert.Equal(t, "b", res.Header.Get("X-2"))
	assert.False(t, fail)
}
