// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/account_budget_proposal_type.proto

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

// The possible types of an AccountBudgetProposal.
type AccountBudgetProposalTypeEnum_AccountBudgetProposalType int32

const (
	// Not specified.
	AccountBudgetProposalTypeEnum_UNSPECIFIED AccountBudgetProposalTypeEnum_AccountBudgetProposalType = 0
	// Used for return value only. Represents value unknown in this version.
	AccountBudgetProposalTypeEnum_UNKNOWN AccountBudgetProposalTypeEnum_AccountBudgetProposalType = 1
	// Identifies a request to create a new budget.
	AccountBudgetProposalTypeEnum_CREATE AccountBudgetProposalTypeEnum_AccountBudgetProposalType = 2
	// Identifies a request to edit an existing budget.
	AccountBudgetProposalTypeEnum_UPDATE AccountBudgetProposalTypeEnum_AccountBudgetProposalType = 3
	// Identifies a request to end a budget that has already started.
	AccountBudgetProposalTypeEnum_END AccountBudgetProposalTypeEnum_AccountBudgetProposalType = 4
	// Identifies a request to remove a budget that hasn't started yet.
	AccountBudgetProposalTypeEnum_REMOVE AccountBudgetProposalTypeEnum_AccountBudgetProposalType = 5
)

var AccountBudgetProposalTypeEnum_AccountBudgetProposalType_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "CREATE",
	3: "UPDATE",
	4: "END",
	5: "REMOVE",
}
var AccountBudgetProposalTypeEnum_AccountBudgetProposalType_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"CREATE":      2,
	"UPDATE":      3,
	"END":         4,
	"REMOVE":      5,
}

func (x AccountBudgetProposalTypeEnum_AccountBudgetProposalType) String() string {
	return proto.EnumName(AccountBudgetProposalTypeEnum_AccountBudgetProposalType_name, int32(x))
}
func (AccountBudgetProposalTypeEnum_AccountBudgetProposalType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_account_budget_proposal_type_181489b317bfe7c1, []int{0, 0}
}

// Message describing AccountBudgetProposal types.
type AccountBudgetProposalTypeEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *AccountBudgetProposalTypeEnum) Reset()         { *m = AccountBudgetProposalTypeEnum{} }
func (m *AccountBudgetProposalTypeEnum) String() string { return proto.CompactTextString(m) }
func (*AccountBudgetProposalTypeEnum) ProtoMessage()    {}
func (*AccountBudgetProposalTypeEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_account_budget_proposal_type_181489b317bfe7c1, []int{0}
}
func (m *AccountBudgetProposalTypeEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AccountBudgetProposalTypeEnum.Unmarshal(m, b)
}
func (m *AccountBudgetProposalTypeEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AccountBudgetProposalTypeEnum.Marshal(b, m, deterministic)
}
func (dst *AccountBudgetProposalTypeEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AccountBudgetProposalTypeEnum.Merge(dst, src)
}
func (m *AccountBudgetProposalTypeEnum) XXX_Size() int {
	return xxx_messageInfo_AccountBudgetProposalTypeEnum.Size(m)
}
func (m *AccountBudgetProposalTypeEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_AccountBudgetProposalTypeEnum.DiscardUnknown(m)
}

var xxx_messageInfo_AccountBudgetProposalTypeEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*AccountBudgetProposalTypeEnum)(nil), "google.ads.googleads.v0.enums.AccountBudgetProposalTypeEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.AccountBudgetProposalTypeEnum_AccountBudgetProposalType", AccountBudgetProposalTypeEnum_AccountBudgetProposalType_name, AccountBudgetProposalTypeEnum_AccountBudgetProposalType_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/account_budget_proposal_type.proto", fileDescriptor_account_budget_proposal_type_181489b317bfe7c1)
}

var fileDescriptor_account_budget_proposal_type_181489b317bfe7c1 = []byte{
	// 317 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0xc1, 0x6a, 0xfa, 0x30,
	0x1c, 0xc7, 0xff, 0xad, 0xff, 0x29, 0xc4, 0xc3, 0x4a, 0x6f, 0x3b, 0x38, 0xd0, 0x07, 0x48, 0x0b,
	0xbb, 0x65, 0x97, 0xa5, 0x9a, 0x89, 0x8c, 0xd5, 0xe2, 0xb4, 0x83, 0x51, 0x90, 0x6a, 0xb2, 0x30,
	0xd0, 0x26, 0x34, 0xad, 0xe0, 0x13, 0xec, 0x3d, 0x76, 0xdc, 0xa3, 0xec, 0x51, 0x76, 0xdd, 0x0b,
	0x8c, 0x24, 0xda, 0x5b, 0x77, 0x29, 0x1f, 0x7e, 0xbf, 0x5f, 0x3f, 0x7c, 0xf3, 0x05, 0x77, 0x5c,
	0x08, 0xbe, 0x63, 0x41, 0x4e, 0x55, 0x60, 0x51, 0xd3, 0x21, 0x0c, 0x58, 0x51, 0xef, 0x55, 0x90,
	0x6f, 0xb7, 0xa2, 0x2e, 0xaa, 0xf5, 0xa6, 0xa6, 0x9c, 0x55, 0x6b, 0x59, 0x0a, 0x29, 0x54, 0xbe,
	0x5b, 0x57, 0x47, 0xc9, 0xa0, 0x2c, 0x45, 0x25, 0xfc, 0x81, 0xfd, 0x0d, 0xe6, 0x54, 0xc1, 0xc6,
	0x00, 0x0f, 0x21, 0x34, 0x86, 0xd1, 0xbb, 0x03, 0x06, 0xd8, 0x5a, 0x22, 0x23, 0x49, 0x4e, 0x8e,
	0xe5, 0x51, 0x32, 0x52, 0xd4, 0xfb, 0xd1, 0x2b, 0xb8, 0x6a, 0x3d, 0xf0, 0x2f, 0x41, 0x7f, 0x15,
	0x3f, 0x25, 0x64, 0x3c, 0xbb, 0x9f, 0x91, 0x89, 0xf7, 0xcf, 0xef, 0x83, 0xde, 0x2a, 0x7e, 0x88,
	0xe7, 0xcf, 0xb1, 0xe7, 0xf8, 0x00, 0x74, 0xc7, 0x0b, 0x82, 0x97, 0xc4, 0x73, 0x35, 0xaf, 0x92,
	0x89, 0xe6, 0x8e, 0xdf, 0x03, 0x1d, 0x12, 0x4f, 0xbc, 0xff, 0x7a, 0xb8, 0x20, 0x8f, 0xf3, 0x94,
	0x78, 0x17, 0xd1, 0x8f, 0x03, 0x86, 0x5b, 0xb1, 0x87, 0x7f, 0xe6, 0x8d, 0xae, 0x5b, 0xb3, 0x24,
	0xfa, 0xb9, 0x89, 0xf3, 0x12, 0x9d, 0x04, 0x5c, 0xec, 0xf2, 0x82, 0x43, 0x51, 0xf2, 0x80, 0xb3,
	0xc2, 0x94, 0x71, 0xae, 0x50, 0xbe, 0xa9, 0x96, 0x46, 0x6f, 0xcd, 0xf7, 0xc3, 0xed, 0x4c, 0x31,
	0xfe, 0x74, 0x07, 0x53, 0xab, 0xc2, 0x54, 0x41, 0x8b, 0x9a, 0xd2, 0x10, 0xea, 0x62, 0xd4, 0xd7,
	0x79, 0x9f, 0x61, 0xaa, 0xb2, 0x66, 0x9f, 0xa5, 0x61, 0x66, 0xf6, 0xdf, 0xee, 0xd0, 0x0e, 0x11,
	0xc2, 0x54, 0x21, 0xd4, 0x5c, 0x20, 0x94, 0x86, 0x08, 0x99, 0x9b, 0x4d, 0xd7, 0x04, 0xbb, 0xf9,
	0x0d, 0x00, 0x00, 0xff, 0xff, 0xd8, 0xbe, 0x0d, 0x46, 0xe9, 0x01, 0x00, 0x00,
}
