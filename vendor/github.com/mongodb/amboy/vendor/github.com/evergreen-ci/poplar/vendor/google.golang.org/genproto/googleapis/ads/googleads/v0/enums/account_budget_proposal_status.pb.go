// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/enums/account_budget_proposal_status.proto

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

// The possible statuses of an AccountBudgetProposal.
type AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus int32

const (
	// Not specified.
	AccountBudgetProposalStatusEnum_UNSPECIFIED AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus = 0
	// Used for return value only. Represents value unknown in this version.
	AccountBudgetProposalStatusEnum_UNKNOWN AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus = 1
	// The proposal is pending approval.
	AccountBudgetProposalStatusEnum_PENDING AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus = 2
	// The proposal has been approved but the corresponding billing setup
	// has not.  This can occur for proposals that set up the first budget
	// when signing up for billing or when performing a change of bill-to
	// operation.
	AccountBudgetProposalStatusEnum_APPROVED_HELD AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus = 3
	// The proposal has been approved.
	AccountBudgetProposalStatusEnum_APPROVED AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus = 4
	// The proposal has been cancelled by the user.
	AccountBudgetProposalStatusEnum_CANCELLED AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus = 5
	// The proposal has been rejected by the user, e.g. by rejecting an
	// acceptance email.
	AccountBudgetProposalStatusEnum_REJECTED AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus = 6
)

var AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "PENDING",
	3: "APPROVED_HELD",
	4: "APPROVED",
	5: "CANCELLED",
	6: "REJECTED",
}
var AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus_value = map[string]int32{
	"UNSPECIFIED":   0,
	"UNKNOWN":       1,
	"PENDING":       2,
	"APPROVED_HELD": 3,
	"APPROVED":      4,
	"CANCELLED":     5,
	"REJECTED":      6,
}

func (x AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus) String() string {
	return proto.EnumName(AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus_name, int32(x))
}
func (AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_account_budget_proposal_status_bfbb3995e19e7c60, []int{0, 0}
}

// Message describing AccountBudgetProposal statuses.
type AccountBudgetProposalStatusEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *AccountBudgetProposalStatusEnum) Reset()         { *m = AccountBudgetProposalStatusEnum{} }
func (m *AccountBudgetProposalStatusEnum) String() string { return proto.CompactTextString(m) }
func (*AccountBudgetProposalStatusEnum) ProtoMessage()    {}
func (*AccountBudgetProposalStatusEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_account_budget_proposal_status_bfbb3995e19e7c60, []int{0}
}
func (m *AccountBudgetProposalStatusEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AccountBudgetProposalStatusEnum.Unmarshal(m, b)
}
func (m *AccountBudgetProposalStatusEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AccountBudgetProposalStatusEnum.Marshal(b, m, deterministic)
}
func (dst *AccountBudgetProposalStatusEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AccountBudgetProposalStatusEnum.Merge(dst, src)
}
func (m *AccountBudgetProposalStatusEnum) XXX_Size() int {
	return xxx_messageInfo_AccountBudgetProposalStatusEnum.Size(m)
}
func (m *AccountBudgetProposalStatusEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_AccountBudgetProposalStatusEnum.DiscardUnknown(m)
}

var xxx_messageInfo_AccountBudgetProposalStatusEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*AccountBudgetProposalStatusEnum)(nil), "google.ads.googleads.v0.enums.AccountBudgetProposalStatusEnum")
	proto.RegisterEnum("google.ads.googleads.v0.enums.AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus", AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus_name, AccountBudgetProposalStatusEnum_AccountBudgetProposalStatus_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/enums/account_budget_proposal_status.proto", fileDescriptor_account_budget_proposal_status_bfbb3995e19e7c60)
}

var fileDescriptor_account_budget_proposal_status_bfbb3995e19e7c60 = []byte{
	// 338 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0xd1, 0x4e, 0xf2, 0x30,
	0x1c, 0xc5, 0xbf, 0x8d, 0x4f, 0xd4, 0x22, 0xb1, 0xee, 0xd6, 0x10, 0x85, 0x07, 0xe8, 0x96, 0x78,
	0x57, 0xaf, 0xba, 0xad, 0x22, 0x4a, 0x4a, 0x03, 0x32, 0x13, 0xb3, 0x64, 0x19, 0x6c, 0x69, 0x4c,
	0x60, 0x5d, 0xe8, 0xc6, 0x23, 0xf8, 0x20, 0x5e, 0xf2, 0x28, 0x3e, 0x8a, 0xf7, 0xde, 0x9b, 0x75,
	0xc0, 0x9d, 0xbb, 0x69, 0x4e, 0xff, 0xe7, 0xf4, 0x97, 0x7f, 0x0f, 0x70, 0x85, 0x94, 0x62, 0x95,
	0xda, 0x71, 0xa2, 0xec, 0x5a, 0x56, 0x6a, 0xeb, 0xd8, 0x69, 0x56, 0xae, 0x95, 0x1d, 0x2f, 0x97,
	0xb2, 0xcc, 0x8a, 0x68, 0x51, 0x26, 0x22, 0x2d, 0xa2, 0x7c, 0x23, 0x73, 0xa9, 0xe2, 0x55, 0xa4,
	0x8a, 0xb8, 0x28, 0x15, 0xca, 0x37, 0xb2, 0x90, 0x56, 0xaf, 0x7e, 0x88, 0xe2, 0x44, 0xa1, 0x23,
	0x03, 0x6d, 0x1d, 0xa4, 0x19, 0x83, 0x9d, 0x01, 0x6e, 0x48, 0xcd, 0x71, 0x35, 0x86, 0xef, 0x29,
	0x33, 0x0d, 0xa1, 0x59, 0xb9, 0x1e, 0x7c, 0x18, 0xe0, 0xba, 0x21, 0x63, 0x5d, 0x82, 0xce, 0x9c,
	0xcd, 0x38, 0xf5, 0x46, 0x0f, 0x23, 0xea, 0xc3, 0x7f, 0x56, 0x07, 0x9c, 0xce, 0xd9, 0x33, 0x9b,
	0xbc, 0x32, 0x68, 0x54, 0x17, 0x4e, 0x99, 0x3f, 0x62, 0x43, 0x68, 0x5a, 0x57, 0xa0, 0x4b, 0x38,
	0x9f, 0x4e, 0x02, 0xea, 0x47, 0x8f, 0x74, 0xec, 0xc3, 0x96, 0x75, 0x01, 0xce, 0x0e, 0x23, 0xf8,
	0xdf, 0xea, 0x82, 0x73, 0x8f, 0x30, 0x8f, 0x8e, 0xc7, 0xd4, 0x87, 0x27, 0x95, 0x39, 0xa5, 0x4f,
	0xd4, 0x7b, 0xa1, 0x3e, 0x6c, 0xbb, 0x3f, 0x06, 0xe8, 0x2f, 0xe5, 0x1a, 0x35, 0x7e, 0xc9, 0xbd,
	0x6d, 0xd8, 0x95, 0x57, 0x9d, 0x70, 0xe3, 0x6d, 0xdf, 0x2c, 0x12, 0x72, 0x15, 0x67, 0x02, 0xc9,
	0x8d, 0xb0, 0x45, 0x9a, 0xe9, 0xc6, 0x0e, 0x4d, 0xe7, 0xef, 0xea, 0x8f, 0xe2, 0xef, 0xf5, 0xf9,
	0x69, 0xb6, 0x86, 0x84, 0xec, 0xcc, 0xde, 0xb0, 0x46, 0x91, 0x44, 0xa1, 0x5a, 0x56, 0x2a, 0x70,
	0x50, 0xd5, 0x9d, 0xfa, 0x3a, 0xf8, 0x21, 0x49, 0x54, 0x78, 0xf4, 0xc3, 0xc0, 0x09, 0xb5, 0xff,
	0x6d, 0xf6, 0xeb, 0x21, 0xc6, 0x24, 0x51, 0x18, 0x1f, 0x13, 0x18, 0x07, 0x0e, 0xc6, 0x3a, 0xb3,
	0x68, 0xeb, 0xc5, 0xee, 0x7e, 0x03, 0x00, 0x00, 0xff, 0xff, 0xb7, 0xbf, 0x74, 0x41, 0x10, 0x02,
	0x00, 0x00,
}
