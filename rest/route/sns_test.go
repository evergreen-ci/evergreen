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
	} {
		t.Run(rhName, func(t *testing.T) {
			ctx := t.Context()

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

func TestBaseSNSParse(t *testing.T) {
	const allowedTopicARN = "arn:aws:sns:us-east-1:1234567890:some-aws-topic"

	for tName, tCase := range map[string]struct {
		topicARNs      []string
		payloadTopic   string
		expectedStatus int
	}{
		"SucceedsWithAnyAllowedTopicARN": {
			topicARNs:    []string{"arn:aws:sns:us-east-1:1234567890:other-topic", allowedTopicARN},
			payloadTopic: allowedTopicARN,
		},
		"FailsWithDisallowedTopicARN": {
			topicARNs:      []string{allowedTopicARN},
			payloadTopic:   "arn:aws:sns:us-east-1:9876543210:bad-topic",
			expectedStatus: http.StatusUnauthorized,
		},
		"FailsWithNoExplicitlyAllowedTopicARNs": {
			payloadTopic:   allowedTopicARN,
			expectedStatus: http.StatusUnauthorized,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx := t.Context()
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.Providers.AWS.SNSTopicARNs = tCase.topicARNs

			payload := sns.Payload{
				TopicArn:  tCase.payloadTopic,
				MessageId: "message_id",
			}
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/hooks/aws", nil)
			require.NoError(t, err)
			req.Header.Set("x-amz-sns-message-type", messageTypeNotification)
			req = setSNSPayload(req, payload)

			bsns := makeBaseSNS(env, env.LocalQueue())
			err = bsns.Parse(ctx, req)

			if tCase.expectedStatus == 0 {
				assert.NoError(t, err)
				assert.Equal(t, payload, bsns.payload)
				return
			}

			require.Error(t, err)
			resp, ok := err.(gimlet.ErrorResponse)
			require.True(t, ok)
			assert.Equal(t, tCase.expectedStatus, resp.StatusCode)
			assert.Zero(t, bsns.payload)
		})
	}
}

func TestHandleEC2SNSNotification(t *testing.T) {
	ctx := t.Context()
	assert.NoError(t, db.Clear(host.Collection))
	rh := ec2SNS{}
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	rh.env = env
	rh.queue = rh.env.LocalQueue()
	rh.payload = sns.Payload{Message: `{"version":"0","id":"qwertyuiop","detail-type":"EC2 Instance State-change Notification","source":"sns.ec2","time":"2020-07-23T14:48:37Z","region":"us-east-1","resources":["arn:aws:ec2:us-east-1:1234567890:instance/i-0123456789"],"detail":{"instance-id":"i-0123456789","state":"terminated"}}`}

	// unknown host
	assert.NoError(t, rh.handleNotification(ctx))
	assert.Equal(t, 0, rh.queue.Stats(ctx).Total)

	// known host
	hostToAdd := host.Host{Id: "i-0123456789"}
	assert.NoError(t, hostToAdd.Insert(ctx))

	assert.NoError(t, rh.handleNotification(ctx))
	require.Equal(t, 1, rh.queue.Stats(ctx).Total)
}

func TestEC2SNSNotificationHandlers(t *testing.T) {
	ctx := t.Context()
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
	assert.NoError(t, agentHost.Insert(ctx))
	assert.NoError(t, spawnHost.Insert(ctx))

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
