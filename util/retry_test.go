package util

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

const TestRetries = 5
const TestSleep = 10 * time.Millisecond
const TriesTillPass = 2

func TestRetriesUsedUp(t *testing.T) {
	Convey("When retrying a function that never succeeds", t, func() {

		failingFunc := func() error {
			return RetriableError{errors.New("something went wrong")}
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
		retryPassingFunc := func() error {
			tryCounter--
			if tryCounter <= 0 {
				return nil
			}
			return RetriableError{errors.New("something went wrong")}
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
		failingFuncNoRetry := func() error {
			return errors.New("something went wrong")
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
