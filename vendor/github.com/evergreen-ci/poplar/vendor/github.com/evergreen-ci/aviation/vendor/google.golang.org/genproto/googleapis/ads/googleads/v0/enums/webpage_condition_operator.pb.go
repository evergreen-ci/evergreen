// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/webpage_condition_operator.proto

package enums // import "google.golang.org/genproto/googleapis/ads/googleads/v0/enums"

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

// The webpage condition operator in webpage criterion.
type WebpageConditionOperatorEnum_WebpageConditionOperator int32

const (
	// Not specified.
	WebpageConditionOperatorEnum_UNSPECIFIED WebpageConditionOperatorEnum_WebpageConditionOperator = 0
	// Used for return value only. Represents value unknown in this version.
	WebpageConditionOperatorEnum_UNKNOWN WebpageConditionOperatorEnum_WebpageConditionOperator = 1
	// The argument web condition is equal to the compared web condition.
	WebpageConditionOperatorEnum_EQUALS WebpageConditionOperatorEnum_WebpageConditionOperator = 2
	// The argument web condition is part of the compared web condition.
	WebpageConditionOperatorEnum_CONTAINS WebpageConditionOperatorEnum_WebpageConditionOperator = 3
)

var WebpageConditionOperatorEnum_WebpageConditionOperator_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "EQUALS",
	3: "CONTAINS",
}
var WebpageConditionOperatorEnum_WebpageConditionOperator_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"EQUALS":      2,
	"CONTAINS":    3,
}

func (x WebpageConditionOperatorEnum_WebpageConditionOperator) String() string {
	return proto.EnumName(WebpageConditionOperatorEnum_WebpageConditionOperator_name, int32(x))
}
func (WebpageConditionOperatorEnum_WebpageConditionOperator) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_webpage_condition_operator_739fe903969cda51, []int{0, 0}
}

// Container for enum describing webpage condition operator in webpage
// criterion.
type WebpageConditionOperatorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *WebpageConditionOperatorEnum) Reset()         { *m = WebpageConditionOperatorEnum{} }
func (m *WebpageConditionOperatorEnum) String() string { return proto.CompactTextString(m) }
func (*WebpageConditionOperatorEnum) ProtoMessage()    {}
func (*WebpageConditionOperatorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_webpage_condition_operator_739fe903969cda51, []int{0}
}
func (m *WebpageConditionOperatorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_WebpageConditionOperatorEnum.Unmarshal(m, b)
}
func (m *WebpageConditionOperatorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_WebpageConditionOperatorEnum.Marshal(b, m, deterministic)
}
func (dst *WebpageConditionOperatorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_WebpageConditionOperatorEnum.Merge(dst, src)
}
func (m *WebpageConditionOperatorEnum) XXX_Size() int {
	return xxx_messageInfo_WebpageConditionOperatorEnum.Size(m)
}
func (m *WebpageConditionOperatorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_WebpageConditionOperatorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_WebpageConditionOperatorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*WebpageConditionOperatorEnum)(nil), "google.ads.googleads.v0.enums.WebpageConditionOperatorEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.WebpageConditionOperatorEnum_WebpageConditionOperator", WebpageConditionOperatorEnum_WebpageConditionOperator_name, WebpageConditionOperatorEnum_WebpageConditionOperator_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/webpage_condition_operator.proto", fileDescriptor_webpage_condition_operator_739fe903969cda51)
}

var fileDescriptor_webpage_condition_operator_739fe903969cda51 = []byte{
	// 298 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x50, 0x41, 0x4b, 0xc3, 0x30,
	0x18, 0x75, 0x1d, 0x4c, 0xc9, 0x04, 0x4b, 0x4f, 0x1e, 0xdc, 0x61, 0xfb, 0x01, 0x69, 0xc1, 0x5b,
	0x04, 0x21, 0x9b, 0x75, 0x0c, 0x25, 0xad, 0xd6, 0x76, 0x20, 0x85, 0xd1, 0x2d, 0x21, 0x14, 0xd6,
	0xa4, 0x34, 0xdd, 0xfc, 0x3f, 0x1e, 0xfd, 0x29, 0xfe, 0x14, 0x8f, 0xfe, 0x02, 0x69, 0xb2, 0xf6,
	0x56, 0x2f, 0xe1, 0x91, 0xf7, 0xbd, 0xf7, 0x7d, 0xef, 0x81, 0x7b, 0x2e, 0x25, 0xdf, 0x33, 0x37,
	0xa3, 0xca, 0x35, 0xb0, 0x41, 0x47, 0xcf, 0x65, 0xe2, 0x50, 0x28, 0xf7, 0x83, 0x6d, 0xcb, 0x8c,
	0xb3, 0xcd, 0x4e, 0x0a, 0x9a, 0xd7, 0xb9, 0x14, 0x1b, 0x59, 0xb2, 0x2a, 0xab, 0x65, 0x05, 0xcb,
	0x4a, 0xd6, 0xd2, 0x99, 0x18, 0x11, 0xcc, 0xa8, 0x82, 0x9d, 0x1e, 0x1e, 0x3d, 0xa8, 0xf5, 0xb3,
	0x0a, 0xdc, 0xac, 0x8d, 0xc5, 0xa2, 0x75, 0x08, 0x4e, 0x06, 0xbe, 0x38, 0x14, 0xb3, 0x57, 0x70,
	0xdd, 0xc7, 0x3b, 0x57, 0x60, 0x1c, 0x93, 0x28, 0xf4, 0x17, 0xab, 0xc7, 0x95, 0xff, 0x60, 0x9f,
	0x39, 0x63, 0x70, 0x1e, 0x93, 0x27, 0x12, 0xac, 0x89, 0x3d, 0x70, 0x00, 0x18, 0xf9, 0x2f, 0x31,
	0x7e, 0x8e, 0x6c, 0xcb, 0xb9, 0x04, 0x17, 0x8b, 0x80, 0xbc, 0xe1, 0x15, 0x89, 0xec, 0xe1, 0xfc,
	0x77, 0x00, 0xa6, 0x3b, 0x59, 0xc0, 0x7f, 0x2f, 0x9b, 0x4f, 0xfa, 0xf6, 0x86, 0x4d, 0xae, 0x70,
	0xf0, 0x3e, 0x3f, 0xe9, 0xb9, 0xdc, 0x67, 0x82, 0x43, 0x59, 0x71, 0x97, 0x33, 0xa1, 0x53, 0xb7,
	0x4d, 0x95, 0xb9, 0xea, 0x29, 0xee, 0x4e, 0xbf, 0x9f, 0xd6, 0x70, 0x89, 0xf1, 0x97, 0x35, 0x59,
	0x1a, 0x2b, 0x4c, 0x15, 0x34, 0xb0, 0x41, 0x89, 0x07, 0x9b, 0x0e, 0xd4, 0x77, 0xcb, 0xa7, 0x98,
	0xaa, 0xb4, 0xe3, 0xd3, 0xc4, 0x4b, 0x35, 0xff, 0x63, 0x4d, 0xcd, 0x27, 0x42, 0x98, 0x2a, 0x84,
	0xba, 0x09, 0x84, 0x12, 0x0f, 0x21, 0x3d, 0xb3, 0x1d, 0xe9, 0xc3, 0x6e, 0xff, 0x02, 0x00, 0x00,
	0xff, 0xff, 0xc3, 0x84, 0xb9, 0x16, 0xd0, 0x01, 0x00, 0x00,
}
