// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/logging/v2/logging_metrics.proto

package logging // import "google.golang.org/genproto/googleapis/logging/v2"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import empty "github.com/golang/protobuf/ptypes/empty"
import _ "github.com/golang/protobuf/ptypes/timestamp"
import _ "google.golang.org/genproto/googleapis/api/annotations"
import distribution "google.golang.org/genproto/googleapis/api/distribution"
import metric "google.golang.org/genproto/googleapis/api/metric"
import _ "google.golang.org/genproto/protobuf/field_mask"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Logging API version.
type LogMetric_ApiVersion int32

const (
	// Logging API v2.
	LogMetric_V2 LogMetric_ApiVersion = 0
	// Logging API v1.
	LogMetric_V1 LogMetric_ApiVersion = 1
)

var LogMetric_ApiVersion_name = map[int32]string{
	0: "V2",
	1: "V1",
}
var LogMetric_ApiVersion_value = map[string]int32{
	"V2": 0,
	"V1": 1,
}

func (x LogMetric_ApiVersion) String() string {
	return proto.EnumName(LogMetric_ApiVersion_name, int32(x))
}
func (LogMetric_ApiVersion) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{0, 0}
}

// Describes a logs-based metric.  The value of the metric is the
// number of log entries that match a logs filter in a given time interval.
//
// Logs-based metric can also be used to extract values from logs and create a
// a distribution of the values. The distribution records the statistics of the
// extracted values along with an optional histogram of the values as specified
// by the bucket options.
type LogMetric struct {
	// Required. The client-assigned metric identifier.
	// Examples: `"error_count"`, `"nginx/requests"`.
	//
	// Metric identifiers are limited to 100 characters and can include
	// only the following characters: `A-Z`, `a-z`, `0-9`, and the
	// special characters `_-.,+!*',()%/`.  The forward-slash character
	// (`/`) denotes a hierarchy of name pieces, and it cannot be the
	// first character of the name.
	//
	// The metric identifier in this field must not be
	// [URL-encoded](https://en.wikipedia.org/wiki/Percent-encoding).
	// However, when the metric identifier appears as the `[METRIC_ID]`
	// part of a `metric_name` API parameter, then the metric identifier
	// must be URL-encoded. Example:
	// `"projects/my-project/metrics/nginx%2Frequests"`.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Optional. A description of this metric, which is used in documentation.
	Description string `protobuf:"bytes,2,opt,name=description,proto3" json:"description,omitempty"`
	// Required. An [advanced logs filter](/logging/docs/view/advanced_filters)
	// which is used to match log entries.
	// Example:
	//
	//     "resource.type=gae_app AND severity>=ERROR"
	//
	// The maximum length of the filter is 20000 characters.
	Filter string `protobuf:"bytes,3,opt,name=filter,proto3" json:"filter,omitempty"`
	// Optional. The metric descriptor associated with the logs-based metric.
	// If unspecified, it uses a default metric descriptor with a DELTA metric
	// kind, INT64 value type, with no labels and a unit of "1". Such a metric
	// counts the number of log entries matching the `filter` expression.
	//
	// The `name`, `type`, and `description` fields in the `metric_descriptor`
	// are output only, and is constructed using the `name` and `description`
	// field in the LogMetric.
	//
	// To create a logs-based metric that records a distribution of log values, a
	// DELTA metric kind with a DISTRIBUTION value type must be used along with
	// a `value_extractor` expression in the LogMetric.
	//
	// Each label in the metric descriptor must have a matching label
	// name as the key and an extractor expression as the value in the
	// `label_extractors` map.
	//
	// The `metric_kind` and `value_type` fields in the `metric_descriptor` cannot
	// be updated once initially configured. New labels can be added in the
	// `metric_descriptor`, but existing labels cannot be modified except for
	// their description.
	MetricDescriptor *metric.MetricDescriptor `protobuf:"bytes,5,opt,name=metric_descriptor,json=metricDescriptor,proto3" json:"metric_descriptor,omitempty"`
	// Optional. A `value_extractor` is required when using a distribution
	// logs-based metric to extract the values to record from a log entry.
	// Two functions are supported for value extraction: `EXTRACT(field)` or
	// `REGEXP_EXTRACT(field, regex)`. The argument are:
	//   1. field: The name of the log entry field from which the value is to be
	//      extracted.
	//   2. regex: A regular expression using the Google RE2 syntax
	//      (https://github.com/google/re2/wiki/Syntax) with a single capture
	//      group to extract data from the specified log entry field. The value
	//      of the field is converted to a string before applying the regex.
	//      It is an error to specify a regex that does not include exactly one
	//      capture group.
	//
	// The result of the extraction must be convertible to a double type, as the
	// distribution always records double values. If either the extraction or
	// the conversion to double fails, then those values are not recorded in the
	// distribution.
	//
	// Example: `REGEXP_EXTRACT(jsonPayload.request, ".*quantity=(\d+).*")`
	ValueExtractor string `protobuf:"bytes,6,opt,name=value_extractor,json=valueExtractor,proto3" json:"value_extractor,omitempty"`
	// Optional. A map from a label key string to an extractor expression which is
	// used to extract data from a log entry field and assign as the label value.
	// Each label key specified in the LabelDescriptor must have an associated
	// extractor expression in this map. The syntax of the extractor expression
	// is the same as for the `value_extractor` field.
	//
	// The extracted value is converted to the type defined in the label
	// descriptor. If the either the extraction or the type conversion fails,
	// the label will have a default value. The default value for a string
	// label is an empty string, for an integer label its 0, and for a boolean
	// label its `false`.
	//
	// Note that there are upper bounds on the maximum number of labels and the
	// number of active time series that are allowed in a project.
	LabelExtractors map[string]string `protobuf:"bytes,7,rep,name=label_extractors,json=labelExtractors,proto3" json:"label_extractors,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// Optional. The `bucket_options` are required when the logs-based metric is
	// using a DISTRIBUTION value type and it describes the bucket boundaries
	// used to create a histogram of the extracted values.
	BucketOptions *distribution.Distribution_BucketOptions `protobuf:"bytes,8,opt,name=bucket_options,json=bucketOptions,proto3" json:"bucket_options,omitempty"`
	// Deprecated. The API version that created or updated this metric.
	// The v2 format is used by default and cannot be changed.
	Version              LogMetric_ApiVersion `protobuf:"varint,4,opt,name=version,proto3,enum=google.logging.v2.LogMetric_ApiVersion" json:"version,omitempty"` // Deprecated: Do not use.
	XXX_NoUnkeyedLiteral struct{}             `json:"-"`
	XXX_unrecognized     []byte               `json:"-"`
	XXX_sizecache        int32                `json:"-"`
}

func (m *LogMetric) Reset()         { *m = LogMetric{} }
func (m *LogMetric) String() string { return proto.CompactTextString(m) }
func (*LogMetric) ProtoMessage()    {}
func (*LogMetric) Descriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{0}
}
func (m *LogMetric) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_LogMetric.Unmarshal(m, b)
}
func (m *LogMetric) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_LogMetric.Marshal(b, m, deterministic)
}
func (dst *LogMetric) XXX_Merge(src proto.Message) {
	xxx_messageInfo_LogMetric.Merge(dst, src)
}
func (m *LogMetric) XXX_Size() int {
	return xxx_messageInfo_LogMetric.Size(m)
}
func (m *LogMetric) XXX_DiscardUnknown() {
	xxx_messageInfo_LogMetric.DiscardUnknown(m)
}

var xxx_messageInfo_LogMetric proto.InternalMessageInfo

func (m *LogMetric) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *LogMetric) GetDescription() string {
	if m != nil {
		return m.Description
	}
	return ""
}

func (m *LogMetric) GetFilter() string {
	if m != nil {
		return m.Filter
	}
	return ""
}

func (m *LogMetric) GetMetricDescriptor() *metric.MetricDescriptor {
	if m != nil {
		return m.MetricDescriptor
	}
	return nil
}

func (m *LogMetric) GetValueExtractor() string {
	if m != nil {
		return m.ValueExtractor
	}
	return ""
}

func (m *LogMetric) GetLabelExtractors() map[string]string {
	if m != nil {
		return m.LabelExtractors
	}
	return nil
}

func (m *LogMetric) GetBucketOptions() *distribution.Distribution_BucketOptions {
	if m != nil {
		return m.BucketOptions
	}
	return nil
}

// Deprecated: Do not use.
func (m *LogMetric) GetVersion() LogMetric_ApiVersion {
	if m != nil {
		return m.Version
	}
	return LogMetric_V2
}

// The parameters to ListLogMetrics.
type ListLogMetricsRequest struct {
	// Required. The name of the project containing the metrics:
	//
	//     "projects/[PROJECT_ID]"
	Parent string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	// Optional. If present, then retrieve the next batch of results from the
	// preceding call to this method.  `pageToken` must be the value of
	// `nextPageToken` from the previous response.  The values of other method
	// parameters should be identical to those in the previous call.
	PageToken string `protobuf:"bytes,2,opt,name=page_token,json=pageToken,proto3" json:"page_token,omitempty"`
	// Optional. The maximum number of results to return from this request.
	// Non-positive values are ignored.  The presence of `nextPageToken` in the
	// response indicates that more results might be available.
	PageSize             int32    `protobuf:"varint,3,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListLogMetricsRequest) Reset()         { *m = ListLogMetricsRequest{} }
func (m *ListLogMetricsRequest) String() string { return proto.CompactTextString(m) }
func (*ListLogMetricsRequest) ProtoMessage()    {}
func (*ListLogMetricsRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{1}
}
func (m *ListLogMetricsRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListLogMetricsRequest.Unmarshal(m, b)
}
func (m *ListLogMetricsRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListLogMetricsRequest.Marshal(b, m, deterministic)
}
func (dst *ListLogMetricsRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListLogMetricsRequest.Merge(dst, src)
}
func (m *ListLogMetricsRequest) XXX_Size() int {
	return xxx_messageInfo_ListLogMetricsRequest.Size(m)
}
func (m *ListLogMetricsRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ListLogMetricsRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ListLogMetricsRequest proto.InternalMessageInfo

func (m *ListLogMetricsRequest) GetParent() string {
	if m != nil {
		return m.Parent
	}
	return ""
}

func (m *ListLogMetricsRequest) GetPageToken() string {
	if m != nil {
		return m.PageToken
	}
	return ""
}

func (m *ListLogMetricsRequest) GetPageSize() int32 {
	if m != nil {
		return m.PageSize
	}
	return 0
}

// Result returned from ListLogMetrics.
type ListLogMetricsResponse struct {
	// A list of logs-based metrics.
	Metrics []*LogMetric `protobuf:"bytes,1,rep,name=metrics,proto3" json:"metrics,omitempty"`
	// If there might be more results than appear in this response, then
	// `nextPageToken` is included.  To get the next set of results, call this
	// method again using the value of `nextPageToken` as `pageToken`.
	NextPageToken        string   `protobuf:"bytes,2,opt,name=next_page_token,json=nextPageToken,proto3" json:"next_page_token,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListLogMetricsResponse) Reset()         { *m = ListLogMetricsResponse{} }
func (m *ListLogMetricsResponse) String() string { return proto.CompactTextString(m) }
func (*ListLogMetricsResponse) ProtoMessage()    {}
func (*ListLogMetricsResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{2}
}
func (m *ListLogMetricsResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListLogMetricsResponse.Unmarshal(m, b)
}
func (m *ListLogMetricsResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListLogMetricsResponse.Marshal(b, m, deterministic)
}
func (dst *ListLogMetricsResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListLogMetricsResponse.Merge(dst, src)
}
func (m *ListLogMetricsResponse) XXX_Size() int {
	return xxx_messageInfo_ListLogMetricsResponse.Size(m)
}
func (m *ListLogMetricsResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ListLogMetricsResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ListLogMetricsResponse proto.InternalMessageInfo

func (m *ListLogMetricsResponse) GetMetrics() []*LogMetric {
	if m != nil {
		return m.Metrics
	}
	return nil
}

func (m *ListLogMetricsResponse) GetNextPageToken() string {
	if m != nil {
		return m.NextPageToken
	}
	return ""
}

// The parameters to GetLogMetric.
type GetLogMetricRequest struct {
	// The resource name of the desired metric:
	//
	//     "projects/[PROJECT_ID]/metrics/[METRIC_ID]"
	MetricName           string   `protobuf:"bytes,1,opt,name=metric_name,json=metricName,proto3" json:"metric_name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetLogMetricRequest) Reset()         { *m = GetLogMetricRequest{} }
func (m *GetLogMetricRequest) String() string { return proto.CompactTextString(m) }
func (*GetLogMetricRequest) ProtoMessage()    {}
func (*GetLogMetricRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{3}
}
func (m *GetLogMetricRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetLogMetricRequest.Unmarshal(m, b)
}
func (m *GetLogMetricRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetLogMetricRequest.Marshal(b, m, deterministic)
}
func (dst *GetLogMetricRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetLogMetricRequest.Merge(dst, src)
}
func (m *GetLogMetricRequest) XXX_Size() int {
	return xxx_messageInfo_GetLogMetricRequest.Size(m)
}
func (m *GetLogMetricRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetLogMetricRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetLogMetricRequest proto.InternalMessageInfo

func (m *GetLogMetricRequest) GetMetricName() string {
	if m != nil {
		return m.MetricName
	}
	return ""
}

// The parameters to CreateLogMetric.
type CreateLogMetricRequest struct {
	// The resource name of the project in which to create the metric:
	//
	//     "projects/[PROJECT_ID]"
	//
	// The new metric must be provided in the request.
	Parent string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	// The new logs-based metric, which must not have an identifier that
	// already exists.
	Metric               *LogMetric `protobuf:"bytes,2,opt,name=metric,proto3" json:"metric,omitempty"`
	XXX_NoUnkeyedLiteral struct{}   `json:"-"`
	XXX_unrecognized     []byte     `json:"-"`
	XXX_sizecache        int32      `json:"-"`
}

func (m *CreateLogMetricRequest) Reset()         { *m = CreateLogMetricRequest{} }
func (m *CreateLogMetricRequest) String() string { return proto.CompactTextString(m) }
func (*CreateLogMetricRequest) ProtoMessage()    {}
func (*CreateLogMetricRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{4}
}
func (m *CreateLogMetricRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_CreateLogMetricRequest.Unmarshal(m, b)
}
func (m *CreateLogMetricRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_CreateLogMetricRequest.Marshal(b, m, deterministic)
}
func (dst *CreateLogMetricRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CreateLogMetricRequest.Merge(dst, src)
}
func (m *CreateLogMetricRequest) XXX_Size() int {
	return xxx_messageInfo_CreateLogMetricRequest.Size(m)
}
func (m *CreateLogMetricRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_CreateLogMetricRequest.DiscardUnknown(m)
}

var xxx_messageInfo_CreateLogMetricRequest proto.InternalMessageInfo

func (m *CreateLogMetricRequest) GetParent() string {
	if m != nil {
		return m.Parent
	}
	return ""
}

func (m *CreateLogMetricRequest) GetMetric() *LogMetric {
	if m != nil {
		return m.Metric
	}
	return nil
}

// The parameters to UpdateLogMetric.
type UpdateLogMetricRequest struct {
	// The resource name of the metric to update:
	//
	//     "projects/[PROJECT_ID]/metrics/[METRIC_ID]"
	//
	// The updated metric must be provided in the request and it's
	// `name` field must be the same as `[METRIC_ID]` If the metric
	// does not exist in `[PROJECT_ID]`, then a new metric is created.
	MetricName string `protobuf:"bytes,1,opt,name=metric_name,json=metricName,proto3" json:"metric_name,omitempty"`
	// The updated metric.
	Metric               *LogMetric `protobuf:"bytes,2,opt,name=metric,proto3" json:"metric,omitempty"`
	XXX_NoUnkeyedLiteral struct{}   `json:"-"`
	XXX_unrecognized     []byte     `json:"-"`
	XXX_sizecache        int32      `json:"-"`
}

func (m *UpdateLogMetricRequest) Reset()         { *m = UpdateLogMetricRequest{} }
func (m *UpdateLogMetricRequest) String() string { return proto.CompactTextString(m) }
func (*UpdateLogMetricRequest) ProtoMessage()    {}
func (*UpdateLogMetricRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{5}
}
func (m *UpdateLogMetricRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UpdateLogMetricRequest.Unmarshal(m, b)
}
func (m *UpdateLogMetricRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UpdateLogMetricRequest.Marshal(b, m, deterministic)
}
func (dst *UpdateLogMetricRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateLogMetricRequest.Merge(dst, src)
}
func (m *UpdateLogMetricRequest) XXX_Size() int {
	return xxx_messageInfo_UpdateLogMetricRequest.Size(m)
}
func (m *UpdateLogMetricRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateLogMetricRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateLogMetricRequest proto.InternalMessageInfo

func (m *UpdateLogMetricRequest) GetMetricName() string {
	if m != nil {
		return m.MetricName
	}
	return ""
}

func (m *UpdateLogMetricRequest) GetMetric() *LogMetric {
	if m != nil {
		return m.Metric
	}
	return nil
}

// The parameters to DeleteLogMetric.
type DeleteLogMetricRequest struct {
	// The resource name of the metric to delete:
	//
	//     "projects/[PROJECT_ID]/metrics/[METRIC_ID]"
	MetricName           string   `protobuf:"bytes,1,opt,name=metric_name,json=metricName,proto3" json:"metric_name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *DeleteLogMetricRequest) Reset()         { *m = DeleteLogMetricRequest{} }
func (m *DeleteLogMetricRequest) String() string { return proto.CompactTextString(m) }
func (*DeleteLogMetricRequest) ProtoMessage()    {}
func (*DeleteLogMetricRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_logging_metrics_6c8e9bf6f41c8035, []int{6}
}
func (m *DeleteLogMetricRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DeleteLogMetricRequest.Unmarshal(m, b)
}
func (m *DeleteLogMetricRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DeleteLogMetricRequest.Marshal(b, m, deterministic)
}
func (dst *DeleteLogMetricRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DeleteLogMetricRequest.Merge(dst, src)
}
func (m *DeleteLogMetricRequest) XXX_Size() int {
	return xxx_messageInfo_DeleteLogMetricRequest.Size(m)
}
func (m *DeleteLogMetricRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_DeleteLogMetricRequest.DiscardUnknown(m)
}

var xxx_messageInfo_DeleteLogMetricRequest proto.InternalMessageInfo

func (m *DeleteLogMetricRequest) GetMetricName() string {
	if m != nil {
		return m.MetricName
	}
	return ""
}

func init() {
	proto.RegisterType((*LogMetric)(nil), "google.logging.v2.LogMetric")
	proto.RegisterMapType((map[string]string)(nil), "google.logging.v2.LogMetric.LabelExtractorsEntry")
	proto.RegisterType((*ListLogMetricsRequest)(nil), "google.logging.v2.ListLogMetricsRequest")
	proto.RegisterType((*ListLogMetricsResponse)(nil), "google.logging.v2.ListLogMetricsResponse")
	proto.RegisterType((*GetLogMetricRequest)(nil), "google.logging.v2.GetLogMetricRequest")
	proto.RegisterType((*CreateLogMetricRequest)(nil), "google.logging.v2.CreateLogMetricRequest")
	proto.RegisterType((*UpdateLogMetricRequest)(nil), "google.logging.v2.UpdateLogMetricRequest")
	proto.RegisterType((*DeleteLogMetricRequest)(nil), "google.logging.v2.DeleteLogMetricRequest")
	proto.RegisterEnum("google.logging.v2.LogMetric_ApiVersion", LogMetric_ApiVersion_name, LogMetric_ApiVersion_value)
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// MetricsServiceV2Client is the client API for MetricsServiceV2 service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type MetricsServiceV2Client interface {
	// Lists logs-based metrics.
	ListLogMetrics(ctx context.Context, in *ListLogMetricsRequest, opts ...grpc.CallOption) (*ListLogMetricsResponse, error)
	// Gets a logs-based metric.
	GetLogMetric(ctx context.Context, in *GetLogMetricRequest, opts ...grpc.CallOption) (*LogMetric, error)
	// Creates a logs-based metric.
	CreateLogMetric(ctx context.Context, in *CreateLogMetricRequest, opts ...grpc.CallOption) (*LogMetric, error)
	// Creates or updates a logs-based metric.
	UpdateLogMetric(ctx context.Context, in *UpdateLogMetricRequest, opts ...grpc.CallOption) (*LogMetric, error)
	// Deletes a logs-based metric.
	DeleteLogMetric(ctx context.Context, in *DeleteLogMetricRequest, opts ...grpc.CallOption) (*empty.Empty, error)
}

type metricsServiceV2Client struct {
	cc *grpc.ClientConn
}

func NewMetricsServiceV2Client(cc *grpc.ClientConn) MetricsServiceV2Client {
	return &metricsServiceV2Client{cc}
}

func (c *metricsServiceV2Client) ListLogMetrics(ctx context.Context, in *ListLogMetricsRequest, opts ...grpc.CallOption) (*ListLogMetricsResponse, error) {
	out := new(ListLogMetricsResponse)
	err := c.cc.Invoke(ctx, "/google.logging.v2.MetricsServiceV2/ListLogMetrics", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metricsServiceV2Client) GetLogMetric(ctx context.Context, in *GetLogMetricRequest, opts ...grpc.CallOption) (*LogMetric, error) {
	out := new(LogMetric)
	err := c.cc.Invoke(ctx, "/google.logging.v2.MetricsServiceV2/GetLogMetric", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metricsServiceV2Client) CreateLogMetric(ctx context.Context, in *CreateLogMetricRequest, opts ...grpc.CallOption) (*LogMetric, error) {
	out := new(LogMetric)
	err := c.cc.Invoke(ctx, "/google.logging.v2.MetricsServiceV2/CreateLogMetric", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metricsServiceV2Client) UpdateLogMetric(ctx context.Context, in *UpdateLogMetricRequest, opts ...grpc.CallOption) (*LogMetric, error) {
	out := new(LogMetric)
	err := c.cc.Invoke(ctx, "/google.logging.v2.MetricsServiceV2/UpdateLogMetric", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metricsServiceV2Client) DeleteLogMetric(ctx context.Context, in *DeleteLogMetricRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/google.logging.v2.MetricsServiceV2/DeleteLogMetric", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MetricsServiceV2Server is the server API for MetricsServiceV2 service.
type MetricsServiceV2Server interface {
	// Lists logs-based metrics.
	ListLogMetrics(context.Context, *ListLogMetricsRequest) (*ListLogMetricsResponse, error)
	// Gets a logs-based metric.
	GetLogMetric(context.Context, *GetLogMetricRequest) (*LogMetric, error)
	// Creates a logs-based metric.
	CreateLogMetric(context.Context, *CreateLogMetricRequest) (*LogMetric, error)
	// Creates or updates a logs-based metric.
	UpdateLogMetric(context.Context, *UpdateLogMetricRequest) (*LogMetric, error)
	// Deletes a logs-based metric.
	DeleteLogMetric(context.Context, *DeleteLogMetricRequest) (*empty.Empty, error)
}

func RegisterMetricsServiceV2Server(s *grpc.Server, srv MetricsServiceV2Server) {
	s.RegisterService(&_MetricsServiceV2_serviceDesc, srv)
}

func _MetricsServiceV2_ListLogMetrics_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListLogMetricsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetricsServiceV2Server).ListLogMetrics(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.logging.v2.MetricsServiceV2/ListLogMetrics",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetricsServiceV2Server).ListLogMetrics(ctx, req.(*ListLogMetricsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetricsServiceV2_GetLogMetric_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetLogMetricRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetricsServiceV2Server).GetLogMetric(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.logging.v2.MetricsServiceV2/GetLogMetric",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetricsServiceV2Server).GetLogMetric(ctx, req.(*GetLogMetricRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetricsServiceV2_CreateLogMetric_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateLogMetricRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetricsServiceV2Server).CreateLogMetric(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.logging.v2.MetricsServiceV2/CreateLogMetric",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetricsServiceV2Server).CreateLogMetric(ctx, req.(*CreateLogMetricRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetricsServiceV2_UpdateLogMetric_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateLogMetricRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetricsServiceV2Server).UpdateLogMetric(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.logging.v2.MetricsServiceV2/UpdateLogMetric",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetricsServiceV2Server).UpdateLogMetric(ctx, req.(*UpdateLogMetricRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetricsServiceV2_DeleteLogMetric_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteLogMetricRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetricsServiceV2Server).DeleteLogMetric(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.logging.v2.MetricsServiceV2/DeleteLogMetric",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetricsServiceV2Server).DeleteLogMetric(ctx, req.(*DeleteLogMetricRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _MetricsServiceV2_serviceDesc = grpc.ServiceDesc{
	ServiceName: "google.logging.v2.MetricsServiceV2",
	HandlerType: (*MetricsServiceV2Server)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListLogMetrics",
			Handler:    _MetricsServiceV2_ListLogMetrics_Handler,
		},
		{
			MethodName: "GetLogMetric",
			Handler:    _MetricsServiceV2_GetLogMetric_Handler,
		},
		{
			MethodName: "CreateLogMetric",
			Handler:    _MetricsServiceV2_CreateLogMetric_Handler,
		},
		{
			MethodName: "UpdateLogMetric",
			Handler:    _MetricsServiceV2_UpdateLogMetric_Handler,
		},
		{
			MethodName: "DeleteLogMetric",
			Handler:    _MetricsServiceV2_DeleteLogMetric_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "google/logging/v2/logging_metrics.proto",
}

func init() {
	proto.RegisterFile("google/logging/v2/logging_metrics.proto", fileDescriptor_logging_metrics_6c8e9bf6f41c8035)
}

var fileDescriptor_logging_metrics_6c8e9bf6f41c8035 = []byte{
	// 861 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x56, 0x4f, 0x6f, 0x1b, 0x45,
	0x14, 0x67, 0x9c, 0xc4, 0x69, 0x5e, 0x68, 0xec, 0x4e, 0x5b, 0xd7, 0x72, 0x53, 0xc5, 0xec, 0x21,
	0x71, 0x73, 0xd8, 0xa5, 0x0b, 0x8a, 0x4a, 0x11, 0x07, 0xdc, 0x44, 0x15, 0x52, 0x0a, 0x91, 0x0b,
	0x3e, 0xa0, 0x48, 0xab, 0xb1, 0xfd, 0xb2, 0x1a, 0xbc, 0xbb, 0xb3, 0xec, 0x8c, 0xad, 0xa4, 0xa8,
	0x17, 0xd4, 0x1b, 0x12, 0x07, 0xf8, 0x00, 0x1c, 0xb8, 0xf1, 0x51, 0xb8, 0xc2, 0x47, 0xe0, 0x43,
	0x70, 0x44, 0x3b, 0x3b, 0xeb, 0x6c, 0xed, 0x25, 0x8e, 0x72, 0xca, 0xcc, 0xfb, 0xfd, 0xde, 0xfc,
	0x7e, 0xef, 0x8f, 0x63, 0xc3, 0x9e, 0x2f, 0x84, 0x1f, 0xa0, 0x13, 0x08, 0xdf, 0xe7, 0x91, 0xef,
	0x4c, 0xdd, 0xfc, 0xe8, 0x85, 0xa8, 0x12, 0x3e, 0x94, 0x76, 0x9c, 0x08, 0x25, 0xe8, 0x9d, 0x8c,
	0x68, 0x1b, 0xd4, 0x9e, 0xba, 0xad, 0x6d, 0x93, 0xcb, 0x62, 0xee, 0xb0, 0x28, 0x12, 0x8a, 0x29,
	0x2e, 0x22, 0x93, 0xd0, 0x7a, 0x54, 0x40, 0x47, 0x5c, 0xaa, 0x84, 0x0f, 0x26, 0x29, 0x6e, 0xe0,
	0x07, 0x05, 0x38, 0x53, 0x32, 0xc0, 0x43, 0x03, 0xe8, 0xdb, 0x60, 0x72, 0xe6, 0x60, 0x18, 0xab,
	0x0b, 0x03, 0xb6, 0xe7, 0xc1, 0x33, 0x8e, 0xc1, 0xc8, 0x0b, 0x99, 0x1c, 0x1b, 0xc6, 0xce, 0x3c,
	0x43, 0xf1, 0x10, 0xa5, 0x62, 0x61, 0x9c, 0x11, 0xac, 0xdf, 0x57, 0x61, 0xe3, 0x58, 0xf8, 0x2f,
	0xb5, 0x26, 0xa5, 0xb0, 0x1a, 0xb1, 0x10, 0x9b, 0xa4, 0x4d, 0x3a, 0x1b, 0x3d, 0x7d, 0xa6, 0x6d,
	0xd8, 0x1c, 0xa1, 0x1c, 0x26, 0x3c, 0x4e, 0xfd, 0x36, 0x2b, 0x1a, 0x2a, 0x86, 0x68, 0x03, 0xaa,
	0x67, 0x3c, 0x50, 0x98, 0x34, 0x57, 0x34, 0x68, 0x6e, 0xf4, 0x0b, 0xb8, 0x93, 0xd5, 0xe2, 0xe5,
	0x6c, 0x91, 0x34, 0xd7, 0xda, 0xa4, 0xb3, 0xe9, 0x6e, 0xdb, 0xa6, 0x81, 0x2c, 0xe6, 0x76, 0x26,
	0x7e, 0x38, 0xe3, 0xf4, 0xea, 0xe1, 0x5c, 0x84, 0xee, 0x41, 0x6d, 0xca, 0x82, 0x09, 0x7a, 0x78,
	0xae, 0x12, 0x36, 0x4c, 0x1f, 0xaa, 0x6a, 0xad, 0x2d, 0x1d, 0x3e, 0xca, 0xa3, 0xf4, 0x14, 0xea,
	0x01, 0x1b, 0x60, 0x70, 0x49, 0x94, 0xcd, 0xf5, 0xf6, 0x4a, 0x67, 0xd3, 0x7d, 0x62, 0x2f, 0xcc,
	0xcc, 0x9e, 0x55, 0x6e, 0x1f, 0xa7, 0x49, 0xb3, 0x67, 0xe4, 0x51, 0xa4, 0x92, 0x8b, 0x5e, 0x2d,
	0x78, 0x37, 0x4a, 0x5f, 0xc2, 0xd6, 0x60, 0x32, 0x1c, 0xa3, 0xf2, 0x84, 0x2e, 0x5d, 0x36, 0x6f,
	0xe9, 0x72, 0x76, 0x8b, 0xe5, 0x1c, 0x16, 0xc7, 0xdb, 0xd5, 0xf4, 0xaf, 0x32, 0x76, 0xef, 0xf6,
	0xa0, 0x78, 0xa5, 0x47, 0xb0, 0x3e, 0xc5, 0x44, 0xa6, 0x6d, 0x5d, 0x6d, 0x93, 0xce, 0x96, 0xbb,
	0x77, 0xa5, 0xc7, 0xcf, 0x63, 0xde, 0xcf, 0xe8, 0xdd, 0x4a, 0x93, 0xf4, 0xf2, 0xdc, 0x56, 0x17,
	0xee, 0x95, 0xd9, 0xa7, 0x75, 0x58, 0x19, 0xe3, 0x85, 0x19, 0x66, 0x7a, 0xa4, 0xf7, 0x60, 0x4d,
	0xf7, 0xcb, 0x4c, 0x31, 0xbb, 0x3c, 0xab, 0x3c, 0x25, 0xd6, 0x36, 0xc0, 0xe5, 0xf3, 0xb4, 0x0a,
	0x95, 0xbe, 0x5b, 0x7f, 0x4f, 0xff, 0x7d, 0x52, 0x27, 0xd6, 0x18, 0xee, 0x1f, 0x73, 0xa9, 0x66,
	0x56, 0x64, 0x0f, 0xbf, 0x9f, 0xa0, 0x54, 0xe9, 0xe8, 0x63, 0x96, 0x60, 0xa4, 0x8c, 0x8a, 0xb9,
	0xd1, 0x47, 0x00, 0x31, 0xf3, 0xd1, 0x53, 0x62, 0x8c, 0xf9, 0xce, 0x6c, 0xa4, 0x91, 0xaf, 0xd3,
	0x00, 0x7d, 0x08, 0xfa, 0xe2, 0x49, 0xfe, 0x1a, 0xf5, 0xd2, 0xac, 0xf5, 0x6e, 0xa5, 0x81, 0x57,
	0xfc, 0x35, 0x5a, 0xe7, 0xd0, 0x98, 0x17, 0x93, 0xb1, 0x88, 0x24, 0xd2, 0x03, 0x58, 0x37, 0x1f,
	0xc3, 0x26, 0xd1, 0x33, 0xdd, 0xbe, 0xaa, 0x5f, 0xbd, 0x9c, 0x4c, 0x77, 0xa1, 0x16, 0xe1, 0xb9,
	0xf2, 0x16, 0x2c, 0xdd, 0x4e, 0xc3, 0x27, 0xb9, 0x2d, 0xeb, 0x00, 0xee, 0xbe, 0xc0, 0x4b, 0xe1,
	0xbc, 0xc8, 0x1d, 0xd8, 0x34, 0x7b, 0x5c, 0xf8, 0x70, 0x40, 0x16, 0xfa, 0x92, 0x85, 0x68, 0x9d,
	0x41, 0xe3, 0x79, 0x82, 0x4c, 0xe1, 0x42, 0xea, 0xff, 0xf5, 0xe7, 0x63, 0xa8, 0x66, 0xf9, 0xda,
	0xc8, 0xb2, 0x42, 0x0c, 0xd7, 0x12, 0xd0, 0xf8, 0x26, 0x1e, 0x95, 0xe9, 0x2c, 0xb3, 0x78, 0x43,
	0xc1, 0x4f, 0xa0, 0x71, 0x88, 0x01, 0xde, 0x40, 0xd0, 0xfd, 0x7b, 0x0d, 0xea, 0x66, 0x7e, 0xaf,
	0x30, 0x99, 0xf2, 0x21, 0xf6, 0x5d, 0xfa, 0x33, 0x81, 0xad, 0x77, 0x67, 0x4b, 0x3b, 0x65, 0x46,
	0xca, 0x76, 0xad, 0xf5, 0xf8, 0x1a, 0xcc, 0x6c, 0x51, 0xac, 0xbd, 0x1f, 0xff, 0xfa, 0xe7, 0xd7,
	0xca, 0x07, 0x74, 0x27, 0xfd, 0x0f, 0xfe, 0x43, 0xd6, 0xf3, 0xcf, 0xe2, 0x44, 0x7c, 0x87, 0x43,
	0x25, 0x9d, 0xfd, 0x37, 0x4e, 0xbe, 0x19, 0x6f, 0x09, 0xbc, 0x5f, 0x1c, 0x39, 0xdd, 0x2d, 0x11,
	0x29, 0xd9, 0x89, 0xd6, 0x95, 0xfd, 0xb3, 0x6c, 0xad, 0xdf, 0xa1, 0xbb, 0x5a, 0xbf, 0xd0, 0xa8,
	0x82, 0x89, 0xdc, 0x83, 0xb3, 0xff, 0x86, 0xfe, 0x44, 0xa0, 0x36, 0xb7, 0x41, 0xb4, 0xac, 0xdc,
	0xf2, 0x2d, 0x5b, 0x62, 0xc6, 0xd1, 0x66, 0x1e, 0x5b, 0xcb, 0x9a, 0xf1, 0xcc, 0x4c, 0x9d, 0xfe,
	0x42, 0xa0, 0x36, 0xb7, 0x67, 0xa5, 0x6e, 0xca, 0x77, 0x71, 0x89, 0x9b, 0x03, 0xed, 0xe6, 0xc3,
	0xd6, 0x35, 0x5b, 0x33, 0x33, 0xf5, 0x96, 0x40, 0x6d, 0x6e, 0x17, 0x4b, 0x4d, 0x95, 0xef, 0x6b,
	0xab, 0x91, 0x53, 0xf3, 0x6f, 0x42, 0xfb, 0x28, 0xfd, 0x22, 0xcd, 0x27, 0xb5, 0x7f, 0x4d, 0x3b,
	0xdd, 0xdf, 0x08, 0xdc, 0x1f, 0x8a, 0x70, 0x51, 0xb8, 0x7b, 0xf7, 0x38, 0x3b, 0x9b, 0x5d, 0x3c,
	0x49, 0x75, 0x4e, 0xc8, 0xb7, 0x4f, 0x0d, 0xd3, 0x17, 0x01, 0x8b, 0x7c, 0x5b, 0x24, 0xbe, 0xe3,
	0x63, 0xa4, 0x5d, 0x38, 0x19, 0xc4, 0x62, 0x2e, 0x0b, 0xbf, 0x38, 0x3e, 0x35, 0xc7, 0x7f, 0x09,
	0xf9, 0xa3, 0xf2, 0xe0, 0x45, 0x96, 0xfd, 0x3c, 0x10, 0x93, 0x91, 0x6d, 0x14, 0xec, 0xbe, 0xfb,
	0x67, 0x8e, 0x9c, 0x6a, 0xe4, 0xd4, 0x20, 0xa7, 0x7d, 0x77, 0x50, 0xd5, 0x6f, 0x7f, 0xf4, 0x5f,
	0x00, 0x00, 0x00, 0xff, 0xff, 0xca, 0x84, 0x19, 0x3d, 0xcc, 0x08, 0x00, 0x00,
}
