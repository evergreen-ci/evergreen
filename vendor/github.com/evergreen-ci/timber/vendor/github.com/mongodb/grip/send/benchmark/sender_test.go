package send

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/grip"
)

const maxBenchTime = 1 * time.Minute

func wrapBenchmark(b *testing.B, bench benchCase) {
	for _, msgSize := range messageSizes() {
		b.Run(fmt.Sprintf("%dBytesPerMessage", msgSize), func(b *testing.B) {
			for _, msgCount := range messageCounts() {
				ctx, cancel := context.WithTimeout(context.Background(), maxBenchTime)
				defer cancel()
				b.Run(fmt.Sprintf("Send%dMessages", msgCount), wrapCase(ctx, b, msgSize, msgCount, bench))
			}
		})
	}
}

func wrapCase(ctx context.Context, b *testing.B, size int, numMsgs int, bench benchCase) func(b *testing.B) {
	return func(b *testing.B) {
		b.ResetTimer()
		err := bench(ctx, b, b.N, size, numMsgs)
		if err != nil {
			grip.Error(err)
		}
	}
}

func BenchmarkBufferedSender(b *testing.B) {
	wrapBenchmark(b, bufferedSenderCase)
}

func BenchmarkCallSiteFileLogger(b *testing.B) {
	wrapBenchmark(b, callSiteFileLoggerCase)
}

func BenchmarkFileLogger(b *testing.B) {
	wrapBenchmark(b, fileLoggerCase)
}

func BenchmarkInMemorySender(b *testing.B) {
	wrapBenchmark(b, inMemorySenderCase)
}

func BenchmarkJSONFileLogger(b *testing.B) {
	wrapBenchmark(b, jsonFileLoggerCase)
}

func BenchmarkStreamLogger(b *testing.B) {
	wrapBenchmark(b, streamLoggerCase)
}
