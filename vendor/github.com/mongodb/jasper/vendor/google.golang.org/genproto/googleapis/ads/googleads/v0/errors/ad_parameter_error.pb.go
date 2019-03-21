// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/errors/ad_parameter_error.proto

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

// Enum describing possible ad parameter errors.
type AdParameterErrorEnum_AdParameterError int32

const (
	// Enum unspecified.
	AdParameterErrorEnum_UNSPECIFIED AdParameterErrorEnum_AdParameterError = 0
	// The received error code is not known in this version.
	AdParameterErrorEnum_UNKNOWN AdParameterErrorEnum_AdParameterError = 1
	// The ad group criterion must be a keyword criterion.
	AdParameterErrorEnum_AD_GROUP_CRITERION_MUST_BE_KEYWORD AdParameterErrorEnum_AdParameterError = 2
	// The insertion text is invalid.
	AdParameterErrorEnum_INVALID_INSERTION_TEXT_FORMAT AdParameterErrorEnum_AdParameterError = 3
)

var AdParameterErrorEnum_AdParameterError_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "AD_GROUP_CRITERION_MUST_BE_KEYWORD",
	3: "INVALID_INSERTION_TEXT_FORMAT",
}
var AdParameterErrorEnum_AdParameterError_value = map[string]int32{
	"UNSPECIFIED":                        0,
	"UNKNOWN":                            1,
	"AD_GROUP_CRITERION_MUST_BE_KEYWORD": 2,
	"INVALID_INSERTION_TEXT_FORMAT":      3,
}

func (x AdParameterErrorEnum_AdParameterError) String() string {
	return proto.EnumName(AdParameterErrorEnum_AdParameterError_name, int32(x))
}
func (AdParameterErrorEnum_AdParameterError) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_ad_parameter_error_ab5f8ee9efdba60c, []int{0, 0}
}

// Container for enum describing possible ad parameter errors.
type AdParameterErrorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *AdParameterErrorEnum) Reset()         { *m = AdParameterErrorEnum{} }
func (m *AdParameterErrorEnum) String() string { return proto.CompactTextString(m) }
func (*AdParameterErrorEnum) ProtoMessage()    {}
func (*AdParameterErrorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_ad_parameter_error_ab5f8ee9efdba60c, []int{0}
}
func (m *AdParameterErrorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AdParameterErrorEnum.Unmarshal(m, b)
}
func (m *AdParameterErrorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AdParameterErrorEnum.Marshal(b, m, deterministic)
}
func (dst *AdParameterErrorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AdParameterErrorEnum.Merge(dst, src)
}
func (m *AdParameterErrorEnum) XXX_Size() int {
	return xxx_messageInfo_AdParameterErrorEnum.Size(m)
}
func (m *AdParameterErrorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_AdParameterErrorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_AdParameterErrorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*AdParameterErrorEnum)(nil), "google.ads.googleads.v0.errors.AdParameterErrorEnum")
	proto.RegisterEnum("google.ads.googleads.v0.errors.AdParameterErrorEnum_AdParameterError", AdParameterErrorEnum_AdParameterError_name, AdParameterErrorEnum_AdParameterError_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/errors/ad_parameter_error.proto", fileDescriptor_ad_parameter_error_ab5f8ee9efdba60c)
}

var fileDescriptor_ad_parameter_error_ab5f8ee9efdba60c = []byte{
	// 329 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0xc1, 0x4a, 0xfb, 0x30,
	0x00, 0xc6, 0xff, 0xed, 0xe0, 0x2f, 0x64, 0x07, 0x4b, 0xd1, 0xa3, 0x03, 0x7b, 0xf0, 0x98, 0x16,
	0x3c, 0x08, 0xf1, 0x94, 0xad, 0xd9, 0x08, 0x73, 0x69, 0xe9, 0xda, 0x4e, 0xa5, 0x10, 0xaa, 0x29,
	0x45, 0xd8, 0x96, 0x91, 0xcc, 0x5d, 0x7c, 0x0c, 0xdf, 0xc0, 0xa3, 0x8f, 0xe2, 0xa3, 0x78, 0xf2,
	0x11, 0xa4, 0x8d, 0xeb, 0x61, 0xa0, 0xa7, 0x7c, 0x7c, 0xfc, 0xbe, 0xe4, 0xcb, 0x07, 0xae, 0x6a,
	0x29, 0xeb, 0x65, 0xe5, 0x97, 0x42, 0xfb, 0x46, 0x36, 0x6a, 0x17, 0xf8, 0x95, 0x52, 0x52, 0x69,
	0xbf, 0x14, 0x7c, 0x53, 0xaa, 0x72, 0x55, 0x6d, 0x2b, 0xc5, 0x5b, 0x0f, 0x6e, 0x94, 0xdc, 0x4a,
	0x77, 0x60, 0x68, 0x58, 0x0a, 0x0d, 0xbb, 0x20, 0xdc, 0x05, 0xd0, 0x04, 0xbd, 0x57, 0x0b, 0x9c,
	0x60, 0x11, 0xef, 0xb3, 0xa4, 0x71, 0xc9, 0xfa, 0x79, 0xe5, 0xbd, 0x00, 0xe7, 0xd0, 0x77, 0x8f,
	0x41, 0x3f, 0x63, 0xf3, 0x98, 0x8c, 0xe8, 0x98, 0x92, 0xd0, 0xf9, 0xe7, 0xf6, 0xc1, 0x51, 0xc6,
	0xa6, 0x2c, 0x5a, 0x30, 0xc7, 0x72, 0x2f, 0x80, 0x87, 0x43, 0x3e, 0x49, 0xa2, 0x2c, 0xe6, 0xa3,
	0x84, 0xa6, 0x24, 0xa1, 0x11, 0xe3, 0xb3, 0x6c, 0x9e, 0xf2, 0x21, 0xe1, 0x53, 0x72, 0xb7, 0x88,
	0x92, 0xd0, 0xb1, 0xdd, 0x73, 0x70, 0x46, 0x59, 0x8e, 0x6f, 0x68, 0xc8, 0x29, 0x9b, 0x93, 0x24,
	0x6d, 0xb0, 0x94, 0xdc, 0xa6, 0x7c, 0x1c, 0x25, 0x33, 0x9c, 0x3a, 0xbd, 0xe1, 0x97, 0x05, 0xbc,
	0x47, 0xb9, 0x82, 0x7f, 0x97, 0x1f, 0x9e, 0x1e, 0x36, 0x8c, 0x9b, 0x3f, 0xc7, 0xd6, 0x7d, 0xf8,
	0x13, 0xac, 0xe5, 0xb2, 0x5c, 0xd7, 0x50, 0xaa, 0xda, 0xaf, 0xab, 0x75, 0xbb, 0xc8, 0x7e, 0xbe,
	0xcd, 0x93, 0xfe, 0x6d, 0xcd, 0x6b, 0x73, 0xbc, 0xd9, 0xbd, 0x09, 0xc6, 0xef, 0xf6, 0x60, 0x62,
	0x2e, 0xc3, 0x42, 0x43, 0x23, 0x1b, 0x95, 0x07, 0xb0, 0x7d, 0x52, 0x7f, 0xec, 0x81, 0x02, 0x0b,
	0x5d, 0x74, 0x40, 0x91, 0x07, 0x85, 0x01, 0x3e, 0x6d, 0xcf, 0xb8, 0x08, 0x61, 0xa1, 0x11, 0xea,
	0x10, 0x84, 0xf2, 0x00, 0x21, 0x03, 0x3d, 0xfc, 0x6f, 0xdb, 0x5d, 0x7e, 0x07, 0x00, 0x00, 0xff,
	0xff, 0x5c, 0x86, 0x5d, 0xee, 0xea, 0x01, 0x00, 0x00,
}
