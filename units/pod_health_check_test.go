package units

import (
	"context"
	"testing"
	"time"

	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodHealthCheckJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	defer func() {
		assert.NoError(t, db.ClearCollections(pod.Collection))
		cocoaMock.ResetGlobalECSService()
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podHealthCheckJob){
		"EnqueuesPodTerminationJobForStoppedCloudPod": func(ctx context.Context, t *testing.T, j *podHealthCheckJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.ecsPod.Delete(ctx))

			j.Run(ctx)
			require.NoError(t, j.Error())

			var podTerminationJobFound bool
			for remoteQueueJob := range j.env.RemoteQueue().JobInfo(ctx) {
				if remoteQueueJob.Type.Name != podTerminationJobName {
					continue
				}
				podTerminationJobFound = true
			}
			assert.True(t, podTerminationJobFound, "should enqueue pod termination job for unhealthy pod")
		},
		"NoopsForRunningCloudPod": func(ctx context.Context, t *testing.T, j *podHealthCheckJob) {
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			var podTerminationJobFound bool
			for remoteQueueJob := range j.env.RemoteQueue().JobInfo(ctx) {
				if remoteQueueJob.Type.Name != podTerminationJobName {
					continue
				}
				podTerminationJobFound = true
			}
			assert.False(t, podTerminationJobFound, "should not enqueue pod termination job for healthy pod")
		},
		"NoopsForAlreadyTerminatedPod": func(ctx context.Context, t *testing.T, j *podHealthCheckJob) {
			j.pod.Status = pod.StatusTerminated
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			var podTerminationJobFound bool
			for remoteQueueJob := range j.env.RemoteQueue().JobInfo(ctx) {
				if remoteQueueJob.Type.Name != podTerminationJobName {
					continue
				}
				podTerminationJobFound = true
			}
			assert.False(t, podTerminationJobFound, "should not check pod health for already terminated pod")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
			defer tcancel()
			tctx = testutil.TestSpan(tctx, t)

			require.NoError(t, db.ClearCollections(pod.Collection))
			cocoaMock.ResetGlobalECSService()

			const cluster = "cluster"
			cocoaMock.GlobalECSService.Clusters[cluster] = cocoaMock.ECSCluster{}

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			p := pod.Pod{
				ID:     "pod_id",
				Status: pod.StatusRunning,
				TimeInfo: pod.TimeInfo{
					LastCommunicated: time.Now().Add(-time.Hour),
				},
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					Image:    "image",
					CPU:      2048,
					MemoryMB: 4096,
				},
			}

			j, ok := NewPodHealthCheckJob(p.ID, time.Now()).(*podHealthCheckJob)
			require.True(t, ok)
			j.env = env
			j.ecsClient = &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(tctx))
			}()
			j.pod = &p

			j.ecsPod = generateTestingECSPod(tctx, t, j.ecsClient, cluster, p.TaskContainerCreationOpts)
			j.pod.Resources = cloud.ImportECSPodResources(j.ecsPod.Resources())

			tCase(tctx, t, j)
		})
	}
}
