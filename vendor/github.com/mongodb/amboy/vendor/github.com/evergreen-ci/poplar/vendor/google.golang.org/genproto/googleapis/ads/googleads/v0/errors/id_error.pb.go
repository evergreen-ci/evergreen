// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/errors/id_error.proto

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

// Enum describing possible id errors.
type IdErrorEnum_IdError int32

const (
	// Enum unspecified.
	IdErrorEnum_UNSPECIFIED IdErrorEnum_IdError = 0
	// The received error code is not known in this version.
	IdErrorEnum_UNKNOWN IdErrorEnum_IdError = 1
	// Id not found
	IdErrorEnum_NOT_FOUND IdErrorEnum_IdError = 2
)

var IdErrorEnum_IdError_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "NOT_FOUND",
}
var IdErrorEnum_IdError_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"NOT_FOUND":   2,
}

func (x IdErrorEnum_IdError) String() string {
	return proto.EnumName(IdErrorEnum_IdError_name, int32(x))
}
func (IdErrorEnum_IdError) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_id_error_14c98c3d0d6f4f53, []int{0, 0}
}

// Container for enum describing possible id errors.
type IdErrorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *IdErrorEnum) Reset()         { *m = IdErrorEnum{} }
func (m *IdErrorEnum) String() string { return proto.CompactTextString(m) }
func (*IdErrorEnum) ProtoMessage()    {}
func (*IdErrorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_id_error_14c98c3d0d6f4f53, []int{0}
}
func (m *IdErrorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_IdErrorEnum.Unmarshal(m, b)
}
func (m *IdErrorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_IdErrorEnum.Marshal(b, m, deterministic)
}
func (dst *IdErrorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_IdErrorEnum.Merge(dst, src)
}
func (m *IdErrorEnum) XXX_Size() int {
	return xxx_messageInfo_IdErrorEnum.Size(m)
}
func (m *IdErrorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_IdErrorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_IdErrorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*IdErrorEnum)(nil), "google.ads.googleads.v0.errors.IdErrorEnum")
	proto.RegisterEnum("google.ads.googleads.v0.errors.IdErrorEnum_IdError", IdErrorEnum_IdError_name, IdErrorEnum_IdError_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/errors/id_error.proto", fileDescriptor_id_error_14c98c3d0d6f4f53)
}

var fileDescriptor_id_error_14c98c3d0d6f4f53 = []byte{
	// 263 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xd2, 0x4d, 0xcf, 0xcf, 0x4f,
	0xcf, 0x49, 0xd5, 0x4f, 0x4c, 0x29, 0xd6, 0x87, 0x30, 0x41, 0xac, 0x32, 0x03, 0xfd, 0xd4, 0xa2,
	0xa2, 0xfc, 0xa2, 0x62, 0xfd, 0xcc, 0x94, 0x78, 0x30, 0x4b, 0xaf, 0xa0, 0x28, 0xbf, 0x24, 0x5f,
	0x48, 0x0e, 0xa2, 0x46, 0x2f, 0x31, 0xa5, 0x58, 0x0f, 0xae, 0x5c, 0xaf, 0xcc, 0x40, 0x0f, 0xa2,
	0x5c, 0xc9, 0x95, 0x8b, 0xdb, 0x33, 0xc5, 0x15, 0xc4, 0x76, 0xcd, 0x2b, 0xcd, 0x55, 0x32, 0xe3,
	0x62, 0x87, 0x72, 0x85, 0xf8, 0xb9, 0xb8, 0x43, 0xfd, 0x82, 0x03, 0x5c, 0x9d, 0x3d, 0xdd, 0x3c,
	0x5d, 0x5d, 0x04, 0x18, 0x84, 0xb8, 0xb9, 0xd8, 0x43, 0xfd, 0xbc, 0xfd, 0xfc, 0xc3, 0xfd, 0x04,
	0x18, 0x85, 0x78, 0xb9, 0x38, 0xfd, 0xfc, 0x43, 0xe2, 0xdd, 0xfc, 0x43, 0xfd, 0x5c, 0x04, 0x98,
	0x9c, 0x9e, 0x33, 0x72, 0x29, 0x25, 0xe7, 0xe7, 0xea, 0xe1, 0xb7, 0xcd, 0x89, 0x07, 0x6a, 0x78,
	0x00, 0xc8, 0x6d, 0x01, 0x8c, 0x51, 0x2e, 0x50, 0xf5, 0xe9, 0xf9, 0x39, 0x89, 0x79, 0xe9, 0x7a,
	0xf9, 0x45, 0xe9, 0xfa, 0xe9, 0xa9, 0x79, 0x60, 0x97, 0xc3, 0x3c, 0x57, 0x90, 0x59, 0x8c, 0xcb,
	0xaf, 0xd6, 0x10, 0x6a, 0x11, 0x13, 0xb3, 0xbb, 0xa3, 0xe3, 0x2a, 0x26, 0x39, 0x77, 0x88, 0x61,
	0x8e, 0x29, 0xc5, 0x7a, 0x10, 0x26, 0x88, 0x15, 0x66, 0xa0, 0x07, 0xb6, 0xb2, 0xf8, 0x14, 0x4c,
	0x41, 0x8c, 0x63, 0x4a, 0x71, 0x0c, 0x5c, 0x41, 0x4c, 0x98, 0x41, 0x0c, 0x44, 0xc1, 0x2b, 0x26,
	0x25, 0x88, 0xa8, 0x95, 0x95, 0x63, 0x4a, 0xb1, 0x95, 0x15, 0x5c, 0x89, 0x95, 0x55, 0x98, 0x81,
	0x95, 0x15, 0x44, 0x51, 0x12, 0x1b, 0xd8, 0x75, 0xc6, 0x80, 0x00, 0x00, 0x00, 0xff, 0xff, 0x31,
	0x8b, 0x2c, 0xdc, 0x88, 0x01, 0x00, 0x00,
}
