// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/resources/campaign_audience_view.proto

package resources // import "google.golang.org/genproto/googleapis/ads/googleads/v0/resources"

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

// A campaign audience view.
// Includes performance data from interests and remarketing lists for Display
// Network and YouTube Network ads, and remarketing lists for search ads (RLSA),
// aggregated by campaign and audience criterion. This view only includes
// audiences attached at the campaign level.
type CampaignAudienceView struct {
	// The resource name of the campaign audience view.
	// Campaign audience view resource names have the form:
	//
	//
	// `customers/{customer_id}/campaignAudienceViews/{campaign_id}_{criterion_id}`
	ResourceName         string   `protobuf:"bytes,1,opt,name=resource_name,json=resourceName,proto3" json:"resource_name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *CampaignAudienceView) Reset()         { *m = CampaignAudienceView{} }
func (m *CampaignAudienceView) String() string { return proto.CompactTextString(m) }
func (*CampaignAudienceView) ProtoMessage()    {}
func (*CampaignAudienceView) Descriptor() ([]byte, []int) {
	return fileDescriptor_campaign_audience_view_8d2f2418290d12bd, []int{0}
}
func (m *CampaignAudienceView) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_CampaignAudienceView.Unmarshal(m, b)
}
func (m *CampaignAudienceView) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_CampaignAudienceView.Marshal(b, m, deterministic)
}
func (dst *CampaignAudienceView) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CampaignAudienceView.Merge(dst, src)
}
func (m *CampaignAudienceView) XXX_Size() int {
	return xxx_messageInfo_CampaignAudienceView.Size(m)
}
func (m *CampaignAudienceView) XXX_DiscardUnknown() {
	xxx_messageInfo_CampaignAudienceView.DiscardUnknown(m)
}

var xxx_messageInfo_CampaignAudienceView proto.InternalMessageInfo

func (m *CampaignAudienceView) GetResourceName() string {
	if m != nil {
		return m.ResourceName
	}
	return ""
}

func init() {
	proto.RegisterType((*CampaignAudienceView)(nil), "google.ads.googleads.v0.resources.CampaignAudienceView")
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/resources/campaign_audience_view.proto", fileDescriptor_campaign_audience_view_8d2f2418290d12bd)
}

var fileDescriptor_campaign_audience_view_8d2f2418290d12bd = []byte{
	// 259 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xb2, 0x4b, 0xcf, 0xcf, 0x4f,
	0xcf, 0x49, 0xd5, 0x4f, 0x4c, 0x29, 0xd6, 0x87, 0x30, 0x41, 0xac, 0x32, 0x03, 0xfd, 0xa2, 0xd4,
	0xe2, 0xfc, 0xd2, 0xa2, 0xe4, 0xd4, 0x62, 0xfd, 0xe4, 0xc4, 0xdc, 0x82, 0xc4, 0xcc, 0xf4, 0xbc,
	0xf8, 0xc4, 0xd2, 0x94, 0xcc, 0xd4, 0xbc, 0xe4, 0xd4, 0xf8, 0xb2, 0xcc, 0xd4, 0x72, 0xbd, 0x82,
	0xa2, 0xfc, 0x92, 0x7c, 0x21, 0x45, 0x88, 0x26, 0xbd, 0xc4, 0x94, 0x62, 0x3d, 0xb8, 0x7e, 0xbd,
	0x32, 0x03, 0x3d, 0xb8, 0x7e, 0x25, 0x6b, 0x2e, 0x11, 0x67, 0xa8, 0x11, 0x8e, 0x50, 0x13, 0xc2,
	0x32, 0x53, 0xcb, 0x85, 0x94, 0xb9, 0x78, 0x61, 0x8a, 0xe2, 0xf3, 0x12, 0x73, 0x53, 0x25, 0x18,
	0x15, 0x18, 0x35, 0x38, 0x83, 0x78, 0x60, 0x82, 0x7e, 0x89, 0xb9, 0xa9, 0x4e, 0x6d, 0x4c, 0x5c,
	0xaa, 0xc9, 0xf9, 0xb9, 0x7a, 0x04, 0xad, 0x71, 0x92, 0xc4, 0x66, 0x49, 0x00, 0xc8, 0x91, 0x01,
	0x8c, 0x51, 0x5e, 0x50, 0xfd, 0xe9, 0xf9, 0x39, 0x89, 0x79, 0xe9, 0x7a, 0xf9, 0x45, 0xe9, 0xfa,
	0xe9, 0xa9, 0x79, 0x60, 0x2f, 0xc0, 0xbc, 0x5d, 0x90, 0x59, 0x8c, 0x27, 0x14, 0xac, 0xe1, 0xac,
	0x45, 0x4c, 0xcc, 0xee, 0x8e, 0x8e, 0xab, 0x98, 0x14, 0xdd, 0x21, 0x46, 0x3a, 0xa6, 0x14, 0xeb,
	0x41, 0x98, 0x20, 0x56, 0x98, 0x81, 0x5e, 0x10, 0x4c, 0xe5, 0x29, 0x98, 0x9a, 0x18, 0xc7, 0x94,
	0xe2, 0x18, 0xb8, 0x9a, 0x98, 0x30, 0x83, 0x18, 0xb8, 0x9a, 0x57, 0x4c, 0xaa, 0x10, 0x09, 0x2b,
	0x2b, 0xc7, 0x94, 0x62, 0x2b, 0x2b, 0xb8, 0x2a, 0x2b, 0xab, 0x30, 0x03, 0x2b, 0x2b, 0xb8, 0xba,
	0x24, 0x36, 0xb0, 0x63, 0x8d, 0x01, 0x01, 0x00, 0x00, 0xff, 0xff, 0xeb, 0xa1, 0x69, 0x4b, 0xb1,
	0x01, 0x00, 0x00,
}
