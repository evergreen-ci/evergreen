// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/resources/billing_setup.proto

package resources // import "google.golang.org/genproto/googleapis/ads/googleads/v0/resources"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import wrappers "github.com/golang/protobuf/ptypes/wrappers"
import enums "google.golang.org/genproto/googleapis/ads/googleads/v0/enums"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// A billing setup across Ads and Payments systems; an association between a
// Payments account and an advertiser. A billing setup is specific to one
// advertiser.
type BillingSetup struct {
	// The resource name of the billing setup.
	// BillingSetup resource names have the form:
	//
	// `customers/{customer_id}/billingSetups/{billing_setup_id}`
	ResourceName string `protobuf:"bytes,1,opt,name=resource_name,json=resourceName,proto3" json:"resource_name,omitempty"`
	// The ID of the billing setup.
	Id *wrappers.Int64Value `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	// The status of the billing setup.
	Status enums.BillingSetupStatusEnum_BillingSetupStatus `protobuf:"varint,3,opt,name=status,proto3,enum=google.ads.googleads.v0.enums.BillingSetupStatusEnum_BillingSetupStatus" json:"status,omitempty"`
	// The resource name of the Payments account associated with this billing
	// setup. Payments resource names have the form:
	//
	// `customers/{customer_id}/paymentsAccounts/{payments_account_id}`
	// When setting up billing, this is used to signup with an existing Payments
	// account (and then payments_account_info should not be set).
	// When getting a billing setup, this and payments_account_info will be
	// populated.
	PaymentsAccount *wrappers.StringValue `protobuf:"bytes,11,opt,name=payments_account,json=paymentsAccount,proto3" json:"payments_account,omitempty"`
	// The Payments account information associated with this billing setup.
	// When setting up billing, this is used to signup with a new Payments account
	// (and then payments_account should not be set).
	// When getting a billing setup, this and payments_account will be
	// populated.
	PaymentsAccountInfo *BillingSetup_PaymentsAccountInfo `protobuf:"bytes,12,opt,name=payments_account_info,json=paymentsAccountInfo,proto3" json:"payments_account_info,omitempty"`
	// When creating a new billing setup, this is when the setup should take
	// effect. NOW is the only acceptable start time if the customer doesn't have
	// any approved setups.
	//
	// When fetching an existing billing setup, this is the requested start time.
	// However, if the setup was approved (see status) after the requested start
	// time, then this is the approval time.
	//
	// Types that are valid to be assigned to StartTime:
	//	*BillingSetup_StartDateTime
	//	*BillingSetup_StartTimeType
	StartTime isBillingSetup_StartTime `protobuf_oneof:"start_time"`
	// When the billing setup ends / ended. This is either FOREVER or the start
	// time of the next scheduled billing setup.
	//
	// Types that are valid to be assigned to EndTime:
	//	*BillingSetup_EndDateTime
	//	*BillingSetup_EndTimeType
	EndTime              isBillingSetup_EndTime `protobuf_oneof:"end_time"`
	XXX_NoUnkeyedLiteral struct{}               `json:"-"`
	XXX_unrecognized     []byte                 `json:"-"`
	XXX_sizecache        int32                  `json:"-"`
}

func (m *BillingSetup) Reset()         { *m = BillingSetup{} }
func (m *BillingSetup) String() string { return proto.CompactTextString(m) }
func (*BillingSetup) ProtoMessage()    {}
func (*BillingSetup) Descriptor() ([]byte, []int) {
	return fileDescriptor_billing_setup_24e9f67b1f70a676, []int{0}
}
func (m *BillingSetup) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BillingSetup.Unmarshal(m, b)
}
func (m *BillingSetup) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BillingSetup.Marshal(b, m, deterministic)
}
func (dst *BillingSetup) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BillingSetup.Merge(dst, src)
}
func (m *BillingSetup) XXX_Size() int {
	return xxx_messageInfo_BillingSetup.Size(m)
}
func (m *BillingSetup) XXX_DiscardUnknown() {
	xxx_messageInfo_BillingSetup.DiscardUnknown(m)
}

var xxx_messageInfo_BillingSetup proto.InternalMessageInfo

func (m *BillingSetup) GetResourceName() string {
	if m != nil {
		return m.ResourceName
	}
	return ""
}

func (m *BillingSetup) GetId() *wrappers.Int64Value {
	if m != nil {
		return m.Id
	}
	return nil
}

func (m *BillingSetup) GetStatus() enums.BillingSetupStatusEnum_BillingSetupStatus {
	if m != nil {
		return m.Status
	}
	return enums.BillingSetupStatusEnum_UNSPECIFIED
}

func (m *BillingSetup) GetPaymentsAccount() *wrappers.StringValue {
	if m != nil {
		return m.PaymentsAccount
	}
	return nil
}

func (m *BillingSetup) GetPaymentsAccountInfo() *BillingSetup_PaymentsAccountInfo {
	if m != nil {
		return m.PaymentsAccountInfo
	}
	return nil
}

type isBillingSetup_StartTime interface {
	isBillingSetup_StartTime()
}

type BillingSetup_StartDateTime struct {
	StartDateTime *wrappers.StringValue `protobuf:"bytes,9,opt,name=start_date_time,json=startDateTime,proto3,oneof"`
}

type BillingSetup_StartTimeType struct {
	StartTimeType enums.TimeTypeEnum_TimeType `protobuf:"varint,10,opt,name=start_time_type,json=startTimeType,proto3,enum=google.ads.googleads.v0.enums.TimeTypeEnum_TimeType,oneof"`
}

func (*BillingSetup_StartDateTime) isBillingSetup_StartTime() {}

func (*BillingSetup_StartTimeType) isBillingSetup_StartTime() {}

func (m *BillingSetup) GetStartTime() isBillingSetup_StartTime {
	if m != nil {
		return m.StartTime
	}
	return nil
}

func (m *BillingSetup) GetStartDateTime() *wrappers.StringValue {
	if x, ok := m.GetStartTime().(*BillingSetup_StartDateTime); ok {
		return x.StartDateTime
	}
	return nil
}

func (m *BillingSetup) GetStartTimeType() enums.TimeTypeEnum_TimeType {
	if x, ok := m.GetStartTime().(*BillingSetup_StartTimeType); ok {
		return x.StartTimeType
	}
	return enums.TimeTypeEnum_UNSPECIFIED
}

type isBillingSetup_EndTime interface {
	isBillingSetup_EndTime()
}

type BillingSetup_EndDateTime struct {
	EndDateTime *wrappers.StringValue `protobuf:"bytes,13,opt,name=end_date_time,json=endDateTime,proto3,oneof"`
}

type BillingSetup_EndTimeType struct {
	EndTimeType enums.TimeTypeEnum_TimeType `protobuf:"varint,14,opt,name=end_time_type,json=endTimeType,proto3,enum=google.ads.googleads.v0.enums.TimeTypeEnum_TimeType,oneof"`
}

func (*BillingSetup_EndDateTime) isBillingSetup_EndTime() {}

func (*BillingSetup_EndTimeType) isBillingSetup_EndTime() {}

func (m *BillingSetup) GetEndTime() isBillingSetup_EndTime {
	if m != nil {
		return m.EndTime
	}
	return nil
}

func (m *BillingSetup) GetEndDateTime() *wrappers.StringValue {
	if x, ok := m.GetEndTime().(*BillingSetup_EndDateTime); ok {
		return x.EndDateTime
	}
	return nil
}

func (m *BillingSetup) GetEndTimeType() enums.TimeTypeEnum_TimeType {
	if x, ok := m.GetEndTime().(*BillingSetup_EndTimeType); ok {
		return x.EndTimeType
	}
	return enums.TimeTypeEnum_UNSPECIFIED
}

// XXX_OneofFuncs is for the internal use of the proto package.
func (*BillingSetup) XXX_OneofFuncs() (func(msg proto.Message, b *proto.Buffer) error, func(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error), func(msg proto.Message) (n int), []interface{}) {
	return _BillingSetup_OneofMarshaler, _BillingSetup_OneofUnmarshaler, _BillingSetup_OneofSizer, []interface{}{
		(*BillingSetup_StartDateTime)(nil),
		(*BillingSetup_StartTimeType)(nil),
		(*BillingSetup_EndDateTime)(nil),
		(*BillingSetup_EndTimeType)(nil),
	}
}

func _BillingSetup_OneofMarshaler(msg proto.Message, b *proto.Buffer) error {
	m := msg.(*BillingSetup)
	// start_time
	switch x := m.StartTime.(type) {
	case *BillingSetup_StartDateTime:
		b.EncodeVarint(9<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.StartDateTime); err != nil {
			return err
		}
	case *BillingSetup_StartTimeType:
		b.EncodeVarint(10<<3 | proto.WireVarint)
		b.EncodeVarint(uint64(x.StartTimeType))
	case nil:
	default:
		return fmt.Errorf("BillingSetup.StartTime has unexpected type %T", x)
	}
	// end_time
	switch x := m.EndTime.(type) {
	case *BillingSetup_EndDateTime:
		b.EncodeVarint(13<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.EndDateTime); err != nil {
			return err
		}
	case *BillingSetup_EndTimeType:
		b.EncodeVarint(14<<3 | proto.WireVarint)
		b.EncodeVarint(uint64(x.EndTimeType))
	case nil:
	default:
		return fmt.Errorf("BillingSetup.EndTime has unexpected type %T", x)
	}
	return nil
}

func _BillingSetup_OneofUnmarshaler(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error) {
	m := msg.(*BillingSetup)
	switch tag {
	case 9: // start_time.start_date_time
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(wrappers.StringValue)
		err := b.DecodeMessage(msg)
		m.StartTime = &BillingSetup_StartDateTime{msg}
		return true, err
	case 10: // start_time.start_time_type
		if wire != proto.WireVarint {
			return true, proto.ErrInternalBadWireType
		}
		x, err := b.DecodeVarint()
		m.StartTime = &BillingSetup_StartTimeType{enums.TimeTypeEnum_TimeType(x)}
		return true, err
	case 13: // end_time.end_date_time
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(wrappers.StringValue)
		err := b.DecodeMessage(msg)
		m.EndTime = &BillingSetup_EndDateTime{msg}
		return true, err
	case 14: // end_time.end_time_type
		if wire != proto.WireVarint {
			return true, proto.ErrInternalBadWireType
		}
		x, err := b.DecodeVarint()
		m.EndTime = &BillingSetup_EndTimeType{enums.TimeTypeEnum_TimeType(x)}
		return true, err
	default:
		return false, nil
	}
}

func _BillingSetup_OneofSizer(msg proto.Message) (n int) {
	m := msg.(*BillingSetup)
	// start_time
	switch x := m.StartTime.(type) {
	case *BillingSetup_StartDateTime:
		s := proto.Size(x.StartDateTime)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *BillingSetup_StartTimeType:
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(x.StartTimeType))
	case nil:
	default:
		panic(fmt.Sprintf("proto: unexpected type %T in oneof", x))
	}
	// end_time
	switch x := m.EndTime.(type) {
	case *BillingSetup_EndDateTime:
		s := proto.Size(x.EndDateTime)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *BillingSetup_EndTimeType:
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(x.EndTimeType))
	case nil:
	default:
		panic(fmt.Sprintf("proto: unexpected type %T in oneof", x))
	}
	return n
}

// Container of Payments account information for this billing.
type BillingSetup_PaymentsAccountInfo struct {
	// A 16 digit id used to identify the Payments account associated with the
	// billing setup.
	//
	// This must be passed as a string with dashes, e.g. "1234-5678-9012-3456".
	PaymentsAccountId *wrappers.StringValue `protobuf:"bytes,1,opt,name=payments_account_id,json=paymentsAccountId,proto3" json:"payments_account_id,omitempty"`
	// The name of the Payments account associated with the billing setup.
	//
	// This enables the user to specify a meaningful name for a Payments account
	// to aid in reconciling monthly invoices.
	//
	// This name will be printed in the monthly invoices.
	PaymentsAccountName *wrappers.StringValue `protobuf:"bytes,2,opt,name=payments_account_name,json=paymentsAccountName,proto3" json:"payments_account_name,omitempty"`
	// A 12 digit id used to identify the Payments profile associated with the
	// billing setup.
	//
	// This must be passed in as a string with dashes, e.g. "1234-5678-9012".
	PaymentsProfileId *wrappers.StringValue `protobuf:"bytes,3,opt,name=payments_profile_id,json=paymentsProfileId,proto3" json:"payments_profile_id,omitempty"`
	// The name of the Payments profile associated with the billing setup.
	PaymentsProfileName *wrappers.StringValue `protobuf:"bytes,4,opt,name=payments_profile_name,json=paymentsProfileName,proto3" json:"payments_profile_name,omitempty"`
	// A secondary payments profile id present in uncommon situations, e.g.
	// when a sequential liability agreement has been arranged.
	SecondaryPaymentsProfileId *wrappers.StringValue `protobuf:"bytes,5,opt,name=secondary_payments_profile_id,json=secondaryPaymentsProfileId,proto3" json:"secondary_payments_profile_id,omitempty"`
	XXX_NoUnkeyedLiteral       struct{}              `json:"-"`
	XXX_unrecognized           []byte                `json:"-"`
	XXX_sizecache              int32                 `json:"-"`
}

func (m *BillingSetup_PaymentsAccountInfo) Reset()         { *m = BillingSetup_PaymentsAccountInfo{} }
func (m *BillingSetup_PaymentsAccountInfo) String() string { return proto.CompactTextString(m) }
func (*BillingSetup_PaymentsAccountInfo) ProtoMessage()    {}
func (*BillingSetup_PaymentsAccountInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_billing_setup_24e9f67b1f70a676, []int{0, 0}
}
func (m *BillingSetup_PaymentsAccountInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BillingSetup_PaymentsAccountInfo.Unmarshal(m, b)
}
func (m *BillingSetup_PaymentsAccountInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BillingSetup_PaymentsAccountInfo.Marshal(b, m, deterministic)
}
func (dst *BillingSetup_PaymentsAccountInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BillingSetup_PaymentsAccountInfo.Merge(dst, src)
}
func (m *BillingSetup_PaymentsAccountInfo) XXX_Size() int {
	return xxx_messageInfo_BillingSetup_PaymentsAccountInfo.Size(m)
}
func (m *BillingSetup_PaymentsAccountInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_BillingSetup_PaymentsAccountInfo.DiscardUnknown(m)
}

var xxx_messageInfo_BillingSetup_PaymentsAccountInfo proto.InternalMessageInfo

func (m *BillingSetup_PaymentsAccountInfo) GetPaymentsAccountId() *wrappers.StringValue {
	if m != nil {
		return m.PaymentsAccountId
	}
	return nil
}

func (m *BillingSetup_PaymentsAccountInfo) GetPaymentsAccountName() *wrappers.StringValue {
	if m != nil {
		return m.PaymentsAccountName
	}
	return nil
}

func (m *BillingSetup_PaymentsAccountInfo) GetPaymentsProfileId() *wrappers.StringValue {
	if m != nil {
		return m.PaymentsProfileId
	}
	return nil
}

func (m *BillingSetup_PaymentsAccountInfo) GetPaymentsProfileName() *wrappers.StringValue {
	if m != nil {
		return m.PaymentsProfileName
	}
	return nil
}

func (m *BillingSetup_PaymentsAccountInfo) GetSecondaryPaymentsProfileId() *wrappers.StringValue {
	if m != nil {
		return m.SecondaryPaymentsProfileId
	}
	return nil
}

func init() {
	proto.RegisterType((*BillingSetup)(nil), "google.ads.googleads.v0.resources.BillingSetup")
	proto.RegisterType((*BillingSetup_PaymentsAccountInfo)(nil), "google.ads.googleads.v0.resources.BillingSetup.PaymentsAccountInfo")
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/resources/billing_setup.proto", fileDescriptor_billing_setup_24e9f67b1f70a676)
}

var fileDescriptor_billing_setup_24e9f67b1f70a676 = []byte{
	// 620 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xa4, 0x94, 0xdf, 0x6a, 0xdb, 0x3e,
	0x14, 0xc7, 0x7f, 0x76, 0x7e, 0x2b, 0xab, 0x9a, 0xb4, 0xab, 0xcb, 0xc0, 0x64, 0x7f, 0x68, 0x37,
	0x0a, 0x85, 0x31, 0x25, 0x74, 0xdd, 0x18, 0xde, 0x95, 0xbd, 0x3f, 0x6d, 0xc7, 0x18, 0xc6, 0x2d,
	0xb9, 0x28, 0x61, 0x9e, 0x1a, 0x29, 0xc6, 0x60, 0x4b, 0xc6, 0x92, 0x5b, 0xf2, 0x34, 0x83, 0x5d,
	0xee, 0x01, 0xf6, 0x10, 0x7b, 0x94, 0x3d, 0xc4, 0x18, 0x96, 0x2c, 0x37, 0x75, 0xd3, 0x26, 0x65,
	0x77, 0x47, 0xf2, 0xf9, 0x7e, 0xf5, 0xd1, 0x39, 0x47, 0x06, 0x2f, 0x23, 0xc6, 0xa2, 0x84, 0xf4,
	0x10, 0xe6, 0x3d, 0x15, 0x96, 0xd1, 0x59, 0xbf, 0x97, 0x13, 0xce, 0x8a, 0x7c, 0x44, 0x78, 0xef,
	0x34, 0x4e, 0x92, 0x98, 0x46, 0x21, 0x27, 0xa2, 0xc8, 0x60, 0x96, 0x33, 0xc1, 0xac, 0x2d, 0x95,
	0x0b, 0x11, 0xe6, 0xb0, 0x96, 0xc1, 0xb3, 0x3e, 0xac, 0x65, 0xdd, 0xd7, 0xd7, 0x39, 0x13, 0x5a,
	0xa4, 0x0d, 0xd7, 0x90, 0x0b, 0x24, 0x0a, 0xae, 0xcc, 0xbb, 0xcf, 0x6f, 0x56, 0x8a, 0x38, 0x25,
	0xa1, 0x98, 0x64, 0xa4, 0x4a, 0x7f, 0x5c, 0xa5, 0xcb, 0xd5, 0x69, 0x31, 0xee, 0x9d, 0xe7, 0x28,
	0xcb, 0x48, 0x5e, 0xd9, 0x3d, 0xf9, 0xb6, 0x0c, 0xda, 0x9e, 0x3a, 0xed, 0xa8, 0x3c, 0xcc, 0x7a,
	0x0a, 0x3a, 0x1a, 0x33, 0xa4, 0x28, 0x25, 0xb6, 0xb1, 0x69, 0xec, 0x2c, 0x07, 0x6d, 0xbd, 0xf9,
	0x19, 0xa5, 0xc4, 0x7a, 0x06, 0xcc, 0x18, 0xdb, 0xe6, 0xa6, 0xb1, 0xb3, 0xb2, 0xfb, 0xa0, 0xba,
	0x23, 0xd4, 0x47, 0xc0, 0x43, 0x2a, 0x5e, 0xed, 0x0d, 0x50, 0x52, 0x90, 0xc0, 0x8c, 0xb1, 0xf5,
	0x15, 0x2c, 0xa9, 0x1b, 0xd8, 0xad, 0x4d, 0x63, 0x67, 0x75, 0xf7, 0x00, 0x5e, 0x57, 0x1f, 0x79,
	0x05, 0x38, 0x8d, 0x73, 0x24, 0x85, 0xef, 0x69, 0x91, 0xce, 0xd8, 0x0e, 0x2a, 0x5f, 0x6b, 0x1f,
	0xdc, 0xcb, 0xd0, 0x24, 0x25, 0x54, 0xf0, 0x10, 0x8d, 0x46, 0xac, 0xa0, 0xc2, 0x5e, 0x91, 0x70,
	0x0f, 0xaf, 0xc0, 0x1d, 0x89, 0x3c, 0xa6, 0x91, 0xa2, 0x5b, 0xd3, 0x2a, 0x57, 0x89, 0xac, 0x73,
	0x70, 0xbf, 0x69, 0x14, 0xc6, 0x74, 0xcc, 0xec, 0xb6, 0x74, 0x7b, 0x0b, 0xe7, 0x76, 0xf6, 0x12,
	0x26, 0xf4, 0x2f, 0xfb, 0x1f, 0xd2, 0x31, 0x0b, 0x36, 0xb2, 0xab, 0x9b, 0xd6, 0x07, 0xb0, 0xc6,
	0x05, 0xca, 0x45, 0x88, 0x91, 0x20, 0x61, 0xd9, 0x44, 0x7b, 0x79, 0xfe, 0x05, 0x0e, 0xfe, 0x0b,
	0x3a, 0x52, 0xf6, 0x0e, 0x09, 0x72, 0x1c, 0xa7, 0xc4, 0xfa, 0xa2, 0x7d, 0xea, 0x39, 0xb0, 0x81,
	0x2c, 0xfa, 0xde, 0x9c, 0xa2, 0x97, 0xea, 0xe3, 0x49, 0x46, 0x64, 0xa9, 0xf5, 0xa2, 0xf6, 0xd7,
	0x1b, 0x96, 0x07, 0x3a, 0x84, 0xe2, 0x29, 0xca, 0xce, 0x02, 0x94, 0x46, 0xb0, 0x42, 0x28, 0xae,
	0x19, 0x4f, 0x94, 0xc7, 0x05, 0xe1, 0xea, 0x3f, 0x10, 0x2a, 0x6f, 0xbd, 0xec, 0xfe, 0x6c, 0x81,
	0x8d, 0x19, 0x45, 0xb7, 0x3e, 0x81, 0x8d, 0xab, 0x8d, 0xc5, 0x72, 0xb6, 0xe7, 0x0d, 0xc9, 0x7a,
	0xb3, 0x5f, 0xd8, 0xf2, 0x67, 0x8c, 0x89, 0x7c, 0x2b, 0xe6, 0x02, 0x7e, 0xcd, 0xfe, 0xcb, 0x07,
	0x35, 0xcd, 0x97, 0xe5, 0x6c, 0x1c, 0x27, 0xa4, 0xe4, 0x6b, 0xdd, 0x86, 0xcf, 0x57, 0xba, 0x06,
	0x9f, 0x76, 0x93, 0x7c, 0xff, 0xdf, 0x86, 0xaf, 0xf2, 0x93, 0x7c, 0x21, 0x78, 0xc4, 0xc9, 0x88,
	0x51, 0x8c, 0xf2, 0x49, 0x38, 0x8b, 0xf4, 0xce, 0x02, 0xce, 0xdd, 0xda, 0xc2, 0x6f, 0x22, 0x7b,
	0x6d, 0x00, 0x2e, 0x06, 0xd7, 0x03, 0xe0, 0xae, 0x1e, 0x11, 0xef, 0x8f, 0x01, 0xb6, 0x47, 0x2c,
	0x9d, 0xff, 0xf4, 0xbc, 0xf5, 0xe9, 0xb7, 0xe7, 0x97, 0x04, 0xbe, 0x71, 0xf2, 0xb1, 0xd2, 0x45,
	0x2c, 0x41, 0x34, 0x82, 0x2c, 0x8f, 0x7a, 0x11, 0xa1, 0x92, 0x4f, 0xff, 0x3f, 0xb3, 0x98, 0xdf,
	0xf0, 0x8b, 0x7f, 0x53, 0x47, 0xdf, 0xcd, 0xd6, 0xbe, 0xeb, 0xfe, 0x30, 0xb7, 0xf6, 0x95, 0xa5,
	0x8b, 0x39, 0x54, 0x61, 0x19, 0x0d, 0xfa, 0x30, 0xd0, 0x99, 0xbf, 0x74, 0xce, 0xd0, 0xc5, 0x7c,
	0x58, 0xe7, 0x0c, 0x07, 0xfd, 0x61, 0x9d, 0xf3, 0xdb, 0xdc, 0x56, 0x1f, 0x1c, 0xc7, 0xc5, 0xdc,
	0x71, 0xea, 0x2c, 0xc7, 0x19, 0xf4, 0x1d, 0xa7, 0xce, 0x3b, 0x5d, 0x92, 0xb0, 0x2f, 0xfe, 0x06,
	0x00, 0x00, 0xff, 0xff, 0x96, 0xe4, 0x6d, 0x72, 0x8e, 0x06, 0x00, 0x00,
}
