// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/errors/bidding_strategy_error.proto

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

// Enum describing possible bidding strategy errors.
type BiddingStrategyErrorEnum_BiddingStrategyError int32

const (
	// Enum unspecified.
	BiddingStrategyErrorEnum_UNSPECIFIED BiddingStrategyErrorEnum_BiddingStrategyError = 0
	// The received error code is not known in this version.
	BiddingStrategyErrorEnum_UNKNOWN BiddingStrategyErrorEnum_BiddingStrategyError = 1
	// Each bidding strategy must have a unique name.
	BiddingStrategyErrorEnum_DUPLICATE_NAME BiddingStrategyErrorEnum_BiddingStrategyError = 2
	// Bidding strategy type is immutable.
	BiddingStrategyErrorEnum_CANNOT_CHANGE_BIDDING_STRATEGY_TYPE BiddingStrategyErrorEnum_BiddingStrategyError = 3
	// Only bidding strategies not linked to campaigns, adgroups or adgroup
	// criteria can be removed.
	BiddingStrategyErrorEnum_CANNOT_REMOVE_ASSOCIATED_STRATEGY BiddingStrategyErrorEnum_BiddingStrategyError = 4
	// The specified bidding strategy is not supported.
	BiddingStrategyErrorEnum_BIDDING_STRATEGY_NOT_SUPPORTED BiddingStrategyErrorEnum_BiddingStrategyError = 5
)

var BiddingStrategyErrorEnum_BiddingStrategyError_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "DUPLICATE_NAME",
	3: "CANNOT_CHANGE_BIDDING_STRATEGY_TYPE",
	4: "CANNOT_REMOVE_ASSOCIATED_STRATEGY",
	5: "BIDDING_STRATEGY_NOT_SUPPORTED",
}
var BiddingStrategyErrorEnum_BiddingStrategyError_value = map[string]int32{
	"UNSPECIFIED":                         0,
	"UNKNOWN":                             1,
	"DUPLICATE_NAME":                      2,
	"CANNOT_CHANGE_BIDDING_STRATEGY_TYPE": 3,
	"CANNOT_REMOVE_ASSOCIATED_STRATEGY":   4,
	"BIDDING_STRATEGY_NOT_SUPPORTED":      5,
}

func (x BiddingStrategyErrorEnum_BiddingStrategyError) String() string {
	return proto.EnumName(BiddingStrategyErrorEnum_BiddingStrategyError_name, int32(x))
}
func (BiddingStrategyErrorEnum_BiddingStrategyError) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_bidding_strategy_error_9326fedde5e767e1, []int{0, 0}
}

// Container for enum describing possible bidding strategy errors.
type BiddingStrategyErrorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *BiddingStrategyErrorEnum) Reset()         { *m = BiddingStrategyErrorEnum{} }
func (m *BiddingStrategyErrorEnum) String() string { return proto.CompactTextString(m) }
func (*BiddingStrategyErrorEnum) ProtoMessage()    {}
func (*BiddingStrategyErrorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_bidding_strategy_error_9326fedde5e767e1, []int{0}
}
func (m *BiddingStrategyErrorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BiddingStrategyErrorEnum.Unmarshal(m, b)
}
func (m *BiddingStrategyErrorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BiddingStrategyErrorEnum.Marshal(b, m, deterministic)
}
func (dst *BiddingStrategyErrorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BiddingStrategyErrorEnum.Merge(dst, src)
}
func (m *BiddingStrategyErrorEnum) XXX_Size() int {
	return xxx_messageInfo_BiddingStrategyErrorEnum.Size(m)
}
func (m *BiddingStrategyErrorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_BiddingStrategyErrorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_BiddingStrategyErrorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*BiddingStrategyErrorEnum)(nil), "google.ads.googleads.v0.errors.BiddingStrategyErrorEnum")
	proto.RegisterEnum("google.ads.googleads.v0.errors.BiddingStrategyErrorEnum_BiddingStrategyError", BiddingStrategyErrorEnum_BiddingStrategyError_name, BiddingStrategyErrorEnum_BiddingStrategyError_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/errors/bidding_strategy_error.proto", fileDescriptor_bidding_strategy_error_9326fedde5e767e1)
}

var fileDescriptor_bidding_strategy_error_9326fedde5e767e1 = []byte{
	// 370 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x91, 0x4f, 0x8a, 0xdb, 0x30,
	0x14, 0xc6, 0x6b, 0xa7, 0x7f, 0x40, 0x81, 0xd6, 0x88, 0x2e, 0xda, 0x4d, 0xa0, 0x2e, 0xa5, 0x3b,
	0xd9, 0xd0, 0x9d, 0xb2, 0x92, 0x6d, 0xd5, 0x35, 0x6d, 0x64, 0x13, 0xff, 0x29, 0x29, 0x06, 0xe1,
	0xd4, 0x46, 0x04, 0x12, 0x2b, 0x58, 0x69, 0xa0, 0xd7, 0xe9, 0xb2, 0x67, 0x98, 0x13, 0xcc, 0x0d,
	0xe6, 0x0a, 0xb3, 0x9e, 0x03, 0x0c, 0xb6, 0x12, 0x6f, 0x26, 0x33, 0x2b, 0x7d, 0xbc, 0xf7, 0xfb,
	0x9e, 0xa4, 0xef, 0x81, 0xb9, 0x90, 0x52, 0x6c, 0x1b, 0xa7, 0xaa, 0x95, 0xa3, 0x65, 0xaf, 0x8e,
	0xae, 0xd3, 0x74, 0x9d, 0xec, 0x94, 0xb3, 0xde, 0xd4, 0xf5, 0xa6, 0x15, 0x5c, 0x1d, 0xba, 0xea,
	0xd0, 0x88, 0xbf, 0x7c, 0xa8, 0xa3, 0x7d, 0x27, 0x0f, 0x12, 0xce, 0xb4, 0x03, 0x55, 0xb5, 0x42,
	0xa3, 0x19, 0x1d, 0x5d, 0xa4, 0xcd, 0xf6, 0x8d, 0x01, 0xde, 0x79, 0x7a, 0x40, 0x7a, 0xf2, 0xd3,
	0xbe, 0x43, 0xdb, 0x3f, 0x3b, 0xfb, 0xca, 0x00, 0x6f, 0x2f, 0x35, 0xe1, 0x1b, 0x30, 0xcd, 0x59,
	0x9a, 0x50, 0x3f, 0xfa, 0x1a, 0xd1, 0xc0, 0x7a, 0x06, 0xa7, 0xe0, 0x55, 0xce, 0xbe, 0xb3, 0xf8,
	0x27, 0xb3, 0x0c, 0x08, 0xc1, 0xeb, 0x20, 0x4f, 0x7e, 0x44, 0x3e, 0xc9, 0x28, 0x67, 0x64, 0x41,
	0x2d, 0x13, 0x7e, 0x06, 0x1f, 0x7d, 0xc2, 0x58, 0x9c, 0x71, 0xff, 0x1b, 0x61, 0x21, 0xe5, 0x5e,
	0x14, 0x04, 0x11, 0x0b, 0x79, 0x9a, 0x2d, 0x49, 0x46, 0xc3, 0x15, 0xcf, 0x56, 0x09, 0xb5, 0x26,
	0xf0, 0x13, 0xf8, 0x70, 0x02, 0x97, 0x74, 0x11, 0x17, 0x94, 0x93, 0x34, 0x8d, 0xfd, 0x88, 0x64,
	0x34, 0x18, 0x59, 0xeb, 0x39, 0xb4, 0xc1, 0xec, 0xc1, 0x84, 0xde, 0x94, 0xe6, 0x49, 0x12, 0x2f,
	0x33, 0x1a, 0x58, 0x2f, 0xbc, 0x3b, 0x03, 0xd8, 0xbf, 0xe5, 0x0e, 0x3d, 0x1d, 0x81, 0xf7, 0xfe,
	0xd2, 0x17, 0x93, 0x3e, 0xbd, 0xc4, 0xf8, 0x15, 0x9c, 0xcc, 0x42, 0x6e, 0xab, 0x56, 0x20, 0xd9,
	0x09, 0x47, 0x34, 0xed, 0x90, 0xed, 0x79, 0x19, 0xfb, 0x8d, 0x7a, 0x6c, 0x37, 0x73, 0x7d, 0xfc,
	0x33, 0x27, 0x21, 0x21, 0xff, 0xcd, 0x59, 0xa8, 0x87, 0x91, 0x5a, 0x21, 0x2d, 0x7b, 0x55, 0xb8,
	0x68, 0xb8, 0x52, 0x5d, 0x9f, 0x81, 0x92, 0xd4, 0xaa, 0x1c, 0x81, 0xb2, 0x70, 0x4b, 0x0d, 0xdc,
	0x9a, 0xb6, 0xae, 0x62, 0x4c, 0x6a, 0x85, 0xf1, 0x88, 0x60, 0x5c, 0xb8, 0x18, 0x6b, 0x68, 0xfd,
	0x72, 0x78, 0xdd, 0x97, 0xfb, 0x00, 0x00, 0x00, 0xff, 0xff, 0x10, 0xa8, 0x2f, 0x13, 0x38, 0x02,
	0x00, 0x00,
}
