// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/advertising_channel_sub_type.proto

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

// Enum describing the different channel subtypes.
type AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType int32

const (
	// Not specified.
	AdvertisingChannelSubTypeEnum_UNSPECIFIED AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 0
	// Used as a return value only. Represents value unknown in this version.
	AdvertisingChannelSubTypeEnum_UNKNOWN AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 1
	// Mobile app campaigns for Search.
	AdvertisingChannelSubTypeEnum_SEARCH_MOBILE_APP AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 2
	// Mobile app campaigns for Display.
	AdvertisingChannelSubTypeEnum_DISPLAY_MOBILE_APP AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 3
	// AdWords express campaigns for search.
	AdvertisingChannelSubTypeEnum_SEARCH_EXPRESS AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 4
	// AdWords Express campaigns for display.
	AdvertisingChannelSubTypeEnum_DISPLAY_EXPRESS AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 5
	// Smart Shopping campaigns.
	AdvertisingChannelSubTypeEnum_SHOPPING_SMART_ADS AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 6
	// Gmail Ad campaigns.
	AdvertisingChannelSubTypeEnum_DISPLAY_GMAIL_AD AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 7
	// Smart display campaigns.
	AdvertisingChannelSubTypeEnum_DISPLAY_SMART_CAMPAIGN AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 8
	// Video Outstream campaigns.
	AdvertisingChannelSubTypeEnum_VIDEO_OUTSTREAM AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 9
	// Video TrueView for Action campaigns.
	AdvertisingChannelSubTypeEnum_VIDEO_ACTION AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType = 10
)

var AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType_name = map[int32]string{
	0:  "UNSPECIFIED",
	1:  "UNKNOWN",
	2:  "SEARCH_MOBILE_APP",
	3:  "DISPLAY_MOBILE_APP",
	4:  "SEARCH_EXPRESS",
	5:  "DISPLAY_EXPRESS",
	6:  "SHOPPING_SMART_ADS",
	7:  "DISPLAY_GMAIL_AD",
	8:  "DISPLAY_SMART_CAMPAIGN",
	9:  "VIDEO_OUTSTREAM",
	10: "VIDEO_ACTION",
}
var AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType_value = map[string]int32{
	"UNSPECIFIED":            0,
	"UNKNOWN":                1,
	"SEARCH_MOBILE_APP":      2,
	"DISPLAY_MOBILE_APP":     3,
	"SEARCH_EXPRESS":         4,
	"DISPLAY_EXPRESS":        5,
	"SHOPPING_SMART_ADS":     6,
	"DISPLAY_GMAIL_AD":       7,
	"DISPLAY_SMART_CAMPAIGN": 8,
	"VIDEO_OUTSTREAM":        9,
	"VIDEO_ACTION":           10,
}

func (x AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType) String() string {
	return proto.EnumName(AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType_name, int32(x))
}
func (AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_advertising_channel_sub_type_7e92338015ff8fdc, []int{0, 0}
}

// An immutable specialization of an Advertising Channel.
type AdvertisingChannelSubTypeEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *AdvertisingChannelSubTypeEnum) Reset()         { *m = AdvertisingChannelSubTypeEnum{} }
func (m *AdvertisingChannelSubTypeEnum) String() string { return proto.CompactTextString(m) }
func (*AdvertisingChannelSubTypeEnum) ProtoMessage()    {}
func (*AdvertisingChannelSubTypeEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_advertising_channel_sub_type_7e92338015ff8fdc, []int{0}
}
func (m *AdvertisingChannelSubTypeEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AdvertisingChannelSubTypeEnum.Unmarshal(m, b)
}
func (m *AdvertisingChannelSubTypeEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AdvertisingChannelSubTypeEnum.Marshal(b, m, deterministic)
}
func (dst *AdvertisingChannelSubTypeEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AdvertisingChannelSubTypeEnum.Merge(dst, src)
}
func (m *AdvertisingChannelSubTypeEnum) XXX_Size() int {
	return xxx_messageInfo_AdvertisingChannelSubTypeEnum.Size(m)
}
func (m *AdvertisingChannelSubTypeEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_AdvertisingChannelSubTypeEnum.DiscardUnknown(m)
}

var xxx_messageInfo_AdvertisingChannelSubTypeEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*AdvertisingChannelSubTypeEnum)(nil), "google.ads.googleads.v0.enums.AdvertisingChannelSubTypeEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType", AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType_name, AdvertisingChannelSubTypeEnum_AdvertisingChannelSubType_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/advertising_channel_sub_type.proto", fileDescriptor_advertising_channel_sub_type_7e92338015ff8fdc)
}

var fileDescriptor_advertising_channel_sub_type_7e92338015ff8fdc = []byte{
	// 415 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x91, 0xdf, 0x8a, 0xd3, 0x40,
	0x14, 0xc6, 0x6d, 0x56, 0x77, 0x75, 0x56, 0xdc, 0x71, 0xd4, 0x05, 0x85, 0x0a, 0xbb, 0x0f, 0x30,
	0x09, 0x78, 0x37, 0xde, 0x78, 0x9a, 0x8c, 0xd9, 0xc1, 0x26, 0x19, 0x3a, 0x69, 0xfd, 0x43, 0x61,
	0x48, 0x37, 0x21, 0x16, 0xda, 0xa4, 0x74, 0xda, 0xc2, 0xde, 0xfa, 0x28, 0xe2, 0x95, 0x8f, 0xe2,
	0xa3, 0x78, 0xeb, 0x0b, 0x48, 0x32, 0x9b, 0xea, 0x4d, 0xf7, 0x26, 0x1c, 0xbe, 0xf3, 0xcb, 0x8f,
	0xe1, 0x7c, 0xe8, 0x5d, 0x59, 0xd7, 0xe5, 0xa2, 0x70, 0xb3, 0xdc, 0xb8, 0x76, 0x6c, 0xa6, 0x9d,
	0xe7, 0x16, 0xd5, 0x76, 0x69, 0xdc, 0x2c, 0xdf, 0x15, 0xeb, 0xcd, 0xdc, 0xcc, 0xab, 0x52, 0x5f,
	0x7f, 0xcd, 0xaa, 0xaa, 0x58, 0x68, 0xb3, 0x9d, 0xe9, 0xcd, 0xcd, 0xaa, 0xa0, 0xab, 0x75, 0xbd,
	0xa9, 0x49, 0xdf, 0xfe, 0x46, 0xb3, 0xdc, 0xd0, 0xbd, 0x81, 0xee, 0x3c, 0xda, 0x1a, 0x2e, 0x7f,
	0x38, 0xa8, 0x0f, 0xff, 0x2c, 0xbe, 0x95, 0xa8, 0xed, 0x2c, 0xbd, 0x59, 0x15, 0xbc, 0xda, 0x2e,
	0x2f, 0xbf, 0x39, 0xe8, 0xe5, 0x41, 0x82, 0x9c, 0xa1, 0xd3, 0x71, 0xac, 0x24, 0xf7, 0xc5, 0x7b,
	0xc1, 0x03, 0x7c, 0x8f, 0x9c, 0xa2, 0x93, 0x71, 0xfc, 0x21, 0x4e, 0x3e, 0xc6, 0xb8, 0x47, 0x5e,
	0xa0, 0xa7, 0x8a, 0xc3, 0xc8, 0xbf, 0xd2, 0x51, 0x32, 0x10, 0x43, 0xae, 0x41, 0x4a, 0xec, 0x90,
	0x73, 0x44, 0x02, 0xa1, 0xe4, 0x10, 0x3e, 0xff, 0x9f, 0x1f, 0x11, 0x82, 0x9e, 0xdc, 0xe2, 0xfc,
	0x93, 0x1c, 0x71, 0xa5, 0xf0, 0x7d, 0xf2, 0x0c, 0x9d, 0x75, 0x6c, 0x17, 0x3e, 0x68, 0x04, 0xea,
	0x2a, 0x91, 0x52, 0xc4, 0xa1, 0x56, 0x11, 0x8c, 0x52, 0x0d, 0x81, 0xc2, 0xc7, 0xe4, 0x39, 0xc2,
	0x1d, 0x1c, 0x46, 0x20, 0x86, 0x1a, 0x02, 0x7c, 0x42, 0x5e, 0xa1, 0xf3, 0x2e, 0xb5, 0xb0, 0x0f,
	0x91, 0x04, 0x11, 0xc6, 0xf8, 0x61, 0xa3, 0x9f, 0x88, 0x80, 0x27, 0x3a, 0x19, 0xa7, 0x2a, 0x1d,
	0x71, 0x88, 0xf0, 0x23, 0x82, 0xd1, 0x63, 0x1b, 0x82, 0x9f, 0x8a, 0x24, 0xc6, 0x68, 0xf0, 0xa7,
	0x87, 0x2e, 0xae, 0xeb, 0x25, 0xbd, 0xf3, 0x98, 0x83, 0xd7, 0x07, 0xef, 0x24, 0x9b, 0x2e, 0x64,
	0xef, 0xcb, 0xe0, 0x56, 0x50, 0xd6, 0x8b, 0xac, 0x2a, 0x69, 0xbd, 0x2e, 0xdd, 0xb2, 0xa8, 0xda,
	0xa6, 0xba, 0x7e, 0x57, 0x73, 0x73, 0xa0, 0xee, 0xb7, 0xed, 0xf7, 0xbb, 0x73, 0x14, 0x02, 0xfc,
	0x74, 0xfa, 0xa1, 0x55, 0x41, 0x6e, 0xa8, 0x1d, 0x9b, 0x69, 0xe2, 0xd1, 0xa6, 0x35, 0xf3, 0xab,
	0xdb, 0x4f, 0x21, 0x37, 0xd3, 0xfd, 0x7e, 0x3a, 0xf1, 0xa6, 0xed, 0xfe, 0xb7, 0x73, 0x61, 0x43,
	0xc6, 0x20, 0x37, 0x8c, 0xed, 0x09, 0xc6, 0x26, 0x1e, 0x63, 0x2d, 0x33, 0x3b, 0x6e, 0x1f, 0xf6,
	0xe6, 0x6f, 0x00, 0x00, 0x00, 0xff, 0xff, 0x55, 0xda, 0x94, 0x9c, 0x86, 0x02, 0x00, 0x00,
}
