// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/genomics/v1/range.proto

package genomics // import "google.golang.org/genproto/googleapis/genomics/v1"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import _ "google.golang.org/genproto/googleapis/api/annotations"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// A 0-based half-open genomic coordinate range for search requests.
type Range struct {
	// The reference sequence name, for example `chr1`,
	// `1`, or `chrX`.
	ReferenceName string `protobuf:"bytes,1,opt,name=reference_name,json=referenceName,proto3" json:"reference_name,omitempty"`
	// The start position of the range on the reference, 0-based inclusive.
	Start int64 `protobuf:"varint,2,opt,name=start,proto3" json:"start,omitempty"`
	// The end position of the range on the reference, 0-based exclusive.
	End                  int64    `protobuf:"varint,3,opt,name=end,proto3" json:"end,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Range) Reset()         { *m = Range{} }
func (m *Range) String() string { return proto.CompactTextString(m) }
func (*Range) ProtoMessage()    {}
func (*Range) Descriptor() ([]byte, []int) {
	return fileDescriptor_range_ea4bc4104a5a55de, []int{0}
}
func (m *Range) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Range.Unmarshal(m, b)
}
func (m *Range) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Range.Marshal(b, m, deterministic)
}
func (dst *Range) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Range.Merge(dst, src)
}
func (m *Range) XXX_Size() int {
	return xxx_messageInfo_Range.Size(m)
}
func (m *Range) XXX_DiscardUnknown() {
	xxx_messageInfo_Range.DiscardUnknown(m)
}

var xxx_messageInfo_Range proto.InternalMessageInfo

func (m *Range) GetReferenceName() string {
	if m != nil {
		return m.ReferenceName
	}
	return ""
}

func (m *Range) GetStart() int64 {
	if m != nil {
		return m.Start
	}
	return 0
}

func (m *Range) GetEnd() int64 {
	if m != nil {
		return m.End
	}
	return 0
}

func init() {
	proto.RegisterType((*Range)(nil), "google.genomics.v1.Range")
}

func init() {
	proto.RegisterFile("google/genomics/v1/range.proto", fileDescriptor_range_ea4bc4104a5a55de)
}

var fileDescriptor_range_ea4bc4104a5a55de = []byte{
	// 209 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x4f, 0x4d, 0x4b, 0xc4, 0x30,
	0x10, 0x25, 0x96, 0x15, 0x0c, 0x28, 0x12, 0x44, 0x8a, 0x88, 0x2c, 0x82, 0xb0, 0xa7, 0x84, 0xe2,
	0x4d, 0x6f, 0xfd, 0x01, 0x52, 0x7a, 0xf0, 0xe0, 0x45, 0xc6, 0x3a, 0x86, 0x40, 0x33, 0x53, 0x92,
	0xd0, 0xdf, 0xee, 0x51, 0x92, 0x58, 0x11, 0xf6, 0x36, 0x79, 0x1f, 0x79, 0xef, 0xc9, 0x3b, 0xcb,
	0x6c, 0x67, 0x34, 0x16, 0x89, 0xbd, 0x9b, 0xa2, 0x59, 0x3b, 0x13, 0x80, 0x2c, 0xea, 0x25, 0x70,
	0x62, 0xa5, 0x2a, 0xaf, 0x37, 0x5e, 0xaf, 0xdd, 0xcd, 0xed, 0xaf, 0x07, 0x16, 0x67, 0x80, 0x88,
	0x13, 0x24, 0xc7, 0x14, 0xab, 0xe3, 0xfe, 0x55, 0xee, 0xc6, 0xfc, 0x81, 0x7a, 0x90, 0x17, 0x01,
	0xbf, 0x30, 0x20, 0x4d, 0xf8, 0x4e, 0xe0, 0xb1, 0x15, 0x7b, 0x71, 0x38, 0x1b, 0xcf, 0xff, 0xd0,
	0x17, 0xf0, 0xa8, 0xae, 0xe4, 0x2e, 0x26, 0x08, 0xa9, 0x3d, 0xd9, 0x8b, 0x43, 0x33, 0xd6, 0x87,
	0xba, 0x94, 0x0d, 0xd2, 0x67, 0xdb, 0x14, 0x2c, 0x9f, 0x3d, 0xca, 0xeb, 0x89, 0xbd, 0x3e, 0xee,
	0xd3, 0xcb, 0x92, 0x37, 0xe4, 0xf4, 0x41, 0xbc, 0x3d, 0x6d, 0x0a, 0x9e, 0x81, 0xac, 0xe6, 0x60,
	0xf3, 0xb8, 0xd2, 0xcd, 0x54, 0x0a, 0x16, 0x17, 0xff, 0x0f, 0x7e, 0xde, 0xee, 0x6f, 0x21, 0x3e,
	0x4e, 0x8b, 0xf2, 0xf1, 0x27, 0x00, 0x00, 0xff, 0xff, 0xb7, 0x3e, 0xf1, 0x62, 0x19, 0x01, 0x00,
	0x00,
}
