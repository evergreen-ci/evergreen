// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/proximity_radius_units.proto

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

// The unit of radius distance in proximity (e.g. MILES)
type ProximityRadiusUnitsEnum_ProximityRadiusUnits int32

const (
	// Not specified.
	ProximityRadiusUnitsEnum_UNSPECIFIED ProximityRadiusUnitsEnum_ProximityRadiusUnits = 0
	// Used for return value only. Represents value unknown in this version.
	ProximityRadiusUnitsEnum_UNKNOWN ProximityRadiusUnitsEnum_ProximityRadiusUnits = 1
	// Miles
	ProximityRadiusUnitsEnum_MILES ProximityRadiusUnitsEnum_ProximityRadiusUnits = 2
	// Kilometers
	ProximityRadiusUnitsEnum_KILOMETERS ProximityRadiusUnitsEnum_ProximityRadiusUnits = 3
)

var ProximityRadiusUnitsEnum_ProximityRadiusUnits_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "MILES",
	3: "KILOMETERS",
}
var ProximityRadiusUnitsEnum_ProximityRadiusUnits_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"MILES":       2,
	"KILOMETERS":  3,
}

func (x ProximityRadiusUnitsEnum_ProximityRadiusUnits) String() string {
	return proto.EnumName(ProximityRadiusUnitsEnum_ProximityRadiusUnits_name, int32(x))
}
func (ProximityRadiusUnitsEnum_ProximityRadiusUnits) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_proximity_radius_units_f7dfd2e5b5bea1a3, []int{0, 0}
}

// Container for enum describing unit of radius in proximity.
type ProximityRadiusUnitsEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ProximityRadiusUnitsEnum) Reset()         { *m = ProximityRadiusUnitsEnum{} }
func (m *ProximityRadiusUnitsEnum) String() string { return proto.CompactTextString(m) }
func (*ProximityRadiusUnitsEnum) ProtoMessage()    {}
func (*ProximityRadiusUnitsEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_proximity_radius_units_f7dfd2e5b5bea1a3, []int{0}
}
func (m *ProximityRadiusUnitsEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ProximityRadiusUnitsEnum.Unmarshal(m, b)
}
func (m *ProximityRadiusUnitsEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ProximityRadiusUnitsEnum.Marshal(b, m, deterministic)
}
func (dst *ProximityRadiusUnitsEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ProximityRadiusUnitsEnum.Merge(dst, src)
}
func (m *ProximityRadiusUnitsEnum) XXX_Size() int {
	return xxx_messageInfo_ProximityRadiusUnitsEnum.Size(m)
}
func (m *ProximityRadiusUnitsEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_ProximityRadiusUnitsEnum.DiscardUnknown(m)
}

var xxx_messageInfo_ProximityRadiusUnitsEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*ProximityRadiusUnitsEnum)(nil), "google.ads.googleads.v0.enums.ProximityRadiusUnitsEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.ProximityRadiusUnitsEnum_ProximityRadiusUnits", ProximityRadiusUnitsEnum_ProximityRadiusUnits_name, ProximityRadiusUnitsEnum_ProximityRadiusUnits_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/proximity_radius_units.proto", fileDescriptor_proximity_radius_units_f7dfd2e5b5bea1a3)
}

var fileDescriptor_proximity_radius_units_f7dfd2e5b5bea1a3 = []byte{
	// 296 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xb2, 0x4a, 0xcf, 0xcf, 0x4f,
	0xcf, 0x49, 0xd5, 0x4f, 0x4c, 0x29, 0xd6, 0x87, 0x30, 0x41, 0xac, 0x32, 0x03, 0xfd, 0xd4, 0xbc,
	0xd2, 0xdc, 0x62, 0xfd, 0x82, 0xa2, 0xfc, 0x8a, 0xcc, 0xdc, 0xcc, 0x92, 0xca, 0xf8, 0xa2, 0xc4,
	0x94, 0xcc, 0xd2, 0xe2, 0xf8, 0xd2, 0xbc, 0xcc, 0x92, 0x62, 0xbd, 0x82, 0xa2, 0xfc, 0x92, 0x7c,
	0x21, 0x59, 0x88, 0x06, 0xbd, 0xc4, 0x94, 0x62, 0x3d, 0xb8, 0x5e, 0xbd, 0x32, 0x03, 0x3d, 0xb0,
	0x5e, 0xa5, 0x6c, 0x2e, 0x89, 0x00, 0x98, 0xf6, 0x20, 0xb0, 0xee, 0x50, 0x90, 0x66, 0xd7, 0xbc,
	0xd2, 0x5c, 0x25, 0x7f, 0x2e, 0x11, 0x6c, 0x72, 0x42, 0xfc, 0x5c, 0xdc, 0xa1, 0x7e, 0xc1, 0x01,
	0xae, 0xce, 0x9e, 0x6e, 0x9e, 0xae, 0x2e, 0x02, 0x0c, 0x42, 0xdc, 0x5c, 0xec, 0xa1, 0x7e, 0xde,
	0x7e, 0xfe, 0xe1, 0x7e, 0x02, 0x8c, 0x42, 0x9c, 0x5c, 0xac, 0xbe, 0x9e, 0x3e, 0xae, 0xc1, 0x02,
	0x4c, 0x42, 0x7c, 0x5c, 0x5c, 0xde, 0x9e, 0x3e, 0xfe, 0xbe, 0xae, 0x21, 0xae, 0x41, 0xc1, 0x02,
	0xcc, 0x4e, 0xef, 0x18, 0xb9, 0x14, 0x93, 0xf3, 0x73, 0xf5, 0xf0, 0x3a, 0xc9, 0x49, 0x12, 0x9b,
	0xa5, 0x01, 0x20, 0xcf, 0x04, 0x30, 0x46, 0x39, 0x41, 0xf5, 0xa6, 0xe7, 0xe7, 0x24, 0xe6, 0xa5,
	0xeb, 0xe5, 0x17, 0xa5, 0xeb, 0xa7, 0xa7, 0xe6, 0x81, 0xbd, 0x0a, 0x0b, 0x9a, 0x82, 0xcc, 0x62,
	0x1c, 0x21, 0x65, 0x0d, 0x26, 0x17, 0x31, 0x31, 0xbb, 0x3b, 0x3a, 0xae, 0x62, 0x92, 0x75, 0x87,
	0x18, 0xe5, 0x98, 0x52, 0xac, 0x07, 0x61, 0x82, 0x58, 0x61, 0x06, 0x7a, 0x20, 0xcf, 0x17, 0x9f,
	0x82, 0xc9, 0xc7, 0x38, 0xa6, 0x14, 0xc7, 0xc0, 0xe5, 0x63, 0xc2, 0x0c, 0x62, 0xc0, 0xf2, 0xaf,
	0x98, 0x14, 0x21, 0x82, 0x56, 0x56, 0x8e, 0x29, 0xc5, 0x56, 0x56, 0x70, 0x15, 0x56, 0x56, 0x61,
	0x06, 0x56, 0x56, 0x60, 0x35, 0x49, 0x6c, 0x60, 0x87, 0x19, 0x03, 0x02, 0x00, 0x00, 0xff, 0xff,
	0x01, 0x45, 0x1e, 0x16, 0xc1, 0x01, 0x00, 0x00,
}
