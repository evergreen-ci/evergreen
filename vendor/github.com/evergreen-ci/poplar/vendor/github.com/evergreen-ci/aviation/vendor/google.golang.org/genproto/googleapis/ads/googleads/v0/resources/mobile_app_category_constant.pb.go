// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/resources/mobile_app_category_constant.proto

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

// A mobile application category constant.
type MobileAppCategoryConstant struct {
	// The resource name of the mobile app category constant.
	// Mobile app category constant resource names have the form:
	//
	// `mobileAppCategoryConstants/{mobile_app_category_id}`
	ResourceName string `protobuf:"bytes,1,opt,name=resource_name,json=resourceName,proto3" json:"resource_name,omitempty"`
	// The ID of the mobile app category constant.
	Id *wrappers.Int32Value `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	// Mobile app category name.
	Name                 *wrappers.StringValue `protobuf:"bytes,3,opt,name=name,proto3" json:"name,omitempty"`
	XXX_NoUnkeyedLiteral struct{}              `json:"-"`
	XXX_unrecognized     []byte                `json:"-"`
	XXX_sizecache        int32                 `json:"-"`
}

func (m *MobileAppCategoryConstant) Reset()         { *m = MobileAppCategoryConstant{} }
func (m *MobileAppCategoryConstant) String() string { return proto.CompactTextString(m) }
func (*MobileAppCategoryConstant) ProtoMessage()    {}
func (*MobileAppCategoryConstant) Descriptor() ([]byte, []int) {
	return fileDescriptor_mobile_app_category_constant_d4fc3e3bbb9f59ef, []int{0}
}
func (m *MobileAppCategoryConstant) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_MobileAppCategoryConstant.Unmarshal(m, b)
}
func (m *MobileAppCategoryConstant) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_MobileAppCategoryConstant.Marshal(b, m, deterministic)
}
func (dst *MobileAppCategoryConstant) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MobileAppCategoryConstant.Merge(dst, src)
}
func (m *MobileAppCategoryConstant) XXX_Size() int {
	return xxx_messageInfo_MobileAppCategoryConstant.Size(m)
}
func (m *MobileAppCategoryConstant) XXX_DiscardUnknown() {
	xxx_messageInfo_MobileAppCategoryConstant.DiscardUnknown(m)
}

var xxx_messageInfo_MobileAppCategoryConstant proto.InternalMessageInfo

func (m *MobileAppCategoryConstant) GetResourceName() string {
	if m != nil {
		return m.ResourceName
	}
	return ""
}

func (m *MobileAppCategoryConstant) GetId() *wrappers.Int32Value {
	if m != nil {
		return m.Id
	}
	return nil
}

func (m *MobileAppCategoryConstant) GetName() *wrappers.StringValue {
	if m != nil {
		return m.Name
	}
	return nil
}

func init() {
	proto.RegisterType((*MobileAppCategoryConstant)(nil), "google.ads.googleads.v0.resources.MobileAppCategoryConstant")
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/resources/mobile_app_category_constant.proto", fileDescriptor_mobile_app_category_constant_d4fc3e3bbb9f59ef)
}

var fileDescriptor_mobile_app_category_constant_d4fc3e3bbb9f59ef = []byte{
	// 336 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x84, 0x91, 0xdf, 0x4a, 0xf3, 0x30,
	0x18, 0xc6, 0x69, 0xf6, 0xf1, 0x81, 0x55, 0x4f, 0x7a, 0x34, 0xff, 0x30, 0x36, 0x65, 0x30, 0x10,
	0xd2, 0xb2, 0x9d, 0xc5, 0xa3, 0x6e, 0xc2, 0x50, 0x50, 0xc6, 0x84, 0x1e, 0x48, 0xa1, 0x64, 0x4d,
	0x0c, 0x85, 0x36, 0x09, 0x49, 0x37, 0xf1, 0x1a, 0xbc, 0x08, 0xc1, 0x43, 0x2f, 0xc5, 0x4b, 0xf1,
	0x2a, 0xa4, 0x49, 0x9b, 0x13, 0x51, 0xcf, 0x1e, 0xda, 0x5f, 0x7e, 0xcf, 0xfb, 0xf2, 0xfa, 0x57,
	0x4c, 0x08, 0x56, 0xd2, 0x10, 0x13, 0x1d, 0xda, 0xd8, 0xa4, 0x5d, 0x14, 0x2a, 0xaa, 0xc5, 0x56,
	0xe5, 0x54, 0x87, 0x95, 0xd8, 0x14, 0x25, 0xcd, 0xb0, 0x94, 0x59, 0x8e, 0x6b, 0xca, 0x84, 0x7a,
	0xce, 0x72, 0xc1, 0x75, 0x8d, 0x79, 0x0d, 0xa5, 0x12, 0xb5, 0x08, 0x46, 0xf6, 0x29, 0xc4, 0x44,
	0x43, 0x67, 0x81, 0xbb, 0x08, 0x3a, 0xcb, 0xf1, 0xa0, 0x2d, 0x32, 0x0f, 0x36, 0xdb, 0xc7, 0xf0,
	0x49, 0x61, 0x29, 0xa9, 0xd2, 0x56, 0x71, 0xf6, 0xea, 0xf9, 0x47, 0xb7, 0xa6, 0x29, 0x96, 0x72,
	0xd1, 0xf6, 0x2c, 0xda, 0x9a, 0xe0, 0xdc, 0x3f, 0xec, 0x54, 0x19, 0xc7, 0x15, 0xed, 0x7b, 0x43,
	0x6f, 0xb2, 0xb7, 0x3e, 0xe8, 0x3e, 0xde, 0xe1, 0x8a, 0x06, 0x17, 0x3e, 0x28, 0x48, 0x1f, 0x0c,
	0xbd, 0xc9, 0xfe, 0xf4, 0xa4, 0x9d, 0x03, 0x76, 0x7d, 0xf0, 0x9a, 0xd7, 0xb3, 0x69, 0x82, 0xcb,
	0x2d, 0x5d, 0x83, 0x82, 0x04, 0x91, 0xff, 0xcf, 0x88, 0x7a, 0x06, 0x3f, 0xfd, 0x86, 0xdf, 0xd7,
	0xaa, 0xe0, 0xcc, 0xf2, 0x86, 0x9c, 0xbf, 0x00, 0x7f, 0x9c, 0x8b, 0x0a, 0xfe, 0xb9, 0xeb, 0x7c,
	0xf0, 0xe3, 0x22, 0xab, 0x46, 0xbf, 0xf2, 0x1e, 0x6e, 0x5a, 0x09, 0x13, 0x25, 0xe6, 0x0c, 0x0a,
	0xc5, 0x42, 0x46, 0xb9, 0x29, 0xef, 0xce, 0x20, 0x0b, 0xfd, 0xcb, 0x55, 0x2e, 0x5d, 0x7a, 0x03,
	0xbd, 0x65, 0x1c, 0xbf, 0x83, 0xd1, 0xd2, 0x2a, 0x63, 0xa2, 0xa1, 0x8d, 0x4d, 0x4a, 0x22, 0xb8,
	0xee, 0xc8, 0x8f, 0x8e, 0x49, 0x63, 0xa2, 0x53, 0xc7, 0xa4, 0x49, 0x94, 0x3a, 0xe6, 0x13, 0x8c,
	0xed, 0x0f, 0x84, 0x62, 0xa2, 0x11, 0x72, 0x14, 0x42, 0x49, 0x84, 0x90, 0xe3, 0x36, 0xff, 0xcd,
	0xb0, 0xb3, 0xaf, 0x00, 0x00, 0x00, 0xff, 0xff, 0x96, 0x34, 0xf0, 0x20, 0x41, 0x02, 0x00, 0x00,
}
