// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/display_ad_format_setting.proto

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

// Enumerates display ad format settings.
type DisplayAdFormatSettingEnum_DisplayAdFormatSetting int32

const (
	// Not specified.
	DisplayAdFormatSettingEnum_UNSPECIFIED DisplayAdFormatSettingEnum_DisplayAdFormatSetting = 0
	// The value is unknown in this version.
	DisplayAdFormatSettingEnum_UNKNOWN DisplayAdFormatSettingEnum_DisplayAdFormatSetting = 1
	// Text, image and native formats.
	DisplayAdFormatSettingEnum_ALL_FORMATS DisplayAdFormatSettingEnum_DisplayAdFormatSetting = 2
	// Text and image formats.
	DisplayAdFormatSettingEnum_NON_NATIVE DisplayAdFormatSettingEnum_DisplayAdFormatSetting = 3
	// Native format, i.e. the format rendering is controlled by the publisher
	// and not by Google.
	DisplayAdFormatSettingEnum_NATIVE DisplayAdFormatSettingEnum_DisplayAdFormatSetting = 4
)

var DisplayAdFormatSettingEnum_DisplayAdFormatSetting_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "ALL_FORMATS",
	3: "NON_NATIVE",
	4: "NATIVE",
}
var DisplayAdFormatSettingEnum_DisplayAdFormatSetting_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"ALL_FORMATS": 2,
	"NON_NATIVE":  3,
	"NATIVE":      4,
}

func (x DisplayAdFormatSettingEnum_DisplayAdFormatSetting) String() string {
	return proto.EnumName(DisplayAdFormatSettingEnum_DisplayAdFormatSetting_name, int32(x))
}
func (DisplayAdFormatSettingEnum_DisplayAdFormatSetting) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_display_ad_format_setting_7b5f6590c96d9abd, []int{0, 0}
}

// Container for display ad format settings.
type DisplayAdFormatSettingEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *DisplayAdFormatSettingEnum) Reset()         { *m = DisplayAdFormatSettingEnum{} }
func (m *DisplayAdFormatSettingEnum) String() string { return proto.CompactTextString(m) }
func (*DisplayAdFormatSettingEnum) ProtoMessage()    {}
func (*DisplayAdFormatSettingEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_display_ad_format_setting_7b5f6590c96d9abd, []int{0}
}
func (m *DisplayAdFormatSettingEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DisplayAdFormatSettingEnum.Unmarshal(m, b)
}
func (m *DisplayAdFormatSettingEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DisplayAdFormatSettingEnum.Marshal(b, m, deterministic)
}
func (dst *DisplayAdFormatSettingEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DisplayAdFormatSettingEnum.Merge(dst, src)
}
func (m *DisplayAdFormatSettingEnum) XXX_Size() int {
	return xxx_messageInfo_DisplayAdFormatSettingEnum.Size(m)
}
func (m *DisplayAdFormatSettingEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_DisplayAdFormatSettingEnum.DiscardUnknown(m)
}

var xxx_messageInfo_DisplayAdFormatSettingEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*DisplayAdFormatSettingEnum)(nil), "google.ads.googleads.v0.enums.DisplayAdFormatSettingEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.DisplayAdFormatSettingEnum_DisplayAdFormatSetting", DisplayAdFormatSettingEnum_DisplayAdFormatSetting_name, DisplayAdFormatSettingEnum_DisplayAdFormatSetting_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/display_ad_format_setting.proto", fileDescriptor_display_ad_format_setting_7b5f6590c96d9abd)
}

var fileDescriptor_display_ad_format_setting_7b5f6590c96d9abd = []byte{
	// 313 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0x41, 0x4a, 0xf3, 0x40,
	0x1c, 0xc5, 0xbf, 0xa4, 0x1f, 0x15, 0xa6, 0xa0, 0x21, 0x0b, 0x17, 0x4a, 0x17, 0xed, 0x01, 0x26,
	0x01, 0x77, 0x23, 0x2e, 0xa6, 0x36, 0x2d, 0xc5, 0x3a, 0x09, 0xa6, 0x8d, 0x20, 0x81, 0x30, 0x76,
	0xe2, 0x10, 0x48, 0x32, 0x21, 0x93, 0x16, 0x5c, 0x7a, 0x15, 0x97, 0x1e, 0xc5, 0xa3, 0xb8, 0xf2,
	0x08, 0x92, 0x99, 0x36, 0xab, 0xea, 0x26, 0x3c, 0xf2, 0xfe, 0xbf, 0xc7, 0x9b, 0x07, 0x6e, 0xb8,
	0x10, 0x3c, 0x4f, 0x1d, 0xca, 0xa4, 0xa3, 0x65, 0xab, 0x76, 0xae, 0x93, 0x96, 0xdb, 0x42, 0x3a,
	0x2c, 0x93, 0x55, 0x4e, 0x5f, 0x13, 0xca, 0x92, 0x17, 0x51, 0x17, 0xb4, 0x49, 0x64, 0xda, 0x34,
	0x59, 0xc9, 0x61, 0x55, 0x8b, 0x46, 0xd8, 0x43, 0xcd, 0x40, 0xca, 0x24, 0xec, 0x70, 0xb8, 0x73,
	0xa1, 0xc2, 0xc7, 0x6f, 0x06, 0xb8, 0x98, 0xea, 0x08, 0xcc, 0x66, 0x2a, 0x20, 0xd4, 0xbc, 0x57,
	0x6e, 0x8b, 0xf1, 0x06, 0x9c, 0x1f, 0x77, 0xed, 0x33, 0x30, 0x58, 0x93, 0x30, 0xf0, 0x6e, 0x17,
	0xb3, 0x85, 0x37, 0xb5, 0xfe, 0xd9, 0x03, 0x70, 0xb2, 0x26, 0x77, 0xc4, 0x7f, 0x24, 0x96, 0xd1,
	0xba, 0x78, 0xb9, 0x4c, 0x66, 0xfe, 0xc3, 0x3d, 0x5e, 0x85, 0x96, 0x69, 0x9f, 0x02, 0x40, 0x7c,
	0x92, 0x10, 0xbc, 0x5a, 0x44, 0x9e, 0xd5, 0xb3, 0x01, 0xe8, 0xef, 0xf5, 0xff, 0xc9, 0xb7, 0x01,
	0x46, 0x1b, 0x51, 0xc0, 0x3f, 0x9b, 0x4e, 0x2e, 0x8f, 0x17, 0x09, 0xda, 0x57, 0x06, 0xc6, 0xd3,
	0x64, 0x4f, 0x73, 0x91, 0xd3, 0x92, 0x43, 0x51, 0x73, 0x87, 0xa7, 0xa5, 0xda, 0xe0, 0x30, 0x5b,
	0x95, 0xc9, 0x5f, 0x56, 0xbc, 0x56, 0xdf, 0x77, 0xb3, 0x37, 0xc7, 0xf8, 0xc3, 0x1c, 0xce, 0x75,
	0x14, 0x66, 0x12, 0x6a, 0xd9, 0xaa, 0xc8, 0x85, 0xed, 0x24, 0xf2, 0xf3, 0xe0, 0xc7, 0x98, 0xc9,
	0xb8, 0xf3, 0xe3, 0xc8, 0x8d, 0x95, 0xff, 0x65, 0x8e, 0xf4, 0x4f, 0x84, 0x30, 0x93, 0x08, 0x75,
	0x17, 0x08, 0x45, 0x2e, 0x42, 0xea, 0xe6, 0xb9, 0xaf, 0x8a, 0x5d, 0xfd, 0x04, 0x00, 0x00, 0xff,
	0xff, 0xa2, 0xdb, 0xb6, 0x98, 0xdd, 0x01, 0x00, 0x00,
}
