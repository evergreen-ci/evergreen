// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/appengine/logging/v1/request_log.proto

package logging // import "google.golang.org/genproto/googleapis/appengine/logging/v1"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import duration "github.com/golang/protobuf/ptypes/duration"
import timestamp "github.com/golang/protobuf/ptypes/timestamp"
import _type "google.golang.org/genproto/googleapis/logging/type"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Application log line emitted while processing a request.
type LogLine struct {
	// Approximate time when this log entry was made.
	Time *timestamp.Timestamp `protobuf:"bytes,1,opt,name=time,proto3" json:"time,omitempty"`
	// Severity of this log entry.
	Severity _type.LogSeverity `protobuf:"varint,2,opt,name=severity,proto3,enum=google.logging.type.LogSeverity" json:"severity,omitempty"`
	// App-provided log message.
	LogMessage string `protobuf:"bytes,3,opt,name=log_message,json=logMessage,proto3" json:"log_message,omitempty"`
	// Where in the source code this log message was written.
	SourceLocation       *SourceLocation `protobuf:"bytes,4,opt,name=source_location,json=sourceLocation,proto3" json:"source_location,omitempty"`
	XXX_NoUnkeyedLiteral struct{}        `json:"-"`
	XXX_unrecognized     []byte          `json:"-"`
	XXX_sizecache        int32           `json:"-"`
}

func (m *LogLine) Reset()         { *m = LogLine{} }
func (m *LogLine) String() string { return proto.CompactTextString(m) }
func (*LogLine) ProtoMessage()    {}
func (*LogLine) Descriptor() ([]byte, []int) {
	return fileDescriptor_request_log_c4e4bcec179d2e52, []int{0}
}
func (m *LogLine) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_LogLine.Unmarshal(m, b)
}
func (m *LogLine) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_LogLine.Marshal(b, m, deterministic)
}
func (dst *LogLine) XXX_Merge(src proto.Message) {
	xxx_messageInfo_LogLine.Merge(dst, src)
}
func (m *LogLine) XXX_Size() int {
	return xxx_messageInfo_LogLine.Size(m)
}
func (m *LogLine) XXX_DiscardUnknown() {
	xxx_messageInfo_LogLine.DiscardUnknown(m)
}

var xxx_messageInfo_LogLine proto.InternalMessageInfo

func (m *LogLine) GetTime() *timestamp.Timestamp {
	if m != nil {
		return m.Time
	}
	return nil
}

func (m *LogLine) GetSeverity() _type.LogSeverity {
	if m != nil {
		return m.Severity
	}
	return _type.LogSeverity_DEFAULT
}

func (m *LogLine) GetLogMessage() string {
	if m != nil {
		return m.LogMessage
	}
	return ""
}

func (m *LogLine) GetSourceLocation() *SourceLocation {
	if m != nil {
		return m.SourceLocation
	}
	return nil
}

// Specifies a location in a source code file.
type SourceLocation struct {
	// Source file name. Depending on the runtime environment, this might be a
	// simple name or a fully-qualified name.
	File string `protobuf:"bytes,1,opt,name=file,proto3" json:"file,omitempty"`
	// Line within the source file.
	Line int64 `protobuf:"varint,2,opt,name=line,proto3" json:"line,omitempty"`
	// Human-readable name of the function or method being invoked, with optional
	// context such as the class or package name. This information is used in
	// contexts such as the logs viewer, where a file and line number are less
	// meaningful. The format can vary by language. For example:
	// `qual.if.ied.Class.method` (Java), `dir/package.func` (Go), `function`
	// (Python).
	FunctionName         string   `protobuf:"bytes,3,opt,name=function_name,json=functionName,proto3" json:"function_name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SourceLocation) Reset()         { *m = SourceLocation{} }
func (m *SourceLocation) String() string { return proto.CompactTextString(m) }
func (*SourceLocation) ProtoMessage()    {}
func (*SourceLocation) Descriptor() ([]byte, []int) {
	return fileDescriptor_request_log_c4e4bcec179d2e52, []int{1}
}
func (m *SourceLocation) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SourceLocation.Unmarshal(m, b)
}
func (m *SourceLocation) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SourceLocation.Marshal(b, m, deterministic)
}
func (dst *SourceLocation) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SourceLocation.Merge(dst, src)
}
func (m *SourceLocation) XXX_Size() int {
	return xxx_messageInfo_SourceLocation.Size(m)
}
func (m *SourceLocation) XXX_DiscardUnknown() {
	xxx_messageInfo_SourceLocation.DiscardUnknown(m)
}

var xxx_messageInfo_SourceLocation proto.InternalMessageInfo

func (m *SourceLocation) GetFile() string {
	if m != nil {
		return m.File
	}
	return ""
}

func (m *SourceLocation) GetLine() int64 {
	if m != nil {
		return m.Line
	}
	return 0
}

func (m *SourceLocation) GetFunctionName() string {
	if m != nil {
		return m.FunctionName
	}
	return ""
}

// A reference to a particular snapshot of the source tree used to build and
// deploy an application.
type SourceReference struct {
	// Optional. A URI string identifying the repository.
	// Example: "https://github.com/GoogleCloudPlatform/kubernetes.git"
	Repository string `protobuf:"bytes,1,opt,name=repository,proto3" json:"repository,omitempty"`
	// The canonical and persistent identifier of the deployed revision.
	// Example (git): "0035781c50ec7aa23385dc841529ce8a4b70db1b"
	RevisionId           string   `protobuf:"bytes,2,opt,name=revision_id,json=revisionId,proto3" json:"revision_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SourceReference) Reset()         { *m = SourceReference{} }
func (m *SourceReference) String() string { return proto.CompactTextString(m) }
func (*SourceReference) ProtoMessage()    {}
func (*SourceReference) Descriptor() ([]byte, []int) {
	return fileDescriptor_request_log_c4e4bcec179d2e52, []int{2}
}
func (m *SourceReference) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SourceReference.Unmarshal(m, b)
}
func (m *SourceReference) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SourceReference.Marshal(b, m, deterministic)
}
func (dst *SourceReference) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SourceReference.Merge(dst, src)
}
func (m *SourceReference) XXX_Size() int {
	return xxx_messageInfo_SourceReference.Size(m)
}
func (m *SourceReference) XXX_DiscardUnknown() {
	xxx_messageInfo_SourceReference.DiscardUnknown(m)
}

var xxx_messageInfo_SourceReference proto.InternalMessageInfo

func (m *SourceReference) GetRepository() string {
	if m != nil {
		return m.Repository
	}
	return ""
}

func (m *SourceReference) GetRevisionId() string {
	if m != nil {
		return m.RevisionId
	}
	return ""
}

// Complete log information about a single HTTP request to an App Engine
// application.
type RequestLog struct {
	// Application that handled this request.
	AppId string `protobuf:"bytes,1,opt,name=app_id,json=appId,proto3" json:"app_id,omitempty"`
	// Module of the application that handled this request.
	ModuleId string `protobuf:"bytes,37,opt,name=module_id,json=moduleId,proto3" json:"module_id,omitempty"`
	// Version of the application that handled this request.
	VersionId string `protobuf:"bytes,2,opt,name=version_id,json=versionId,proto3" json:"version_id,omitempty"`
	// Globally unique identifier for a request, which is based on the request
	// start time.  Request IDs for requests which started later will compare
	// greater as strings than those for requests which started earlier.
	RequestId string `protobuf:"bytes,3,opt,name=request_id,json=requestId,proto3" json:"request_id,omitempty"`
	// Origin IP address.
	Ip string `protobuf:"bytes,4,opt,name=ip,proto3" json:"ip,omitempty"`
	// Time when the request started.
	StartTime *timestamp.Timestamp `protobuf:"bytes,6,opt,name=start_time,json=startTime,proto3" json:"start_time,omitempty"`
	// Time when the request finished.
	EndTime *timestamp.Timestamp `protobuf:"bytes,7,opt,name=end_time,json=endTime,proto3" json:"end_time,omitempty"`
	// Latency of the request.
	Latency *duration.Duration `protobuf:"bytes,8,opt,name=latency,proto3" json:"latency,omitempty"`
	// Number of CPU megacycles used to process request.
	MegaCycles int64 `protobuf:"varint,9,opt,name=mega_cycles,json=megaCycles,proto3" json:"mega_cycles,omitempty"`
	// Request method. Example: `"GET"`, `"HEAD"`, `"PUT"`, `"POST"`, `"DELETE"`.
	Method string `protobuf:"bytes,10,opt,name=method,proto3" json:"method,omitempty"`
	// Contains the path and query portion of the URL that was requested. For
	// example, if the URL was "http://example.com/app?name=val", the resource
	// would be "/app?name=val".  The fragment identifier, which is identified by
	// the `#` character, is not included.
	Resource string `protobuf:"bytes,11,opt,name=resource,proto3" json:"resource,omitempty"`
	// HTTP version of request. Example: `"HTTP/1.1"`.
	HttpVersion string `protobuf:"bytes,12,opt,name=http_version,json=httpVersion,proto3" json:"http_version,omitempty"`
	// HTTP response status code. Example: 200, 404.
	Status int32 `protobuf:"varint,13,opt,name=status,proto3" json:"status,omitempty"`
	// Size in bytes sent back to client by request.
	ResponseSize int64 `protobuf:"varint,14,opt,name=response_size,json=responseSize,proto3" json:"response_size,omitempty"`
	// Referrer URL of request.
	Referrer string `protobuf:"bytes,15,opt,name=referrer,proto3" json:"referrer,omitempty"`
	// User agent that made the request.
	UserAgent string `protobuf:"bytes,16,opt,name=user_agent,json=userAgent,proto3" json:"user_agent,omitempty"`
	// The logged-in user who made the request.
	//
	// Most likely, this is the part of the user's email before the `@` sign.  The
	// field value is the same for different requests from the same user, but
	// different users can have similar names.  This information is also
	// available to the application via the App Engine Users API.
	//
	// This field will be populated starting with App Engine 1.9.21.
	Nickname string `protobuf:"bytes,40,opt,name=nickname,proto3" json:"nickname,omitempty"`
	// File or class that handled the request.
	UrlMapEntry string `protobuf:"bytes,17,opt,name=url_map_entry,json=urlMapEntry,proto3" json:"url_map_entry,omitempty"`
	// Internet host and port number of the resource being requested.
	Host string `protobuf:"bytes,20,opt,name=host,proto3" json:"host,omitempty"`
	// An indication of the relative cost of serving this request.
	Cost float64 `protobuf:"fixed64,21,opt,name=cost,proto3" json:"cost,omitempty"`
	// Queue name of the request, in the case of an offline request.
	TaskQueueName string `protobuf:"bytes,22,opt,name=task_queue_name,json=taskQueueName,proto3" json:"task_queue_name,omitempty"`
	// Task name of the request, in the case of an offline request.
	TaskName string `protobuf:"bytes,23,opt,name=task_name,json=taskName,proto3" json:"task_name,omitempty"`
	// Whether this was a loading request for the instance.
	WasLoadingRequest bool `protobuf:"varint,24,opt,name=was_loading_request,json=wasLoadingRequest,proto3" json:"was_loading_request,omitempty"`
	// Time this request spent in the pending request queue.
	PendingTime *duration.Duration `protobuf:"bytes,25,opt,name=pending_time,json=pendingTime,proto3" json:"pending_time,omitempty"`
	// If the instance processing this request belongs to a manually scaled
	// module, then this is the 0-based index of the instance. Otherwise, this
	// value is -1.
	InstanceIndex int32 `protobuf:"varint,26,opt,name=instance_index,json=instanceIndex,proto3" json:"instance_index,omitempty"`
	// Whether this request is finished or active.
	Finished bool `protobuf:"varint,27,opt,name=finished,proto3" json:"finished,omitempty"`
	// Whether this is the first `RequestLog` entry for this request.  If an
	// active request has several `RequestLog` entries written to Stackdriver
	// Logging, then this field will be set for one of them.
	First bool `protobuf:"varint,42,opt,name=first,proto3" json:"first,omitempty"`
	// An identifier for the instance that handled the request.
	InstanceId string `protobuf:"bytes,28,opt,name=instance_id,json=instanceId,proto3" json:"instance_id,omitempty"`
	// A list of log lines emitted by the application while serving this request.
	Line []*LogLine `protobuf:"bytes,29,rep,name=line,proto3" json:"line,omitempty"`
	// App Engine release version.
	AppEngineRelease string `protobuf:"bytes,38,opt,name=app_engine_release,json=appEngineRelease,proto3" json:"app_engine_release,omitempty"`
	// Stackdriver Trace identifier for this request.
	TraceId string `protobuf:"bytes,39,opt,name=trace_id,json=traceId,proto3" json:"trace_id,omitempty"`
	// Source code for the application that handled this request. There can be
	// more than one source reference per deployed application if source code is
	// distributed among multiple repositories.
	SourceReference      []*SourceReference `protobuf:"bytes,41,rep,name=source_reference,json=sourceReference,proto3" json:"source_reference,omitempty"`
	XXX_NoUnkeyedLiteral struct{}           `json:"-"`
	XXX_unrecognized     []byte             `json:"-"`
	XXX_sizecache        int32              `json:"-"`
}

func (m *RequestLog) Reset()         { *m = RequestLog{} }
func (m *RequestLog) String() string { return proto.CompactTextString(m) }
func (*RequestLog) ProtoMessage()    {}
func (*RequestLog) Descriptor() ([]byte, []int) {
	return fileDescriptor_request_log_c4e4bcec179d2e52, []int{3}
}
func (m *RequestLog) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RequestLog.Unmarshal(m, b)
}
func (m *RequestLog) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RequestLog.Marshal(b, m, deterministic)
}
func (dst *RequestLog) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RequestLog.Merge(dst, src)
}
func (m *RequestLog) XXX_Size() int {
	return xxx_messageInfo_RequestLog.Size(m)
}
func (m *RequestLog) XXX_DiscardUnknown() {
	xxx_messageInfo_RequestLog.DiscardUnknown(m)
}

var xxx_messageInfo_RequestLog proto.InternalMessageInfo

func (m *RequestLog) GetAppId() string {
	if m != nil {
		return m.AppId
	}
	return ""
}

func (m *RequestLog) GetModuleId() string {
	if m != nil {
		return m.ModuleId
	}
	return ""
}

func (m *RequestLog) GetVersionId() string {
	if m != nil {
		return m.VersionId
	}
	return ""
}

func (m *RequestLog) GetRequestId() string {
	if m != nil {
		return m.RequestId
	}
	return ""
}

func (m *RequestLog) GetIp() string {
	if m != nil {
		return m.Ip
	}
	return ""
}

func (m *RequestLog) GetStartTime() *timestamp.Timestamp {
	if m != nil {
		return m.StartTime
	}
	return nil
}

func (m *RequestLog) GetEndTime() *timestamp.Timestamp {
	if m != nil {
		return m.EndTime
	}
	return nil
}

func (m *RequestLog) GetLatency() *duration.Duration {
	if m != nil {
		return m.Latency
	}
	return nil
}

func (m *RequestLog) GetMegaCycles() int64 {
	if m != nil {
		return m.MegaCycles
	}
	return 0
}

func (m *RequestLog) GetMethod() string {
	if m != nil {
		return m.Method
	}
	return ""
}

func (m *RequestLog) GetResource() string {
	if m != nil {
		return m.Resource
	}
	return ""
}

func (m *RequestLog) GetHttpVersion() string {
	if m != nil {
		return m.HttpVersion
	}
	return ""
}

func (m *RequestLog) GetStatus() int32 {
	if m != nil {
		return m.Status
	}
	return 0
}

func (m *RequestLog) GetResponseSize() int64 {
	if m != nil {
		return m.ResponseSize
	}
	return 0
}

func (m *RequestLog) GetReferrer() string {
	if m != nil {
		return m.Referrer
	}
	return ""
}

func (m *RequestLog) GetUserAgent() string {
	if m != nil {
		return m.UserAgent
	}
	return ""
}

func (m *RequestLog) GetNickname() string {
	if m != nil {
		return m.Nickname
	}
	return ""
}

func (m *RequestLog) GetUrlMapEntry() string {
	if m != nil {
		return m.UrlMapEntry
	}
	return ""
}

func (m *RequestLog) GetHost() string {
	if m != nil {
		return m.Host
	}
	return ""
}

func (m *RequestLog) GetCost() float64 {
	if m != nil {
		return m.Cost
	}
	return 0
}

func (m *RequestLog) GetTaskQueueName() string {
	if m != nil {
		return m.TaskQueueName
	}
	return ""
}

func (m *RequestLog) GetTaskName() string {
	if m != nil {
		return m.TaskName
	}
	return ""
}

func (m *RequestLog) GetWasLoadingRequest() bool {
	if m != nil {
		return m.WasLoadingRequest
	}
	return false
}

func (m *RequestLog) GetPendingTime() *duration.Duration {
	if m != nil {
		return m.PendingTime
	}
	return nil
}

func (m *RequestLog) GetInstanceIndex() int32 {
	if m != nil {
		return m.InstanceIndex
	}
	return 0
}

func (m *RequestLog) GetFinished() bool {
	if m != nil {
		return m.Finished
	}
	return false
}

func (m *RequestLog) GetFirst() bool {
	if m != nil {
		return m.First
	}
	return false
}

func (m *RequestLog) GetInstanceId() string {
	if m != nil {
		return m.InstanceId
	}
	return ""
}

func (m *RequestLog) GetLine() []*LogLine {
	if m != nil {
		return m.Line
	}
	return nil
}

func (m *RequestLog) GetAppEngineRelease() string {
	if m != nil {
		return m.AppEngineRelease
	}
	return ""
}

func (m *RequestLog) GetTraceId() string {
	if m != nil {
		return m.TraceId
	}
	return ""
}

func (m *RequestLog) GetSourceReference() []*SourceReference {
	if m != nil {
		return m.SourceReference
	}
	return nil
}

func init() {
	proto.RegisterType((*LogLine)(nil), "google.appengine.logging.v1.LogLine")
	proto.RegisterType((*SourceLocation)(nil), "google.appengine.logging.v1.SourceLocation")
	proto.RegisterType((*SourceReference)(nil), "google.appengine.logging.v1.SourceReference")
	proto.RegisterType((*RequestLog)(nil), "google.appengine.logging.v1.RequestLog")
}

func init() {
	proto.RegisterFile("google/appengine/logging/v1/request_log.proto", fileDescriptor_request_log_c4e4bcec179d2e52)
}

var fileDescriptor_request_log_c4e4bcec179d2e52 = []byte{
	// 921 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x55, 0x7f, 0x6f, 0x1c, 0x35,
	0x10, 0xd5, 0xe6, 0xe7, 0x9d, 0x2f, 0xb9, 0xa4, 0xa6, 0x2d, 0x4e, 0x42, 0x9b, 0x23, 0xd0, 0x70,
	0x40, 0xd9, 0x53, 0x53, 0x21, 0x81, 0xe8, 0x3f, 0x04, 0xfa, 0xc7, 0x49, 0x57, 0x54, 0x36, 0x15,
	0x48, 0x48, 0x68, 0xe5, 0xee, 0xce, 0x6d, 0xac, 0xee, 0xda, 0xae, 0xed, 0xbd, 0xf6, 0xfa, 0x41,
	0xf8, 0x98, 0x7c, 0x06, 0xe4, 0xb1, 0xf7, 0x4a, 0x00, 0x25, 0xfc, 0xb7, 0xf3, 0xe6, 0xbd, 0xf1,
	0xd8, 0x7e, 0x9e, 0x25, 0x5f, 0x55, 0x4a, 0x55, 0x35, 0x4c, 0xb8, 0xd6, 0x20, 0x2b, 0x21, 0x61,
	0x52, 0xab, 0xaa, 0x12, 0xb2, 0x9a, 0x2c, 0x1e, 0x4d, 0x0c, 0xbc, 0x6e, 0xc1, 0xba, 0xbc, 0x56,
	0x55, 0xaa, 0x8d, 0x72, 0x8a, 0x1e, 0x05, 0x7a, 0xba, 0xa2, 0xa7, 0x91, 0x9e, 0x2e, 0x1e, 0x1d,
	0x9e, 0xc6, 0x5a, 0x5d, 0x05, 0xb7, 0xd4, 0x18, 0xe4, 0x16, 0x16, 0x60, 0x84, 0x5b, 0x86, 0x22,
	0x87, 0xf7, 0x23, 0x0f, 0xa3, 0x97, 0xed, 0x7c, 0x52, 0xb6, 0x86, 0x3b, 0xa1, 0x64, 0xcc, 0x1f,
	0xff, 0x33, 0xef, 0x44, 0x03, 0xd6, 0xf1, 0x46, 0x07, 0xc2, 0xc9, 0x9f, 0x09, 0xd9, 0x9e, 0xa9,
	0x6a, 0x26, 0x24, 0xd0, 0x94, 0x6c, 0xf8, 0x34, 0x4b, 0x46, 0xc9, 0x78, 0x70, 0x76, 0x98, 0xc6,
	0x06, 0x3b, 0x6d, 0xfa, 0xa2, 0xd3, 0x66, 0xc8, 0xa3, 0x4f, 0x48, 0xaf, 0x6b, 0x87, 0xad, 0x8d,
	0x92, 0xf1, 0xf0, 0x6c, 0xd4, 0x69, 0xba, 0xad, 0xf8, 0xbe, 0xd3, 0x99, 0xaa, 0x2e, 0x22, 0x2f,
	0x5b, 0x29, 0xe8, 0x31, 0x19, 0xf8, 0x0d, 0x35, 0x60, 0x2d, 0xaf, 0x80, 0xad, 0x8f, 0x92, 0x71,
	0x3f, 0x23, 0xb5, 0xaa, 0x9e, 0x05, 0x84, 0xbe, 0x20, 0x7b, 0x56, 0xb5, 0xa6, 0x80, 0xbc, 0x56,
	0x05, 0x6e, 0x8a, 0x6d, 0x60, 0x67, 0x5f, 0xa6, 0xd7, 0x1c, 0x5d, 0x7a, 0x81, 0x9a, 0x59, 0x94,
	0x64, 0x43, 0x7b, 0x25, 0x3e, 0xf9, 0x9d, 0x0c, 0xaf, 0x32, 0x28, 0x25, 0x1b, 0x73, 0x51, 0x87,
	0x6d, 0xf7, 0x33, 0xfc, 0xf6, 0x58, 0x2d, 0x24, 0xe0, 0xb6, 0xd6, 0x33, 0xfc, 0xa6, 0x9f, 0x90,
	0xdd, 0x79, 0x2b, 0x0b, 0xaf, 0xc9, 0x25, 0x6f, 0xba, 0x96, 0x77, 0x3a, 0xf0, 0x27, 0xde, 0xc0,
	0x49, 0x46, 0xf6, 0x42, 0xf9, 0x0c, 0xe6, 0x60, 0x40, 0x16, 0x40, 0xef, 0x13, 0x62, 0x40, 0x2b,
	0x2b, 0x9c, 0x32, 0xcb, 0xb8, 0xca, 0xdf, 0x10, 0x7f, 0x10, 0x06, 0x16, 0xc2, 0xfa, 0xba, 0xa2,
	0xc4, 0x25, 0x91, 0x10, 0xa0, 0x69, 0x79, 0xf2, 0x47, 0x9f, 0x90, 0x2c, 0xf8, 0x67, 0xa6, 0x2a,
	0x7a, 0x87, 0x6c, 0x71, 0xad, 0x3d, 0x35, 0xd4, 0xda, 0xe4, 0x5a, 0x4f, 0x4b, 0x7a, 0x44, 0xfa,
	0x8d, 0x2a, 0xdb, 0x1a, 0x7c, 0xe6, 0x01, 0x66, 0x7a, 0x01, 0x98, 0x96, 0xf4, 0x1e, 0x21, 0x0b,
	0x30, 0x57, 0x97, 0xe8, 0x47, 0x24, 0xa4, 0x3b, 0x83, 0x8a, 0x32, 0xee, 0xab, 0x1f, 0x91, 0x69,
	0x49, 0x87, 0x64, 0x4d, 0x68, 0x3c, 0xfc, 0x7e, 0xb6, 0x26, 0x34, 0xfd, 0x96, 0x10, 0xeb, 0xb8,
	0x71, 0x39, 0xda, 0x65, 0xeb, 0x46, 0xbb, 0xf4, 0x91, 0xed, 0x63, 0xfa, 0x35, 0xe9, 0x81, 0x2c,
	0x83, 0x70, 0xfb, 0x46, 0xe1, 0x36, 0xc8, 0x12, 0x65, 0x8f, 0xc9, 0x76, 0xcd, 0x1d, 0xc8, 0x62,
	0xc9, 0x7a, 0xa8, 0x3a, 0xf8, 0x97, 0xea, 0xc7, 0xe8, 0xfc, 0xac, 0x63, 0xfa, 0x83, 0x6d, 0xa0,
	0xe2, 0x79, 0xb1, 0x2c, 0x6a, 0xb0, 0xac, 0x8f, 0x77, 0x49, 0x3c, 0xf4, 0x03, 0x22, 0xf4, 0x2e,
	0xd9, 0x6a, 0xc0, 0x5d, 0xaa, 0x92, 0x11, 0xdc, 0x5b, 0x8c, 0xe8, 0x21, 0xe9, 0x19, 0x08, 0xbe,
	0x61, 0x83, 0x70, 0x92, 0x5d, 0x4c, 0x3f, 0x26, 0x3b, 0x97, 0xce, 0xe9, 0x3c, 0x1e, 0x1e, 0xdb,
	0xc1, 0xfc, 0xc0, 0x63, 0xbf, 0x04, 0xc8, 0x97, 0xb5, 0x8e, 0xbb, 0xd6, 0xb2, 0xdd, 0x51, 0x32,
	0xde, 0xcc, 0x62, 0xe4, 0x0d, 0x64, 0xc0, 0x6a, 0x25, 0x2d, 0xe4, 0x56, 0xbc, 0x03, 0x36, 0xc4,
	0x8e, 0x76, 0x3a, 0xf0, 0x42, 0xbc, 0x83, 0xb0, 0xf6, 0x1c, 0x8c, 0x01, 0xc3, 0xf6, 0xba, 0xb5,
	0x43, 0xec, 0xaf, 0xa9, 0xb5, 0x60, 0x72, 0x5e, 0x81, 0x74, 0x6c, 0x3f, 0x5c, 0x93, 0x47, 0xbe,
	0xf7, 0x80, 0x97, 0x4a, 0x51, 0xbc, 0x42, 0x6f, 0x8e, 0x83, 0xb4, 0x8b, 0xe9, 0x09, 0xd9, 0x6d,
	0x4d, 0x9d, 0x37, 0x5c, 0xe7, 0x20, 0x9d, 0x59, 0xb2, 0x5b, 0xa1, 0xef, 0xd6, 0xd4, 0xcf, 0xb8,
	0x7e, 0xea, 0x21, 0x6f, 0xfa, 0x4b, 0x65, 0x1d, 0xbb, 0x1d, 0x1e, 0x82, 0xff, 0xf6, 0x58, 0xe1,
	0xb1, 0x3b, 0xa3, 0x64, 0x9c, 0x64, 0xf8, 0x4d, 0x4f, 0xc9, 0x9e, 0xe3, 0xf6, 0x55, 0xfe, 0xba,
	0x85, 0x16, 0xc2, 0x53, 0xb8, 0x8b, 0x92, 0x5d, 0x0f, 0xff, 0xec, 0x51, 0xff, 0x16, 0xbc, 0x23,
	0x91, 0x87, 0x8c, 0x0f, 0x43, 0x43, 0x1e, 0xc0, 0x64, 0x4a, 0x3e, 0x78, 0xc3, 0x6d, 0x5e, 0x2b,
	0x5e, 0x0a, 0x59, 0xe5, 0xd1, 0x6c, 0x8c, 0x8d, 0x92, 0x71, 0x2f, 0xbb, 0xf5, 0x86, 0xdb, 0x59,
	0xc8, 0x44, 0xe3, 0xd3, 0x27, 0x64, 0x47, 0x83, 0x44, 0x2e, 0x9a, 0xe7, 0xe0, 0x26, 0x1b, 0x0c,
	0x22, 0x1d, 0xfd, 0xf3, 0x80, 0x0c, 0x85, 0xb4, 0x8e, 0xcb, 0x02, 0x72, 0x21, 0x4b, 0x78, 0xcb,
	0x0e, 0xf1, 0x6a, 0x76, 0x3b, 0x74, 0xea, 0x41, 0x7f, 0x82, 0x73, 0x21, 0x85, 0xbd, 0x84, 0x92,
	0x1d, 0x61, 0x27, 0xab, 0x98, 0xde, 0x26, 0x9b, 0x73, 0x61, 0xac, 0x63, 0x5f, 0x60, 0x22, 0x04,
	0xde, 0x63, 0xef, 0x0b, 0x97, 0xec, 0xa3, 0xf0, 0x78, 0x57, 0x55, 0x4b, 0xfa, 0x4d, 0x9c, 0x24,
	0xf7, 0x46, 0xeb, 0xe3, 0xc1, 0xd9, 0xa7, 0xd7, 0x8e, 0xae, 0x38, 0x88, 0xe3, 0xbc, 0x79, 0x48,
	0xa8, 0x7f, 0xe7, 0x81, 0x96, 0x1b, 0xa8, 0x81, 0x5b, 0x60, 0xa7, 0xb8, 0xc2, 0x3e, 0xd7, 0xfa,
	0x29, 0x26, 0xb2, 0x80, 0xd3, 0x03, 0xd2, 0x73, 0x86, 0x87, 0x2e, 0x3e, 0x43, 0xce, 0x36, 0xc6,
	0xd3, 0x92, 0xfe, 0x4a, 0xf6, 0xe3, 0x20, 0x35, 0xdd, 0x50, 0x62, 0x9f, 0x63, 0x3b, 0x0f, 0xff,
	0xc7, 0x24, 0x5d, 0x0d, 0xb2, 0x2c, 0x8e, 0xe3, 0x15, 0x70, 0xfe, 0x96, 0x1c, 0x17, 0xaa, 0xb9,
	0xae, 0xc6, 0xf9, 0xde, 0xfb, 0xc1, 0xf5, 0xdc, 0x5f, 0xd1, 0xf3, 0xe4, 0xb7, 0xf3, 0xc8, 0xaf,
	0x54, 0xcd, 0x65, 0x95, 0x2a, 0x53, 0x4d, 0x2a, 0x90, 0x78, 0x81, 0x93, 0x90, 0xe2, 0x5a, 0xd8,
	0xff, 0xfc, 0x8d, 0x7e, 0x17, 0x3f, 0x5f, 0x6e, 0x21, 0xfd, 0xf1, 0x5f, 0x01, 0x00, 0x00, 0xff,
	0xff, 0x05, 0xf7, 0x68, 0xa8, 0x74, 0x07, 0x00, 0x00,
}
