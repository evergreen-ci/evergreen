// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/errors/context_error.proto

package errors // import "google.golang.org/genproto/googleapis/ads/googleads/v0/errors"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Enum describing possible context errors.
type ContextErrorEnum_ContextError int32

const (
	// Enum unspecified.
	ContextErrorEnum_UNSPECIFIED ContextErrorEnum_ContextError = 0
	// The received error code is not known in this version.
	ContextErrorEnum_UNKNOWN ContextErrorEnum_ContextError = 1
	// The operation is not allowed for the given context.
	ContextErrorEnum_OPERATION_NOT_PERMITTED_FOR_CONTEXT ContextErrorEnum_ContextError = 2
	// The operation is not allowed for removed resources.
	ContextErrorEnum_OPERATION_NOT_PERMITTED_FOR_REMOVED_RESOURCE ContextErrorEnum_ContextError = 3
)

var ContextErrorEnum_ContextError_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "OPERATION_NOT_PERMITTED_FOR_CONTEXT",
	3: "OPERATION_NOT_PERMITTED_FOR_REMOVED_RESOURCE",
}
var ContextErrorEnum_ContextError_value = map[string]int32{
	"UNSPECIFIED":                         0,
	"UNKNOWN":                             1,
	"OPERATION_NOT_PERMITTED_FOR_CONTEXT": 2,
	"OPERATION_NOT_PERMITTED_FOR_REMOVED_RESOURCE": 3,
}

func (x ContextErrorEnum_ContextError) String() string {
	return proto.EnumName(ContextErrorEnum_ContextError_name, int32(x))
}
func (ContextErrorEnum_ContextError) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_context_error_94c6b53eb3a56cfd, []int{0, 0}
}

// Container for enum describing possible context errors.
type ContextErrorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ContextErrorEnum) Reset()         { *m = ContextErrorEnum{} }
func (m *ContextErrorEnum) String() string { return proto.CompactTextString(m) }
func (*ContextErrorEnum) ProtoMessage()    {}
func (*ContextErrorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_context_error_94c6b53eb3a56cfd, []int{0}
}
func (m *ContextErrorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ContextErrorEnum.Unmarshal(m, b)
}
func (m *ContextErrorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ContextErrorEnum.Marshal(b, m, deterministic)
}
func (dst *ContextErrorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ContextErrorEnum.Merge(dst, src)
}
func (m *ContextErrorEnum) XXX_Size() int {
	return xxx_messageInfo_ContextErrorEnum.Size(m)
}
func (m *ContextErrorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_ContextErrorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_ContextErrorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*ContextErrorEnum)(nil), "google.ads.googleads.v0.errors.ContextErrorEnum")
	proto.RegisterEnum("google.ads.googleads.v0.errors.ContextErrorEnum_ContextError", ContextErrorEnum_ContextError_name, ContextErrorEnum_ContextError_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/errors/context_error.proto", fileDescriptor_context_error_94c6b53eb3a56cfd)
}

var fileDescriptor_context_error_94c6b53eb3a56cfd = []byte{
	// 318 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0xc1, 0x4a, 0xc3, 0x30,
	0x1c, 0xc6, 0x6d, 0x07, 0x0a, 0x99, 0x60, 0xed, 0x03, 0xec, 0x50, 0x0f, 0x5e, 0x24, 0x2d, 0x7a,
	0x8b, 0xa7, 0xae, 0xfd, 0x6f, 0x14, 0x59, 0x52, 0xba, 0xae, 0x8a, 0x14, 0xc2, 0x5c, 0x4b, 0x10,
	0xb6, 0x66, 0x34, 0x73, 0xf8, 0x06, 0xbe, 0x84, 0x27, 0x8f, 0x3e, 0x8a, 0x8f, 0x22, 0x3e, 0x84,
	0xb4, 0x71, 0x65, 0x17, 0x77, 0xca, 0x97, 0x8f, 0xdf, 0x97, 0xfc, 0xff, 0x1f, 0xba, 0x16, 0x52,
	0x8a, 0x65, 0xe9, 0xce, 0x0b, 0xe5, 0x6a, 0xd9, 0xa8, 0xad, 0xe7, 0x96, 0x75, 0x2d, 0x6b, 0xe5,
	0x2e, 0x64, 0xb5, 0x29, 0x5f, 0x37, 0xbc, 0xbd, 0xe2, 0x75, 0x2d, 0x37, 0xd2, 0x1e, 0x68, 0x10,
	0xcf, 0x0b, 0x85, 0xbb, 0x0c, 0xde, 0x7a, 0x58, 0x67, 0x9c, 0x77, 0x03, 0x59, 0x81, 0xce, 0x41,
	0xe3, 0x40, 0xf5, 0xb2, 0x72, 0xde, 0x0c, 0x74, 0xba, 0x6f, 0xda, 0x67, 0xa8, 0x3f, 0xa3, 0xd3,
	0x18, 0x82, 0x68, 0x14, 0x41, 0x68, 0x1d, 0xd9, 0x7d, 0x74, 0x32, 0xa3, 0x77, 0x94, 0xdd, 0x53,
	0xcb, 0xb0, 0x2f, 0xd1, 0x05, 0x8b, 0x21, 0xf1, 0xd3, 0x88, 0x51, 0x4e, 0x59, 0xca, 0x63, 0x48,
	0x26, 0x51, 0x9a, 0x42, 0xc8, 0x47, 0x2c, 0xe1, 0x01, 0xa3, 0x29, 0x3c, 0xa4, 0x96, 0x69, 0x7b,
	0xe8, 0xea, 0x10, 0x98, 0xc0, 0x84, 0x65, 0x10, 0xf2, 0x04, 0xa6, 0x6c, 0x96, 0x04, 0x60, 0xf5,
	0x86, 0x3f, 0x06, 0x72, 0x16, 0x72, 0x85, 0x0f, 0x6f, 0x31, 0x3c, 0xdf, 0x9f, 0x36, 0x6e, 0x16,
	0x8f, 0x8d, 0xc7, 0xf0, 0x2f, 0x24, 0xe4, 0x72, 0x5e, 0x09, 0x2c, 0x6b, 0xe1, 0x8a, 0xb2, 0x6a,
	0x6b, 0xd9, 0xd5, 0xb7, 0x7e, 0x56, 0xff, 0xb5, 0x79, 0xab, 0x8f, 0x0f, 0xb3, 0x37, 0xf6, 0xfd,
	0x4f, 0x73, 0x30, 0xd6, 0x8f, 0xf9, 0x85, 0xc2, 0x5a, 0x36, 0x2a, 0xf3, 0x70, 0xfb, 0xa5, 0xfa,
	0xda, 0x01, 0xb9, 0x5f, 0xa8, 0xbc, 0x03, 0xf2, 0xcc, 0xcb, 0x35, 0xf0, 0x6d, 0x3a, 0xda, 0x25,
	0xc4, 0x2f, 0x14, 0x21, 0x1d, 0x42, 0x48, 0xe6, 0x11, 0xa2, 0xa1, 0xa7, 0xe3, 0x76, 0xba, 0x9b,
	0xdf, 0x00, 0x00, 0x00, 0xff, 0xff, 0x59, 0x28, 0x3d, 0xd6, 0xea, 0x01, 0x00, 0x00,
}
