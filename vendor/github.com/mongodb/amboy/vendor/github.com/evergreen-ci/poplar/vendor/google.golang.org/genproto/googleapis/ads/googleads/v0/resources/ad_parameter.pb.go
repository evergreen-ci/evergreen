// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/resources/ad_parameter.proto

package resources // import "google.golang.org/genproto/googleapis/ads/googleads/v0/resources"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import wrappers "github.com/golang/protobuf/ptypes/wrappers"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// An ad parameter that is used to update numeric values (such as prices or
// inventory levels) in any text line of an ad (including URLs). There can
// be a maximum of two AdParameters per ad group criterion. (One with
// parameter_index = 1 and one with parameter_index = 2.)
// In the ad the parameters are referenced by a placeholder of the form
// "{param#:value}". E.g. "{param1:$17}"
type AdParameter struct {
	// The resource name of the ad parameter.
	// Ad parameter resource names have the form:
	//
	//
	// `customers/{customer_id}/adParameters/{ad_group_id}_{criterion_id}_{parameter_index}`
	ResourceName string `protobuf:"bytes,1,opt,name=resource_name,json=resourceName,proto3" json:"resource_name,omitempty"`
	// The ad group criterion that this ad parameter belongs to.
	AdGroupCriterion *wrappers.StringValue `protobuf:"bytes,2,opt,name=ad_group_criterion,json=adGroupCriterion,proto3" json:"ad_group_criterion,omitempty"`
	// The unique index of this ad parameter. Must be either 1 or 2.
	ParameterIndex *wrappers.Int64Value `protobuf:"bytes,3,opt,name=parameter_index,json=parameterIndex,proto3" json:"parameter_index,omitempty"`
	// Numeric value to insert into the ad text. The following restrictions
	//  apply:
	//  - Can use comma or period as a separator, with an optional period or
	//    comma (respectively) for fractional values. For example, 1,000,000.00
	//    and 2.000.000,10 are valid.
	//  - Can be prepended or appended with a currency symbol. For example,
	//    $99.99 and 200£ are valid.
	//  - Can be prepended or appended with a currency code. For example, 99.99USD
	//    and EUR200 are valid.
	//  - Can use '%'. For example, 1.0% and 1,0% are valid.
	//  - Can use plus or minus. For example, -10.99 and 25+ are valid.
	//  - Can use '/' between two numbers. For example 4/1 and 0.95/0.45 are
	//    valid.
	InsertionText        *wrappers.StringValue `protobuf:"bytes,4,opt,name=insertion_text,json=insertionText,proto3" json:"insertion_text,omitempty"`
	XXX_NoUnkeyedLiteral struct{}              `json:"-"`
	XXX_unrecognized     []byte                `json:"-"`
	XXX_sizecache        int32                 `json:"-"`
}

func (m *AdParameter) Reset()         { *m = AdParameter{} }
func (m *AdParameter) String() string { return proto.CompactTextString(m) }
func (*AdParameter) ProtoMessage()    {}
func (*AdParameter) Descriptor() ([]byte, []int) {
	return fileDescriptor_ad_parameter_1bb16849ec243a1f, []int{0}
}
func (m *AdParameter) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AdParameter.Unmarshal(m, b)
}
func (m *AdParameter) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AdParameter.Marshal(b, m, deterministic)
}
func (dst *AdParameter) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AdParameter.Merge(dst, src)
}
func (m *AdParameter) XXX_Size() int {
	return xxx_messageInfo_AdParameter.Size(m)
}
func (m *AdParameter) XXX_DiscardUnknown() {
	xxx_messageInfo_AdParameter.DiscardUnknown(m)
}

var xxx_messageInfo_AdParameter proto.InternalMessageInfo

func (m *AdParameter) GetResourceName() string {
	if m != nil {
		return m.ResourceName
	}
	return ""
}

func (m *AdParameter) GetAdGroupCriterion() *wrappers.StringValue {
	if m != nil {
		return m.AdGroupCriterion
	}
	return nil
}

func (m *AdParameter) GetParameterIndex() *wrappers.Int64Value {
	if m != nil {
		return m.ParameterIndex
	}
	return nil
}

func (m *AdParameter) GetInsertionText() *wrappers.StringValue {
	if m != nil {
		return m.InsertionText
	}
	return nil
}

func init() {
	proto.RegisterType((*AdParameter)(nil), "google.ads.googleads.v0.resources.AdParameter")
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/resources/ad_parameter.proto", fileDescriptor_ad_parameter_1bb16849ec243a1f)
}

var fileDescriptor_ad_parameter_1bb16849ec243a1f = []byte{
	// 366 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x91, 0xc1, 0x4a, 0xc3, 0x30,
	0x18, 0xc7, 0x69, 0x27, 0x82, 0x9d, 0x9b, 0xa3, 0xa7, 0xa2, 0x22, 0x9b, 0x32, 0xd8, 0x29, 0x2d,
	0x3a, 0x3c, 0xc4, 0x53, 0x37, 0x61, 0x6c, 0x07, 0x19, 0x53, 0x7a, 0x90, 0x42, 0xc9, 0x96, 0x18,
	0x0a, 0x6b, 0x52, 0x92, 0x74, 0xee, 0x15, 0x7c, 0x0d, 0x8f, 0x3e, 0x8a, 0x8f, 0xe2, 0x3b, 0x08,
	0xd2, 0x76, 0x09, 0x82, 0xa0, 0xde, 0xfe, 0xb4, 0xff, 0xdf, 0x2f, 0x5f, 0xf2, 0x39, 0x43, 0xca,
	0x39, 0x5d, 0x13, 0x1f, 0x61, 0xe9, 0xd7, 0xb1, 0x4c, 0x9b, 0xc0, 0x17, 0x44, 0xf2, 0x42, 0xac,
	0x88, 0xf4, 0x11, 0x4e, 0x72, 0x24, 0x50, 0x46, 0x14, 0x11, 0x20, 0x17, 0x5c, 0x71, 0xb7, 0x57,
	0x57, 0x01, 0xc2, 0x12, 0x18, 0x0a, 0x6c, 0x02, 0x60, 0xa8, 0xe3, 0xb3, 0x9d, 0xb8, 0x02, 0x96,
	0xc5, 0x93, 0xff, 0x2c, 0x50, 0x9e, 0x13, 0x21, 0x6b, 0xc5, 0xf9, 0x8b, 0xed, 0x34, 0x43, 0x3c,
	0xd7, 0x62, 0xf7, 0xc2, 0x69, 0x69, 0x38, 0x61, 0x28, 0x23, 0x9e, 0xd5, 0xb5, 0x06, 0x07, 0x8b,
	0x43, 0xfd, 0xf1, 0x0e, 0x65, 0xc4, 0x9d, 0x39, 0x2e, 0xc2, 0x09, 0x15, 0xbc, 0xc8, 0x93, 0x95,
	0x48, 0x15, 0x11, 0x29, 0x67, 0x9e, 0xdd, 0xb5, 0x06, 0xcd, 0xcb, 0xd3, 0xdd, 0x24, 0x40, 0x9f,
	0x08, 0xee, 0x95, 0x48, 0x19, 0x8d, 0xd0, 0xba, 0x20, 0x8b, 0x0e, 0xc2, 0x93, 0x12, 0x1b, 0x6b,
	0xca, 0xbd, 0x75, 0x8e, 0xcc, 0xb5, 0x92, 0x94, 0x61, 0xb2, 0xf5, 0x1a, 0x95, 0xe8, 0xe4, 0x87,
	0x68, 0xca, 0xd4, 0xf5, 0xb0, 0xf6, 0xb4, 0x0d, 0x33, 0x2d, 0x11, 0x77, 0xec, 0xb4, 0x53, 0x26,
	0x89, 0x50, 0x29, 0x67, 0x89, 0x22, 0x5b, 0xe5, 0xed, 0xfd, 0x63, 0x9a, 0x96, 0x61, 0x1e, 0xc8,
	0x56, 0x8d, 0x3e, 0x2d, 0xa7, 0xbf, 0xe2, 0x19, 0xf8, 0xf3, 0x55, 0x47, 0x9d, 0x6f, 0x4f, 0x36,
	0x2f, 0xcd, 0x73, 0xeb, 0x71, 0xb6, 0xc3, 0x28, 0x5f, 0x23, 0x46, 0x01, 0x17, 0xd4, 0xa7, 0x84,
	0x55, 0xe7, 0xea, 0x95, 0xe6, 0xa9, 0xfc, 0x65, 0xc3, 0x37, 0x26, 0xbd, 0xda, 0x8d, 0x49, 0x18,
	0xbe, 0xd9, 0xbd, 0x49, 0xad, 0x0c, 0xb1, 0x04, 0x75, 0x2c, 0x53, 0x14, 0x80, 0x85, 0x6e, 0xbe,
	0xeb, 0x4e, 0x1c, 0x62, 0x19, 0x9b, 0x4e, 0x1c, 0x05, 0xb1, 0xe9, 0x7c, 0xd8, 0xfd, 0xfa, 0x07,
	0x84, 0x21, 0x96, 0x10, 0x9a, 0x16, 0x84, 0x51, 0x00, 0xa1, 0xe9, 0x2d, 0xf7, 0xab, 0x61, 0xaf,
	0xbe, 0x02, 0x00, 0x00, 0xff, 0xff, 0x36, 0x75, 0x37, 0x96, 0x8d, 0x02, 0x00, 0x00,
}
