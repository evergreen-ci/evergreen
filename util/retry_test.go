package util

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
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

func TestRetryDoesNotPauseBeforeFirstTry(t *testing.T) {
	now := time.Now()
	require.NoError(t, Retry(context.Background(), func() (bool, error) { return false, nil }, 10, time.Second, time.Minute))
	require.True(t, time.Since(now) < time.Millisecond)
}
