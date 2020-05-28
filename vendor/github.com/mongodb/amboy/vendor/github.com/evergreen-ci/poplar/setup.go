package poplar

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/evergreen-ci/aviation/services"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	yaml "gopkg.in/yaml.v2"
)

const (
	ProjectEnv      = "project"
	VersionEnv      = "version_id"
	OrderEnv        = "revision_order_id"
	VariantEnv      = "build_variant"
	TaskNameEnv     = "task_name"
	ExecutionEnv    = "execution"
	MainlineEnv     = "is_patch"
	APIKeyEnv       = "API_KEY"
	APISecretEnv    = "API_SECRET"
	APITokenEnv     = "API_TOKEN"
	BucketNameEnv   = "BUCKET_NAME"
	BucketPrefixEnv = "BUCKET_PREFIX"
	BucketRegionEnv = "BUCKET_REGION"
)

// ReportType describes the marshalled report type.
type ReportType string

const (
	ReportTypeJSON ReportType = "JSON"
	ReportTypeBSON ReportType = "BSON"
	ReportTypeYAML ReportType = "YAML"
	ReportTypeEnv  ReportType = "ENV"
)

// ReportSetup sets up a Report struct with the given ReportType and filename.
// Note that not all ReportTypes require a filename (such as ReportTypeEnv), if
// this is the case, pass in an empty string for the filename.
func ReportSetup(reportType ReportType, filename string) (*Report, error) {
	switch reportType {
	case ReportTypeJSON:
		return reportSetupUnmarshal(reportType, filename, json.Unmarshal)
	case ReportTypeBSON:
		return reportSetupUnmarshal(reportType, filename, bson.Unmarshal)
	case ReportTypeYAML:
		return reportSetupUnmarshal(reportType, filename, yaml.Unmarshal)
	case ReportTypeEnv:
		return reportSetupEnv()
	default:
		return nil, errors.Errorf("invalid report type %s", reportType)
	}
}

func reportSetupUnmarshal(reportType ReportType, filename string, unmarshal func([]byte, interface{}) error) (*Report, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "problem opening file %s", filename)
	}

	report := &Report{}
	if err := unmarshal(data, report); err != nil {
		return nil, errors.Wrapf(err, "problem unmarshalling %s from %s", reportType, filename)
	}

	return report, nil
}

func reportSetupEnv() (*Report, error) {
	var order int
	var execution int
	var mainline bool
	var err error
	if os.Getenv(MainlineEnv) != "" {
		mainline, err = strconv.ParseBool(os.Getenv(MainlineEnv))
		if err != nil {
			return nil, errors.Wrapf(err, "env var %s should be a bool", MainlineEnv)
		}
	}
	if mainline && os.Getenv(OrderEnv) != "" {
		order, err = strconv.Atoi(os.Getenv(OrderEnv))
		if err != nil {
			return nil, errors.Wrapf(err, "env var %s should be an int", OrderEnv)
		}
	}
	if os.Getenv(ExecutionEnv) != "" {
		execution, err = strconv.Atoi(os.Getenv(ExecutionEnv))
		if err != nil {
			return nil, errors.Wrapf(err, "env var %s should be an int", ExecutionEnv)
		}
	}

	return &Report{
		Project:   os.Getenv(ProjectEnv),
		Version:   os.Getenv(VersionEnv),
		Order:     order,
		Variant:   os.Getenv(VariantEnv),
		TaskName:  os.Getenv(TaskNameEnv),
		Execution: execution,
		Mainline:  mainline,
		BucketConf: BucketConfiguration{
			APIKey:    os.Getenv(APIKeyEnv),
			APISecret: os.Getenv(APISecretEnv),
			APIToken:  os.Getenv(APITokenEnv),
			Name:      os.Getenv(BucketNameEnv),
			Prefix:    os.Getenv(BucketPrefixEnv),
			Region:    os.Getenv(BucketRegionEnv),
		},
		Tests: []Test{},
	}, nil
}

// DialCedarOptions describes the options for the DialCedar function. The base
// address defaults to `cedar.mongodb.com` and the RPC port to 7070. If a base
// address is provided the RPC port must also be provided. Username and
// password must always be provided. This aliases the same type in aviation in
// order to avoid users having to vendor aviation.
type DialCedarOptions services.DialCedarOptions

// DialCedar is a convenience function for creating a RPC client connection
// with cedar via gRPC. This wraps the same function in aviation in order to
// avoid users having to vendor aviation.
func DialCedar(ctx context.Context, client *http.Client, opts DialCedarOptions) (*grpc.ClientConn, error) {
	serviceOpts := services.DialCedarOptions(opts)
	return services.DialCedar(ctx, client, &serviceOpts)
}
