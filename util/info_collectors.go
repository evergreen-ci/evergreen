package util

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"golang.org/x/net/context"
)

// SystemInfoCollector is meant to run in a goroutine and log
// aggregate system resource utilization (cpu, memory, network, i/o)
// every 15 seconds. The information is logged to process' default
// grip logger.
//
// In general, the collector should run in the background of the API
// server and the UI server.
func SystemInfoCollector(ctx context.Context) {
	defer RecoverLogStackTraceAndContinue("system info collector")
	const sysInfoLoggingInterval = 15 * time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			grip.Info("system logging operation canceled")
			return
		case <-timer.C:
			grip.Info(message.CollectSystemInfo())
			grip.Info(message.CollectGoStats())
			timer.Reset(sysInfoLoggingInterval)
		}
	}
}
