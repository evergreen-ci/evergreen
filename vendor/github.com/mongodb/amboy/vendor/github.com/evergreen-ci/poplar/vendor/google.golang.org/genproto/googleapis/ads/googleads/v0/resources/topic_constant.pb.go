// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/resources/topic_constant.proto

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

// Use topics to target or exclude placements in the Google Display Network
// based on the category into which the placement falls (for example,
// "Pets & Animals/Pets/Dogs").
type TopicConstant struct {
	// The resource name of the topic constant.
	// topic constant resource names have the form:
	//
	// `topicConstants/{topic_id}`
	ResourceName string `protobuf:"bytes,1,opt,name=resource_name,json=resourceName,proto3" json:"resource_name,omitempty"`
	// The ID of the topic.
	Id *wrappers.Int64Value `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	// Resource name of parent of the topic constant.
	TopicConstantParent *wrappers.StringValue `protobuf:"bytes,3,opt,name=topic_constant_parent,json=topicConstantParent,proto3" json:"topic_constant_parent,omitempty"`
	// The category to target or exclude. Each subsequent element in the array
	// describes a more specific sub-category. For example,
	// {"Pets & Animals", "Pets", "Dogs"} represents the
	// "Pets & Animals/Pets/Dogs" category. A complete list of available topic
	// categories is available
	// <a
	// href="https://developers.google.com/adwords/api/docs/appendix/verticals">
	// here</a>
	Path                 []*wrappers.StringValue `protobuf:"bytes,4,rep,name=path,proto3" json:"path,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                `json:"-"`
	XXX_unrecognized     []byte                  `json:"-"`
	XXX_sizecache        int32                   `json:"-"`
}

func (m *TopicConstant) Reset()         { *m = TopicConstant{} }
func (m *TopicConstant) String() string { return proto.CompactTextString(m) }
func (*TopicConstant) ProtoMessage()    {}
func (*TopicConstant) Descriptor() ([]byte, []int) {
	return fileDescriptor_topic_constant_5ad8d982dffab021, []int{0}
}
func (m *TopicConstant) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TopicConstant.Unmarshal(m, b)
}
func (m *TopicConstant) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TopicConstant.Marshal(b, m, deterministic)
}
func (dst *TopicConstant) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TopicConstant.Merge(dst, src)
}
func (m *TopicConstant) XXX_Size() int {
	return xxx_messageInfo_TopicConstant.Size(m)
}
func (m *TopicConstant) XXX_DiscardUnknown() {
	xxx_messageInfo_TopicConstant.DiscardUnknown(m)
}

var xxx_messageInfo_TopicConstant proto.InternalMessageInfo

func (m *TopicConstant) GetResourceName() string {
	if m != nil {
		return m.ResourceName
	}
	return ""
}

func (m *TopicConstant) GetId() *wrappers.Int64Value {
	if m != nil {
		return m.Id
	}
	return nil
}

func (m *TopicConstant) GetTopicConstantParent() *wrappers.StringValue {
	if m != nil {
		return m.TopicConstantParent
	}
	return nil
}

func (m *TopicConstant) GetPath() []*wrappers.StringValue {
	if m != nil {
		return m.Path
	}
	return nil
}

func init() {
	proto.RegisterType((*TopicConstant)(nil), "google.ads.googleads.v0.resources.TopicConstant")
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/resources/topic_constant.proto", fileDescriptor_topic_constant_5ad8d982dffab021)
}

var fileDescriptor_topic_constant_5ad8d982dffab021 = []byte{
	// 347 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x84, 0x91, 0xd1, 0x4a, 0xeb, 0x30,
	0x18, 0xc7, 0x69, 0x37, 0x0e, 0x9c, 0x9c, 0xb3, 0x9b, 0x1e, 0x0e, 0x14, 0x15, 0xd9, 0x94, 0xc1,
	0x40, 0x48, 0x8b, 0xca, 0x2e, 0xe2, 0x55, 0xe7, 0xc5, 0xd0, 0x0b, 0x29, 0x53, 0x7a, 0x21, 0x85,
	0x91, 0x35, 0x31, 0x16, 0xb6, 0x24, 0x24, 0xd9, 0x7c, 0x1f, 0x2f, 0x7d, 0x14, 0xdf, 0xc3, 0x1b,
	0x5f, 0x42, 0x69, 0xd3, 0x06, 0x87, 0xe0, 0xee, 0xfe, 0xb4, 0xff, 0xdf, 0xef, 0xfb, 0xc8, 0x07,
	0xc6, 0x4c, 0x08, 0xb6, 0xa4, 0x11, 0x26, 0x3a, 0xb2, 0xb1, 0x4a, 0x9b, 0x38, 0x52, 0x54, 0x8b,
	0xb5, 0x2a, 0xa8, 0x8e, 0x8c, 0x90, 0x65, 0x31, 0x2f, 0x04, 0xd7, 0x06, 0x73, 0x03, 0xa5, 0x12,
	0x46, 0x04, 0x03, 0x5b, 0x86, 0x98, 0x68, 0xe8, 0x38, 0xb8, 0x89, 0xa1, 0xe3, 0xf6, 0x0e, 0x1b,
	0x75, 0x0d, 0x2c, 0xd6, 0x0f, 0xd1, 0x93, 0xc2, 0x52, 0x52, 0xa5, 0xad, 0xe2, 0xe8, 0xcd, 0x03,
	0xbd, 0xbb, 0xca, 0x7d, 0xd9, 0xa8, 0x83, 0x63, 0xd0, 0x6b, 0xf1, 0x39, 0xc7, 0x2b, 0x1a, 0x7a,
	0x7d, 0x6f, 0xf4, 0x7b, 0xf6, 0xb7, 0xfd, 0x78, 0x83, 0x57, 0x34, 0x38, 0x01, 0x7e, 0x49, 0x42,
	0xbf, 0xef, 0x8d, 0xfe, 0x9c, 0xee, 0x37, 0xb3, 0x61, 0x3b, 0x03, 0x5e, 0x71, 0x33, 0x3e, 0xcf,
	0xf0, 0x72, 0x4d, 0x67, 0x7e, 0x49, 0x82, 0x14, 0xfc, 0xdf, 0x5e, 0x7f, 0x2e, 0xb1, 0xa2, 0xdc,
	0x84, 0x9d, 0x9a, 0x3f, 0xf8, 0xc6, 0xdf, 0x1a, 0x55, 0x72, 0x66, 0x05, 0xff, 0xcc, 0xd7, 0xed,
	0xd2, 0x1a, 0x0c, 0x62, 0xd0, 0x95, 0xd8, 0x3c, 0x86, 0xdd, 0x7e, 0x67, 0xa7, 0xa0, 0x6e, 0x4e,
	0x3e, 0x3c, 0x30, 0x2c, 0xc4, 0x0a, 0xee, 0x7c, 0xb1, 0x49, 0xb0, 0xf5, 0x1c, 0x69, 0xa5, 0x4c,
	0xbd, 0xfb, 0xeb, 0x06, 0x64, 0x62, 0x89, 0x39, 0x83, 0x42, 0xb1, 0x88, 0x51, 0x5e, 0x0f, 0x6c,
	0x4f, 0x26, 0x4b, 0xfd, 0xc3, 0x05, 0x2f, 0x5c, 0x7a, 0xf6, 0x3b, 0xd3, 0x24, 0x79, 0xf1, 0x07,
	0x53, 0xab, 0x4c, 0x88, 0x86, 0x36, 0x56, 0x29, 0x8b, 0xe1, 0xac, 0x6d, 0xbe, 0xb6, 0x9d, 0x3c,
	0x21, 0x3a, 0x77, 0x9d, 0x3c, 0x8b, 0x73, 0xd7, 0x79, 0xf7, 0x87, 0xf6, 0x07, 0x42, 0x09, 0xd1,
	0x08, 0xb9, 0x16, 0x42, 0x59, 0x8c, 0x90, 0xeb, 0x2d, 0x7e, 0xd5, 0xcb, 0x9e, 0x7d, 0x06, 0x00,
	0x00, 0xff, 0xff, 0xb4, 0xe0, 0x6f, 0x5d, 0x6d, 0x02, 0x00, 0x00,
}
