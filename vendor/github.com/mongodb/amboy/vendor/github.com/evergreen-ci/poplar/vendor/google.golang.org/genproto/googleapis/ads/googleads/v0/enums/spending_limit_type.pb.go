// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/spending_limit_type.proto

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

// The possible spending limit types used by certain resources as an
// alternative to absolute money values in micros.
type SpendingLimitTypeEnum_SpendingLimitType int32

const (
	// Not specified.
	SpendingLimitTypeEnum_UNSPECIFIED SpendingLimitTypeEnum_SpendingLimitType = 0
	// Used for return value only. Represents value unknown in this version.
	SpendingLimitTypeEnum_UNKNOWN SpendingLimitTypeEnum_SpendingLimitType = 1
	// Infinite, indicates unlimited spending power.
	SpendingLimitTypeEnum_INFINITE SpendingLimitTypeEnum_SpendingLimitType = 2
)

var SpendingLimitTypeEnum_SpendingLimitType_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "INFINITE",
}
var SpendingLimitTypeEnum_SpendingLimitType_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"INFINITE":    2,
}

func (x SpendingLimitTypeEnum_SpendingLimitType) String() string {
	return proto.EnumName(SpendingLimitTypeEnum_SpendingLimitType_name, int32(x))
}
func (SpendingLimitTypeEnum_SpendingLimitType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_spending_limit_type_62608a37ce3dea80, []int{0, 0}
}

// Message describing spending limit types.
type SpendingLimitTypeEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SpendingLimitTypeEnum) Reset()         { *m = SpendingLimitTypeEnum{} }
func (m *SpendingLimitTypeEnum) String() string { return proto.CompactTextString(m) }
func (*SpendingLimitTypeEnum) ProtoMessage()    {}
func (*SpendingLimitTypeEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_spending_limit_type_62608a37ce3dea80, []int{0}
}
func (m *SpendingLimitTypeEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SpendingLimitTypeEnum.Unmarshal(m, b)
}
func (m *SpendingLimitTypeEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SpendingLimitTypeEnum.Marshal(b, m, deterministic)
}
func (dst *SpendingLimitTypeEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SpendingLimitTypeEnum.Merge(dst, src)
}
func (m *SpendingLimitTypeEnum) XXX_Size() int {
	return xxx_messageInfo_SpendingLimitTypeEnum.Size(m)
}
func (m *SpendingLimitTypeEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_SpendingLimitTypeEnum.DiscardUnknown(m)
}

var xxx_messageInfo_SpendingLimitTypeEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*SpendingLimitTypeEnum)(nil), "google.ads.googleads.v0.enums.SpendingLimitTypeEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.SpendingLimitTypeEnum_SpendingLimitType", SpendingLimitTypeEnum_SpendingLimitType_name, SpendingLimitTypeEnum_SpendingLimitType_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/spending_limit_type.proto", fileDescriptor_spending_limit_type_62608a37ce3dea80)
}

var fileDescriptor_spending_limit_type_62608a37ce3dea80 = []byte{
	// 281 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x32, 0x4f, 0xcf, 0xcf, 0x4f,
	0xcf, 0x49, 0xd5, 0x4f, 0x4c, 0x29, 0xd6, 0x87, 0x30, 0x41, 0xac, 0x32, 0x03, 0xfd, 0xd4, 0xbc,
	0xd2, 0xdc, 0x62, 0xfd, 0xe2, 0x82, 0xd4, 0xbc, 0x94, 0xcc, 0xbc, 0xf4, 0xf8, 0x9c, 0xcc, 0xdc,
	0xcc, 0x92, 0xf8, 0x92, 0xca, 0x82, 0x54, 0xbd, 0x82, 0xa2, 0xfc, 0x92, 0x7c, 0x21, 0x59, 0x88,
	0x6a, 0xbd, 0xc4, 0x94, 0x62, 0x3d, 0xb8, 0x46, 0xbd, 0x32, 0x03, 0x3d, 0xb0, 0x46, 0xa5, 0x08,
	0x2e, 0xd1, 0x60, 0xa8, 0x5e, 0x1f, 0x90, 0xd6, 0x90, 0xca, 0x82, 0x54, 0xd7, 0xbc, 0xd2, 0x5c,
	0x25, 0x7b, 0x2e, 0x41, 0x0c, 0x09, 0x21, 0x7e, 0x2e, 0xee, 0x50, 0xbf, 0xe0, 0x00, 0x57, 0x67,
	0x4f, 0x37, 0x4f, 0x57, 0x17, 0x01, 0x06, 0x21, 0x6e, 0x2e, 0xf6, 0x50, 0x3f, 0x6f, 0x3f, 0xff,
	0x70, 0x3f, 0x01, 0x46, 0x21, 0x1e, 0x2e, 0x0e, 0x4f, 0x3f, 0x37, 0x4f, 0x3f, 0xcf, 0x10, 0x57,
	0x01, 0x26, 0xa7, 0xd7, 0x8c, 0x5c, 0x8a, 0xc9, 0xf9, 0xb9, 0x7a, 0x78, 0xed, 0x77, 0x12, 0xc3,
	0xb0, 0x24, 0x00, 0xe4, 0xec, 0x00, 0xc6, 0x28, 0x27, 0xa8, 0xc6, 0xf4, 0xfc, 0x9c, 0xc4, 0xbc,
	0x74, 0xbd, 0xfc, 0xa2, 0x74, 0xfd, 0xf4, 0xd4, 0x3c, 0xb0, 0xa7, 0x60, 0x21, 0x50, 0x90, 0x59,
	0x8c, 0x23, 0x40, 0xac, 0xc1, 0xe4, 0x22, 0x26, 0x66, 0x77, 0x47, 0xc7, 0x55, 0x4c, 0xb2, 0xee,
	0x10, 0xa3, 0x1c, 0x53, 0x8a, 0xf5, 0x20, 0x4c, 0x10, 0x2b, 0xcc, 0x40, 0x0f, 0xe4, 0xd3, 0xe2,
	0x53, 0x30, 0xf9, 0x18, 0xc7, 0x94, 0xe2, 0x18, 0xb8, 0x7c, 0x4c, 0x98, 0x41, 0x0c, 0x58, 0xfe,
	0x15, 0x93, 0x22, 0x44, 0xd0, 0xca, 0xca, 0x31, 0xa5, 0xd8, 0xca, 0x0a, 0xae, 0xc2, 0xca, 0x2a,
	0xcc, 0xc0, 0xca, 0x0a, 0xac, 0x26, 0x89, 0x0d, 0xec, 0x30, 0x63, 0x40, 0x00, 0x00, 0x00, 0xff,
	0xff, 0x32, 0xe8, 0x65, 0x0d, 0xa8, 0x01, 0x00, 0x00,
}
