// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/frequency_cap_level.proto

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

// The level on which the cap is to be applied (e.g ad group ad, ad group).
// Cap is applied to all the resources of this level.
type FrequencyCapLevelEnum_FrequencyCapLevel int32

const (
	// Not specified.
	FrequencyCapLevelEnum_UNSPECIFIED FrequencyCapLevelEnum_FrequencyCapLevel = 0
	// Used for return value only. Represents value unknown in this version.
	FrequencyCapLevelEnum_UNKNOWN FrequencyCapLevelEnum_FrequencyCapLevel = 1
	// The cap is applied at the ad group ad level.
	FrequencyCapLevelEnum_AD_GROUP_AD FrequencyCapLevelEnum_FrequencyCapLevel = 2
	// The cap is applied at the ad group level.
	FrequencyCapLevelEnum_AD_GROUP FrequencyCapLevelEnum_FrequencyCapLevel = 3
	// The cap is applied at the campaign level.
	FrequencyCapLevelEnum_CAMPAIGN FrequencyCapLevelEnum_FrequencyCapLevel = 4
)

var FrequencyCapLevelEnum_FrequencyCapLevel_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "AD_GROUP_AD",
	3: "AD_GROUP",
	4: "CAMPAIGN",
}
var FrequencyCapLevelEnum_FrequencyCapLevel_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"AD_GROUP_AD": 2,
	"AD_GROUP":    3,
	"CAMPAIGN":    4,
}

func (x FrequencyCapLevelEnum_FrequencyCapLevel) String() string {
	return proto.EnumName(FrequencyCapLevelEnum_FrequencyCapLevel_name, int32(x))
}
func (FrequencyCapLevelEnum_FrequencyCapLevel) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_frequency_cap_level_ca8c642a1d3010c4, []int{0, 0}
}

// Container for enum describing the level on which the cap is to be applied.
type FrequencyCapLevelEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *FrequencyCapLevelEnum) Reset()         { *m = FrequencyCapLevelEnum{} }
func (m *FrequencyCapLevelEnum) String() string { return proto.CompactTextString(m) }
func (*FrequencyCapLevelEnum) ProtoMessage()    {}
func (*FrequencyCapLevelEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_frequency_cap_level_ca8c642a1d3010c4, []int{0}
}
func (m *FrequencyCapLevelEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_FrequencyCapLevelEnum.Unmarshal(m, b)
}
func (m *FrequencyCapLevelEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_FrequencyCapLevelEnum.Marshal(b, m, deterministic)
}
func (dst *FrequencyCapLevelEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_FrequencyCapLevelEnum.Merge(dst, src)
}
func (m *FrequencyCapLevelEnum) XXX_Size() int {
	return xxx_messageInfo_FrequencyCapLevelEnum.Size(m)
}
func (m *FrequencyCapLevelEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_FrequencyCapLevelEnum.DiscardUnknown(m)
}

var xxx_messageInfo_FrequencyCapLevelEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*FrequencyCapLevelEnum)(nil), "google.ads.googleads.v0.enums.FrequencyCapLevelEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.FrequencyCapLevelEnum_FrequencyCapLevel", FrequencyCapLevelEnum_FrequencyCapLevel_name, FrequencyCapLevelEnum_FrequencyCapLevel_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/frequency_cap_level.proto", fileDescriptor_frequency_cap_level_ca8c642a1d3010c4)
}

var fileDescriptor_frequency_cap_level_ca8c642a1d3010c4 = []byte{
	// 300 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x32, 0x4f, 0xcf, 0xcf, 0x4f,
	0xcf, 0x49, 0xd5, 0x4f, 0x4c, 0x29, 0xd6, 0x87, 0x30, 0x41, 0xac, 0x32, 0x03, 0xfd, 0xd4, 0xbc,
	0xd2, 0xdc, 0x62, 0xfd, 0xb4, 0xa2, 0xd4, 0xc2, 0xd2, 0xd4, 0xbc, 0xe4, 0xca, 0xf8, 0xe4, 0xc4,
	0x82, 0xf8, 0x9c, 0xd4, 0xb2, 0xd4, 0x1c, 0xbd, 0x82, 0xa2, 0xfc, 0x92, 0x7c, 0x21, 0x59, 0x88,
	0x6a, 0xbd, 0xc4, 0x94, 0x62, 0x3d, 0xb8, 0x46, 0xbd, 0x32, 0x03, 0x3d, 0xb0, 0x46, 0xa5, 0x72,
	0x2e, 0x51, 0x37, 0x98, 0x5e, 0xe7, 0xc4, 0x02, 0x1f, 0x90, 0x4e, 0xd7, 0xbc, 0xd2, 0x5c, 0xa5,
	0x38, 0x2e, 0x41, 0x0c, 0x09, 0x21, 0x7e, 0x2e, 0xee, 0x50, 0xbf, 0xe0, 0x00, 0x57, 0x67, 0x4f,
	0x37, 0x4f, 0x57, 0x17, 0x01, 0x06, 0x21, 0x6e, 0x2e, 0xf6, 0x50, 0x3f, 0x6f, 0x3f, 0xff, 0x70,
	0x3f, 0x01, 0x46, 0x90, 0xac, 0xa3, 0x4b, 0xbc, 0x7b, 0x90, 0x7f, 0x68, 0x40, 0xbc, 0xa3, 0x8b,
	0x00, 0x93, 0x10, 0x0f, 0x17, 0x07, 0x4c, 0x40, 0x80, 0x19, 0xc4, 0x73, 0x76, 0xf4, 0x0d, 0x70,
	0xf4, 0x74, 0xf7, 0x13, 0x60, 0x71, 0x7a, 0xcd, 0xc8, 0xa5, 0x98, 0x9c, 0x9f, 0xab, 0x87, 0xd7,
	0x79, 0x4e, 0x62, 0x18, 0x6e, 0x08, 0x00, 0xf9, 0x2a, 0x80, 0x31, 0xca, 0x09, 0xaa, 0x31, 0x3d,
	0x3f, 0x27, 0x31, 0x2f, 0x5d, 0x2f, 0xbf, 0x28, 0x5d, 0x3f, 0x3d, 0x35, 0x0f, 0xec, 0x67, 0x58,
	0x00, 0x15, 0x64, 0x16, 0xe3, 0x08, 0x2f, 0x6b, 0x30, 0xb9, 0x88, 0x89, 0xd9, 0xdd, 0xd1, 0x71,
	0x15, 0x93, 0xac, 0x3b, 0xc4, 0x28, 0xc7, 0x94, 0x62, 0x3d, 0x08, 0x13, 0xc4, 0x0a, 0x33, 0xd0,
	0x03, 0x05, 0x44, 0xf1, 0x29, 0x98, 0x7c, 0x8c, 0x63, 0x4a, 0x71, 0x0c, 0x5c, 0x3e, 0x26, 0xcc,
	0x20, 0x06, 0x2c, 0xff, 0x8a, 0x49, 0x11, 0x22, 0x68, 0x65, 0xe5, 0x98, 0x52, 0x6c, 0x65, 0x05,
	0x57, 0x61, 0x65, 0x15, 0x66, 0x60, 0x65, 0x05, 0x56, 0x93, 0xc4, 0x06, 0x76, 0x98, 0x31, 0x20,
	0x00, 0x00, 0xff, 0xff, 0x1e, 0x6c, 0x55, 0x4b, 0xc7, 0x01, 0x00, 0x00,
}
