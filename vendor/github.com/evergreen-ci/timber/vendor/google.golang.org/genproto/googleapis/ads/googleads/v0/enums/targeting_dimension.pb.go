// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/targeting_dimension.proto

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

// Enum describing possible targeting dimensions.
type TargetingDimensionEnum_TargetingDimension int32

const (
	// Not specified.
	TargetingDimensionEnum_UNSPECIFIED TargetingDimensionEnum_TargetingDimension = 0
	// Used for return value only. Represents value unknown in this version.
	TargetingDimensionEnum_UNKNOWN TargetingDimensionEnum_TargetingDimension = 1
	// Keyword criteria, e.g. 'mars cruise'. KEYWORD may be used as a custom bid
	// dimension. Keywords are always a targeting dimension, so may not be set
	// as a target "ALL" dimension with TargetRestriction.
	TargetingDimensionEnum_KEYWORD TargetingDimensionEnum_TargetingDimension = 2
	// Audience criteria, which include user list, user interest, custom
	// affinity,  and custom in market.
	TargetingDimensionEnum_AUDIENCE TargetingDimensionEnum_TargetingDimension = 3
	// Topic criteria for targeting categories of content, e.g.
	// 'category::Animals>Pets' Used for Display and Video targeting.
	TargetingDimensionEnum_TOPIC TargetingDimensionEnum_TargetingDimension = 4
	// Criteria for targeting gender.
	TargetingDimensionEnum_GENDER TargetingDimensionEnum_TargetingDimension = 5
	// Criteria for targeting age ranges.
	TargetingDimensionEnum_AGE_RANGE TargetingDimensionEnum_TargetingDimension = 6
	// Placement criteria, which include websites like 'www.flowers4sale.com',
	// as well as mobile applications, mobile app categories, YouTube videos,
	// and YouTube channels.
	TargetingDimensionEnum_PLACEMENT TargetingDimensionEnum_TargetingDimension = 7
	// Criteria for parental status targeting.
	TargetingDimensionEnum_PARENTAL_STATUS TargetingDimensionEnum_TargetingDimension = 8
	// Criteria for income range targeting.
	TargetingDimensionEnum_INCOME_RANGE TargetingDimensionEnum_TargetingDimension = 9
)

var TargetingDimensionEnum_TargetingDimension_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "KEYWORD",
	3: "AUDIENCE",
	4: "TOPIC",
	5: "GENDER",
	6: "AGE_RANGE",
	7: "PLACEMENT",
	8: "PARENTAL_STATUS",
	9: "INCOME_RANGE",
}
var TargetingDimensionEnum_TargetingDimension_value = map[string]int32{
	"UNSPECIFIED":     0,
	"UNKNOWN":         1,
	"KEYWORD":         2,
	"AUDIENCE":        3,
	"TOPIC":           4,
	"GENDER":          5,
	"AGE_RANGE":       6,
	"PLACEMENT":       7,
	"PARENTAL_STATUS": 8,
	"INCOME_RANGE":    9,
}

func (x TargetingDimensionEnum_TargetingDimension) String() string {
	return proto.EnumName(TargetingDimensionEnum_TargetingDimension_name, int32(x))
}
func (TargetingDimensionEnum_TargetingDimension) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_targeting_dimension_c3c653490aa65df5, []int{0, 0}
}

// The dimensions that can be targeted.
type TargetingDimensionEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *TargetingDimensionEnum) Reset()         { *m = TargetingDimensionEnum{} }
func (m *TargetingDimensionEnum) String() string { return proto.CompactTextString(m) }
func (*TargetingDimensionEnum) ProtoMessage()    {}
func (*TargetingDimensionEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_targeting_dimension_c3c653490aa65df5, []int{0}
}
func (m *TargetingDimensionEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TargetingDimensionEnum.Unmarshal(m, b)
}
func (m *TargetingDimensionEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TargetingDimensionEnum.Marshal(b, m, deterministic)
}
func (dst *TargetingDimensionEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TargetingDimensionEnum.Merge(dst, src)
}
func (m *TargetingDimensionEnum) XXX_Size() int {
	return xxx_messageInfo_TargetingDimensionEnum.Size(m)
}
func (m *TargetingDimensionEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_TargetingDimensionEnum.DiscardUnknown(m)
}

var xxx_messageInfo_TargetingDimensionEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*TargetingDimensionEnum)(nil), "google.ads.googleads.v0.enums.TargetingDimensionEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.TargetingDimensionEnum_TargetingDimension", TargetingDimensionEnum_TargetingDimension_name, TargetingDimensionEnum_TargetingDimension_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/targeting_dimension.proto", fileDescriptor_targeting_dimension_c3c653490aa65df5)
}

var fileDescriptor_targeting_dimension_c3c653490aa65df5 = []byte{
	// 361 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0x4f, 0x8a, 0xdb, 0x30,
	0x18, 0xc5, 0x6b, 0xa7, 0xf9, 0xa7, 0xa4, 0x44, 0xa8, 0xd0, 0xae, 0xb2, 0x48, 0x0e, 0x20, 0x1b,
	0xba, 0x28, 0xa8, 0x2b, 0xc5, 0x56, 0x8d, 0x49, 0x22, 0x1b, 0xc7, 0x4e, 0x68, 0x31, 0x04, 0xb7,
	0x36, 0xc2, 0x10, 0x5b, 0x21, 0x4a, 0x72, 0xa0, 0xee, 0xda, 0x73, 0x74, 0x35, 0x47, 0x19, 0xe6,
	0x10, 0x83, 0xed, 0x71, 0x36, 0x61, 0x66, 0x23, 0xde, 0xa7, 0xf7, 0x7e, 0x42, 0xdf, 0x03, 0x5f,
	0x85, 0x94, 0xe2, 0x90, 0x19, 0x49, 0xaa, 0x8c, 0x46, 0x56, 0xea, 0x6a, 0x1a, 0x59, 0x79, 0x29,
	0x94, 0x71, 0x4e, 0x4e, 0x22, 0x3b, 0xe7, 0xa5, 0xd8, 0xa7, 0x79, 0x91, 0x95, 0x2a, 0x97, 0x25,
	0x3e, 0x9e, 0xe4, 0x59, 0xa2, 0x69, 0x93, 0xc6, 0x49, 0xaa, 0xf0, 0x0d, 0xc4, 0x57, 0x13, 0xd7,
	0xe0, 0xfc, 0xbf, 0x06, 0x3e, 0x85, 0x2d, 0x6c, 0xb7, 0x2c, 0x2b, 0x2f, 0xc5, 0xfc, 0xaf, 0x06,
	0xd0, 0xbd, 0x85, 0x26, 0x60, 0x14, 0xf1, 0x8d, 0xcf, 0x2c, 0xf7, 0xbb, 0xcb, 0x6c, 0xf8, 0x0e,
	0x8d, 0x40, 0x3f, 0xe2, 0x4b, 0xee, 0xed, 0x38, 0xd4, 0xaa, 0x61, 0xc9, 0x7e, 0xec, 0xbc, 0xc0,
	0x86, 0x3a, 0x1a, 0x83, 0x01, 0x8d, 0x6c, 0x97, 0x71, 0x8b, 0xc1, 0x0e, 0x1a, 0x82, 0x6e, 0xe8,
	0xf9, 0xae, 0x05, 0xdf, 0x23, 0x00, 0x7a, 0x0e, 0xe3, 0x36, 0x0b, 0x60, 0x17, 0x7d, 0x00, 0x43,
	0xea, 0xb0, 0x7d, 0x40, 0xb9, 0xc3, 0x60, 0xaf, 0x1a, 0xfd, 0x15, 0xb5, 0xd8, 0x9a, 0xf1, 0x10,
	0xf6, 0xd1, 0x47, 0x30, 0xf1, 0x69, 0xc0, 0x78, 0x48, 0x57, 0xfb, 0x4d, 0x48, 0xc3, 0x68, 0x03,
	0x07, 0x08, 0x82, 0xb1, 0xcb, 0x2d, 0x6f, 0xdd, 0x52, 0xc3, 0xc5, 0x93, 0x06, 0x66, 0xbf, 0x65,
	0x81, 0xdf, 0x5c, 0x76, 0xf1, 0xf9, 0x7e, 0x1d, 0xbf, 0x2a, 0xc9, 0xd7, 0x7e, 0x2e, 0x5e, 0x48,
	0x21, 0x0f, 0x49, 0x29, 0xb0, 0x3c, 0x09, 0x43, 0x64, 0x65, 0x5d, 0x61, 0xdb, 0xf7, 0x31, 0x57,
	0xaf, 0xd4, 0xff, 0xad, 0x3e, 0xff, 0xe8, 0x1d, 0x87, 0xd2, 0x7f, 0xfa, 0xd4, 0x69, 0x9e, 0xa2,
	0xa9, 0xc2, 0x8d, 0xac, 0xd4, 0xd6, 0xc4, 0x55, 0xab, 0xea, 0xa1, 0xf5, 0x63, 0x9a, 0xaa, 0xf8,
	0xe6, 0xc7, 0x5b, 0x33, 0xae, 0xfd, 0x47, 0x7d, 0xd6, 0x5c, 0x12, 0x42, 0x53, 0x45, 0xc8, 0x2d,
	0x41, 0xc8, 0xd6, 0x24, 0xa4, 0xce, 0xfc, 0xea, 0xd5, 0x1f, 0xfb, 0xf2, 0x1c, 0x00, 0x00, 0xff,
	0xff, 0xc4, 0xb2, 0x9d, 0xe7, 0x16, 0x02, 0x00, 0x00,
}
