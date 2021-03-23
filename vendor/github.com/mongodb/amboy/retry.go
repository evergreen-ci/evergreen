package amboy

// WithRetryableQueue is a convenience function to perform an operation if the
// Queue is a RetryableQueue; otherwise, it is a no-op. Returns whether ot not
// the queue was a RetryableQueue.
func WithRetryableQueue(q Queue, op func(RetryableQueue)) bool {
	rq, ok := q.(RetryableQueue)
	if !ok {
		return false
	}

	op(rq)

	return true
}
