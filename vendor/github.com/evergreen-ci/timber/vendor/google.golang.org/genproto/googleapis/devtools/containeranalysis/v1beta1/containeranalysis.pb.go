// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/devtools/containeranalysis/v1beta1/containeranalysis.proto

package containeranalysis // import "google.golang.org/genproto/googleapis/devtools/containeranalysis/v1beta1"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import timestamp "github.com/golang/protobuf/ptypes/timestamp"
import _ "google.golang.org/genproto/googleapis/api/annotations"
import v1 "google.golang.org/genproto/googleapis/iam/v1"

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

// A scan configuration specifies whether Cloud components in a project have a
// particular type of analysis being run. For example, it can configure whether
// vulnerability scanning is being done on Docker images or not.
type ScanConfig struct {
	// Output only. The name of the scan configuration in the form of
	// `projects/[PROJECT_ID]/scanConfigs/[SCAN_CONFIG_ID]`.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Output only. A human-readable description of what the scan configuration
	// does.
	Description string `protobuf:"bytes,2,opt,name=description,proto3" json:"description,omitempty"`
	// Whether the scan is enabled.
	Enabled bool `protobuf:"varint,3,opt,name=enabled,proto3" json:"enabled,omitempty"`
	// Output only. The time this scan config was created.
	CreateTime *timestamp.Timestamp `protobuf:"bytes,4,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
	// Output only. The time this scan config was last updated.
	UpdateTime           *timestamp.Timestamp `protobuf:"bytes,5,opt,name=update_time,json=updateTime,proto3" json:"update_time,omitempty"`
	XXX_NoUnkeyedLiteral struct{}             `json:"-"`
	XXX_unrecognized     []byte               `json:"-"`
	XXX_sizecache        int32                `json:"-"`
}

func (m *ScanConfig) Reset()         { *m = ScanConfig{} }
func (m *ScanConfig) String() string { return proto.CompactTextString(m) }
func (*ScanConfig) ProtoMessage()    {}
func (*ScanConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_containeranalysis_a170acd3c74dfdfb, []int{0}
}
func (m *ScanConfig) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ScanConfig.Unmarshal(m, b)
}
func (m *ScanConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ScanConfig.Marshal(b, m, deterministic)
}
func (dst *ScanConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ScanConfig.Merge(dst, src)
}
func (m *ScanConfig) XXX_Size() int {
	return xxx_messageInfo_ScanConfig.Size(m)
}
func (m *ScanConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_ScanConfig.DiscardUnknown(m)
}

var xxx_messageInfo_ScanConfig proto.InternalMessageInfo

func (m *ScanConfig) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *ScanConfig) GetDescription() string {
	if m != nil {
		return m.Description
	}
	return ""
}

func (m *ScanConfig) GetEnabled() bool {
	if m != nil {
		return m.Enabled
	}
	return false
}

func (m *ScanConfig) GetCreateTime() *timestamp.Timestamp {
	if m != nil {
		return m.CreateTime
	}
	return nil
}

func (m *ScanConfig) GetUpdateTime() *timestamp.Timestamp {
	if m != nil {
		return m.UpdateTime
	}
	return nil
}

// Request to get a scan configuration.
type GetScanConfigRequest struct {
	// The name of the scan configuration in the form of
	// `projects/[PROJECT_ID]/scanConfigs/[SCAN_CONFIG_ID]`.
	Name                 string   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetScanConfigRequest) Reset()         { *m = GetScanConfigRequest{} }
func (m *GetScanConfigRequest) String() string { return proto.CompactTextString(m) }
func (*GetScanConfigRequest) ProtoMessage()    {}
func (*GetScanConfigRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_containeranalysis_a170acd3c74dfdfb, []int{1}
}
func (m *GetScanConfigRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetScanConfigRequest.Unmarshal(m, b)
}
func (m *GetScanConfigRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetScanConfigRequest.Marshal(b, m, deterministic)
}
func (dst *GetScanConfigRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetScanConfigRequest.Merge(dst, src)
}
func (m *GetScanConfigRequest) XXX_Size() int {
	return xxx_messageInfo_GetScanConfigRequest.Size(m)
}
func (m *GetScanConfigRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetScanConfigRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetScanConfigRequest proto.InternalMessageInfo

func (m *GetScanConfigRequest) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

// Request to list scan configurations.
type ListScanConfigsRequest struct {
	// The name of the project to list scan configurations for in the form of
	// `projects/[PROJECT_ID]`.
	Parent string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	// The filter expression.
	Filter string `protobuf:"bytes,2,opt,name=filter,proto3" json:"filter,omitempty"`
	// The number of scan configs to return in the list.
	PageSize int32 `protobuf:"varint,3,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	// Token to provide to skip to a particular spot in the list.
	PageToken            string   `protobuf:"bytes,4,opt,name=page_token,json=pageToken,proto3" json:"page_token,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListScanConfigsRequest) Reset()         { *m = ListScanConfigsRequest{} }
func (m *ListScanConfigsRequest) String() string { return proto.CompactTextString(m) }
func (*ListScanConfigsRequest) ProtoMessage()    {}
func (*ListScanConfigsRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_containeranalysis_a170acd3c74dfdfb, []int{2}
}
func (m *ListScanConfigsRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListScanConfigsRequest.Unmarshal(m, b)
}
func (m *ListScanConfigsRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListScanConfigsRequest.Marshal(b, m, deterministic)
}
func (dst *ListScanConfigsRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListScanConfigsRequest.Merge(dst, src)
}
func (m *ListScanConfigsRequest) XXX_Size() int {
	return xxx_messageInfo_ListScanConfigsRequest.Size(m)
}
func (m *ListScanConfigsRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ListScanConfigsRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ListScanConfigsRequest proto.InternalMessageInfo

func (m *ListScanConfigsRequest) GetParent() string {
	if m != nil {
		return m.Parent
	}
	return ""
}

func (m *ListScanConfigsRequest) GetFilter() string {
	if m != nil {
		return m.Filter
	}
	return ""
}

func (m *ListScanConfigsRequest) GetPageSize() int32 {
	if m != nil {
		return m.PageSize
	}
	return 0
}

func (m *ListScanConfigsRequest) GetPageToken() string {
	if m != nil {
		return m.PageToken
	}
	return ""
}

// Response for listing scan configurations.
type ListScanConfigsResponse struct {
	// The scan configurations requested.
	ScanConfigs []*ScanConfig `protobuf:"bytes,1,rep,name=scan_configs,json=scanConfigs,proto3" json:"scan_configs,omitempty"`
	// The next pagination token in the list response. It should be used as
	// `page_token` for the following request. An empty value means no more
	// results.
	NextPageToken        string   `protobuf:"bytes,2,opt,name=next_page_token,json=nextPageToken,proto3" json:"next_page_token,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListScanConfigsResponse) Reset()         { *m = ListScanConfigsResponse{} }
func (m *ListScanConfigsResponse) String() string { return proto.CompactTextString(m) }
func (*ListScanConfigsResponse) ProtoMessage()    {}
func (*ListScanConfigsResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_containeranalysis_a170acd3c74dfdfb, []int{3}
}
func (m *ListScanConfigsResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListScanConfigsResponse.Unmarshal(m, b)
}
func (m *ListScanConfigsResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListScanConfigsResponse.Marshal(b, m, deterministic)
}
func (dst *ListScanConfigsResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListScanConfigsResponse.Merge(dst, src)
}
func (m *ListScanConfigsResponse) XXX_Size() int {
	return xxx_messageInfo_ListScanConfigsResponse.Size(m)
}
func (m *ListScanConfigsResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ListScanConfigsResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ListScanConfigsResponse proto.InternalMessageInfo

func (m *ListScanConfigsResponse) GetScanConfigs() []*ScanConfig {
	if m != nil {
		return m.ScanConfigs
	}
	return nil
}

func (m *ListScanConfigsResponse) GetNextPageToken() string {
	if m != nil {
		return m.NextPageToken
	}
	return ""
}

// A request to update a scan configuration.
type UpdateScanConfigRequest struct {
	// The name of the scan configuration in the form of
	// `projects/[PROJECT_ID]/scanConfigs/[SCAN_CONFIG_ID]`.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The updated scan configuration.
	ScanConfig           *ScanConfig `protobuf:"bytes,2,opt,name=scan_config,json=scanConfig,proto3" json:"scan_config,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *UpdateScanConfigRequest) Reset()         { *m = UpdateScanConfigRequest{} }
func (m *UpdateScanConfigRequest) String() string { return proto.CompactTextString(m) }
func (*UpdateScanConfigRequest) ProtoMessage()    {}
func (*UpdateScanConfigRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_containeranalysis_a170acd3c74dfdfb, []int{4}
}
func (m *UpdateScanConfigRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UpdateScanConfigRequest.Unmarshal(m, b)
}
func (m *UpdateScanConfigRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UpdateScanConfigRequest.Marshal(b, m, deterministic)
}
func (dst *UpdateScanConfigRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateScanConfigRequest.Merge(dst, src)
}
func (m *UpdateScanConfigRequest) XXX_Size() int {
	return xxx_messageInfo_UpdateScanConfigRequest.Size(m)
}
func (m *UpdateScanConfigRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateScanConfigRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateScanConfigRequest proto.InternalMessageInfo

func (m *UpdateScanConfigRequest) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *UpdateScanConfigRequest) GetScanConfig() *ScanConfig {
	if m != nil {
		return m.ScanConfig
	}
	return nil
}

func init() {
	proto.RegisterType((*ScanConfig)(nil), "google.devtools.containeranalysis.v1beta1.ScanConfig")
	proto.RegisterType((*GetScanConfigRequest)(nil), "google.devtools.containeranalysis.v1beta1.GetScanConfigRequest")
	proto.RegisterType((*ListScanConfigsRequest)(nil), "google.devtools.containeranalysis.v1beta1.ListScanConfigsRequest")
	proto.RegisterType((*ListScanConfigsResponse)(nil), "google.devtools.containeranalysis.v1beta1.ListScanConfigsResponse")
	proto.RegisterType((*UpdateScanConfigRequest)(nil), "google.devtools.containeranalysis.v1beta1.UpdateScanConfigRequest")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// ContainerAnalysisV1Beta1Client is the client API for ContainerAnalysisV1Beta1 service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type ContainerAnalysisV1Beta1Client interface {
	// Sets the access control policy on the specified note or occurrence.
	// Requires `containeranalysis.notes.setIamPolicy` or
	// `containeranalysis.occurrences.setIamPolicy` permission if the resource is
	// a note or an occurrence, respectively.
	//
	// The resource takes the format `projects/[PROJECT_ID]/notes/[NOTE_ID]` for
	// notes and `projects/[PROJECT_ID]/occurrences/[OCCURRENCE_ID]` for
	// occurrences.
	SetIamPolicy(ctx context.Context, in *v1.SetIamPolicyRequest, opts ...grpc.CallOption) (*v1.Policy, error)
	// Gets the access control policy for a note or an occurrence resource.
	// Requires `containeranalysis.notes.setIamPolicy` or
	// `containeranalysis.occurrences.setIamPolicy` permission if the resource is
	// a note or occurrence, respectively.
	//
	// The resource takes the format `projects/[PROJECT_ID]/notes/[NOTE_ID]` for
	// notes and `projects/[PROJECT_ID]/occurrences/[OCCURRENCE_ID]` for
	// occurrences.
	GetIamPolicy(ctx context.Context, in *v1.GetIamPolicyRequest, opts ...grpc.CallOption) (*v1.Policy, error)
	// Returns the permissions that a caller has on the specified note or
	// occurrence. Requires list permission on the project (for example,
	// `containeranalysis.notes.list`).
	//
	// The resource takes the format `projects/[PROJECT_ID]/notes/[NOTE_ID]` for
	// notes and `projects/[PROJECT_ID]/occurrences/[OCCURRENCE_ID]` for
	// occurrences.
	TestIamPermissions(ctx context.Context, in *v1.TestIamPermissionsRequest, opts ...grpc.CallOption) (*v1.TestIamPermissionsResponse, error)
	// Gets the specified scan configuration.
	GetScanConfig(ctx context.Context, in *GetScanConfigRequest, opts ...grpc.CallOption) (*ScanConfig, error)
	// Lists scan configurations for the specified project.
	ListScanConfigs(ctx context.Context, in *ListScanConfigsRequest, opts ...grpc.CallOption) (*ListScanConfigsResponse, error)
	// Updates the specified scan configuration.
	UpdateScanConfig(ctx context.Context, in *UpdateScanConfigRequest, opts ...grpc.CallOption) (*ScanConfig, error)
}

type containerAnalysisV1Beta1Client struct {
	cc *grpc.ClientConn
}

func NewContainerAnalysisV1Beta1Client(cc *grpc.ClientConn) ContainerAnalysisV1Beta1Client {
	return &containerAnalysisV1Beta1Client{cc}
}

func (c *containerAnalysisV1Beta1Client) SetIamPolicy(ctx context.Context, in *v1.SetIamPolicyRequest, opts ...grpc.CallOption) (*v1.Policy, error) {
	out := new(v1.Policy)
	err := c.cc.Invoke(ctx, "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/SetIamPolicy", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *containerAnalysisV1Beta1Client) GetIamPolicy(ctx context.Context, in *v1.GetIamPolicyRequest, opts ...grpc.CallOption) (*v1.Policy, error) {
	out := new(v1.Policy)
	err := c.cc.Invoke(ctx, "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/GetIamPolicy", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *containerAnalysisV1Beta1Client) TestIamPermissions(ctx context.Context, in *v1.TestIamPermissionsRequest, opts ...grpc.CallOption) (*v1.TestIamPermissionsResponse, error) {
	out := new(v1.TestIamPermissionsResponse)
	err := c.cc.Invoke(ctx, "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/TestIamPermissions", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *containerAnalysisV1Beta1Client) GetScanConfig(ctx context.Context, in *GetScanConfigRequest, opts ...grpc.CallOption) (*ScanConfig, error) {
	out := new(ScanConfig)
	err := c.cc.Invoke(ctx, "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/GetScanConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *containerAnalysisV1Beta1Client) ListScanConfigs(ctx context.Context, in *ListScanConfigsRequest, opts ...grpc.CallOption) (*ListScanConfigsResponse, error) {
	out := new(ListScanConfigsResponse)
	err := c.cc.Invoke(ctx, "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/ListScanConfigs", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *containerAnalysisV1Beta1Client) UpdateScanConfig(ctx context.Context, in *UpdateScanConfigRequest, opts ...grpc.CallOption) (*ScanConfig, error) {
	out := new(ScanConfig)
	err := c.cc.Invoke(ctx, "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/UpdateScanConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ContainerAnalysisV1Beta1Server is the server API for ContainerAnalysisV1Beta1 service.
type ContainerAnalysisV1Beta1Server interface {
	// Sets the access control policy on the specified note or occurrence.
	// Requires `containeranalysis.notes.setIamPolicy` or
	// `containeranalysis.occurrences.setIamPolicy` permission if the resource is
	// a note or an occurrence, respectively.
	//
	// The resource takes the format `projects/[PROJECT_ID]/notes/[NOTE_ID]` for
	// notes and `projects/[PROJECT_ID]/occurrences/[OCCURRENCE_ID]` for
	// occurrences.
	SetIamPolicy(context.Context, *v1.SetIamPolicyRequest) (*v1.Policy, error)
	// Gets the access control policy for a note or an occurrence resource.
	// Requires `containeranalysis.notes.setIamPolicy` or
	// `containeranalysis.occurrences.setIamPolicy` permission if the resource is
	// a note or occurrence, respectively.
	//
	// The resource takes the format `projects/[PROJECT_ID]/notes/[NOTE_ID]` for
	// notes and `projects/[PROJECT_ID]/occurrences/[OCCURRENCE_ID]` for
	// occurrences.
	GetIamPolicy(context.Context, *v1.GetIamPolicyRequest) (*v1.Policy, error)
	// Returns the permissions that a caller has on the specified note or
	// occurrence. Requires list permission on the project (for example,
	// `containeranalysis.notes.list`).
	//
	// The resource takes the format `projects/[PROJECT_ID]/notes/[NOTE_ID]` for
	// notes and `projects/[PROJECT_ID]/occurrences/[OCCURRENCE_ID]` for
	// occurrences.
	TestIamPermissions(context.Context, *v1.TestIamPermissionsRequest) (*v1.TestIamPermissionsResponse, error)
	// Gets the specified scan configuration.
	GetScanConfig(context.Context, *GetScanConfigRequest) (*ScanConfig, error)
	// Lists scan configurations for the specified project.
	ListScanConfigs(context.Context, *ListScanConfigsRequest) (*ListScanConfigsResponse, error)
	// Updates the specified scan configuration.
	UpdateScanConfig(context.Context, *UpdateScanConfigRequest) (*ScanConfig, error)
}

func RegisterContainerAnalysisV1Beta1Server(s *grpc.Server, srv ContainerAnalysisV1Beta1Server) {
	s.RegisterService(&_ContainerAnalysisV1Beta1_serviceDesc, srv)
}

func _ContainerAnalysisV1Beta1_SetIamPolicy_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.SetIamPolicyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ContainerAnalysisV1Beta1Server).SetIamPolicy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/SetIamPolicy",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ContainerAnalysisV1Beta1Server).SetIamPolicy(ctx, req.(*v1.SetIamPolicyRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ContainerAnalysisV1Beta1_GetIamPolicy_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.GetIamPolicyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ContainerAnalysisV1Beta1Server).GetIamPolicy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/GetIamPolicy",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ContainerAnalysisV1Beta1Server).GetIamPolicy(ctx, req.(*v1.GetIamPolicyRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ContainerAnalysisV1Beta1_TestIamPermissions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.TestIamPermissionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ContainerAnalysisV1Beta1Server).TestIamPermissions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/TestIamPermissions",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ContainerAnalysisV1Beta1Server).TestIamPermissions(ctx, req.(*v1.TestIamPermissionsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ContainerAnalysisV1Beta1_GetScanConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetScanConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ContainerAnalysisV1Beta1Server).GetScanConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/GetScanConfig",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ContainerAnalysisV1Beta1Server).GetScanConfig(ctx, req.(*GetScanConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ContainerAnalysisV1Beta1_ListScanConfigs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListScanConfigsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ContainerAnalysisV1Beta1Server).ListScanConfigs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/ListScanConfigs",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ContainerAnalysisV1Beta1Server).ListScanConfigs(ctx, req.(*ListScanConfigsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ContainerAnalysisV1Beta1_UpdateScanConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateScanConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ContainerAnalysisV1Beta1Server).UpdateScanConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1/UpdateScanConfig",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ContainerAnalysisV1Beta1Server).UpdateScanConfig(ctx, req.(*UpdateScanConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _ContainerAnalysisV1Beta1_serviceDesc = grpc.ServiceDesc{
	ServiceName: "google.devtools.containeranalysis.v1beta1.ContainerAnalysisV1Beta1",
	HandlerType: (*ContainerAnalysisV1Beta1Server)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SetIamPolicy",
			Handler:    _ContainerAnalysisV1Beta1_SetIamPolicy_Handler,
		},
		{
			MethodName: "GetIamPolicy",
			Handler:    _ContainerAnalysisV1Beta1_GetIamPolicy_Handler,
		},
		{
			MethodName: "TestIamPermissions",
			Handler:    _ContainerAnalysisV1Beta1_TestIamPermissions_Handler,
		},
		{
			MethodName: "GetScanConfig",
			Handler:    _ContainerAnalysisV1Beta1_GetScanConfig_Handler,
		},
		{
			MethodName: "ListScanConfigs",
			Handler:    _ContainerAnalysisV1Beta1_ListScanConfigs_Handler,
		},
		{
			MethodName: "UpdateScanConfig",
			Handler:    _ContainerAnalysisV1Beta1_UpdateScanConfig_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "google/devtools/containeranalysis/v1beta1/containeranalysis.proto",
}

func init() {
	proto.RegisterFile("google/devtools/containeranalysis/v1beta1/containeranalysis.proto", fileDescriptor_containeranalysis_a170acd3c74dfdfb)
}

var fileDescriptor_containeranalysis_a170acd3c74dfdfb = []byte{
	// 766 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xb4, 0x56, 0xcd, 0x4e, 0x1b, 0x49,
	0x10, 0x56, 0xf3, 0xb7, 0xd0, 0x06, 0xb1, 0x6a, 0xed, 0x82, 0x35, 0xfb, 0x67, 0x8d, 0x56, 0x2b,
	0xe3, 0xc3, 0xcc, 0x1a, 0xb4, 0x5a, 0x05, 0x44, 0x90, 0x21, 0x92, 0x15, 0x29, 0x07, 0x64, 0x08,
	0x8a, 0xb8, 0x58, 0xed, 0x71, 0x31, 0xea, 0xc4, 0xd3, 0x3d, 0x99, 0x6e, 0x5b, 0x40, 0x44, 0x0e,
	0x11, 0xb9, 0x24, 0xc7, 0x1c, 0x73, 0xcb, 0x5b, 0xe4, 0x11, 0x12, 0xe5, 0x16, 0x29, 0x4f, 0x90,
	0x77, 0xc8, 0x35, 0x9a, 0x9e, 0x1e, 0x7b, 0xb0, 0x0d, 0xd8, 0xa0, 0x9c, 0xa0, 0xeb, 0xfb, 0xaa,
	0xea, 0xfb, 0xaa, 0xdd, 0x65, 0xe3, 0x8a, 0x2f, 0x84, 0xdf, 0x02, 0xb7, 0x09, 0x1d, 0x25, 0x44,
	0x4b, 0xba, 0x9e, 0xe0, 0x8a, 0x32, 0x0e, 0x11, 0xe5, 0xb4, 0x75, 0x22, 0x99, 0x74, 0x3b, 0xe5,
	0x06, 0x28, 0x5a, 0x1e, 0x44, 0x9c, 0x30, 0x12, 0x4a, 0x90, 0x95, 0xa4, 0x84, 0x93, 0x96, 0x70,
	0x06, 0x89, 0xa6, 0x84, 0xf5, 0xbb, 0xe9, 0x46, 0x43, 0xe6, 0x52, 0xce, 0x85, 0xa2, 0x8a, 0x09,
	0x6e, 0x0a, 0x59, 0x7f, 0x1a, 0x94, 0xd1, 0xc0, 0xed, 0x94, 0xe3, 0x3f, 0xf5, 0x50, 0xb4, 0x98,
	0x77, 0x62, 0x70, 0xeb, 0x22, 0x7e, 0x01, 0xfb, 0xcb, 0x60, 0xfa, 0xd4, 0x68, 0x1f, 0xb9, 0x8a,
	0x05, 0x20, 0x15, 0x0d, 0xc2, 0x84, 0x60, 0x7f, 0x41, 0x18, 0xef, 0x79, 0x94, 0xef, 0x08, 0x7e,
	0xc4, 0x7c, 0x42, 0xf0, 0x14, 0xa7, 0x01, 0xe4, 0x51, 0x01, 0x15, 0xe7, 0x6a, 0xfa, 0x7f, 0x52,
	0xc0, 0xb9, 0x26, 0x48, 0x2f, 0x62, 0x61, 0xac, 0x2a, 0x3f, 0xa1, 0xa1, 0x6c, 0x88, 0xe4, 0xf1,
	0x4f, 0xc0, 0x69, 0xa3, 0x05, 0xcd, 0xfc, 0x64, 0x01, 0x15, 0x67, 0x6b, 0xe9, 0x91, 0x6c, 0xe0,
	0x9c, 0x17, 0x01, 0x55, 0x50, 0x8f, 0x1b, 0xe7, 0xa7, 0x0a, 0xa8, 0x98, 0x5b, 0xb5, 0x1c, 0x33,
	0x9a, 0x54, 0x95, 0xb3, 0x9f, 0xaa, 0xaa, 0xe1, 0x84, 0x1e, 0x07, 0xe2, 0xe4, 0x76, 0xd8, 0xec,
	0x26, 0x4f, 0x5f, 0x9f, 0x9c, 0xd0, 0xe3, 0x80, 0x5d, 0xc2, 0xbf, 0x54, 0x41, 0xf5, 0xac, 0xd5,
	0xe0, 0x69, 0x1b, 0xa4, 0x1a, 0xe6, 0xd0, 0x3e, 0x47, 0x78, 0xe9, 0x01, 0x93, 0x19, 0xb6, 0x4c,
	0xe9, 0x4b, 0x78, 0x26, 0xa4, 0x11, 0x70, 0x65, 0x12, 0xcc, 0x29, 0x8e, 0x1f, 0xb1, 0x96, 0x82,
	0xc8, 0xcc, 0xc3, 0x9c, 0xc8, 0x6f, 0x78, 0x2e, 0xa4, 0x3e, 0xd4, 0x25, 0x3b, 0x05, 0x3d, 0x8c,
	0xe9, 0xda, 0x6c, 0x1c, 0xd8, 0x63, 0xa7, 0x40, 0xfe, 0xc0, 0x58, 0x83, 0x4a, 0x3c, 0x01, 0xae,
	0x87, 0x31, 0x57, 0xd3, 0xf4, 0xfd, 0x38, 0x60, 0xbf, 0x45, 0x78, 0x79, 0x40, 0x86, 0x0c, 0x05,
	0x97, 0x40, 0x1e, 0xe1, 0x79, 0xe9, 0x51, 0x5e, 0xf7, 0x92, 0x78, 0x1e, 0x15, 0x26, 0x8b, 0xb9,
	0xd5, 0xff, 0x9c, 0x91, 0x3f, 0x64, 0x4e, 0x66, 0x14, 0x39, 0xd9, 0xeb, 0x40, 0xfe, 0xc1, 0x8b,
	0x1c, 0x8e, 0x55, 0x3d, 0xa3, 0x2c, 0xb1, 0xb4, 0x10, 0x87, 0x77, 0xbb, 0xea, 0x5e, 0x22, 0xbc,
	0xfc, 0x50, 0xcf, 0x77, 0xa4, 0xa1, 0x92, 0x03, 0x9c, 0xcb, 0x28, 0xd6, 0x35, 0x6f, 0x2c, 0x18,
	0xf7, 0x04, 0xaf, 0x9e, 0x63, 0x9c, 0xdf, 0x49, 0x93, 0x2a, 0x26, 0xe9, 0xa0, 0xbc, 0x1d, 0xe7,
	0x90, 0x0f, 0x08, 0xcf, 0xef, 0x81, 0xba, 0x4f, 0x83, 0x5d, 0xfd, 0x0c, 0x88, 0x9d, 0x36, 0x64,
	0x34, 0x70, 0x3a, 0x65, 0x27, 0x0b, 0x1a, 0xf5, 0xd6, 0xaf, 0x7d, 0x9c, 0x04, 0xb5, 0x9f, 0xbf,
	0xf8, 0xfc, 0xf5, 0xcd, 0xc4, 0xb1, 0xbd, 0xd6, 0x7d, 0xea, 0xcf, 0x22, 0x90, 0xa2, 0x1d, 0x79,
	0xb0, 0x19, 0x46, 0xe2, 0x31, 0x78, 0x4a, 0xba, 0x25, 0x97, 0x0b, 0x05, 0xd2, 0x2d, 0x9d, 0xad,
	0xcb, 0x4c, 0xe9, 0x75, 0x54, 0x3a, 0xbc, 0x6b, 0xdf, 0xb9, 0x3a, 0x53, 0x78, 0x5e, 0x3b, 0x8a,
	0x80, 0x7b, 0x43, 0xf3, 0xb5, 0x97, 0xea, 0x55, 0x5e, 0xaa, 0x3f, 0xce, 0x8b, 0x7f, 0x4b, 0x2f,
	0x7d, 0xf9, 0xe4, 0x1b, 0xc2, 0x64, 0x1f, 0xa4, 0x0e, 0x42, 0x14, 0x30, 0x29, 0xe3, 0x05, 0x47,
	0x8a, 0x7d, 0x6a, 0x07, 0x29, 0xa9, 0xaf, 0x95, 0x11, 0x98, 0xc9, 0x53, 0xb1, 0x5f, 0x23, 0x6d,
	0xf6, 0x1c, 0x5d, 0xa7, 0xb9, 0xeb, 0x56, 0x0d, 0x14, 0x8b, 0x3d, 0xdf, 0xb3, 0xb7, 0xc6, 0xf2,
	0x3c, 0xb4, 0x0a, 0x79, 0x8f, 0xf0, 0xc2, 0x85, 0x45, 0x44, 0xb6, 0xc6, 0x78, 0x03, 0xc3, 0x56,
	0x98, 0x75, 0xb3, 0x47, 0x64, 0xff, 0xab, 0xc7, 0x52, 0x22, 0xc5, 0x9e, 0xab, 0xf8, 0xa1, 0x66,
	0x1d, 0x65, 0xf6, 0x82, 0x5b, 0x3a, 0x23, 0x1f, 0x11, 0x5e, 0xec, 0x5b, 0x48, 0xa4, 0x32, 0x46,
	0xf3, 0xe1, 0x3b, 0xd5, 0xda, 0xbe, 0x4d, 0x09, 0x73, 0xc9, 0x43, 0xcc, 0x24, 0x9b, 0x39, 0x63,
	0xe7, 0x2c, 0xeb, 0x87, 0x7c, 0x42, 0xf8, 0xe7, 0xfe, 0xfd, 0x45, 0xc6, 0x91, 0x72, 0xc9, 0xf2,
	0xbb, 0xe9, 0x75, 0x6c, 0x6a, 0x07, 0xff, 0x5b, 0x23, 0x5f, 0xc7, 0x7a, 0x76, 0x9f, 0x6e, 0xbf,
	0x42, 0xf8, 0x6f, 0x4f, 0x04, 0x69, 0xef, 0x4b, 0x5b, 0xee, 0xa2, 0xc3, 0x43, 0xc3, 0xf1, 0x45,
	0x8b, 0x72, 0xdf, 0x11, 0x91, 0xef, 0xfa, 0xc0, 0xf5, 0xf7, 0xa7, 0x9b, 0x40, 0x34, 0x64, 0x72,
	0x84, 0xdf, 0x3a, 0x1b, 0x03, 0xc8, 0xbb, 0x89, 0xc9, 0xea, 0x4e, 0xa5, 0x31, 0xa3, 0x8b, 0xad,
	0x7d, 0x0f, 0x00, 0x00, 0xff, 0xff, 0x6c, 0xf6, 0x5c, 0x69, 0x37, 0x09, 0x00, 0x00,
}
