package util

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
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
		retryFail, err := Retry(failingFunc, TestRetries, TestSleep)
		end := time.Now()

		Convey("calling it with Retry should return an error", func() {
			So(err, ShouldNotBeNil)
		})
		Convey("the 'retried till failure' flag should be true", func() {
			So(retryFail, ShouldBeTrue)
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
		retryFail, err := Retry(retryPassingFunc, TestRetries, TestSleep)
		end := time.Now()

		Convey("calling it with Retry should not return any error", func() {
			So(err, ShouldBeNil)
		})
		Convey("the 'retried till failure' flag should be false", func() {
			So(retryFail, ShouldBeFalse)
		})
		Convey("time spent should be retry sleep * attempts needed to pass", func() {
			backoff := getBackoff(TestSleep, TestRetries)

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

		retryFail, err := Retry(failingFuncNoRetry, TestRetries, TestSleep)

		Convey("calling it with Retry should return an error", func() {
			So(err, ShouldNotBeNil)
		})
		Convey("the 'retried till failure' flag should be false", func() {
			So(retryFail, ShouldBeFalse)
		})
	})
}

//func TestRehttpDelay(t *testing.T) {
// 	assert := assert.New(t)
//
// 	f := rehttpDelay(time.Second, 5)
// 	b := getBackoff(time.Second, 5)
//
// 	for i := 0; i < 5; i++ {
// 		attempt := rehttp.Attempt{
// 			Index: i,
// 		}
// 		assert.Equal(b.Duration(), f(attempt))
// 	}
// }

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
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestRetryableOauthClient")

	assert := assert.New(t)
	token, err := testConfig.GetGithubOauthToken()
	assert.NoError(err)

	c, err := GetRetryableHTTPClientForOauth2(token, func(attempt rehttp.Attempt) bool {
		if attempt.Index == 4 {
			return false
		}
		return true
	}, rehttpDelay(time.Nanosecond, 5))
	defer PutRetryableHTTPClientForOauth2(c)
	assert.NoError(err)

	split := strings.Split(token, " ")
	transport := &mockTransport{expectedToken: split[1]}
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
