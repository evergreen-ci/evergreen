package testresults

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const basePort = 3000

func TestNewClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := testutil.NewMockMetricsServer(ctx, testutil.GetPortNumber(basePort))
	require.NoError(t, err)

	t.Run("Succeeds", func(t *testing.T) {
		httpClient := utility.GetHTTPClient()
		defer utility.PutHTTPClient(httpClient)
		client, err := NewClient(ctx, timber.ConnectionOptions{
			Client:   *httpClient,
			DialOpts: srv.DialOpts,
		})
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
	t.Run("FailsWithInvalidOptions", func(t *testing.T) {
		client, err := NewClient(ctx, timber.ConnectionOptions{})
		assert.Error(t, err)
		assert.Zero(t, client)
	})

	t.Run("WithExistingConnection", func(t *testing.T) {
		t.Run("Succeeds", func(t *testing.T) {
			conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
			require.NoError(t, err)
			c, err := NewClientWithExistingConnection(ctx, conn)
			require.NoError(t, err)
			assert.NotZero(t, c)
		})
		t.Run("FailsWithoutClient", func(t *testing.T) {
			c, err := NewClientWithExistingConnection(ctx, nil)
			assert.Error(t, err)
			assert.Zero(t, c)
		})
	})
}

func makeClient(ctx context.Context, t *testing.T, httpClient *http.Client, opts timber.DialCedarOptions) *Client {
	client, err := NewClient(ctx, timber.ConnectionOptions{
		Client:   *httpClient,
		DialOpts: opts,
	})
	require.NoError(t, err)
	return client
}
func makeClientWithConn(ctx context.Context, t *testing.T, conn *grpc.ClientConn) *Client {
	client, err := NewClientWithExistingConnection(ctx, conn)
	require.NoError(t, err)
	return client
}
func TestClient(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client){
		"CreateRecordSucceeds": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			opts := validCreateOptions()
			id, err := client.CreateRecord(ctx, opts)
			require.NoError(t, err)
			assert.NotZero(t, id)

			assert.Equal(t, *opts.export(), *srv.Create)
		},
		"CreateRecordFailsWithServerError": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			srv.CreateErr = true
			id, err := client.CreateRecord(ctx, validCreateOptions())
			assert.Error(t, err)
			assert.Zero(t, id)
		},
		"AddResultsSucceeds": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			id, err := client.CreateRecord(ctx, validCreateOptions())
			require.NoError(t, err)
			require.NotZero(t, id)

			rs := validResults(id)
			require.NoError(t, client.AddResults(ctx, rs))

			require.Len(t, srv.Results[rs.ID], 1)
			exported, err := rs.export()
			require.NoError(t, err)
			assert.Equal(t, *srv.Results[rs.ID][0].Results[0], *exported.Results[0])
		},
		"AddResultsFailsWithInvalidOptions": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			id, err := client.CreateRecord(ctx, validCreateOptions())
			require.NoError(t, err)
			require.NotZero(t, id)

			assert.Error(t, client.AddResults(ctx, Results{}))
			assert.Zero(t, srv.Results)
		},
		"AddResultsFailsWithServerError": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			id, err := client.CreateRecord(ctx, validCreateOptions())
			require.NoError(t, err)
			require.NotZero(t, id)

			srv.AddErr = true
			assert.Error(t, client.AddResults(ctx, validResults(id)))
		},
		"CloseRecordSucceeds": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			id, err := client.CreateRecord(ctx, validCreateOptions())
			require.NoError(t, err)
			require.NotZero(t, id)

			require.NoError(t, client.CloseRecord(ctx, id))
			assert.Equal(t, gopb.TestResultsEndInfo{TestResultsRecordId: id}, *srv.Close)
		},
		"CloseRecordFailsWithoutID": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			id, err := client.CreateRecord(ctx, validCreateOptions())
			require.NoError(t, err)
			require.NotZero(t, id)

			require.Error(t, client.CloseRecord(ctx, ""))
			assert.Zero(t, srv.Close)
		},
		"CloseRecordFailsWithServerError": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			id, err := client.CreateRecord(ctx, validCreateOptions())
			require.NoError(t, err)
			require.NotZero(t, id)

			srv.CloseErr = true
			require.Error(t, client.CloseRecord(ctx, ""))
			assert.Zero(t, srv.Close)
		},
		"CloseClientClosesConnection": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, client *Client) {
			require.NoError(t, client.CloseClient())
			_, err := client.CreateRecord(ctx, validCreateOptions())
			assert.Error(t, err)
		},
		"CloseClientDoesNotClosePreexistingConnection": func(ctx context.Context, t *testing.T, srv *testutil.MockTestResultsServer, _ *Client) {
			conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
			require.NoError(t, err)
			client := makeClientWithConn(ctx, t, conn)
			require.NoError(t, err)
			require.NoError(t, client.CloseClient())

			opts := validCreateOptions()
			resp, err := gopb.NewCedarTestResultsClient(conn).CreateTestResultsRecord(ctx, opts.export())
			require.NoError(t, err)
			assert.NotZero(t, resp.TestResultsRecordId)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			srv, err := testutil.NewMockTestResultsServer(ctx, testutil.GetPortNumber(basePort))
			require.NoError(t, err)

			httpClient := utility.GetHTTPClient()
			defer utility.PutHTTPClient(httpClient)

			client := makeClient(ctx, t, httpClient, srv.DialOpts)
			require.NotZero(t, client)

			testCase(ctx, t, srv, client)
		})
	}
}

func validCreateOptions() CreateOptions {
	return CreateOptions{
		Project:         "project",
		Version:         "version",
		Variant:         "variant",
		TaskID:          "task_id",
		TaskName:        "task_name",
		DisplayTaskName: "display_task_name",
		DisplayTaskID:   "display_task_id",
		RequestType:     "request_type",
	}
}

func validResults(id string) Results {
	return Results{
		ID: id,
		Results: []Result{
			{
				TestName:        "name",
				DisplayTestName: "display",
				GroupID:         "group",
				Status:          "status",
				TaskCreated:     time.Now().Add(-time.Hour),
				TestStarted:     time.Now().Add(-time.Minute),
				TestEnded:       time.Now(),
			},
		},
	}
}
