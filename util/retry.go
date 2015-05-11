package util

import (
	"time"
)

// RetriableError can be returned by any function called with Retry(),
// to indicate that it should be retried again after a sleep interval.
type RetriableError struct {
	Failure error
}

// RetriableFunc is any function that takes no parameters and returns only
// an error interface. These functions can be used with util.Retry.
type RetriableFunc func() error

func (retriable RetriableError) Error() string {
	return retriable.Failure.Error()
}

// BackoffCalc is a custom function signature that can be implemented by
// any implementation that returns a sleep duration for backoffs
type BackoffCalc func(minSleep time.Duration, maxTries,
	triesLeft int) time.Duration

// linearBackoffCalc returns the duration to sleep (based on a linear
// progression) that takes into account the minimum sleep duration, the maximum
// number of iterations and the number of retry attempts left
func linearBackoffCalc(minSleep time.Duration, maxTries,
	triesLeft int) time.Duration {
	return minSleep
}

// arithmeticBackoffCalc returns the duration to sleep (based on a arithmetic
// progression) that takes into account the minimum sleep duration, the maximum
// number of iterations and the number of retry attempts left
func arithmeticBackoffCalc(minSleep time.Duration, maxTries,
	triesLeft int) time.Duration {
	sleepTime := minSleep.Nanoseconds() * int64(maxTries-triesLeft)
	return time.Duration(int(sleepTime)) * time.Nanosecond
}

// geometricBackoffCalc returns the duration to sleep (based on a geometric
// progression) that takes into account the minimum sleep duration, the maximum
// number of iterations and the number of retry attempts left
func geometricBackoffCalc(minSleep time.Duration, maxTries,
	triesLeft int) time.Duration {
	sleepMultiplier := minSleep.Nanoseconds() * int64(2)
	return time.Duration(int(sleepMultiplier)) * time.Nanosecond
}

// doRetry is a helper method that reries a given function with a sleep
// interval determined by the RetryType
func doRetry(backoffCalc BackoffCalc, attemptFunc RetriableFunc, maxTries int,
	sleep time.Duration) (bool, error) {
	triesLeft := maxTries
	for {
		err := attemptFunc()
		if err == nil {
			//the attempt succeeded, so we return no error
			return false, nil
		}
		triesLeft--

		if retriableErr, ok := err.(RetriableError); ok {
			if triesLeft <= 0 {
				// used up all retry attempts, so return the failure.
				return true, retriableErr.Failure
			} else {
				// it's safe to retry this, so sleep for a moment and try again
				time.Sleep(backoffCalc(sleep, maxTries, triesLeft))
			}
		} else {
			//function returned err but it can't be retried - fail immediately
			return false, err
		}
	}
}

// Retry will call attemptFunc up to maxTries until it returns nil,
// sleeping the specified amount of time between each call.
// The function can return an error to abort the retrying, or return
// RetriableError to allow the function to be called again.
func Retry(attemptFunc RetriableFunc, maxTries int,
	sleep time.Duration) (bool, error) {
	return doRetry(linearBackoffCalc, attemptFunc, maxTries, sleep)
}

// RetryArithmeticBackoff will call attemptFunc up to maxTries until it returns
// nil, leeping for an arithmetic progressed duration after each retry attempt.
// e.g. For if attemptFunc errors out immediately, the sleep duration for a call
// like RetryArithmeticBackoff(func, 3, 5) would be thus:
//
// 1st iteration: Sleep duration 5 seconds
// 2nd iteration: Sleep duration 10 seconds
// 3rd iteration: Sleep duration 15 seconds
// ...
// The function can return an error to abort the retrying, or return
// RetriableError to allow the function to be called again.
func RetryArithmeticBackoff(attemptFunc RetriableFunc, maxTries int,
	sleep time.Duration) (bool, error) {
	return doRetry(arithmeticBackoffCalc, attemptFunc, maxTries, sleep)
}

// RetryGeometricBackoff will call attemptFunc up to maxTries until it returns
// nil, leeping for an geometric progressed duration after each retry attempt.
// e.g. For if attemptFunc errors out immediately, the sleep duration for a call
// like RetryGeometricBackoff(func, 3, 5) would be thus:
//
// 1st iteration: Sleep duration 5 seconds
// 2nd iteration: Sleep duration 25 seconds
// 3rd iteration: Sleep duration 125 seconds
// ...
// The function can return an error to abort the retrying, or return
// RetriableError to allow the function to be called again.
func RetryGeometricBackoff(attemptFunc RetriableFunc, maxTries int,
	sleep time.Duration) (bool, error) {
	return doRetry(geometricBackoffCalc, attemptFunc, maxTries, sleep)
}
