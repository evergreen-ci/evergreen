// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/errors/billing_setup_error.proto

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

// Enum describing possible billing setup errors.
type BillingSetupErrorEnum_BillingSetupError int32

const (
	// Enum unspecified.
	BillingSetupErrorEnum_UNSPECIFIED BillingSetupErrorEnum_BillingSetupError = 0
	// The received error code is not known in this version.
	BillingSetupErrorEnum_UNKNOWN BillingSetupErrorEnum_BillingSetupError = 1
	// Cannot use both an existing Payments account and a new Payments account
	// when setting up billing.
	BillingSetupErrorEnum_CANNOT_USE_EXISTING_AND_NEW_ACCOUNT BillingSetupErrorEnum_BillingSetupError = 2
	// Cannot cancel an APPROVED billing setup whose start time has passed.
	BillingSetupErrorEnum_CANNOT_REMOVE_STARTED_BILLING_SETUP BillingSetupErrorEnum_BillingSetupError = 3
	// Cannot perform a Change of Bill-To (CBT) to the same Payments account.
	BillingSetupErrorEnum_CANNOT_CHANGE_BILLING_TO_SAME_PAYMENTS_ACCOUNT BillingSetupErrorEnum_BillingSetupError = 4
	// Billing Setups can only be used by customers with ENABLED or DRAFT
	// status.
	BillingSetupErrorEnum_BILLING_SETUP_NOT_PERMITTED_FOR_CUSTOMER_STATUS BillingSetupErrorEnum_BillingSetupError = 5
	// Billing Setups must either include a correctly formatted existing
	// Payments account id, or a non-empty new Payments account name.
	BillingSetupErrorEnum_INVALID_PAYMENTS_ACCOUNT BillingSetupErrorEnum_BillingSetupError = 6
	// Only billable and third party customers can create billing setups.
	BillingSetupErrorEnum_BILLING_SETUP_NOT_PERMITTED_FOR_CUSTOMER_CATEGORY BillingSetupErrorEnum_BillingSetupError = 7
	// Billing Setup creations can only use NOW for start time type.
	BillingSetupErrorEnum_INVALID_START_TIME_TYPE BillingSetupErrorEnum_BillingSetupError = 8
	// Billing Setups can only be created for a third party customer if they do
	// not already have a setup.
	BillingSetupErrorEnum_THIRD_PARTY_ALREADY_HAS_BILLING BillingSetupErrorEnum_BillingSetupError = 9
	// Billing Setups cannot be created if there is already a pending billing in
	// progress, ie. a billing known to Payments.
	BillingSetupErrorEnum_BILLING_SETUP_IN_PROGRESS BillingSetupErrorEnum_BillingSetupError = 10
	// Billing Setups can only be created by customers who have permission to
	// setup billings. Users can contact a representative for help setting up
	// permissions.
	BillingSetupErrorEnum_NO_SIGNUP_PERMISSION BillingSetupErrorEnum_BillingSetupError = 11
	// Billing Setups cannot be created if there is already a future-approved
	// billing.
	BillingSetupErrorEnum_CHANGE_OF_BILL_TO_IN_PROGRESS BillingSetupErrorEnum_BillingSetupError = 12
	// Billing Setup creation failed because Payments could not find the
	// requested Payments profile.
	BillingSetupErrorEnum_PAYMENTS_PROFILE_NOT_FOUND BillingSetupErrorEnum_BillingSetupError = 13
	// Billing Setup creation failed because Payments could not find the
	// requested Payments account.
	BillingSetupErrorEnum_PAYMENTS_ACCOUNT_NOT_FOUND BillingSetupErrorEnum_BillingSetupError = 14
	// Billing Setup creation failed because Payments considers requested
	// Payments profile ineligible.
	BillingSetupErrorEnum_PAYMENTS_PROFILE_INELIGIBLE BillingSetupErrorEnum_BillingSetupError = 15
	// Billing Setup creation failed because Payments considers requested
	// Payments account ineligible.
	BillingSetupErrorEnum_PAYMENTS_ACCOUNT_INELIGIBLE BillingSetupErrorEnum_BillingSetupError = 16
)

var BillingSetupErrorEnum_BillingSetupError_name = map[int32]string{
	0:  "UNSPECIFIED",
	1:  "UNKNOWN",
	2:  "CANNOT_USE_EXISTING_AND_NEW_ACCOUNT",
	3:  "CANNOT_REMOVE_STARTED_BILLING_SETUP",
	4:  "CANNOT_CHANGE_BILLING_TO_SAME_PAYMENTS_ACCOUNT",
	5:  "BILLING_SETUP_NOT_PERMITTED_FOR_CUSTOMER_STATUS",
	6:  "INVALID_PAYMENTS_ACCOUNT",
	7:  "BILLING_SETUP_NOT_PERMITTED_FOR_CUSTOMER_CATEGORY",
	8:  "INVALID_START_TIME_TYPE",
	9:  "THIRD_PARTY_ALREADY_HAS_BILLING",
	10: "BILLING_SETUP_IN_PROGRESS",
	11: "NO_SIGNUP_PERMISSION",
	12: "CHANGE_OF_BILL_TO_IN_PROGRESS",
	13: "PAYMENTS_PROFILE_NOT_FOUND",
	14: "PAYMENTS_ACCOUNT_NOT_FOUND",
	15: "PAYMENTS_PROFILE_INELIGIBLE",
	16: "PAYMENTS_ACCOUNT_INELIGIBLE",
}
var BillingSetupErrorEnum_BillingSetupError_value = map[string]int32{
	"UNSPECIFIED":                                       0,
	"UNKNOWN":                                           1,
	"CANNOT_USE_EXISTING_AND_NEW_ACCOUNT":               2,
	"CANNOT_REMOVE_STARTED_BILLING_SETUP":               3,
	"CANNOT_CHANGE_BILLING_TO_SAME_PAYMENTS_ACCOUNT":    4,
	"BILLING_SETUP_NOT_PERMITTED_FOR_CUSTOMER_STATUS":   5,
	"INVALID_PAYMENTS_ACCOUNT":                          6,
	"BILLING_SETUP_NOT_PERMITTED_FOR_CUSTOMER_CATEGORY": 7,
	"INVALID_START_TIME_TYPE":                           8,
	"THIRD_PARTY_ALREADY_HAS_BILLING":                   9,
	"BILLING_SETUP_IN_PROGRESS":                         10,
	"NO_SIGNUP_PERMISSION":                              11,
	"CHANGE_OF_BILL_TO_IN_PROGRESS":                     12,
	"PAYMENTS_PROFILE_NOT_FOUND":                        13,
	"PAYMENTS_ACCOUNT_NOT_FOUND":                        14,
	"PAYMENTS_PROFILE_INELIGIBLE":                       15,
	"PAYMENTS_ACCOUNT_INELIGIBLE":                       16,
}

func (x BillingSetupErrorEnum_BillingSetupError) String() string {
	return proto.EnumName(BillingSetupErrorEnum_BillingSetupError_name, int32(x))
}
func (BillingSetupErrorEnum_BillingSetupError) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_billing_setup_error_96667c311bd7b1a0, []int{0, 0}
}

// Container for enum describing possible billing setup errors.
type BillingSetupErrorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *BillingSetupErrorEnum) Reset()         { *m = BillingSetupErrorEnum{} }
func (m *BillingSetupErrorEnum) String() string { return proto.CompactTextString(m) }
func (*BillingSetupErrorEnum) ProtoMessage()    {}
func (*BillingSetupErrorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_billing_setup_error_96667c311bd7b1a0, []int{0}
}
func (m *BillingSetupErrorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BillingSetupErrorEnum.Unmarshal(m, b)
}
func (m *BillingSetupErrorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BillingSetupErrorEnum.Marshal(b, m, deterministic)
}
func (dst *BillingSetupErrorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BillingSetupErrorEnum.Merge(dst, src)
}
func (m *BillingSetupErrorEnum) XXX_Size() int {
	return xxx_messageInfo_BillingSetupErrorEnum.Size(m)
}
func (m *BillingSetupErrorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_BillingSetupErrorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_BillingSetupErrorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*BillingSetupErrorEnum)(nil), "google.ads.googleads.v0.errors.BillingSetupErrorEnum")
	proto.RegisterEnum("google.ads.googleads.v0.errors.BillingSetupErrorEnum_BillingSetupError", BillingSetupErrorEnum_BillingSetupError_name, BillingSetupErrorEnum_BillingSetupError_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/errors/billing_setup_error.proto", fileDescriptor_billing_setup_error_96667c311bd7b1a0)
}

var fileDescriptor_billing_setup_error_96667c311bd7b1a0 = []byte{
	// 552 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x93, 0x4f, 0x6f, 0xd3, 0x4c,
	0x10, 0xc6, 0xdf, 0xa6, 0x7d, 0x5b, 0xd8, 0x00, 0x5d, 0x56, 0xfc, 0x29, 0x94, 0xa6, 0x22, 0x3d,
	0x70, 0xb3, 0x03, 0x15, 0x12, 0x32, 0xa7, 0x8d, 0x3d, 0x71, 0x56, 0xd8, 0xbb, 0xd6, 0xee, 0x3a,
	0x25, 0x28, 0xd2, 0x2a, 0x25, 0x91, 0x15, 0x29, 0x8d, 0xa3, 0xb8, 0xed, 0x07, 0xe2, 0xc8, 0x47,
	0xe1, 0x5b, 0x70, 0xe5, 0xc6, 0x99, 0x0b, 0xb2, 0x9d, 0x58, 0x89, 0x22, 0x10, 0x27, 0x8f, 0x66,
	0x7e, 0xcf, 0x33, 0x9e, 0xd1, 0x2c, 0x7a, 0x97, 0xa4, 0x69, 0x32, 0x1d, 0xdb, 0xc3, 0x51, 0x66,
	0x97, 0x61, 0x1e, 0xdd, 0xb6, 0xec, 0xf1, 0x62, 0x91, 0x2e, 0x32, 0xfb, 0x72, 0x32, 0x9d, 0x4e,
	0x66, 0x89, 0xc9, 0xc6, 0xd7, 0x37, 0x73, 0x53, 0x24, 0xad, 0xf9, 0x22, 0xbd, 0x4e, 0x49, 0xa3,
	0xc4, 0xad, 0xe1, 0x28, 0xb3, 0x2a, 0xa5, 0x75, 0xdb, 0xb2, 0x4a, 0x65, 0xf3, 0xd7, 0x1e, 0x7a,
	0xdc, 0x2e, 0xd5, 0x2a, 0x17, 0x43, 0x9e, 0x86, 0xd9, 0xcd, 0x55, 0xf3, 0xfb, 0x1e, 0x7a, 0xb8,
	0x55, 0x21, 0x87, 0xa8, 0x1e, 0x73, 0x15, 0x81, 0xcb, 0x3a, 0x0c, 0x3c, 0xfc, 0x1f, 0xa9, 0xa3,
	0x83, 0x98, 0x7f, 0xe0, 0xe2, 0x82, 0xe3, 0x1d, 0xf2, 0x0a, 0x9d, 0xb9, 0x94, 0x73, 0xa1, 0x4d,
	0xac, 0xc0, 0xc0, 0x47, 0xa6, 0x34, 0xe3, 0xbe, 0xa1, 0xdc, 0x33, 0x1c, 0x2e, 0x0c, 0x75, 0x5d,
	0x11, 0x73, 0x8d, 0x6b, 0x6b, 0xa0, 0x84, 0x50, 0xf4, 0xc0, 0x28, 0x4d, 0xa5, 0x06, 0xcf, 0xb4,
	0x59, 0x10, 0xe4, 0x12, 0x05, 0x3a, 0x8e, 0xf0, 0x2e, 0x79, 0x83, 0xac, 0x25, 0xe8, 0x76, 0x29,
	0xf7, 0xa1, 0x02, 0xb4, 0x30, 0x8a, 0x86, 0x60, 0x22, 0xda, 0x0f, 0x81, 0x6b, 0x55, 0x99, 0xef,
	0x91, 0x73, 0x64, 0x6f, 0xd8, 0x98, 0x5c, 0x1e, 0x81, 0x0c, 0x99, 0xce, 0x5b, 0x74, 0x84, 0x34,
	0x6e, 0xac, 0xb4, 0x08, 0x41, 0xe6, 0x7d, 0x75, 0xac, 0xf0, 0xff, 0xe4, 0x05, 0x3a, 0x62, 0xbc,
	0x47, 0x03, 0xe6, 0x6d, 0x5b, 0xee, 0x93, 0xb7, 0xe8, 0xf5, 0x3f, 0x5b, 0xba, 0x54, 0x83, 0x2f,
	0x64, 0x1f, 0x1f, 0x90, 0x63, 0xf4, 0x74, 0x65, 0x5a, 0x0c, 0x68, 0x34, 0x0b, 0xc1, 0xe8, 0x7e,
	0x04, 0xf8, 0x0e, 0x39, 0x43, 0xa7, 0xba, 0xcb, 0x64, 0xde, 0x4f, 0xea, 0xbe, 0xa1, 0x81, 0x04,
	0xea, 0xf5, 0x4d, 0x97, 0xaa, 0xd5, 0x90, 0xf8, 0x2e, 0x39, 0x41, 0xcf, 0x36, 0x1b, 0x33, 0x6e,
	0x22, 0x29, 0x7c, 0x09, 0x4a, 0x61, 0x44, 0x8e, 0xd0, 0x23, 0x2e, 0x8c, 0x62, 0x3e, 0x8f, 0xa3,
	0xf2, 0x7f, 0x94, 0x62, 0x82, 0xe3, 0x3a, 0x79, 0x89, 0x4e, 0x96, 0x1b, 0x13, 0x9d, 0xc2, 0x2f,
	0xdf, 0xd8, 0xba, 0xf8, 0x1e, 0x69, 0xa0, 0xe7, 0xd5, 0xa8, 0x91, 0x14, 0x1d, 0x16, 0x40, 0x31,
	0x57, 0x47, 0xc4, 0xdc, 0xc3, 0xf7, 0x37, 0xea, 0xcb, 0x55, 0xac, 0xd5, 0x1f, 0x90, 0x53, 0x74,
	0xbc, 0xa5, 0x67, 0x1c, 0x02, 0xe6, 0xb3, 0x76, 0x00, 0xf8, 0x70, 0x03, 0x58, 0x19, 0xac, 0x01,
	0xb8, 0xfd, 0x73, 0x07, 0x35, 0x3f, 0xa7, 0x57, 0xd6, 0xdf, 0x8f, 0xb4, 0xfd, 0x64, 0xeb, 0x0e,
	0xa3, 0xfc, 0xb8, 0xa3, 0x9d, 0x4f, 0xde, 0x52, 0x99, 0xa4, 0xd3, 0xe1, 0x2c, 0xb1, 0xd2, 0x45,
	0x62, 0x27, 0xe3, 0x59, 0x71, 0xfa, 0xab, 0x87, 0x32, 0x9f, 0x64, 0x7f, 0x7a, 0x37, 0xef, 0xcb,
	0xcf, 0x97, 0xda, 0xae, 0x4f, 0xe9, 0xd7, 0x5a, 0xc3, 0x2f, 0xcd, 0xe8, 0x28, 0xb3, 0xca, 0x30,
	0x8f, 0x7a, 0x2d, 0xab, 0x68, 0x99, 0x7d, 0x5b, 0x01, 0x03, 0x3a, 0xca, 0x06, 0x15, 0x30, 0xe8,
	0xb5, 0x06, 0x25, 0xf0, 0xa3, 0xd6, 0x2c, 0xb3, 0x8e, 0x43, 0x47, 0x99, 0xe3, 0x54, 0x88, 0xe3,
	0xf4, 0x5a, 0x8e, 0x53, 0x42, 0x97, 0xfb, 0xc5, 0xdf, 0x9d, 0xff, 0x0e, 0x00, 0x00, 0xff, 0xff,
	0x44, 0x6b, 0x9a, 0xae, 0xd4, 0x03, 0x00, 0x00,
}
