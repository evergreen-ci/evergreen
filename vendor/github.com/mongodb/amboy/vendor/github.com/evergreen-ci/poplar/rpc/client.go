package rpc

import (
	"context"
	"runtime"
	"time"

	"github.com/evergreen-ci/poplar"
	"github.com/evergreen-ci/poplar/rpc/internal"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type UploadReportOptions struct {
	Report          *poplar.Report
	ClientConn      *grpc.ClientConn
	SerializeUpload bool
	DryRun          bool
}

func UploadReport(ctx context.Context, opts UploadReportOptions) error {
	if err := opts.convertAndUploadArtifacts(ctx); err != nil {
		return errors.Wrap(err, "problem uploading tests for report")
	}
	return errors.Wrap(uploadTests(ctx, internal.NewCedarPerformanceMetricsClient(opts.ClientConn), opts.Report, opts.Report.Tests, opts.DryRun),
		"problem uploading tests for report")
}

func (opts *UploadReportOptions) convertAndUploadArtifacts(ctx context.Context) error {
	jobQueue := queue.NewLocalLimitedSize(runtime.NumCPU(), len(opts.Report.Tests)*2)
	if !opts.SerializeUpload {
		if err := jobQueue.Start(ctx); err != nil {
			return errors.Wrap(err, "problem starting artifact upload queue")
		}
		defer jobQueue.Runner().Close(ctx)
	}

	queue := make([]poplar.Test, len(opts.Report.Tests))
	for i, test := range opts.Report.Tests {
		queue[i] = test
	}

	for len(queue) != 0 {
		test := queue[0]
		queue = queue[1:]

		for _, subtest := range test.SubTests {
			queue = append(queue, subtest)
		}

		for i := range test.Artifacts {
			var err error

			if err = test.Artifacts[i].Convert(ctx); err != nil {
				return errors.Wrap(err, "problem converting artifact")
			}

			err = test.Artifacts[i].SetBucketInfo(opts.Report.BucketConf)
			if err != nil {
				return errors.Wrap(err, "problem setting bucket info")
			}

			job := NewUploadJob(test.Artifacts[i], opts.Report.BucketConf, opts.DryRun)
			if opts.SerializeUpload {
				job.Run(ctx)
				if err = job.Error(); err != nil {
					return errors.Wrap(err, "problem converting and uploading artifacts")
				}
			} else if err = jobQueue.Put(ctx, job); err != nil {
				return errors.Wrap(err, "problem adding artifact job to upload queue")
			}
		}
	}

	if opts.SerializeUpload {
		return nil
	}

	if !amboy.WaitInterval(ctx, jobQueue, 10*time.Millisecond) {
		return errors.New("context canceled while waiting for artifact jobs to complete")
	}

	catcher := grip.NewBasicCatcher()
	for job := range jobQueue.Results(ctx) {
		catcher.Add(job.Error())
	}

	return errors.Wrap(catcher.Resolve(), "problem uploading artifacts")
}

func uploadTests(ctx context.Context, client internal.CedarPerformanceMetricsClient, report *poplar.Report, tests []poplar.Test, dryRun bool) error {
	for idx, test := range tests {
		grip.Info(message.Fields{
			"num":     idx,
			"total":   len(tests),
			"parent":  test.Info.Parent != "",
			"name":    test.Info.TestName,
			"task":    report.TaskID,
			"dry_run": dryRun,
		})

		createdAt, err := internal.ExportTimestamp(test.CreatedAt)
		if err != nil {
			return err
		}
		artifacts, err := extractArtifacts(ctx, report, test)
		if err != nil {
			return err
		}
		resultData := &internal.ResultData{
			Id: &internal.ResultID{
				Project:   report.Project,
				Version:   report.Version,
				Order:     int32(report.Order),
				Variant:   report.Variant,
				TaskName:  report.TaskName,
				TaskId:    report.TaskID,
				Mainline:  report.Mainline,
				Execution: int32(report.Execution),
				TestName:  test.Info.TestName,
				Trial:     int32(test.Info.Trial),
				Tags:      test.Info.Tags,
				Arguments: test.Info.Arguments,
				Parent:    test.Info.Parent,
				CreatedAt: createdAt,
			},
			Artifacts: artifacts,
			Rollups:   extractMetrics(ctx, test),
		}

		if dryRun {
			grip.Info(message.Fields{
				"message":     "dry-run mode",
				"function":    "CreateMetricSeries",
				"result_data": resultData,
			})
		} else {
			var resp *internal.MetricsResponse
			resp, err = client.CreateMetricSeries(ctx, resultData)
			if err != nil {
				return errors.Wrapf(err, "problem submitting test %d of %d", idx, len(tests))
			} else if !resp.Success {
				return errors.New("operation return failed state")
			}

			test.ID = resp.Id
			for i := range test.SubTests {
				test.SubTests[i].Info.Parent = test.ID
			}
		}

		if err = uploadTests(ctx, client, report, test.SubTests, dryRun); err != nil {
			return errors.Wrapf(err, "problem submitting subtests of '%s'", test.ID)
		}

		completedAt, err := internal.ExportTimestamp(test.CompletedAt)
		if err != nil {
			return err
		}
		end := &internal.MetricsSeriesEnd{
			Id:          test.ID,
			IsComplete:  true,
			CompletedAt: completedAt,
		}

		if dryRun {
			grip.Info(message.Fields{
				"message":           "dry-run mode",
				"function":          "CloseMetrics",
				"metric_series_end": end,
			})
		} else {
			var resp *internal.MetricsResponse
			resp, err = client.CloseMetrics(ctx, end)
			if err != nil {
				return errors.Wrapf(err, "problem closing metrics series for '%s'", test.ID)
			} else if !resp.Success {
				return errors.New("operation return failed state")
			}
		}

	}

	return nil
}

func extractArtifacts(ctx context.Context, report *poplar.Report, test poplar.Test) ([]*internal.ArtifactInfo, error) {
	artifacts := make([]*internal.ArtifactInfo, 0, len(test.Artifacts))
	for _, a := range test.Artifacts {
		if err := a.Validate(); err != nil {
			return nil, errors.Wrap(err, "problem validating artifact")
		}
		artifacts = append(artifacts, internal.ExportArtifactInfo(&a))
		artifacts[len(artifacts)-1].Location = internal.StorageLocation_CEDAR_S3
	}

	return artifacts, nil
}

func extractMetrics(ctx context.Context, test poplar.Test) []*internal.RollupValue {
	rollups := make([]*internal.RollupValue, 0, len(test.Metrics))
	for _, r := range test.Metrics {
		rollups = append(rollups, internal.ExportRollup(&r))
	}

	return rollups
}
