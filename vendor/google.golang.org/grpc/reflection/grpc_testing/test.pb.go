// Code generated by protoc-gen-go. DO NOT EDIT.
// source: test.proto

package grpc_testing

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

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

type SearchResponse struct {
	Results              []*SearchResponse_Result `protobuf:"bytes,1,rep,name=results,proto3" json:"results,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                 `json:"-"`
	XXX_unrecognized     []byte                   `json:"-"`
	XXX_sizecache        int32                    `json:"-"`
}

func (m *SearchResponse) Reset()         { *m = SearchResponse{} }
func (m *SearchResponse) String() string { return proto.CompactTextString(m) }
func (*SearchResponse) ProtoMessage()    {}
func (*SearchResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_test_a0c753075da50dd4, []int{0}
}
func (m *SearchResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SearchResponse.Unmarshal(m, b)
}
func (m *SearchResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SearchResponse.Marshal(b, m, deterministic)
}
func (dst *SearchResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SearchResponse.Merge(dst, src)
}
func (m *SearchResponse) XXX_Size() int {
	return xxx_messageInfo_SearchResponse.Size(m)
}
func (m *SearchResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_SearchResponse.DiscardUnknown(m)
}

var xxx_messageInfo_SearchResponse proto.InternalMessageInfo

func (m *SearchResponse) GetResults() []*SearchResponse_Result {
	if m != nil {
		return m.Results
	}
	return nil
}

type SearchResponse_Result struct {
	Url                  string   `protobuf:"bytes,1,opt,name=url,proto3" json:"url,omitempty"`
	Title                string   `protobuf:"bytes,2,opt,name=title,proto3" json:"title,omitempty"`
	Snippets             []string `protobuf:"bytes,3,rep,name=snippets,proto3" json:"snippets,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SearchResponse_Result) Reset()         { *m = SearchResponse_Result{} }
func (m *SearchResponse_Result) String() string { return proto.CompactTextString(m) }
func (*SearchResponse_Result) ProtoMessage()    {}
func (*SearchResponse_Result) Descriptor() ([]byte, []int) {
	return fileDescriptor_test_a0c753075da50dd4, []int{0, 0}
}
func (m *SearchResponse_Result) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SearchResponse_Result.Unmarshal(m, b)
}
func (m *SearchResponse_Result) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SearchResponse_Result.Marshal(b, m, deterministic)
}
func (dst *SearchResponse_Result) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SearchResponse_Result.Merge(dst, src)
}
func (m *SearchResponse_Result) XXX_Size() int {
	return xxx_messageInfo_SearchResponse_Result.Size(m)
}
func (m *SearchResponse_Result) XXX_DiscardUnknown() {
	xxx_messageInfo_SearchResponse_Result.DiscardUnknown(m)
}

var xxx_messageInfo_SearchResponse_Result proto.InternalMessageInfo

func (m *SearchResponse_Result) GetUrl() string {
	if m != nil {
		return m.Url
	}
	return ""
}

func (m *SearchResponse_Result) GetTitle() string {
	if m != nil {
		return m.Title
	}
	return ""
}

func (m *SearchResponse_Result) GetSnippets() []string {
	if m != nil {
		return m.Snippets
	}
	return nil
}

type SearchRequest struct {
	Query                string   `protobuf:"bytes,1,opt,name=query,proto3" json:"query,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SearchRequest) Reset()         { *m = SearchRequest{} }
func (m *SearchRequest) String() string { return proto.CompactTextString(m) }
func (*SearchRequest) ProtoMessage()    {}
func (*SearchRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_test_a0c753075da50dd4, []int{1}
}
func (m *SearchRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SearchRequest.Unmarshal(m, b)
}
func (m *SearchRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SearchRequest.Marshal(b, m, deterministic)
}
func (dst *SearchRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SearchRequest.Merge(dst, src)
}
func (m *SearchRequest) XXX_Size() int {
	return xxx_messageInfo_SearchRequest.Size(m)
}
func (m *SearchRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_SearchRequest.DiscardUnknown(m)
}

var xxx_messageInfo_SearchRequest proto.InternalMessageInfo

func (m *SearchRequest) GetQuery() string {
	if m != nil {
		return m.Query
	}
	return ""
}

func init() {
	proto.RegisterType((*SearchResponse)(nil), "grpc.testing.SearchResponse")
	proto.RegisterType((*SearchResponse_Result)(nil), "grpc.testing.SearchResponse.Result")
	proto.RegisterType((*SearchRequest)(nil), "grpc.testing.SearchRequest")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// SearchServiceClient is the client API for SearchService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type SearchServiceClient interface {
	Search(ctx context.Context, in *SearchRequest, opts ...grpc.CallOption) (*SearchResponse, error)
	StreamingSearch(ctx context.Context, opts ...grpc.CallOption) (SearchService_StreamingSearchClient, error)
}

type searchServiceClient struct {
	cc *grpc.ClientConn
}

func NewSearchServiceClient(cc *grpc.ClientConn) SearchServiceClient {
	return &searchServiceClient{cc}
}

func (c *searchServiceClient) Search(ctx context.Context, in *SearchRequest, opts ...grpc.CallOption) (*SearchResponse, error) {
	out := new(SearchResponse)
	err := c.cc.Invoke(ctx, "/grpc.testing.SearchService/Search", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *searchServiceClient) StreamingSearch(ctx context.Context, opts ...grpc.CallOption) (SearchService_StreamingSearchClient, error) {
	stream, err := c.cc.NewStream(ctx, &_SearchService_serviceDesc.Streams[0], "/grpc.testing.SearchService/StreamingSearch", opts...)
	if err != nil {
		return nil, err
	}
	x := &searchServiceStreamingSearchClient{stream}
	return x, nil
}

type SearchService_StreamingSearchClient interface {
	Send(*SearchRequest) error
	Recv() (*SearchResponse, error)
	grpc.ClientStream
}

type searchServiceStreamingSearchClient struct {
	grpc.ClientStream
}

func (x *searchServiceStreamingSearchClient) Send(m *SearchRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *searchServiceStreamingSearchClient) Recv() (*SearchResponse, error) {
	m := new(SearchResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// SearchServiceServer is the server API for SearchService service.
type SearchServiceServer interface {
	Search(context.Context, *SearchRequest) (*SearchResponse, error)
	StreamingSearch(SearchService_StreamingSearchServer) error
}

func RegisterSearchServiceServer(s *grpc.Server, srv SearchServiceServer) {
	s.RegisterService(&_SearchService_serviceDesc, srv)
}

func _SearchService_Search_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SearchRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SearchServiceServer).Search(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/grpc.testing.SearchService/Search",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SearchServiceServer).Search(ctx, req.(*SearchRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SearchService_StreamingSearch_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(SearchServiceServer).StreamingSearch(&searchServiceStreamingSearchServer{stream})
}

type SearchService_StreamingSearchServer interface {
	Send(*SearchResponse) error
	Recv() (*SearchRequest, error)
	grpc.ServerStream
}

type searchServiceStreamingSearchServer struct {
	grpc.ServerStream
}

func (x *searchServiceStreamingSearchServer) Send(m *SearchResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *searchServiceStreamingSearchServer) Recv() (*SearchRequest, error) {
	m := new(SearchRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var _SearchService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "grpc.testing.SearchService",
	HandlerType: (*SearchServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Search",
			Handler:    _SearchService_Search_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamingSearch",
			Handler:       _SearchService_StreamingSearch_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "test.proto",
}

func init() { proto.RegisterFile("test.proto", fileDescriptor_test_a0c753075da50dd4) }

var fileDescriptor_test_a0c753075da50dd4 = []byte{
	// 231 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xa4, 0x91, 0xbd, 0x4a, 0xc5, 0x40,
	0x10, 0x85, 0x59, 0x83, 0xd1, 0x3b, 0xfe, 0x32, 0x58, 0x84, 0x68, 0x11, 0xae, 0x08, 0xa9, 0x16,
	0xb9, 0xd6, 0x56, 0xb6, 0x16, 0xb2, 0x79, 0x82, 0x6b, 0x18, 0xe2, 0x42, 0x4c, 0x36, 0x33, 0x13,
	0xc1, 0x87, 0xb1, 0xf5, 0x39, 0x25, 0x59, 0x23, 0x0a, 0x62, 0x63, 0xb7, 0xe7, 0xe3, 0xcc, 0xb7,
	0xbb, 0x0c, 0x80, 0x92, 0xa8, 0x0d, 0xdc, 0x6b, 0x8f, 0x87, 0x0d, 0x87, 0xda, 0x4e, 0xc0, 0x77,
	0xcd, 0xfa, 0xcd, 0xc0, 0x71, 0x45, 0x5b, 0xae, 0x9f, 0x1c, 0x49, 0xe8, 0x3b, 0x21, 0xbc, 0x85,
	0x3d, 0x26, 0x19, 0x5b, 0x95, 0xcc, 0x14, 0x49, 0x79, 0xb0, 0xb9, 0xb4, 0xdf, 0x47, 0xec, 0xcf,
	0xba, 0x75, 0x73, 0xd7, 0x2d, 0x33, 0xf9, 0x3d, 0xa4, 0x11, 0xe1, 0x29, 0x24, 0x23, 0xb7, 0x99,
	0x29, 0x4c, 0xb9, 0x72, 0xd3, 0x11, 0xcf, 0x60, 0x57, 0xbd, 0xb6, 0x94, 0xed, 0xcc, 0x2c, 0x06,
	0xcc, 0x61, 0x5f, 0x3a, 0x1f, 0x02, 0xa9, 0x64, 0x49, 0x91, 0x94, 0x2b, 0xf7, 0x95, 0xd7, 0x57,
	0x70, 0xb4, 0xdc, 0x37, 0x8c, 0x24, 0x3a, 0x29, 0x86, 0x91, 0xf8, 0xf5, 0x53, 0x1b, 0xc3, 0xe6,
	0xdd, 0x2c, 0xbd, 0x8a, 0xf8, 0xc5, 0xd7, 0x84, 0x77, 0x90, 0x46, 0x80, 0xe7, 0xbf, 0x3f, 0x7f,
	0xd6, 0xe5, 0x17, 0x7f, 0xfd, 0x0d, 0x1f, 0xe0, 0xa4, 0x52, 0xa6, 0xed, 0xb3, 0xef, 0x9a, 0x7f,
	0xdb, 0x4a, 0x73, 0x6d, 0x1e, 0xd3, 0x79, 0x09, 0x37, 0x1f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x20,
	0xd6, 0x09, 0xb8, 0x92, 0x01, 0x00, 0x00,
}
