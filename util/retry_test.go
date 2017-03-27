package util

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

const TestRetries = 10
const TestSleep = 100 * time.Millisecond
const TriesTillPass = 5

func TestRetriesUsedUp(t *testing.T) {
	Convey("When retrying a function that never succeeds", t, func() {

		failingFunc := RetriableFunc(
			func() error {
				return RetriableError{errors.New("something went wrong!")}
			},
		)

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
		retryPassingFunc := RetriableFunc(
			func() error {
				tryCounter--
				if tryCounter <= 0 {
					return nil
				}
				return RetriableError{errors.New("something went wrong!")}
			},
		)

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
			So(end, ShouldHappenOnOrAfter, start.Add((TriesTillPass-1)*TestSleep))
			So(end, ShouldHappenBefore, start.Add((TestRetries-1)*TestSleep))
		})

	})
}

func TestNonRetriableFailure(t *testing.T) {
	Convey("When retrying a func that returns non-retriable err", t, func() {

		failingFuncNoRetry := RetriableFunc(
			func() error {
				return errors.New("something went wrong!")
			},
		)

		retryFail, err := Retry(failingFuncNoRetry, TestRetries, TestSleep)

		Convey("calling it with Retry should return an error", func() {
			So(err, ShouldNotBeNil)
		})
		Convey("the 'retried till failure' flag should be false", func() {
			So(retryFail, ShouldBeFalse)
		})
	})
}

func TestArithmethicRetryUntilSuccess(t *testing.T) {
	Convey("With arithmetic backoff when retrying a function that succeeds "+
		"after 3 tries", t, func() {

		tryCounter := TriesTillPass
		retryPassingFunc := RetriableFunc(
			func() error {
				tryCounter--
				if tryCounter <= 0 {
					return nil
				}
				return RetriableError{errors.New("something went wrong!")}
			},
		)

		start := time.Now()
		retryFail, err := RetryArithmeticBackoff(retryPassingFunc,
			TestRetries, TestSleep)
		end := time.Now()

		Convey("calling it with RetryArithmeticBackoff should not return "+
			"any error", func() {
			So(err, ShouldBeNil)
		})
		Convey("the 'retried till failure' flag should be false", func() {
			So(retryFail, ShouldBeFalse)
		})
		Convey("time spent should be combined arithmetic retry sleep * "+
			"attempts needed to pass", func() {
			sleepRetry := TestSleep * (TriesTillPass - 1)
			sleepDuration := time.Duration(sleepRetry)
			So(end, ShouldHappenOnOrAfter, start.Add(sleepDuration))
		})
	})
}

func TestGeometricRetryUntilSuccess(t *testing.T) {
	Convey("With geometric backoff when retrying a function that succeeds after 3 tries", t, func() {

		tryCounter := TriesTillPass
		retryPassingFunc := RetriableFunc(
			func() error {
				tryCounter--
				if tryCounter <= 0 {
					return nil
				}
				return RetriableError{errors.New("something went wrong!")}
			},
		)

		start := time.Now()
		retryFail, err := RetryGeometricBackoff(retryPassingFunc,
			TestRetries, TestSleep)
		end := time.Now()

		Convey("calling it with RetryGeometricBackoff should not return any error", func() {
			So(err, ShouldBeNil)
		})
		Convey("the 'retried till failure' flag should be false", func() {
			So(retryFail, ShouldBeFalse)
		})
		Convey("time spent should be geometric retry sleep * attempts needed to pass", func() {
			sleepDuration := TestSleep * 2 * 2 * 2 // 2^3
			So(end, ShouldHappenOnOrAfter, start.Add(sleepDuration))
		})
	})
}
