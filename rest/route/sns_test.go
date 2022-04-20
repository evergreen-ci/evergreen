package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
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
			return &ecsSNS{base}
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
	rh.env = testutil.NewEnvironment(ctx, t)
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
	assert.NoError(t, db.Clear(host.Collection))
	agentHost := host.Host{
		Id:        "agent_host",
		StartTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
		StartedBy: evergreen.User,
		Provider:  evergreen.ProviderNameMock,
	}
	spawnHost := host.Host{
		Id:        "spawn_host",
		StartedBy: "user",
		UserHost:  true,
	}
	messageID := "m0"
	rh := ec2SNS{}
	rh.payload.MessageId = messageID
	assert.NoError(t, agentHost.Insert())
	assert.NoError(t, spawnHost.Insert())

	for name, test := range map[string]func(*testing.T){
		"InstanceTerminatedInitiatesInstanceStatusCheck": func(t *testing.T) {
			require.NoError(t, rh.handleInstanceTerminated(ctx, agentHost.Id))
			require.Equal(t, 1, rh.queue.Stats(ctx).Total)
		},
		"InstanceStoppedWithAgentHostInitiatesInstanceStatusCheck": func(t *testing.T) {
			require.NoError(t, rh.handleInstanceStopped(ctx, agentHost.Id))
			require.Equal(t, 1, rh.queue.Stats(ctx).Total)
		},
		"InstanceStoppedWithSpawnHostNoops": func(t *testing.T) {
			require.NoError(t, rh.handleInstanceStopped(ctx, spawnHost.Id))
			assert.Zero(t, rh.queue.Stats(ctx).Total)
		},
	} {
		rh.queue = queue.NewLocalLimitedSize(1, 1)
		require.NoError(t, rh.queue.Start(ctx))

		t.Run(name, test)
	}
}

func TestECSSNSHandleNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *ecsSNS){
		"MarksRunningPodForTerminationWhenStopped": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: ecsEventDetail{
					TaskARN:       "external_id",
					LastStatus:    "STOPPED",
					StoppedReason: "reason",
				},
			}
			require.NoError(t, rh.handleNotification(ctx, notification))

			p, err := data.FindPodByID("id")
			assert.NoError(t, err)
			assert.Equal(t, model.PodStatusDecommissioned, p.Status)
		},
		"NoopsWithNonexistentPod": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: ecsEventDetail{
					TaskARN:       "nonexistent",
					LastStatus:    "STOPPED",
					StoppedReason: "reason",
				},
			}
			assert.NoError(t, rh.handleNotification(ctx, notification))

			p, err := data.FindPodByID("id")
			assert.NoError(t, err)
			assert.Equal(t, model.PodStatusRunning, p.Status)
		},
		"FailsWithoutStatus": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: ecsTaskStateChangeType,
				Detail: ecsEventDetail{
					TaskARN:       "external_id",
					StoppedReason: "reason",
				},
			}
			assert.Error(t, rh.handleNotification(ctx, notification))

			p, err := data.FindPodByID("id")
			assert.NoError(t, err)
			assert.Equal(t, model.PodStatusRunning, p.Status)
		},
		"FailsWithUnknownNotificationType": func(ctx context.Context, t *testing.T, rh *ecsSNS) {
			notification := ecsEventBridgeNotification{
				DetailType: "unknown",
				Detail: ecsEventDetail{
					TaskARN:       "external_id",
					LastStatus:    "STOPPED",
					StoppedReason: "reason",
				},
			}
			assert.Error(t, rh.handleNotification(ctx, notification))

			p, err := data.FindPodByID("id")
			assert.NoError(t, err)
			assert.Equal(t, model.PodStatusRunning, p.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.Clear(pod.Collection))
			podToCreate := pod.Pod{
				ID:     "id",
				Type:   pod.TypeAgent,
				Status: pod.StatusRunning,
				Resources: pod.ResourceInfo{
					ExternalID: "external_id",
				},
			}
			require.NoError(t, podToCreate.Insert())
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			q, err := queue.NewLocalLimitedSizeSerializable(1, 1)
			require.NoError(t, err)
			rh, ok := makeECSSNS(env, q).(*ecsSNS)
			require.True(t, ok)

			tCase(ctx, t, rh)
		})
	}
}
