package route

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/cocoa/ecs"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	sns "github.com/robbiet480/go.sns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseSNSRoute(t *testing.T) {
	for rhName, makeRoute := range map[string]func(base baseSNS) gimlet.RouteHandler{
		"EC2": func(base baseSNS) gimlet.RouteHandler {
			return &ec2SNS{base}
		},
		"ECS": func(base baseSNS) gimlet.RouteHandler {
			return &ecsSNS{
				baseSNS: base,
			}
		},
	} {
		t.Run(rhName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			t.Run("FailsWithBadMessageType", func(t *testing.T) {
				rh := makeRoute(baseSNS{messageType: "unknown"})
				responder := rh.Run(ctx)
				assert.Equal(t, http.StatusBadRequest, responder.Status())
			})
			t.Run("SucceedsWithSubscriptionConfirmation", func(t *testing.T) {
				defer func(s send.Sender) {
					assert.NoError(t, grip.SetSender(s))
				}(grip.GetSender())

				sender := send.MakeInternalLogger()
				assert.NoError(t, grip.SetSender(sender))

				rh := makeRoute(baseSNS{messageType: messageTypeSubscriptionConfirmation})
				resp := rh.Run(context.Background())
				assert.Equal(t, http.StatusOK, resp.Status())
				assert.True(t, sender.HasMessage())
				m := sender.GetMessage()
				assert.Equal(t, level.Alert, m.Priority)
				fields, ok := m.Message.Raw().(message.Fields)
				require.True(t, ok)
				assert.Equal(t, "got AWS SNS subscription confirmation. Visit subscribe_url to confirm", fields["message"])
			})
			t.Run("SucceedsWithUnsubscribeConfirmation", func(t *testing.T) {
				defer func(s send.Sender) {
					assert.NoError(t, grip.SetSender(s))
				}(grip.GetSender())

				sender := send.MakeInternalLogger()
				assert.NoError(t, grip.SetSender(sender))

				rh := makeRoute(baseSNS{messageType: messageTypeUnsubscribeConfirmation})
				resp := rh.Run(context.Background())
				assert.Equal(t, http.StatusOK, resp.Status())
				assert.True(t, sender.HasMessage())
				m := sender.GetMessage()
				assert.Equal(t, level.Alert, m.Priority)
				fields, ok := m.Message.Raw().(message.Fields)
				require.True(t, ok)
				assert.Equal(t, "got AWS SNS unsubscription confirmation. Visit unsubscribe_url to confirm", fields["message"])
			})
		})
	}
}

func TestHandleEC2SNSNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.Clear(host.Collection))
	rh := ec2SNS{}
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	rh.env = env
	rh.queue = rh.env.LocalQueue()
	rh.payload = sns.Payload{Message: `{"version":"0","id":"qwertyuiop","detail-type":"EC2 Instance State-change Notification","source":"sns.ec2","time":"2020-07-23T14:48:37Z","region":"us-east-1","resources":["arn:aws:ec2:us-east-1:1234567890:instance/i-0123456789"],"detail":{"instance-id":"i-0123456789","state":"terminated"}}`}

	// unknown host
	assert.NoError(t, rh.handleNotification(ctx))
	assert.Equal(t, rh.queue.Stats(ctx).Total, 0)

	// known host
	hostToAdd := host.Host{Id: "i-0123456789"}
	assert.NoError(t, hostToAdd.Insert())

	assert.NoError(t, rh.handleNotification(ctx))
	require.Equal(t, 1, rh.queue.Stats(ctx).Total)
}

func TestEC2SNSNotificationHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		assert.NoError(t, db.Clear(host.Collection))
	}()
	assert.NoError(t, db.Clear(host.Collection))

	agentHost := host.Host{
		Id:        "agent_host",
		StartTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
		StartedBy: evergreen.User,
		Provider:  evergreen.ProviderNameMock,
		Status:    evergreen.HostRunning,
	}
	spawnHost := host.Host{
		Id:        "spawn_host",
		StartedBy: "user",
		UserHost:  true,
		Status:    evergreen.HostRunning,
	}
	messageID := "m0"
	rh := ec2SNS{}
	rh.payload.MessageId = messageID
	assert.NoError(t, agentHost.Insert())
	assert.NoError(t, spawnHost.Insert())

	checkStatus := func(t *testing.T, hostID, status string) {
		dbHost, err := host.FindOneId(ctx, hostID)
		require.NoError(t, err)
		require.NotZero(t, dbHost)
		assert.Equal(t, status, dbHost.Status)
	}

	for name, test := range map[string]func(ctx context.Context, t *testing.T){
		"InstanceTerminatedInitiatesInstanceStatusCheck": func(ctx context.Context, t *testing.T) {
			require.NoError(t, rh.handleInstanceTerminated(ctx, agentHost.Id))
			checkStatus(t, agentHost.Id, evergreen.HostDecommissioned)
			require.Equal(t, 1, rh.queue.Stats(ctx).Total)
		},
		"InstanceStoppedWithAgentHostInitiatesInstanceStatusCheck": func(ctx context.Context, t *testing.T) {
			require.NoError(t, rh.handleInstanceStopped(ctx, agentHost.Id))
			checkStatus(t, agentHost.Id, evergreen.HostDecommissioned)
			require.Equal(t, 1, rh.queue.Stats(ctx).Total)
		},
		"InstanceStoppedWithSpawnHostNoops": func(ctx context.Context, t *testing.T) {
			originalStatus := spawnHost.Status
			require.NoError(t, rh.handleInstanceStopped(ctx, spawnHost.Id))
			checkStatus(t, spawnHost.Id, originalStatus)
			assert.Zero(t, rh.queue.Stats(ctx).Total)
		},
	} {
		t.Run(name, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 5*time.Second)
			defer tcancel()

			queue, err := queue.NewLocalLimitedSizeSerializable(1, 1)
			require.NoError(t, err)
			rh.queue = queue
			require.NoError(t, rh.queue.Start(ctx))

			test(tctx, t)
		})
	}
}

func TestECSSNSHandleNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer cocoaMock.ResetGlobalECSService()

	const (
		taskARN = "external_id"
	)

	makeJSON := func(t *testing.T, i interface{}) json.RawMessage {
		b, err := json.Marshal(i)
		require.NoError(t, err)
		return json.RawMessage(b)
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *ecsSNS){
		"MarksRunningPodForTerminationWhenStopped": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: makeJSON(t, ecsTaskEventDetail{
					TaskARN:       taskARN,
					LastStatus:    string(ecs.TaskStatusStopped),
					StoppedReason: "reason",
				}),
			}
			require.NoError(t, rh.handleNotification(ctx, notification))

			p, err := pod.FindOneByExternalID(taskARN)
			assert.NoError(t, err)
			assert.Equal(t, pod.StatusDecommissioned, p.Status)
		},
		"CleansUpUnrecognizedPodTryingToStart": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			originalFlags, err := evergreen.GetServiceFlags()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, originalFlags.Set(ctx))
			}()

			updatedFlags := *originalFlags
			updatedFlags.UnrecognizedPodCleanupDisabled = false
			require.NoError(t, updatedFlags.Set(ctx))

			// Set up the fake ECS testing service and the route's ECS client so
			// that it tests cleaning up the pod in the fake service rather than
			// actually trying to perform cleanup in AWS.
			const (
				clusterID     = "ecs-cluster"
				taskID        = "nonexistent-ecs-task"
				status        = string(ecs.TaskStatusActivating)
				desiredStatus = string(ecs.TaskStatusRunning)
			)
			rh.env.Settings().Providers.AWS.Pod.ECS.Clusters = []evergreen.ECSClusterConfig{
				{
					Name: clusterID,
					OS:   evergreen.ECSOSLinux,
				},
			}
			cocoaMock.GlobalECSService.Clusters[clusterID] = cocoaMock.ECSCluster{
				taskID: cocoaMock.ECSTask{
					ARN:        taskID,
					Cluster:    utility.ToStringPtr(clusterID),
					Created:    utility.ToTimePtr(time.Now().Add(-10 * time.Minute)),
					Status:     utility.ToStringPtr(status),
					GoalStatus: utility.ToStringPtr(desiredStatus),
				},
			}

			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: makeJSON(t, ecsTaskEventDetail{
					TaskARN:       taskID,
					ClusterARN:    clusterID,
					LastStatus:    status,
					DesiredStatus: desiredStatus,
				}),
			}
			assert.NoError(t, rh.handleNotification(ctx, notification))

			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterID], 1)
			assert.EqualValues(t, ecs.TaskStatusStopped, utility.FromStringPtr(cocoaMock.GlobalECSService.Clusters[clusterID][taskID].Status), "unrecognized cloud pod should have been stopped")
		},
		"NoopsWhenUnrecognizedPodIsTryingToStartInUnrecognizedCluster": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			originalFlags, err := evergreen.GetServiceFlags()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, originalFlags.Set(ctx))
			}()

			updatedFlags := *originalFlags
			updatedFlags.UnrecognizedPodCleanupDisabled = false
			require.NoError(t, updatedFlags.Set(ctx))

			// Set up the fake ECS testing service and the route's ECS client so
			// that it tests cleaning up the pod in the fake service rather than
			// actually trying to perform cleanup in AWS.
			const (
				clusterID     = "ecs-cluster"
				taskID        = "nonexistent-ecs-task"
				status        = string(ecs.TaskStatusActivating)
				desiredStatus = string(ecs.TaskStatusRunning)
			)
			cocoaMock.GlobalECSService.Clusters[clusterID] = cocoaMock.ECSCluster{
				taskID: cocoaMock.ECSTask{
					ARN:        taskID,
					Cluster:    utility.ToStringPtr(clusterID),
					Created:    utility.ToTimePtr(time.Now().Add(-10 * time.Minute)),
					Status:     utility.ToStringPtr(status),
					GoalStatus: utility.ToStringPtr(desiredStatus),
				},
			}

			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: makeJSON(t, ecsTaskEventDetail{
					TaskARN:       taskID,
					ClusterARN:    clusterID,
					LastStatus:    status,
					DesiredStatus: desiredStatus,
				}),
			}
			assert.NoError(t, rh.handleNotification(ctx, notification))

			assert.EqualValues(t, status, utility.FromStringPtr(cocoaMock.GlobalECSService.Clusters[clusterID][taskID].Status), "unrecognized cloud pod in unrecognized cluster should not have been stopped")
		},
		"NoopsWhenUnrecognizedPodIsDetectedButAlreadyShuttingDown": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			originalFlags, err := evergreen.GetServiceFlags()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, originalFlags.Set(ctx))
			}()

			updatedFlags := *originalFlags
			updatedFlags.UnrecognizedPodCleanupDisabled = false
			require.NoError(t, updatedFlags.Set(ctx))

			// Set up the fake ECS testing service and the route's ECS client so
			// that it tests cleaning up the pod in the fake service rather than
			// actually trying to perform cleanup in AWS.
			const (
				clusterID     = "ecs-cluster"
				taskID        = "nonexistent-ecs-task"
				status        = string(ecs.TaskStatusDeprovisioning)
				desiredStatus = string(ecs.TaskStatusStopped)
			)
			rh.env.Settings().Providers.AWS.Pod.ECS.Clusters = []evergreen.ECSClusterConfig{
				{
					Name: clusterID,
					OS:   evergreen.ECSOSLinux,
				},
			}
			cocoaMock.GlobalECSService.Clusters[clusterID] = cocoaMock.ECSCluster{
				taskID: cocoaMock.ECSTask{
					ARN:        taskID,
					Cluster:    utility.ToStringPtr(clusterID),
					Created:    utility.ToTimePtr(time.Now().Add(-10 * time.Minute)),
					Status:     utility.ToStringPtr(status),
					GoalStatus: utility.ToStringPtr(desiredStatus),
				},
			}

			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: makeJSON(t, ecsTaskEventDetail{
					TaskARN:       taskID,
					ClusterARN:    clusterID,
					LastStatus:    status,
					DesiredStatus: desiredStatus,
				}),
			}
			assert.NoError(t, rh.handleNotification(ctx, notification))

			assert.EqualValues(t, status, utility.FromStringPtr(cocoaMock.GlobalECSService.Clusters[clusterID][taskID].Status), "unrecognized cloud pod in unrecognized cluster should not have been stopped")
		},
		"NoopsWhenUnrecognizedPodIsDetectedButUnrecognizedPodCleanupIsDisabled": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			originalFlags, err := evergreen.GetServiceFlags()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, originalFlags.Set(ctx))
			}()

			updatedFlags := *originalFlags
			updatedFlags.UnrecognizedPodCleanupDisabled = true
			require.NoError(t, updatedFlags.Set(ctx))

			const (
				clusterID     = "ecs-cluster"
				taskID        = "nonexistent-ecs-task"
				status        = string(ecs.TaskStatusActivating)
				desiredStatus = string(ecs.TaskStatusRunning)
			)
			rh.env.Settings().Providers.AWS.Pod.ECS.Clusters = []evergreen.ECSClusterConfig{
				{
					Name: clusterID,
					OS:   evergreen.ECSOSLinux,
				},
			}
			cocoaMock.GlobalECSService.Clusters[clusterID] = cocoaMock.ECSCluster{
				taskID: cocoaMock.ECSTask{
					ARN:        taskID,
					Cluster:    utility.ToStringPtr(clusterID),
					Created:    utility.ToTimePtr(time.Now().Add(-10 * time.Minute)),
					Status:     utility.ToStringPtr(status),
					GoalStatus: utility.ToStringPtr(desiredStatus),
				},
			}

			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: makeJSON(t, ecsTaskEventDetail{
					TaskARN:       taskID,
					ClusterARN:    clusterID,
					LastStatus:    status,
					DesiredStatus: desiredStatus,
				}),
			}
			assert.NoError(t, rh.handleNotification(ctx, notification))

			assert.EqualValues(t, status, utility.FromStringPtr(cocoaMock.GlobalECSService.Clusters[clusterID][taskID].Status), "cloud pod should not be cleaned up when the service flag flag is disabled")
		},
		"FailsWithoutStatus": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: makeJSON(t, ecsTaskEventDetail{
					TaskARN:       taskARN,
					StoppedReason: "reason",
				}),
			}
			assert.Error(t, rh.handleNotification(ctx, notification))

			p, err := pod.FindOneByExternalID(taskARN)
			require.NoError(t, err)
			require.NotZero(t, p)
			assert.Equal(t, pod.StatusRunning, p.Status)
		},
		"FailsWithUnknownNotificationType": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: "unknown",
				Detail: makeJSON(t, ecsTaskEventDetail{
					TaskARN:       taskARN,
					LastStatus:    string(ecs.TaskStatusStopped),
					StoppedReason: "reason",
				}),
			}
			assert.Error(t, rh.handleNotification(ctx, notification))

			p, err := pod.FindOneByExternalID(taskARN)
			require.NoError(t, err)
			require.NotZero(t, p)
			assert.Equal(t, pod.StatusRunning, p.Status)
		},
		"FailsWithInvalidDetails": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: "unknown",
				Detail:     makeJSON(t, "lol what"),
			}
			assert.Error(t, rh.handleNotification(ctx, notification))

			p, err := pod.FindOneByExternalID(taskARN)
			require.NoError(t, err)
			require.NotZero(t, p)
			assert.Equal(t, pod.StatusRunning, p.Status)
		},
		"MarksRunningPodForTerminationWhenItsContainerInstanceIsDraining": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			// Set up the fake ECS testing service and the route's ECS client so
			// that the testing pod is returned from the fake ECS service when
			// listing tasks in this container instance.
			const (
				clusterID           = "ecs-cluster"
				containerInstanceID = "ecs-container-instance"
				status              = string(ecs.ContainerInstanceStatusDraining)
			)
			rh.env.Settings().Providers.AWS.Pod.ECS.Clusters = []evergreen.ECSClusterConfig{
				{
					Name: clusterID,
					OS:   evergreen.ECSOSLinux,
				},
			}
			cocoaMock.GlobalECSService.Clusters[clusterID] = cocoaMock.ECSCluster{
				taskARN: cocoaMock.ECSTask{
					ARN:               taskARN,
					ContainerInstance: utility.ToStringPtr(containerInstanceID),
					Cluster:           utility.ToStringPtr(clusterID),
					Created:           utility.ToTimePtr(time.Now().Add(-10 * time.Minute)),
					Status:            utility.ToStringPtr(status),
				},
			}

			notification := ecsEventBridgeNotification{
				DetailType: ecsContainerInstanceStateChangeType,
				Detail: makeJSON(t, ecsContainerInstanceEventDetail{
					ContainerInstanceARN: containerInstanceID,
					ClusterARN:           clusterID,
					Status:               status,
				}),
			}
			assert.NoError(t, rh.handleNotification(ctx, notification))

			p, err := pod.FindOneByExternalID(taskARN)
			require.NoError(t, err)
			require.NotZero(t, p)
			assert.Equal(t, pod.StatusDecommissioned, p.Status)
		},
		"NoopsWithContainerInstanceForIrreleventStatusChange": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: ecsContainerInstanceStateChangeType,
				Detail: makeJSON(t, ecsContainerInstanceEventDetail{
					ContainerInstanceARN: "container_instance_arn",
					Status:               string(ecs.ContainerInstanceStatusActive),
					ClusterARN:           "cluster_arn",
				}),
			}
			assert.NoError(t, rh.handleNotification(ctx, notification))

			p, err := pod.FindOneByExternalID(taskARN)
			require.NoError(t, err)
			require.NotZero(t, p)
			assert.Equal(t, pod.StatusRunning, p.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.Clear(pod.Collection))
			podToCreate := pod.Pod{
				ID:     "pod_id",
				Type:   pod.TypeAgent,
				Status: pod.StatusRunning,
				Resources: pod.ResourceInfo{
					ExternalID: taskARN,
				},
			}
			require.NoError(t, podToCreate.Insert())
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			q, err := queue.NewLocalLimitedSizeSerializable(1, 1)
			require.NoError(t, err)
			rh, ok := makeECSSNS(env, q).(*ecsSNS)
			require.True(t, ok)

			cocoaMock.ResetGlobalECSService()

			tCase(ctx, t, rh)
		})
	}
}
