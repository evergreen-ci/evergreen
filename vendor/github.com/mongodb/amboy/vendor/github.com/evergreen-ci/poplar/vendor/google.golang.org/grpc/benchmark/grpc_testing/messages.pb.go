// Code generated by protoc-gen-go. DO NOT EDIT.
// source: messages.proto

package grpc_testing

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

// The type of payload that should be returned.
type PayloadType int32

const (
	// Compressable text format.
	PayloadType_COMPRESSABLE PayloadType = 0
	// Uncompressable binary format.
	PayloadType_UNCOMPRESSABLE PayloadType = 1
	// Randomly chosen from all other formats defined in this enum.
	PayloadType_RANDOM PayloadType = 2
)

var PayloadType_name = map[int32]string{
	0: "COMPRESSABLE",
	1: "UNCOMPRESSABLE",
	2: "RANDOM",
}

var PayloadType_value = map[string]int32{
	"COMPRESSABLE":   0,
	"UNCOMPRESSABLE": 1,
	"RANDOM":         2,
}

func (x PayloadType) String() string {
	return proto.EnumName(PayloadType_name, int32(x))
}

func (PayloadType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{0}
}

// Compression algorithms
type CompressionType int32

const (
	// No compression
	CompressionType_NONE    CompressionType = 0
	CompressionType_GZIP    CompressionType = 1
	CompressionType_DEFLATE CompressionType = 2
)

var CompressionType_name = map[int32]string{
	0: "NONE",
	1: "GZIP",
	2: "DEFLATE",
}

var CompressionType_value = map[string]int32{
	"NONE":    0,
	"GZIP":    1,
	"DEFLATE": 2,
}

func (x CompressionType) String() string {
	return proto.EnumName(CompressionType_name, int32(x))
}

func (CompressionType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{1}
}

// A block of data, to simply increase gRPC message size.
type Payload struct {
	// The type of data in body.
	Type PayloadType `protobuf:"varint,1,opt,name=type,proto3,enum=grpc.testing.PayloadType" json:"type,omitempty"`
	// Primary contents of payload.
	Body                 []byte   `protobuf:"bytes,2,opt,name=body,proto3" json:"body,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Payload) Reset()         { *m = Payload{} }
func (m *Payload) String() string { return proto.CompactTextString(m) }
func (*Payload) ProtoMessage()    {}
func (*Payload) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{0}
}

func (m *Payload) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Payload.Unmarshal(m, b)
}
func (m *Payload) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Payload.Marshal(b, m, deterministic)
}
func (m *Payload) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Payload.Merge(m, src)
}
func (m *Payload) XXX_Size() int {
	return xxx_messageInfo_Payload.Size(m)
}
func (m *Payload) XXX_DiscardUnknown() {
	xxx_messageInfo_Payload.DiscardUnknown(m)
}

var xxx_messageInfo_Payload proto.InternalMessageInfo

func (m *Payload) GetType() PayloadType {
	if m != nil {
		return m.Type
	}
	return PayloadType_COMPRESSABLE
}

func (m *Payload) GetBody() []byte {
	if m != nil {
		return m.Body
	}
	return nil
}

// A protobuf representation for grpc status. This is used by test
// clients to specify a status that the server should attempt to return.
type EchoStatus struct {
	Code                 int32    `protobuf:"varint,1,opt,name=code,proto3" json:"code,omitempty"`
	Message              string   `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *EchoStatus) Reset()         { *m = EchoStatus{} }
func (m *EchoStatus) String() string { return proto.CompactTextString(m) }
func (*EchoStatus) ProtoMessage()    {}
func (*EchoStatus) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{1}
}

func (m *EchoStatus) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_EchoStatus.Unmarshal(m, b)
}
func (m *EchoStatus) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_EchoStatus.Marshal(b, m, deterministic)
}
func (m *EchoStatus) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EchoStatus.Merge(m, src)
}
func (m *EchoStatus) XXX_Size() int {
	return xxx_messageInfo_EchoStatus.Size(m)
}
func (m *EchoStatus) XXX_DiscardUnknown() {
	xxx_messageInfo_EchoStatus.DiscardUnknown(m)
}

var xxx_messageInfo_EchoStatus proto.InternalMessageInfo

func (m *EchoStatus) GetCode() int32 {
	if m != nil {
		return m.Code
	}
	return 0
}

func (m *EchoStatus) GetMessage() string {
	if m != nil {
		return m.Message
	}
	return ""
}

// Unary request.
type SimpleRequest struct {
	// Desired payload type in the response from the server.
	// If response_type is RANDOM, server randomly chooses one from other formats.
	ResponseType PayloadType `protobuf:"varint,1,opt,name=response_type,json=responseType,proto3,enum=grpc.testing.PayloadType" json:"response_type,omitempty"`
	// Desired payload size in the response from the server.
	// If response_type is COMPRESSABLE, this denotes the size before compression.
	ResponseSize int32 `protobuf:"varint,2,opt,name=response_size,json=responseSize,proto3" json:"response_size,omitempty"`
	// Optional input payload sent along with the request.
	Payload *Payload `protobuf:"bytes,3,opt,name=payload,proto3" json:"payload,omitempty"`
	// Whether SimpleResponse should include username.
	FillUsername bool `protobuf:"varint,4,opt,name=fill_username,json=fillUsername,proto3" json:"fill_username,omitempty"`
	// Whether SimpleResponse should include OAuth scope.
	FillOauthScope bool `protobuf:"varint,5,opt,name=fill_oauth_scope,json=fillOauthScope,proto3" json:"fill_oauth_scope,omitempty"`
	// Compression algorithm to be used by the server for the response (stream)
	ResponseCompression CompressionType `protobuf:"varint,6,opt,name=response_compression,json=responseCompression,proto3,enum=grpc.testing.CompressionType" json:"response_compression,omitempty"`
	// Whether server should return a given status
	ResponseStatus       *EchoStatus `protobuf:"bytes,7,opt,name=response_status,json=responseStatus,proto3" json:"response_status,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *SimpleRequest) Reset()         { *m = SimpleRequest{} }
func (m *SimpleRequest) String() string { return proto.CompactTextString(m) }
func (*SimpleRequest) ProtoMessage()    {}
func (*SimpleRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{2}
}

func (m *SimpleRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SimpleRequest.Unmarshal(m, b)
}
func (m *SimpleRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SimpleRequest.Marshal(b, m, deterministic)
}
func (m *SimpleRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SimpleRequest.Merge(m, src)
}
func (m *SimpleRequest) XXX_Size() int {
	return xxx_messageInfo_SimpleRequest.Size(m)
}
func (m *SimpleRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_SimpleRequest.DiscardUnknown(m)
}

var xxx_messageInfo_SimpleRequest proto.InternalMessageInfo

func (m *SimpleRequest) GetResponseType() PayloadType {
	if m != nil {
		return m.ResponseType
	}
	return PayloadType_COMPRESSABLE
}

func (m *SimpleRequest) GetResponseSize() int32 {
	if m != nil {
		return m.ResponseSize
	}
	return 0
}

func (m *SimpleRequest) GetPayload() *Payload {
	if m != nil {
		return m.Payload
	}
	return nil
}

func (m *SimpleRequest) GetFillUsername() bool {
	if m != nil {
		return m.FillUsername
	}
	return false
}

func (m *SimpleRequest) GetFillOauthScope() bool {
	if m != nil {
		return m.FillOauthScope
	}
	return false
}

func (m *SimpleRequest) GetResponseCompression() CompressionType {
	if m != nil {
		return m.ResponseCompression
	}
	return CompressionType_NONE
}

func (m *SimpleRequest) GetResponseStatus() *EchoStatus {
	if m != nil {
		return m.ResponseStatus
	}
	return nil
}

// Unary response, as configured by the request.
type SimpleResponse struct {
	// Payload to increase message size.
	Payload *Payload `protobuf:"bytes,1,opt,name=payload,proto3" json:"payload,omitempty"`
	// The user the request came from, for verifying authentication was
	// successful when the client expected it.
	Username string `protobuf:"bytes,2,opt,name=username,proto3" json:"username,omitempty"`
	// OAuth scope.
	OauthScope           string   `protobuf:"bytes,3,opt,name=oauth_scope,json=oauthScope,proto3" json:"oauth_scope,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SimpleResponse) Reset()         { *m = SimpleResponse{} }
func (m *SimpleResponse) String() string { return proto.CompactTextString(m) }
func (*SimpleResponse) ProtoMessage()    {}
func (*SimpleResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{3}
}

func (m *SimpleResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SimpleResponse.Unmarshal(m, b)
}
func (m *SimpleResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SimpleResponse.Marshal(b, m, deterministic)
}
func (m *SimpleResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SimpleResponse.Merge(m, src)
}
func (m *SimpleResponse) XXX_Size() int {
	return xxx_messageInfo_SimpleResponse.Size(m)
}
func (m *SimpleResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_SimpleResponse.DiscardUnknown(m)
}

var xxx_messageInfo_SimpleResponse proto.InternalMessageInfo

func (m *SimpleResponse) GetPayload() *Payload {
	if m != nil {
		return m.Payload
	}
	return nil
}

func (m *SimpleResponse) GetUsername() string {
	if m != nil {
		return m.Username
	}
	return ""
}

func (m *SimpleResponse) GetOauthScope() string {
	if m != nil {
		return m.OauthScope
	}
	return ""
}

// Client-streaming request.
type StreamingInputCallRequest struct {
	// Optional input payload sent along with the request.
	Payload              *Payload `protobuf:"bytes,1,opt,name=payload,proto3" json:"payload,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StreamingInputCallRequest) Reset()         { *m = StreamingInputCallRequest{} }
func (m *StreamingInputCallRequest) String() string { return proto.CompactTextString(m) }
func (*StreamingInputCallRequest) ProtoMessage()    {}
func (*StreamingInputCallRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{4}
}

func (m *StreamingInputCallRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StreamingInputCallRequest.Unmarshal(m, b)
}
func (m *StreamingInputCallRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StreamingInputCallRequest.Marshal(b, m, deterministic)
}
func (m *StreamingInputCallRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StreamingInputCallRequest.Merge(m, src)
}
func (m *StreamingInputCallRequest) XXX_Size() int {
	return xxx_messageInfo_StreamingInputCallRequest.Size(m)
}
func (m *StreamingInputCallRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_StreamingInputCallRequest.DiscardUnknown(m)
}

var xxx_messageInfo_StreamingInputCallRequest proto.InternalMessageInfo

func (m *StreamingInputCallRequest) GetPayload() *Payload {
	if m != nil {
		return m.Payload
	}
	return nil
}

// Client-streaming response.
type StreamingInputCallResponse struct {
	// Aggregated size of payloads received from the client.
	AggregatedPayloadSize int32    `protobuf:"varint,1,opt,name=aggregated_payload_size,json=aggregatedPayloadSize,proto3" json:"aggregated_payload_size,omitempty"`
	XXX_NoUnkeyedLiteral  struct{} `json:"-"`
	XXX_unrecognized      []byte   `json:"-"`
	XXX_sizecache         int32    `json:"-"`
}

func (m *StreamingInputCallResponse) Reset()         { *m = StreamingInputCallResponse{} }
func (m *StreamingInputCallResponse) String() string { return proto.CompactTextString(m) }
func (*StreamingInputCallResponse) ProtoMessage()    {}
func (*StreamingInputCallResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{5}
}

func (m *StreamingInputCallResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StreamingInputCallResponse.Unmarshal(m, b)
}
func (m *StreamingInputCallResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StreamingInputCallResponse.Marshal(b, m, deterministic)
}
func (m *StreamingInputCallResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StreamingInputCallResponse.Merge(m, src)
}
func (m *StreamingInputCallResponse) XXX_Size() int {
	return xxx_messageInfo_StreamingInputCallResponse.Size(m)
}
func (m *StreamingInputCallResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_StreamingInputCallResponse.DiscardUnknown(m)
}

var xxx_messageInfo_StreamingInputCallResponse proto.InternalMessageInfo

func (m *StreamingInputCallResponse) GetAggregatedPayloadSize() int32 {
	if m != nil {
		return m.AggregatedPayloadSize
	}
	return 0
}

// Configuration for a particular response.
type ResponseParameters struct {
	// Desired payload sizes in responses from the server.
	// If response_type is COMPRESSABLE, this denotes the size before compression.
	Size int32 `protobuf:"varint,1,opt,name=size,proto3" json:"size,omitempty"`
	// Desired interval between consecutive responses in the response stream in
	// microseconds.
	IntervalUs           int32    `protobuf:"varint,2,opt,name=interval_us,json=intervalUs,proto3" json:"interval_us,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ResponseParameters) Reset()         { *m = ResponseParameters{} }
func (m *ResponseParameters) String() string { return proto.CompactTextString(m) }
func (*ResponseParameters) ProtoMessage()    {}
func (*ResponseParameters) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{6}
}

func (m *ResponseParameters) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ResponseParameters.Unmarshal(m, b)
}
func (m *ResponseParameters) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ResponseParameters.Marshal(b, m, deterministic)
}
func (m *ResponseParameters) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ResponseParameters.Merge(m, src)
}
func (m *ResponseParameters) XXX_Size() int {
	return xxx_messageInfo_ResponseParameters.Size(m)
}
func (m *ResponseParameters) XXX_DiscardUnknown() {
	xxx_messageInfo_ResponseParameters.DiscardUnknown(m)
}

var xxx_messageInfo_ResponseParameters proto.InternalMessageInfo

func (m *ResponseParameters) GetSize() int32 {
	if m != nil {
		return m.Size
	}
	return 0
}

func (m *ResponseParameters) GetIntervalUs() int32 {
	if m != nil {
		return m.IntervalUs
	}
	return 0
}

// Server-streaming request.
type StreamingOutputCallRequest struct {
	// Desired payload type in the response from the server.
	// If response_type is RANDOM, the payload from each response in the stream
	// might be of different types. This is to simulate a mixed type of payload
	// stream.
	ResponseType PayloadType `protobuf:"varint,1,opt,name=response_type,json=responseType,proto3,enum=grpc.testing.PayloadType" json:"response_type,omitempty"`
	// Configuration for each expected response message.
	ResponseParameters []*ResponseParameters `protobuf:"bytes,2,rep,name=response_parameters,json=responseParameters,proto3" json:"response_parameters,omitempty"`
	// Optional input payload sent along with the request.
	Payload *Payload `protobuf:"bytes,3,opt,name=payload,proto3" json:"payload,omitempty"`
	// Compression algorithm to be used by the server for the response (stream)
	ResponseCompression CompressionType `protobuf:"varint,6,opt,name=response_compression,json=responseCompression,proto3,enum=grpc.testing.CompressionType" json:"response_compression,omitempty"`
	// Whether server should return a given status
	ResponseStatus       *EchoStatus `protobuf:"bytes,7,opt,name=response_status,json=responseStatus,proto3" json:"response_status,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *StreamingOutputCallRequest) Reset()         { *m = StreamingOutputCallRequest{} }
func (m *StreamingOutputCallRequest) String() string { return proto.CompactTextString(m) }
func (*StreamingOutputCallRequest) ProtoMessage()    {}
func (*StreamingOutputCallRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{7}
}

func (m *StreamingOutputCallRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StreamingOutputCallRequest.Unmarshal(m, b)
}
func (m *StreamingOutputCallRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StreamingOutputCallRequest.Marshal(b, m, deterministic)
}
func (m *StreamingOutputCallRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StreamingOutputCallRequest.Merge(m, src)
}
func (m *StreamingOutputCallRequest) XXX_Size() int {
	return xxx_messageInfo_StreamingOutputCallRequest.Size(m)
}
func (m *StreamingOutputCallRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_StreamingOutputCallRequest.DiscardUnknown(m)
}

var xxx_messageInfo_StreamingOutputCallRequest proto.InternalMessageInfo

func (m *StreamingOutputCallRequest) GetResponseType() PayloadType {
	if m != nil {
		return m.ResponseType
	}
	return PayloadType_COMPRESSABLE
}

func (m *StreamingOutputCallRequest) GetResponseParameters() []*ResponseParameters {
	if m != nil {
		return m.ResponseParameters
	}
	return nil
}

func (m *StreamingOutputCallRequest) GetPayload() *Payload {
	if m != nil {
		return m.Payload
	}
	return nil
}

func (m *StreamingOutputCallRequest) GetResponseCompression() CompressionType {
	if m != nil {
		return m.ResponseCompression
	}
	return CompressionType_NONE
}

func (m *StreamingOutputCallRequest) GetResponseStatus() *EchoStatus {
	if m != nil {
		return m.ResponseStatus
	}
	return nil
}

// Server-streaming response, as configured by the request and parameters.
type StreamingOutputCallResponse struct {
	// Payload to increase response size.
	Payload              *Payload `protobuf:"bytes,1,opt,name=payload,proto3" json:"payload,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StreamingOutputCallResponse) Reset()         { *m = StreamingOutputCallResponse{} }
func (m *StreamingOutputCallResponse) String() string { return proto.CompactTextString(m) }
func (*StreamingOutputCallResponse) ProtoMessage()    {}
func (*StreamingOutputCallResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{8}
}

func (m *StreamingOutputCallResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StreamingOutputCallResponse.Unmarshal(m, b)
}
func (m *StreamingOutputCallResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StreamingOutputCallResponse.Marshal(b, m, deterministic)
}
func (m *StreamingOutputCallResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StreamingOutputCallResponse.Merge(m, src)
}
func (m *StreamingOutputCallResponse) XXX_Size() int {
	return xxx_messageInfo_StreamingOutputCallResponse.Size(m)
}
func (m *StreamingOutputCallResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_StreamingOutputCallResponse.DiscardUnknown(m)
}

var xxx_messageInfo_StreamingOutputCallResponse proto.InternalMessageInfo

func (m *StreamingOutputCallResponse) GetPayload() *Payload {
	if m != nil {
		return m.Payload
	}
	return nil
}

// For reconnect interop test only.
// Client tells server what reconnection parameters it used.
type ReconnectParams struct {
	MaxReconnectBackoffMs int32    `protobuf:"varint,1,opt,name=max_reconnect_backoff_ms,json=maxReconnectBackoffMs,proto3" json:"max_reconnect_backoff_ms,omitempty"`
	XXX_NoUnkeyedLiteral  struct{} `json:"-"`
	XXX_unrecognized      []byte   `json:"-"`
	XXX_sizecache         int32    `json:"-"`
}

func (m *ReconnectParams) Reset()         { *m = ReconnectParams{} }
func (m *ReconnectParams) String() string { return proto.CompactTextString(m) }
func (*ReconnectParams) ProtoMessage()    {}
func (*ReconnectParams) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{9}
}

func (m *ReconnectParams) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ReconnectParams.Unmarshal(m, b)
}
func (m *ReconnectParams) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ReconnectParams.Marshal(b, m, deterministic)
}
func (m *ReconnectParams) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ReconnectParams.Merge(m, src)
}
func (m *ReconnectParams) XXX_Size() int {
	return xxx_messageInfo_ReconnectParams.Size(m)
}
func (m *ReconnectParams) XXX_DiscardUnknown() {
	xxx_messageInfo_ReconnectParams.DiscardUnknown(m)
}

var xxx_messageInfo_ReconnectParams proto.InternalMessageInfo

func (m *ReconnectParams) GetMaxReconnectBackoffMs() int32 {
	if m != nil {
		return m.MaxReconnectBackoffMs
	}
	return 0
}

// For reconnect interop test only.
// Server tells client whether its reconnects are following the spec and the
// reconnect backoffs it saw.
type ReconnectInfo struct {
	Passed               bool     `protobuf:"varint,1,opt,name=passed,proto3" json:"passed,omitempty"`
	BackoffMs            []int32  `protobuf:"varint,2,rep,packed,name=backoff_ms,json=backoffMs,proto3" json:"backoff_ms,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ReconnectInfo) Reset()         { *m = ReconnectInfo{} }
func (m *ReconnectInfo) String() string { return proto.CompactTextString(m) }
func (*ReconnectInfo) ProtoMessage()    {}
func (*ReconnectInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_4dc296cbfe5ffcd5, []int{10}
}

func (m *ReconnectInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ReconnectInfo.Unmarshal(m, b)
}
func (m *ReconnectInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ReconnectInfo.Marshal(b, m, deterministic)
}
func (m *ReconnectInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ReconnectInfo.Merge(m, src)
}
func (m *ReconnectInfo) XXX_Size() int {
	return xxx_messageInfo_ReconnectInfo.Size(m)
}
func (m *ReconnectInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_ReconnectInfo.DiscardUnknown(m)
}

var xxx_messageInfo_ReconnectInfo proto.InternalMessageInfo

func (m *ReconnectInfo) GetPassed() bool {
	if m != nil {
		return m.Passed
	}
	return false
}

func (m *ReconnectInfo) GetBackoffMs() []int32 {
	if m != nil {
		return m.BackoffMs
	}
	return nil
}

func init() {
	proto.RegisterEnum("grpc.testing.PayloadType", PayloadType_name, PayloadType_value)
	proto.RegisterEnum("grpc.testing.CompressionType", CompressionType_name, CompressionType_value)
	proto.RegisterType((*Payload)(nil), "grpc.testing.Payload")
	proto.RegisterType((*EchoStatus)(nil), "grpc.testing.EchoStatus")
	proto.RegisterType((*SimpleRequest)(nil), "grpc.testing.SimpleRequest")
	proto.RegisterType((*SimpleResponse)(nil), "grpc.testing.SimpleResponse")
	proto.RegisterType((*StreamingInputCallRequest)(nil), "grpc.testing.StreamingInputCallRequest")
	proto.RegisterType((*StreamingInputCallResponse)(nil), "grpc.testing.StreamingInputCallResponse")
	proto.RegisterType((*ResponseParameters)(nil), "grpc.testing.ResponseParameters")
	proto.RegisterType((*StreamingOutputCallRequest)(nil), "grpc.testing.StreamingOutputCallRequest")
	proto.RegisterType((*StreamingOutputCallResponse)(nil), "grpc.testing.StreamingOutputCallResponse")
	proto.RegisterType((*ReconnectParams)(nil), "grpc.testing.ReconnectParams")
	proto.RegisterType((*ReconnectInfo)(nil), "grpc.testing.ReconnectInfo")
}

func init() { proto.RegisterFile("messages.proto", fileDescriptor_4dc296cbfe5ffcd5) }

var fileDescriptor_4dc296cbfe5ffcd5 = []byte{
	// 652 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xcc, 0x55, 0x4d, 0x6f, 0xd3, 0x40,
	0x10, 0xc5, 0xf9, 0xee, 0x24, 0x4d, 0xa3, 0x85, 0x82, 0x5b, 0x54, 0x11, 0x99, 0x4b, 0x54, 0x89,
	0x20, 0x05, 0x09, 0x24, 0x0e, 0xa0, 0xb4, 0x4d, 0x51, 0x50, 0x9a, 0x84, 0x75, 0x7b, 0xe1, 0x62,
	0x6d, 0x9c, 0x8d, 0x6b, 0x11, 0x7b, 0x8d, 0x77, 0x8d, 0x9a, 0x1e, 0xb8, 0xf3, 0x83, 0xb9, 0xa3,
	0x5d, 0x7f, 0xc4, 0x69, 0x7b, 0x68, 0xe1, 0xc2, 0x6d, 0xf7, 0xed, 0x9b, 0x97, 0x79, 0x33, 0xcf,
	0x0a, 0x34, 0x3d, 0xca, 0x39, 0x71, 0x28, 0xef, 0x06, 0x21, 0x13, 0x0c, 0x35, 0x9c, 0x30, 0xb0,
	0xbb, 0x82, 0x72, 0xe1, 0xfa, 0x8e, 0x31, 0x82, 0xea, 0x94, 0xac, 0x96, 0x8c, 0xcc, 0xd1, 0x2b,
	0x28, 0x89, 0x55, 0x40, 0x75, 0xad, 0xad, 0x75, 0x9a, 0xbd, 0xbd, 0x6e, 0x9e, 0xd7, 0x4d, 0x48,
	0xe7, 0xab, 0x80, 0x62, 0x45, 0x43, 0x08, 0x4a, 0x33, 0x36, 0x5f, 0xe9, 0x85, 0xb6, 0xd6, 0x69,
	0x60, 0x75, 0x36, 0xde, 0x03, 0x0c, 0xec, 0x4b, 0x66, 0x0a, 0x22, 0x22, 0x2e, 0x19, 0x36, 0x9b,
	0xc7, 0x82, 0x65, 0xac, 0xce, 0x48, 0x87, 0x6a, 0xd2, 0x8f, 0x2a, 0xdc, 0xc2, 0xe9, 0xd5, 0xf8,
	0x55, 0x84, 0x6d, 0xd3, 0xf5, 0x82, 0x25, 0xc5, 0xf4, 0x7b, 0x44, 0xb9, 0x40, 0x1f, 0x60, 0x3b,
	0xa4, 0x3c, 0x60, 0x3e, 0xa7, 0xd6, 0xfd, 0x3a, 0x6b, 0xa4, 0x7c, 0x79, 0x43, 0x2f, 0x73, 0xf5,
	0xdc, 0xbd, 0x8e, 0x7f, 0xb1, 0xbc, 0x26, 0x99, 0xee, 0x35, 0x45, 0xaf, 0xa1, 0x1a, 0xc4, 0x0a,
	0x7a, 0xb1, 0xad, 0x75, 0xea, 0xbd, 0xdd, 0x3b, 0xe5, 0x71, 0xca, 0x92, 0xaa, 0x0b, 0x77, 0xb9,
	0xb4, 0x22, 0x4e, 0x43, 0x9f, 0x78, 0x54, 0x2f, 0xb5, 0xb5, 0x4e, 0x0d, 0x37, 0x24, 0x78, 0x91,
	0x60, 0xa8, 0x03, 0x2d, 0x45, 0x62, 0x24, 0x12, 0x97, 0x16, 0xb7, 0x59, 0x40, 0xf5, 0xb2, 0xe2,
	0x35, 0x25, 0x3e, 0x91, 0xb0, 0x29, 0x51, 0x34, 0x85, 0x27, 0x59, 0x93, 0x36, 0xf3, 0x82, 0x90,
	0x72, 0xee, 0x32, 0x5f, 0xaf, 0x28, 0xaf, 0x07, 0x9b, 0xcd, 0x1c, 0xaf, 0x09, 0xca, 0xef, 0xe3,
	0xb4, 0x34, 0xf7, 0x80, 0xfa, 0xb0, 0xb3, 0xb6, 0xad, 0x36, 0xa1, 0x57, 0x95, 0x33, 0x7d, 0x53,
	0x6c, 0xbd, 0x29, 0xdc, 0xcc, 0x46, 0xa2, 0xee, 0xc6, 0x4f, 0x68, 0xa6, 0xab, 0x88, 0xf1, 0xfc,
	0x98, 0xb4, 0x7b, 0x8d, 0x69, 0x1f, 0x6a, 0xd9, 0x84, 0xe2, 0x4d, 0x67, 0x77, 0xf4, 0x02, 0xea,
	0xf9, 0xc1, 0x14, 0xd5, 0x33, 0xb0, 0x6c, 0x28, 0xc6, 0x08, 0xf6, 0x4c, 0x11, 0x52, 0xe2, 0xb9,
	0xbe, 0x33, 0xf4, 0x83, 0x48, 0x1c, 0x93, 0xe5, 0x32, 0x8d, 0xc5, 0x43, 0x5b, 0x31, 0xce, 0x61,
	0xff, 0x2e, 0xb5, 0xc4, 0xd9, 0x5b, 0x78, 0x46, 0x1c, 0x27, 0xa4, 0x0e, 0x11, 0x74, 0x6e, 0x25,
	0x35, 0x71, 0x5e, 0xe2, 0xe0, 0xee, 0xae, 0x9f, 0x13, 0x69, 0x19, 0x1c, 0x63, 0x08, 0x28, 0xd5,
	0x98, 0x92, 0x90, 0x78, 0x54, 0xd0, 0x50, 0x65, 0x3e, 0x57, 0xaa, 0xce, 0xd2, 0xae, 0xeb, 0x0b,
	0x1a, 0xfe, 0x20, 0x32, 0x35, 0x49, 0x0a, 0x21, 0x85, 0x2e, 0xb8, 0xf1, 0xbb, 0x90, 0xeb, 0x70,
	0x12, 0x89, 0x1b, 0x86, 0xff, 0xf5, 0x3b, 0xf8, 0x02, 0x59, 0x4e, 0xac, 0x20, 0x6b, 0x55, 0x2f,
	0xb4, 0x8b, 0x9d, 0x7a, 0xaf, 0xbd, 0xa9, 0x72, 0xdb, 0x12, 0x46, 0xe1, 0x6d, 0x9b, 0x0f, 0xfe,
	0x6a, 0xfe, 0xcb, 0x98, 0x8f, 0xe1, 0xf9, 0x9d, 0x63, 0xff, 0xcb, 0xcc, 0x1b, 0x9f, 0x61, 0x07,
	0x53, 0x9b, 0xf9, 0x3e, 0xb5, 0x85, 0x1a, 0x16, 0x47, 0xef, 0x40, 0xf7, 0xc8, 0x95, 0x15, 0xa6,
	0xb0, 0x35, 0x23, 0xf6, 0x37, 0xb6, 0x58, 0x58, 0x1e, 0x4f, 0xe3, 0xe5, 0x91, 0xab, 0xac, 0xea,
	0x28, 0x7e, 0x3d, 0xe3, 0xc6, 0x29, 0x6c, 0x67, 0xe8, 0xd0, 0x5f, 0x30, 0xf4, 0x14, 0x2a, 0x01,
	0xe1, 0x9c, 0xc6, 0xcd, 0xd4, 0x70, 0x72, 0x43, 0x07, 0x00, 0x39, 0x4d, 0xb9, 0xd4, 0x32, 0xde,
	0x9a, 0xa5, 0x3a, 0x87, 0x1f, 0xa1, 0x9e, 0x4b, 0x06, 0x6a, 0x41, 0xe3, 0x78, 0x72, 0x36, 0xc5,
	0x03, 0xd3, 0xec, 0x1f, 0x8d, 0x06, 0xad, 0x47, 0x08, 0x41, 0xf3, 0x62, 0xbc, 0x81, 0x69, 0x08,
	0xa0, 0x82, 0xfb, 0xe3, 0x93, 0xc9, 0x59, 0xab, 0x70, 0xd8, 0x83, 0x9d, 0x1b, 0xfb, 0x40, 0x35,
	0x28, 0x8d, 0x27, 0x63, 0x59, 0x5c, 0x83, 0xd2, 0xa7, 0xaf, 0xc3, 0x69, 0x4b, 0x43, 0x75, 0xa8,
	0x9e, 0x0c, 0x4e, 0x47, 0xfd, 0xf3, 0x41, 0xab, 0x30, 0xab, 0xa8, 0xbf, 0x9a, 0x37, 0x7f, 0x02,
	0x00, 0x00, 0xff, 0xff, 0xc2, 0x6a, 0xce, 0x1e, 0x7c, 0x06, 0x00, 0x00,
}
