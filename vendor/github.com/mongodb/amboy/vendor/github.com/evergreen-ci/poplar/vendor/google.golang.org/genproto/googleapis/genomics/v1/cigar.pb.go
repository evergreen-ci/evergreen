// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/genomics/v1/cigar.proto

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

// Describes the different types of CIGAR alignment operations that exist.
// Used wherever CIGAR alignments are used.
type CigarUnit_Operation int32

const (
	CigarUnit_OPERATION_UNSPECIFIED CigarUnit_Operation = 0
	// An alignment match indicates that a sequence can be aligned to the
	// reference without evidence of an INDEL. Unlike the
	// `SEQUENCE_MATCH` and `SEQUENCE_MISMATCH` operators,
	// the `ALIGNMENT_MATCH` operator does not indicate whether the
	// reference and read sequences are an exact match. This operator is
	// equivalent to SAM's `M`.
	CigarUnit_ALIGNMENT_MATCH CigarUnit_Operation = 1
	// The insert operator indicates that the read contains evidence of bases
	// being inserted into the reference. This operator is equivalent to SAM's
	// `I`.
	CigarUnit_INSERT CigarUnit_Operation = 2
	// The delete operator indicates that the read contains evidence of bases
	// being deleted from the reference. This operator is equivalent to SAM's
	// `D`.
	CigarUnit_DELETE CigarUnit_Operation = 3
	// The skip operator indicates that this read skips a long segment of the
	// reference, but the bases have not been deleted. This operator is commonly
	// used when working with RNA-seq data, where reads may skip long segments
	// of the reference between exons. This operator is equivalent to SAM's
	// `N`.
	CigarUnit_SKIP CigarUnit_Operation = 4
	// The soft clip operator indicates that bases at the start/end of a read
	// have not been considered during alignment. This may occur if the majority
	// of a read maps, except for low quality bases at the start/end of a read.
	// This operator is equivalent to SAM's `S`. Bases that are soft
	// clipped will still be stored in the read.
	CigarUnit_CLIP_SOFT CigarUnit_Operation = 5
	// The hard clip operator indicates that bases at the start/end of a read
	// have been omitted from this alignment. This may occur if this linear
	// alignment is part of a chimeric alignment, or if the read has been
	// trimmed (for example, during error correction or to trim poly-A tails for
	// RNA-seq). This operator is equivalent to SAM's `H`.
	CigarUnit_CLIP_HARD CigarUnit_Operation = 6
	// The pad operator indicates that there is padding in an alignment. This
	// operator is equivalent to SAM's `P`.
	CigarUnit_PAD CigarUnit_Operation = 7
	// This operator indicates that this portion of the aligned sequence exactly
	// matches the reference. This operator is equivalent to SAM's `=`.
	CigarUnit_SEQUENCE_MATCH CigarUnit_Operation = 8
	// This operator indicates that this portion of the aligned sequence is an
	// alignment match to the reference, but a sequence mismatch. This can
	// indicate a SNP or a read error. This operator is equivalent to SAM's
	// `X`.
	CigarUnit_SEQUENCE_MISMATCH CigarUnit_Operation = 9
)

var CigarUnit_Operation_name = map[int32]string{
	0: "OPERATION_UNSPECIFIED",
	1: "ALIGNMENT_MATCH",
	2: "INSERT",
	3: "DELETE",
	4: "SKIP",
	5: "CLIP_SOFT",
	6: "CLIP_HARD",
	7: "PAD",
	8: "SEQUENCE_MATCH",
	9: "SEQUENCE_MISMATCH",
}
var CigarUnit_Operation_value = map[string]int32{
	"OPERATION_UNSPECIFIED": 0,
	"ALIGNMENT_MATCH":       1,
	"INSERT":                2,
	"DELETE":                3,
	"SKIP":                  4,
	"CLIP_SOFT":             5,
	"CLIP_HARD":             6,
	"PAD":                   7,
	"SEQUENCE_MATCH":        8,
	"SEQUENCE_MISMATCH":     9,
}

func (x CigarUnit_Operation) String() string {
	return proto.EnumName(CigarUnit_Operation_name, int32(x))
}
func (CigarUnit_Operation) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_cigar_ce8c8036b76f9461, []int{0, 0}
}

// A single CIGAR operation.
type CigarUnit struct {
	Operation CigarUnit_Operation `protobuf:"varint,1,opt,name=operation,proto3,enum=google.genomics.v1.CigarUnit_Operation" json:"operation,omitempty"`
	// The number of genomic bases that the operation runs for. Required.
	OperationLength int64 `protobuf:"varint,2,opt,name=operation_length,json=operationLength,proto3" json:"operation_length,omitempty"`
	// `referenceSequence` is only used at mismatches
	// (`SEQUENCE_MISMATCH`) and deletions (`DELETE`).
	// Filling this field replaces SAM's MD tag. If the relevant information is
	// not available, this field is unset.
	ReferenceSequence    string   `protobuf:"bytes,3,opt,name=reference_sequence,json=referenceSequence,proto3" json:"reference_sequence,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *CigarUnit) Reset()         { *m = CigarUnit{} }
func (m *CigarUnit) String() string { return proto.CompactTextString(m) }
func (*CigarUnit) ProtoMessage()    {}
func (*CigarUnit) Descriptor() ([]byte, []int) {
	return fileDescriptor_cigar_ce8c8036b76f9461, []int{0}
}
func (m *CigarUnit) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_CigarUnit.Unmarshal(m, b)
}
func (m *CigarUnit) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_CigarUnit.Marshal(b, m, deterministic)
}
func (dst *CigarUnit) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CigarUnit.Merge(dst, src)
}
func (m *CigarUnit) XXX_Size() int {
	return xxx_messageInfo_CigarUnit.Size(m)
}
func (m *CigarUnit) XXX_DiscardUnknown() {
	xxx_messageInfo_CigarUnit.DiscardUnknown(m)
}

var xxx_messageInfo_CigarUnit proto.InternalMessageInfo

func (m *CigarUnit) GetOperation() CigarUnit_Operation {
	if m != nil {
		return m.Operation
	}
	return CigarUnit_OPERATION_UNSPECIFIED
}

func (m *CigarUnit) GetOperationLength() int64 {
	if m != nil {
		return m.OperationLength
	}
	return 0
}

func (m *CigarUnit) GetReferenceSequence() string {
	if m != nil {
		return m.ReferenceSequence
	}
	return ""
}

func init() {
	proto.RegisterType((*CigarUnit)(nil), "google.genomics.v1.CigarUnit")
	proto.RegisterEnum("google.genomics.v1.CigarUnit_Operation", CigarUnit_Operation_name, CigarUnit_Operation_value)
}

func init() {
	proto.RegisterFile("google/genomics/v1/cigar.proto", fileDescriptor_cigar_ce8c8036b76f9461)
}

var fileDescriptor_cigar_ce8c8036b76f9461 = []byte{
	// 367 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x6c, 0x51, 0xcf, 0x0e, 0x93, 0x30,
	0x1c, 0xb6, 0x63, 0x6e, 0xe3, 0x97, 0xb8, 0x75, 0x35, 0x33, 0xd3, 0x18, 0xb3, 0xec, 0xe2, 0x3c,
	0x08, 0x99, 0xde, 0xf4, 0xc4, 0xa0, 0x73, 0x8d, 0x0c, 0x10, 0xd8, 0xc5, 0x0b, 0x41, 0x52, 0x91,
	0x64, 0x6b, 0x11, 0x70, 0xaf, 0xe5, 0x1b, 0xf9, 0x1c, 0x1e, 0x0d, 0x30, 0x98, 0x89, 0xde, 0xbe,
	0x7e, 0xff, 0x9a, 0xfc, 0x3e, 0x78, 0x91, 0x4a, 0x99, 0x9e, 0xb9, 0x9e, 0x72, 0x21, 0x2f, 0x59,
	0x52, 0xea, 0xd7, 0xad, 0x9e, 0x64, 0x69, 0x5c, 0x68, 0x79, 0x21, 0x2b, 0x49, 0x48, 0xab, 0x6b,
	0x9d, 0xae, 0x5d, 0xb7, 0xcf, 0x9e, 0xdf, 0x32, 0x71, 0x9e, 0xe9, 0xb1, 0x10, 0xb2, 0x8a, 0xab,
	0x4c, 0x8a, 0xb2, 0x4d, 0xac, 0x7f, 0x0d, 0x40, 0x35, 0xeb, 0x86, 0x93, 0xc8, 0x2a, 0x42, 0x41,
	0x95, 0x39, 0x2f, 0x1a, 0xc7, 0x12, 0xad, 0xd0, 0x66, 0xfa, 0xe6, 0xa5, 0xf6, 0x6f, 0xa7, 0xd6,
	0x27, 0x34, 0xb7, 0xb3, 0xfb, 0xf7, 0x24, 0x79, 0x05, 0xb8, 0x7f, 0x44, 0x67, 0x2e, 0xd2, 0xea,
	0xdb, 0x72, 0xb0, 0x42, 0x1b, 0xc5, 0x9f, 0xf5, 0xbc, 0xdd, 0xd0, 0xe4, 0x35, 0x90, 0x82, 0x7f,
	0xe5, 0x05, 0x17, 0x09, 0x8f, 0x4a, 0xfe, 0xfd, 0x47, 0x0d, 0x96, 0xca, 0x0a, 0x6d, 0x54, 0x7f,
	0xde, 0x2b, 0xc1, 0x4d, 0x58, 0xff, 0x44, 0xa0, 0xf6, 0x5f, 0x92, 0xa7, 0xb0, 0x70, 0x3d, 0xea,
	0x1b, 0x21, 0x73, 0x9d, 0xe8, 0xe4, 0x04, 0x1e, 0x35, 0xd9, 0x9e, 0x51, 0x0b, 0x3f, 0x20, 0x8f,
	0x61, 0x66, 0xd8, 0xec, 0x83, 0x73, 0xa4, 0x4e, 0x18, 0x1d, 0x8d, 0xd0, 0x3c, 0x60, 0x44, 0x00,
	0x46, 0xcc, 0x09, 0xa8, 0x1f, 0xe2, 0x41, 0x8d, 0x2d, 0x6a, 0xd3, 0x90, 0x62, 0x85, 0x4c, 0x60,
	0x18, 0x7c, 0x64, 0x1e, 0x1e, 0x92, 0x47, 0xa0, 0x9a, 0x36, 0xf3, 0xa2, 0xc0, 0xdd, 0x87, 0xf8,
	0x61, 0xff, 0x3c, 0x18, 0xbe, 0x85, 0x47, 0x64, 0x0c, 0x8a, 0x67, 0x58, 0x78, 0x4c, 0x08, 0x4c,
	0x03, 0xfa, 0xe9, 0x44, 0x1d, 0x93, 0xde, 0xca, 0x27, 0x64, 0x01, 0xf3, 0x3b, 0xc7, 0x82, 0x96,
	0x56, 0x77, 0x1c, 0x9e, 0x24, 0xf2, 0xf2, 0x9f, 0x23, 0xee, 0xa0, 0xb9, 0xa2, 0x57, 0xcf, 0xe0,
	0xa1, 0xcf, 0xef, 0x3a, 0x87, 0x3c, 0xc7, 0x22, 0xd5, 0x64, 0x91, 0xd6, 0x2b, 0x37, 0x23, 0xe9,
	0xad, 0x14, 0xe7, 0x59, 0xf9, 0xf7, 0xf2, 0xef, 0x3b, 0xfc, 0x1b, 0xa1, 0x2f, 0xa3, 0xc6, 0xf9,
	0xf6, 0x4f, 0x00, 0x00, 0x00, 0xff, 0xff, 0x98, 0xcc, 0xce, 0xde, 0x22, 0x02, 0x00, 0x00,
}
