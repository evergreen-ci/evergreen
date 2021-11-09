package rpc

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/poplar"
	"github.com/evergreen-ci/poplar/rpc/internal"
	"github.com/evergreen-ci/utility"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockClient struct {
	resultData []*internal.ResultData
	endData    map[string]*internal.MetricsSeriesEnd
}

func NewMockClient() *mockClient {
	return &mockClient{endData: map[string]*internal.MetricsSeriesEnd{}}
}

func (mc *mockClient) CreateMetricSeries(_ context.Context, in *internal.ResultData, _ ...grpc.CallOption) (*internal.MetricsResponse, error) {
	mc.resultData = append(mc.resultData, in)
	return &internal.MetricsResponse{Id: in.Id.TestName, Success: true}, nil
}
func (*mockClient) AttachResultData(_ context.Context, _ *internal.ResultData, _ ...grpc.CallOption) (*internal.MetricsResponse, error) {
	return nil, nil
}
func (*mockClient) AttachArtifacts(_ context.Context, _ *internal.ArtifactData, _ ...grpc.CallOption) (*internal.MetricsResponse, error) {
	return nil, nil
}
func (*mockClient) AttachRollups(_ context.Context, _ *internal.RollupData, _ ...grpc.CallOption) (*internal.MetricsResponse, error) {
	return nil, nil
}
func (*mockClient) SendMetrics(_ context.Context, _ ...grpc.CallOption) (internal.CedarPerformanceMetrics_SendMetricsClient, error) {
	return nil, nil
}
func (mc *mockClient) CloseMetrics(_ context.Context, in *internal.MetricsSeriesEnd, _ ...grpc.CallOption) (*internal.MetricsResponse, error) {
	mc.endData[in.Id] = in
	return &internal.MetricsResponse{Success: true}, nil
}

func mockUploadReport(ctx context.Context, report *poplar.Report, client internal.CedarPerformanceMetricsClient, serialize, dryRun bool) error {
	opts := UploadReportOptions{
		Report:          report,
		SerializeUpload: serialize,
		DryRun:          dryRun,
	}
	if err := opts.convertAndUploadArtifacts(ctx); err != nil {
		return errors.Wrap(err, "problem uploading tests for report")
	}
	return errors.Wrap(uploadTests(ctx, client, report, report.Tests, dryRun),
		"problem uploading tests for report")
}

func TestClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testdataDir := filepath.Join("..", "testdata")
	s3Name := "build-test-curator"
	s3Prefix := "poplar-client-test"
	s3Opts := pail.S3Options{
		Name:   s3Name,
		Prefix: s3Prefix,
		Region: "us-east-1",
	}

	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	s3Bucket, err := pail.NewS3BucketWithHTTPClient(client, s3Opts)
	require.NoError(t, err)

	report := generateTestReport(testdataDir, s3Name, s3Prefix)
	expectedTests := []poplar.Test{
		report.Tests[0],
		report.Tests[0].SubTests[0],
		report.Tests[1],
		report.Tests[1].SubTests[0],
	}
	expectedParents := map[string]string{
		"test0":  "",
		"test00": "test0",
		"test1":  "",
		"test10": "test1",
	}
	for i := range expectedTests {
		for j := range expectedTests[i].Artifacts {
			require.NoError(t, expectedTests[i].Artifacts[j].Convert(ctx))
			require.NoError(t, expectedTests[i].Artifacts[j].SetBucketInfo(report.BucketConf))
			require.NoError(t, os.RemoveAll(filepath.Join(testdataDir, expectedTests[i].Artifacts[j].Path)))
		}
	}

	defer func() {
		for _, test := range expectedTests {
			for _, artifact := range test.Artifacts {
				assert.NoError(t, s3Bucket.Remove(ctx, artifact.Path))
				assert.NoError(t, os.RemoveAll(filepath.Join(testdataDir, artifact.Path)))
			}
		}
	}()

	t.Run("WetRun", func(t *testing.T) {
		for _, serialize := range []bool{true, false} {
			testReport := generateTestReport(testdataDir, s3Name, s3Prefix)
			mc := NewMockClient()
			require.NoError(t, mockUploadReport(ctx, &testReport, mc, serialize, false))
			require.Len(t, mc.resultData, len(expectedTests))
			require.Equal(t, len(mc.resultData), len(mc.endData))
			for i, result := range mc.resultData {
				assert.Equal(t, testReport.Project, result.Id.Project)
				assert.Equal(t, testReport.Version, result.Id.Version)
				assert.Equal(t, testReport.Order, int(result.Id.Order))
				assert.Equal(t, testReport.Variant, result.Id.Variant)
				assert.Equal(t, testReport.TaskName, result.Id.TaskName)
				assert.Equal(t, testReport.TaskID, result.Id.TaskId)
				assert.Equal(t, testReport.Mainline, result.Id.Mainline)
				assert.Equal(t, testReport.Execution, int(result.Id.Execution))
				assert.Equal(t, expectedTests[i].Info.TestName, result.Id.TestName)
				assert.Equal(t, expectedTests[i].Info.Trial, int(result.Id.Trial))
				assert.Equal(t, expectedTests[i].Info.Tags, result.Id.Tags)
				assert.Equal(t, expectedTests[i].Info.Arguments, result.Id.Arguments)
				assert.Equal(t, expectedParents[expectedTests[i].Info.TestName], result.Id.Parent)
				var expectedCreatedAt *timestamp.Timestamp
				var expectedCompletedAt *timestamp.Timestamp
				if !expectedTests[i].CreatedAt.IsZero() {
					expectedCreatedAt, err = ptypes.TimestampProto(expectedTests[i].CreatedAt)
					require.NoError(t, err)
				}
				if !expectedTests[i].CompletedAt.IsZero() {
					expectedCompletedAt, err = ptypes.TimestampProto(expectedTests[i].CompletedAt)
					require.NoError(t, err)
				}
				assert.Equal(t, expectedCreatedAt, result.Id.CreatedAt)
				assert.Equal(t, expectedCompletedAt, mc.endData[result.Id.TestName].CompletedAt)

				require.Len(t, result.Artifacts, len(expectedTests[i].Artifacts))
				for j, artifact := range expectedTests[i].Artifacts {
					require.NoError(t, artifact.Validate())
					expectedArtifact := internal.ExportArtifactInfo(&artifact)
					expectedArtifact.Location = internal.StorageLocation_CEDAR_S3
					assert.Equal(t, expectedArtifact, result.Artifacts[j])
					r, err := s3Bucket.Get(ctx, artifact.Path)
					require.NoError(t, err)
					remoteData, err := ioutil.ReadAll(r)
					require.NoError(t, err)
					f, err := os.Open(filepath.Join(testdataDir, artifact.Path))
					require.NoError(t, err)
					localData, err := ioutil.ReadAll(f)
					require.NoError(t, err)
					assert.Equal(t, localData, remoteData)
					require.NoError(t, f.Close())
				}

				require.Len(t, result.Rollups, len(expectedTests[i].Metrics))
				for k, metric := range expectedTests[i].Metrics {
					assert.Equal(t, internal.ExportRollup(&metric), result.Rollups[k])
				}
			}
		}
	})

	for _, test := range expectedTests {
		for _, artifact := range test.Artifacts {
			require.NoError(t, s3Bucket.Remove(ctx, artifact.Path))
			require.NoError(t, os.RemoveAll(filepath.Join(testdataDir, artifact.Path)))
		}
	}

	t.Run("DryRun", func(t *testing.T) {
		for _, serialize := range []bool{true, false} {
			testReport := generateTestReport(testdataDir, s3Name, s3Prefix)
			mc := NewMockClient()
			require.NoError(t, mockUploadReport(ctx, &testReport, mc, serialize, true))
			assert.Empty(t, mc.resultData)
			assert.Empty(t, mc.endData)
			for _, expectedTest := range expectedTests {
				for _, artifact := range expectedTest.Artifacts {
					require.NoError(t, artifact.Validate())
					r, err := s3Bucket.Get(ctx, artifact.Path)
					assert.Error(t, err)
					assert.Nil(t, r)
					_, err = os.Stat(filepath.Join(testdataDir, artifact.Path))
					require.NoError(t, err)
				}
			}
		}
	})
}

func generateTestReport(testdataDir, s3Name, s3Prefix string) poplar.Report {
	return poplar.Report{
		Project:   "project",
		Version:   "version",
		Order:     2,
		Variant:   "variant",
		TaskName:  "taskName",
		TaskID:    "taskID",
		Mainline:  true,
		Execution: 2,

		BucketConf: poplar.BucketConfiguration{
			Name:   s3Name,
			Region: "us-east-1",
		},

		Tests: []poplar.Test{
			{
				Info: poplar.TestInfo{
					TestName:  "test0",
					Trial:     2,
					Tags:      []string{"tag0", "tag1"},
					Arguments: map[string]int32{"thread_level": 1},
				},
				Artifacts: []poplar.TestArtifact{
					{
						Bucket:           s3Name,
						Prefix:           s3Prefix,
						Path:             "bson_example.ftdc",
						LocalFile:        filepath.Join(testdataDir, "bson_example.bson"),
						ConvertBSON2FTDC: true,
					},
					{
						Prefix:      s3Prefix,
						LocalFile:   filepath.Join(testdataDir, "bson_example.bson"),
						ConvertGzip: true,
					},
				},
				CreatedAt:   time.Date(2018, time.July, 4, 12, 0, 0, 0, time.UTC),
				CompletedAt: time.Date(2018, time.July, 4, 12, 1, 0, 0, time.UTC),
				SubTests: []poplar.Test{
					{
						Info: poplar.TestInfo{
							TestName: "test00",
						},
						Metrics: []poplar.TestMetrics{
							{
								Name:    "mean",
								Version: 1,
								Value:   1.5,
								Type:    "MEAN",
							},
							{
								Name:    "sum",
								Version: 1,
								Value:   10,
								Type:    "SUM",
							},
						},
					},
				},
			},
			{
				Info: poplar.TestInfo{
					TestName: "test1",
				},
				Artifacts: []poplar.TestArtifact{
					{
						Bucket:           s3Name,
						Prefix:           s3Prefix,
						Path:             "json_example.ftdc",
						LocalFile:        filepath.Join(testdataDir, "json_example.json"),
						CreatedAt:        time.Date(2018, time.July, 4, 11, 59, 0, 0, time.UTC),
						ConvertJSON2FTDC: true,
					},
				},
				SubTests: []poplar.Test{
					{
						Info: poplar.TestInfo{
							TestName: "test10",
						},
						Metrics: []poplar.TestMetrics{
							{
								Name:    "mean",
								Version: 1,
								Value:   1.5,
								Type:    "MEAN",
							},
							{
								Name:    "sum",
								Version: 1,
								Value:   10,
								Type:    "SUM",
							},
						},
					},
				},
			},
		},
	}
}
