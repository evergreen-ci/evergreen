package agent

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	sysInfoCollectorInterval = 30 * time.Second
	procInfoInterval         = 10 * time.Second
)

// metricsCollector holds the functionality for running two system
// information (metrics) collecting go-routines. The structure holds a TaskCommunicator object
type metricsCollector struct {
	comm     client.Communicator
	taskData client.TaskData
}

// start validates the struct and launches two go routines.
func (c *metricsCollector) start(ctx context.Context) error {
	if c.taskData.ID == "" || c.taskData.Secret == "" {
		return errors.New("invalid or incomplete task metadata specified")
	}

	if c.comm == nil {
		return errors.New("no task communicator specified")
	}

	go c.sysInfoCollector(ctx, sysInfoCollectorInterval)
	go c.processInfoCollector(ctx, procInfoInterval)

	return nil
}

// processInfoCollector collects the process tree for the current
// process and all spawned processes on a specified interval and sends
// these data to the API server. The interval is controlled by the
// arguments to this method, which allow for collection at one interval
// for a number of iterations and a second interval for all subsequent
// iterations. The intention of these intervals is to collect data with a
// high granularity after beginning to run a task and with a lower
// granularity throughout the life of a commit.
func (c *metricsCollector) processInfoCollector(ctx context.Context, interval time.Duration) {
	grip.Info("starting process metrics collector")

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			grip.Info("process metrics collector terminated.")
			return
		case <-timer.C:
			msgs := message.CollectProcessInfoSelfWithChildren()
			grip.CatchNotice(c.comm.SendProcessInfo(ctx, c.taskData, convertProcInfo(msgs)))
			grip.DebugWhen(sometimes.Fifth(), msgs)

			timer.Reset(interval)
		}
	}
}

func convertProcInfo(messages []message.Composer) []*message.ProcessInfo {
	out := []*message.ProcessInfo{}
	for _, msg := range messages {
		proc, ok := msg.(*message.ProcessInfo)
		if !ok {
			continue
		}

		out = append(out, proc)
	}

	return out
}

// sysInfoCollector collects aggregated system stats on the specified
// interval as long as the metrics collector is running, and sends these
// data to the API server.
func (c *metricsCollector) sysInfoCollector(ctx context.Context, interval time.Duration) {
	grip.Info("starting system metrics collector")
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			grip.Info("system metrics collector terminated.")
			return
		case <-timer.C:
			msg := message.CollectSystemInfo()
			sysinfo, ok := msg.(*message.SystemInfo)
			if !ok {
				grip.Warning("not collecting sysinfo because of an issue in grip")
				return
			}

			grip.CatchNotice(c.comm.SendSystemInfo(ctx, c.taskData, sysinfo))
			grip.DebugWhen(sometimes.Fifth(), msg)

			timer.Reset(interval)
		}
	}
}
