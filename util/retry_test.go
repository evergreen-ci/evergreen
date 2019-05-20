package util

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

const TestRetries = 5
const TestSleep = 10 * time.Millisecond
const TriesTillPass = 2

func TestRetriesUsedUp(t *testing.T) {
	Convey("When retrying a function that never succeeds", t, func() {

		failingFunc := func() (bool, error) {
			return true, errors.New("something went wrong")
		}

		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := Retry(ctx, failingFunc, TestRetries, TestSleep, 0)
		end := time.Now()

		Convey("calling it with Retry should return an error", func() {
			So(err, ShouldNotBeNil)
		})
		Convey("Time spent doing Retry() should be total time sleeping", func() {
			So(end, ShouldHappenOnOrAfter, start.Add((TestRetries-1)*TestSleep))
		})
	})
}

func TestRetryUntilSuccess(t *testing.T) {
	Convey("When retrying a function that succeeds after 3 tries", t, func() {

		tryCounter := TriesTillPass
		retryPassingFunc := func() (bool, error) {
			tryCounter--
			if tryCounter <= 0 {
				return false, nil
			}
			return true, errors.New("something went wrong")
		}

		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := Retry(ctx, retryPassingFunc, TestRetries, TestSleep, 0)
		end := time.Now()

		Convey("calling it with Retry should not return any error", func() {
			So(err, ShouldBeNil)
		})
		Convey("time spent should be retry sleep * attempts needed to pass", func() {
			backoff := getBackoff(TestRetries, TestSleep, 0)

			So(end, ShouldHappenOnOrAfter, start.Add((TriesTillPass-1)*TestSleep))
			So(end, ShouldHappenBefore, start.Add(backoff.Max))
		})

	})
}

func TestNonRetriableFailure(t *testing.T) {
	Convey("When retrying a func that returns non-retriable err", t, func() {
		failingFuncNoRetry := func() (bool, error) {
			return false, errors.New("something went wrong")
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := Retry(ctx, failingFuncNoRetry, TestRetries, TestSleep, 0)

		Convey("calling it with Retry should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

type mockTransport struct {
	count         int
	expectedToken string
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.count++

	resp := http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       ioutil.NopCloser(strings.NewReader("hi")),
	}

	token := req.Header.Get("Authorization")
	split := strings.Split(token, " ")
	if len(split) != 2 || split[0] != "Bearer" || split[1] != t.expectedToken {
		resp.StatusCode = http.StatusForbidden
	}
	return &resp, nil
}

func TestRetryableOauthClient(t *testing.T) {
	assert := assert.New(t)
	c, err := GetRetryableOauth2HTTPClient("hi", rehttp.RetryMaxRetries(4),
		RehttpDelay(time.Nanosecond, 5))
	assert.NoError(err)
	defer PutHTTPClient(c)

	transport := &mockTransport{expectedToken: "hi"}
	oldTransport := c.Transport.(*rehttp.Transport).RoundTripper.(*oauth2.Transport).Base
	defer func() {
		c.Transport.(*rehttp.Transport).RoundTripper.(*oauth2.Transport).Base = oldTransport
	}()
	c.Transport.(*rehttp.Transport).RoundTripper.(*oauth2.Transport).Base = transport

	resp, err := c.Get("https://example.com")
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Equal(5, transport.count)
	assert.Equal(http.StatusOK, resp.StatusCode)
}

func TestRetryableOauthClient4xxDoesntRetry(t *testing.T) {
	assert := assert.New(t)

	c, err := GetRetryableOauth2HTTPClient("token something", rehttp.RetryAll(rehttp.RetryTemporaryErr(), rehttp.RetryMaxRetries(4)),
		RehttpDelay(time.Nanosecond, 5))
	assert.NoError(err)
	defer PutHTTPClient(c)

	transport := &mockTransport{expectedToken: "nope"}
	oldTransport := c.Transport.(*rehttp.Transport).RoundTripper.(*oauth2.Transport).Base
	defer func() {
		c.Transport.(*rehttp.Transport).RoundTripper.(*oauth2.Transport).Base = oldTransport
	}()
	c.Transport.(*rehttp.Transport).RoundTripper.(*oauth2.Transport).Base = transport

	resp, err := c.Get("https://example.com")
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Equal(1, transport.count)
	assert.Equal(http.StatusForbidden, resp.StatusCode)
}

func TestRetryDoesNotPauseBeforeFirstTry(t *testing.T) {
	now := time.Now()
	require.NoError(t, Retry(context.Background(), func() (bool, error) { return false, nil }, 10, time.Second, time.Minute))
	require.True(t, time.Since(now) < time.Millisecond)
}
