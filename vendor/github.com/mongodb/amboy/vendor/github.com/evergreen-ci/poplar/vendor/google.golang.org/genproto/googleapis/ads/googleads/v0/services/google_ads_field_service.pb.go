// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/services/google_ads_field_service.proto

package services // import "google.golang.org/genproto/googleapis/ads/googleads/v0/services"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import resources "google.golang.org/genproto/googleapis/ads/googleads/v0/resources"
import _ "google.golang.org/genproto/googleapis/api/annotations"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Request message for [GoogleAdsFieldService.GetGoogleAdsField][google.ads.googleads.v0.services.GoogleAdsFieldService.GetGoogleAdsField].
type GetGoogleAdsFieldRequest struct {
	// The resource name of the field to get.
	ResourceName         string   `protobuf:"bytes,1,opt,name=resource_name,json=resourceName,proto3" json:"resource_name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetGoogleAdsFieldRequest) Reset()         { *m = GetGoogleAdsFieldRequest{} }
func (m *GetGoogleAdsFieldRequest) String() string { return proto.CompactTextString(m) }
func (*GetGoogleAdsFieldRequest) ProtoMessage()    {}
func (*GetGoogleAdsFieldRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_google_ads_field_service_754a158d817e1db4, []int{0}
}
func (m *GetGoogleAdsFieldRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetGoogleAdsFieldRequest.Unmarshal(m, b)
}
func (m *GetGoogleAdsFieldRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetGoogleAdsFieldRequest.Marshal(b, m, deterministic)
}
func (dst *GetGoogleAdsFieldRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetGoogleAdsFieldRequest.Merge(dst, src)
}
func (m *GetGoogleAdsFieldRequest) XXX_Size() int {
	return xxx_messageInfo_GetGoogleAdsFieldRequest.Size(m)
}
func (m *GetGoogleAdsFieldRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetGoogleAdsFieldRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetGoogleAdsFieldRequest proto.InternalMessageInfo

func (m *GetGoogleAdsFieldRequest) GetResourceName() string {
	if m != nil {
		return m.ResourceName
	}
	return ""
}

// Request message for [GoogleAdsFieldService.SearchGoogleAdsFields][google.ads.googleads.v0.services.GoogleAdsFieldService.SearchGoogleAdsFields].
type SearchGoogleAdsFieldsRequest struct {
	// The query string.
	Query string `protobuf:"bytes,1,opt,name=query,proto3" json:"query,omitempty"`
	// Token of the page to retrieve. If not specified, the first page of
	// results will be returned. Use the value obtained from `next_page_token`
	// in the previous response in order to request the next page of results.
	PageToken string `protobuf:"bytes,2,opt,name=page_token,json=pageToken,proto3" json:"page_token,omitempty"`
	// Number of elements to retrieve in a single page.
	// When too large a page is requested, the server may decide to further
	// limit the number of returned resources.
	PageSize             int32    `protobuf:"varint,3,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SearchGoogleAdsFieldsRequest) Reset()         { *m = SearchGoogleAdsFieldsRequest{} }
func (m *SearchGoogleAdsFieldsRequest) String() string { return proto.CompactTextString(m) }
func (*SearchGoogleAdsFieldsRequest) ProtoMessage()    {}
func (*SearchGoogleAdsFieldsRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_google_ads_field_service_754a158d817e1db4, []int{1}
}
func (m *SearchGoogleAdsFieldsRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SearchGoogleAdsFieldsRequest.Unmarshal(m, b)
}
func (m *SearchGoogleAdsFieldsRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SearchGoogleAdsFieldsRequest.Marshal(b, m, deterministic)
}
func (dst *SearchGoogleAdsFieldsRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SearchGoogleAdsFieldsRequest.Merge(dst, src)
}
func (m *SearchGoogleAdsFieldsRequest) XXX_Size() int {
	return xxx_messageInfo_SearchGoogleAdsFieldsRequest.Size(m)
}
func (m *SearchGoogleAdsFieldsRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_SearchGoogleAdsFieldsRequest.DiscardUnknown(m)
}

var xxx_messageInfo_SearchGoogleAdsFieldsRequest proto.InternalMessageInfo

func (m *SearchGoogleAdsFieldsRequest) GetQuery() string {
	if m != nil {
		return m.Query
	}
	return ""
}

func (m *SearchGoogleAdsFieldsRequest) GetPageToken() string {
	if m != nil {
		return m.PageToken
	}
	return ""
}

func (m *SearchGoogleAdsFieldsRequest) GetPageSize() int32 {
	if m != nil {
		return m.PageSize
	}
	return 0
}

// Response message for [GoogleAdsFieldService.SearchGoogleAdsFields][google.ads.googleads.v0.services.GoogleAdsFieldService.SearchGoogleAdsFields].
type SearchGoogleAdsFieldsResponse struct {
	// The list of fields that matched the query.
	Results []*resources.GoogleAdsField `protobuf:"bytes,1,rep,name=results,proto3" json:"results,omitempty"`
	// Pagination token used to retrieve the next page of results. Pass the
	// content of this string as the `page_token` attribute of the next request.
	// `next_page_token` is not returned for the last page.
	NextPageToken string `protobuf:"bytes,2,opt,name=next_page_token,json=nextPageToken,proto3" json:"next_page_token,omitempty"`
	// Total number of results that match the query ignoring the LIMIT clause.
	TotalResultsCount    int64    `protobuf:"varint,3,opt,name=total_results_count,json=totalResultsCount,proto3" json:"total_results_count,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SearchGoogleAdsFieldsResponse) Reset()         { *m = SearchGoogleAdsFieldsResponse{} }
func (m *SearchGoogleAdsFieldsResponse) String() string { return proto.CompactTextString(m) }
func (*SearchGoogleAdsFieldsResponse) ProtoMessage()    {}
func (*SearchGoogleAdsFieldsResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_google_ads_field_service_754a158d817e1db4, []int{2}
}
func (m *SearchGoogleAdsFieldsResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SearchGoogleAdsFieldsResponse.Unmarshal(m, b)
}
func (m *SearchGoogleAdsFieldsResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SearchGoogleAdsFieldsResponse.Marshal(b, m, deterministic)
}
func (dst *SearchGoogleAdsFieldsResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SearchGoogleAdsFieldsResponse.Merge(dst, src)
}
func (m *SearchGoogleAdsFieldsResponse) XXX_Size() int {
	return xxx_messageInfo_SearchGoogleAdsFieldsResponse.Size(m)
}
func (m *SearchGoogleAdsFieldsResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_SearchGoogleAdsFieldsResponse.DiscardUnknown(m)
}

var xxx_messageInfo_SearchGoogleAdsFieldsResponse proto.InternalMessageInfo

func (m *SearchGoogleAdsFieldsResponse) GetResults() []*resources.GoogleAdsField {
	if m != nil {
		return m.Results
	}
	return nil
}

func (m *SearchGoogleAdsFieldsResponse) GetNextPageToken() string {
	if m != nil {
		return m.NextPageToken
	}
	return ""
}

func (m *SearchGoogleAdsFieldsResponse) GetTotalResultsCount() int64 {
	if m != nil {
		return m.TotalResultsCount
	}
	return 0
}

func init() {
	proto.RegisterType((*GetGoogleAdsFieldRequest)(nil), "google.ads.googleads.v0.services.GetGoogleAdsFieldRequest")
	proto.RegisterType((*SearchGoogleAdsFieldsRequest)(nil), "google.ads.googleads.v0.services.SearchGoogleAdsFieldsRequest")
	proto.RegisterType((*SearchGoogleAdsFieldsResponse)(nil), "google.ads.googleads.v0.services.SearchGoogleAdsFieldsResponse")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// GoogleAdsFieldServiceClient is the client API for GoogleAdsFieldService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type GoogleAdsFieldServiceClient interface {
	// Returns just the requested field.
	GetGoogleAdsField(ctx context.Context, in *GetGoogleAdsFieldRequest, opts ...grpc.CallOption) (*resources.GoogleAdsField, error)
	// Returns all fields that match the search query.
	SearchGoogleAdsFields(ctx context.Context, in *SearchGoogleAdsFieldsRequest, opts ...grpc.CallOption) (*SearchGoogleAdsFieldsResponse, error)
}

type googleAdsFieldServiceClient struct {
	cc *grpc.ClientConn
}

func NewGoogleAdsFieldServiceClient(cc *grpc.ClientConn) GoogleAdsFieldServiceClient {
	return &googleAdsFieldServiceClient{cc}
}

func (c *googleAdsFieldServiceClient) GetGoogleAdsField(ctx context.Context, in *GetGoogleAdsFieldRequest, opts ...grpc.CallOption) (*resources.GoogleAdsField, error) {
	out := new(resources.GoogleAdsField)
	err := c.cc.Invoke(ctx, "/google.ads.googleads.v0.services.GoogleAdsFieldService/GetGoogleAdsField", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *googleAdsFieldServiceClient) SearchGoogleAdsFields(ctx context.Context, in *SearchGoogleAdsFieldsRequest, opts ...grpc.CallOption) (*SearchGoogleAdsFieldsResponse, error) {
	out := new(SearchGoogleAdsFieldsResponse)
	err := c.cc.Invoke(ctx, "/google.ads.googleads.v0.services.GoogleAdsFieldService/SearchGoogleAdsFields", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GoogleAdsFieldServiceServer is the server API for GoogleAdsFieldService service.
type GoogleAdsFieldServiceServer interface {
	// Returns just the requested field.
	GetGoogleAdsField(context.Context, *GetGoogleAdsFieldRequest) (*resources.GoogleAdsField, error)
	// Returns all fields that match the search query.
	SearchGoogleAdsFields(context.Context, *SearchGoogleAdsFieldsRequest) (*SearchGoogleAdsFieldsResponse, error)
}

func RegisterGoogleAdsFieldServiceServer(s *grpc.Server, srv GoogleAdsFieldServiceServer) {
	s.RegisterService(&_GoogleAdsFieldService_serviceDesc, srv)
}

func _GoogleAdsFieldService_GetGoogleAdsField_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetGoogleAdsFieldRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GoogleAdsFieldServiceServer).GetGoogleAdsField(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.ads.googleads.v0.services.GoogleAdsFieldService/GetGoogleAdsField",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GoogleAdsFieldServiceServer).GetGoogleAdsField(ctx, req.(*GetGoogleAdsFieldRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GoogleAdsFieldService_SearchGoogleAdsFields_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SearchGoogleAdsFieldsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GoogleAdsFieldServiceServer).SearchGoogleAdsFields(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.ads.googleads.v0.services.GoogleAdsFieldService/SearchGoogleAdsFields",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GoogleAdsFieldServiceServer).SearchGoogleAdsFields(ctx, req.(*SearchGoogleAdsFieldsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _GoogleAdsFieldService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "google.ads.googleads.v0.services.GoogleAdsFieldService",
	HandlerType: (*GoogleAdsFieldServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetGoogleAdsField",
			Handler:    _GoogleAdsFieldService_GetGoogleAdsField_Handler,
		},
		{
			MethodName: "SearchGoogleAdsFields",
			Handler:    _GoogleAdsFieldService_SearchGoogleAdsFields_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "google/ads/googleads/v0/services/google_ads_field_service.proto",
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/services/google_ads_field_service.proto", fileDescriptor_google_ads_field_service_754a158d817e1db4)
}

var fileDescriptor_google_ads_field_service_754a158d817e1db4 = []byte{
	// 539 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x54, 0x41, 0x8b, 0xd3, 0x4e,
	0x14, 0x27, 0x29, 0xfb, 0xff, 0xbb, 0xa3, 0x8b, 0xec, 0xe8, 0x42, 0x89, 0xbb, 0x58, 0xe2, 0xae,
	0x96, 0x05, 0x27, 0x75, 0xbd, 0xc8, 0x88, 0x96, 0xac, 0x60, 0x05, 0x41, 0x4a, 0x2b, 0x3d, 0x48,
	0x21, 0x8c, 0xcd, 0x33, 0x06, 0xdb, 0x99, 0x6c, 0x66, 0x5a, 0x74, 0xc5, 0x83, 0xde, 0x3d, 0xf9,
	0x0d, 0x3c, 0x7a, 0xf3, 0x13, 0x78, 0xf1, 0xe4, 0xd5, 0xaf, 0xe0, 0xc9, 0x4f, 0x21, 0x93, 0xe9,
	0x04, 0xbb, 0xdb, 0x58, 0xf4, 0x36, 0x79, 0xbf, 0xf7, 0xfb, 0xfd, 0xde, 0xbc, 0xf7, 0x26, 0xa8,
	0x9d, 0x08, 0x91, 0x8c, 0x21, 0x60, 0xb1, 0x0c, 0xcc, 0x51, 0x9f, 0x66, 0xad, 0x40, 0x42, 0x3e,
	0x4b, 0x47, 0x60, 0xa3, 0x11, 0x8b, 0x65, 0xf4, 0x2c, 0x85, 0x71, 0x1c, 0xcd, 0x11, 0x92, 0xe5,
	0x42, 0x09, 0xdc, 0x30, 0x38, 0x61, 0xb1, 0x24, 0xa5, 0x00, 0x99, 0xb5, 0x88, 0x15, 0xf0, 0x6e,
	0x55, 0x59, 0xe4, 0x20, 0xc5, 0x34, 0x5f, 0xe6, 0x61, 0xb4, 0xbd, 0x6d, 0xcb, 0xcc, 0xd2, 0x80,
	0x71, 0x2e, 0x14, 0x53, 0xa9, 0xe0, 0xd2, 0xa0, 0x7e, 0x1b, 0xd5, 0x3b, 0xa0, 0x3a, 0x45, 0x4a,
	0x18, 0xcb, 0xfb, 0x9a, 0xd8, 0x83, 0xa3, 0x29, 0x48, 0x85, 0xaf, 0xa0, 0x0d, 0xab, 0x1e, 0x71,
	0x36, 0x81, 0xba, 0xd3, 0x70, 0x9a, 0xeb, 0xbd, 0x73, 0x36, 0xf8, 0x88, 0x4d, 0xc0, 0xcf, 0xd0,
	0x76, 0x1f, 0x58, 0x3e, 0x7a, 0xbe, 0xa8, 0x21, 0xad, 0xc8, 0x45, 0xb4, 0x76, 0x34, 0x85, 0xfc,
	0xd5, 0x9c, 0x6c, 0x3e, 0xf0, 0x0e, 0x42, 0x19, 0x4b, 0x20, 0x52, 0xe2, 0x05, 0xf0, 0xba, 0x5b,
	0x40, 0xeb, 0x3a, 0xf2, 0x58, 0x07, 0xf0, 0x25, 0x54, 0x7c, 0x44, 0x32, 0x3d, 0x86, 0x7a, 0xad,
	0xe1, 0x34, 0xd7, 0x7a, 0x67, 0x74, 0xa0, 0x9f, 0x1e, 0x83, 0xff, 0xd5, 0x41, 0x3b, 0x15, 0x96,
	0x32, 0x13, 0x5c, 0x02, 0x7e, 0x88, 0xfe, 0xcf, 0x41, 0x4e, 0xc7, 0x4a, 0xd6, 0x9d, 0x46, 0xad,
	0x79, 0xf6, 0xe0, 0x06, 0xa9, 0x6a, 0x70, 0xd9, 0x3e, 0x72, 0xa2, 0x07, 0x56, 0x01, 0x5f, 0x45,
	0xe7, 0x39, 0xbc, 0x54, 0xd1, 0xa9, 0x7a, 0x37, 0x74, 0xb8, 0x5b, 0xd6, 0x4c, 0xd0, 0x05, 0x25,
	0x14, 0x1b, 0x47, 0x73, 0x62, 0x34, 0x12, 0x53, 0xae, 0x8a, 0xea, 0x6b, 0xbd, 0xcd, 0x02, 0xea,
	0x19, 0xe4, 0x9e, 0x06, 0x0e, 0xde, 0xd7, 0xd0, 0xd6, 0xa2, 0x67, 0xdf, 0x0c, 0x1b, 0x7f, 0x76,
	0xd0, 0xe6, 0xa9, 0xa1, 0x60, 0x4a, 0x56, 0x2d, 0x09, 0xa9, 0x9a, 0xa4, 0xf7, 0xf7, 0xf7, 0xf7,
	0xaf, 0xbf, 0xfb, 0xfe, 0xe3, 0x83, 0x7b, 0x0d, 0xef, 0xe9, 0x25, 0x7b, 0xbd, 0xb0, 0x07, 0x77,
	0x92, 0xc5, 0xbe, 0x07, 0xfb, 0x6f, 0xf0, 0x17, 0x07, 0x6d, 0x2d, 0x1d, 0x0a, 0xbe, 0xbb, 0xba,
	0xee, 0x3f, 0x2d, 0x90, 0xd7, 0xfe, 0x67, 0xbe, 0xd9, 0x06, 0x7f, 0xaf, 0xb8, 0xc9, 0x65, 0xdf,
	0xd3, 0x37, 0x39, 0x51, 0x3a, 0x95, 0x05, 0x95, 0x3a, 0xfb, 0x87, 0x6f, 0x5d, 0xb4, 0x3b, 0x12,
	0x93, 0x95, 0x6e, 0x87, 0xde, 0xd2, 0xa9, 0x75, 0xf5, 0x73, 0xea, 0x3a, 0x4f, 0x1e, 0xcc, 0xf9,
	0x89, 0x18, 0x33, 0x9e, 0x10, 0x91, 0x27, 0x41, 0x02, 0xbc, 0x78, 0x6c, 0xf6, 0xe1, 0x66, 0xa9,
	0xac, 0xfe, 0x55, 0xdc, 0xb6, 0x87, 0x8f, 0x6e, 0xad, 0x13, 0x86, 0x9f, 0xdc, 0x86, 0xb1, 0x23,
	0x61, 0xfc, 0xdb, 0x8c, 0xc8, 0xa0, 0x45, 0xe6, 0xc6, 0xf2, 0x9b, 0x4d, 0x19, 0x86, 0xb1, 0x1c,
	0x96, 0x29, 0xc3, 0x41, 0x6b, 0x68, 0x53, 0x7e, 0xba, 0xbb, 0x26, 0x4e, 0x69, 0x18, 0x4b, 0x4a,
	0xcb, 0x24, 0x4a, 0x07, 0x2d, 0x4a, 0x6d, 0xda, 0xd3, 0xff, 0x8a, 0x3a, 0x6f, 0xfe, 0x0a, 0x00,
	0x00, 0xff, 0xff, 0x44, 0xa9, 0x28, 0xa6, 0xd1, 0x04, 0x00, 0x00,
}
