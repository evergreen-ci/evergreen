// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/devtools/clouddebugger/v2/controller.proto

package clouddebugger // import "google.golang.org/genproto/googleapis/devtools/clouddebugger/v2"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import _ "github.com/golang/protobuf/ptypes/empty"
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

// Request to register a debuggee.
type RegisterDebuggeeRequest struct {
	// Debuggee information to register.
	// The fields `project`, `uniquifier`, `description` and `agent_version`
	// of the debuggee must be set.
	Debuggee             *Debuggee `protobuf:"bytes,1,opt,name=debuggee,proto3" json:"debuggee,omitempty"`
	XXX_NoUnkeyedLiteral struct{}  `json:"-"`
	XXX_unrecognized     []byte    `json:"-"`
	XXX_sizecache        int32     `json:"-"`
}

func (m *RegisterDebuggeeRequest) Reset()         { *m = RegisterDebuggeeRequest{} }
func (m *RegisterDebuggeeRequest) String() string { return proto.CompactTextString(m) }
func (*RegisterDebuggeeRequest) ProtoMessage()    {}
func (*RegisterDebuggeeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_controller_3156ecf89afb2d41, []int{0}
}
func (m *RegisterDebuggeeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RegisterDebuggeeRequest.Unmarshal(m, b)
}
func (m *RegisterDebuggeeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RegisterDebuggeeRequest.Marshal(b, m, deterministic)
}
func (dst *RegisterDebuggeeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RegisterDebuggeeRequest.Merge(dst, src)
}
func (m *RegisterDebuggeeRequest) XXX_Size() int {
	return xxx_messageInfo_RegisterDebuggeeRequest.Size(m)
}
func (m *RegisterDebuggeeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_RegisterDebuggeeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_RegisterDebuggeeRequest proto.InternalMessageInfo

func (m *RegisterDebuggeeRequest) GetDebuggee() *Debuggee {
	if m != nil {
		return m.Debuggee
	}
	return nil
}

// Response for registering a debuggee.
type RegisterDebuggeeResponse struct {
	// Debuggee resource.
	// The field `id` is guaranteed to be set (in addition to the echoed fields).
	// If the field `is_disabled` is set to `true`, the agent should disable
	// itself by removing all breakpoints and detaching from the application.
	// It should however continue to poll `RegisterDebuggee` until reenabled.
	Debuggee             *Debuggee `protobuf:"bytes,1,opt,name=debuggee,proto3" json:"debuggee,omitempty"`
	XXX_NoUnkeyedLiteral struct{}  `json:"-"`
	XXX_unrecognized     []byte    `json:"-"`
	XXX_sizecache        int32     `json:"-"`
}

func (m *RegisterDebuggeeResponse) Reset()         { *m = RegisterDebuggeeResponse{} }
func (m *RegisterDebuggeeResponse) String() string { return proto.CompactTextString(m) }
func (*RegisterDebuggeeResponse) ProtoMessage()    {}
func (*RegisterDebuggeeResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_controller_3156ecf89afb2d41, []int{1}
}
func (m *RegisterDebuggeeResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RegisterDebuggeeResponse.Unmarshal(m, b)
}
func (m *RegisterDebuggeeResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RegisterDebuggeeResponse.Marshal(b, m, deterministic)
}
func (dst *RegisterDebuggeeResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RegisterDebuggeeResponse.Merge(dst, src)
}
func (m *RegisterDebuggeeResponse) XXX_Size() int {
	return xxx_messageInfo_RegisterDebuggeeResponse.Size(m)
}
func (m *RegisterDebuggeeResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_RegisterDebuggeeResponse.DiscardUnknown(m)
}

var xxx_messageInfo_RegisterDebuggeeResponse proto.InternalMessageInfo

func (m *RegisterDebuggeeResponse) GetDebuggee() *Debuggee {
	if m != nil {
		return m.Debuggee
	}
	return nil
}

// Request to list active breakpoints.
type ListActiveBreakpointsRequest struct {
	// Identifies the debuggee.
	DebuggeeId string `protobuf:"bytes,1,opt,name=debuggee_id,json=debuggeeId,proto3" json:"debuggee_id,omitempty"`
	// A token that, if specified, blocks the method call until the list
	// of active breakpoints has changed, or a server-selected timeout has
	// expired. The value should be set from the `next_wait_token` field in
	// the last response. The initial value should be set to `"init"`.
	WaitToken string `protobuf:"bytes,2,opt,name=wait_token,json=waitToken,proto3" json:"wait_token,omitempty"`
	// If set to `true` (recommended), returns `google.rpc.Code.OK` status and
	// sets the `wait_expired` response field to `true` when the server-selected
	// timeout has expired.
	//
	// If set to `false` (deprecated), returns `google.rpc.Code.ABORTED` status
	// when the server-selected timeout has expired.
	SuccessOnTimeout     bool     `protobuf:"varint,3,opt,name=success_on_timeout,json=successOnTimeout,proto3" json:"success_on_timeout,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListActiveBreakpointsRequest) Reset()         { *m = ListActiveBreakpointsRequest{} }
func (m *ListActiveBreakpointsRequest) String() string { return proto.CompactTextString(m) }
func (*ListActiveBreakpointsRequest) ProtoMessage()    {}
func (*ListActiveBreakpointsRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_controller_3156ecf89afb2d41, []int{2}
}
func (m *ListActiveBreakpointsRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListActiveBreakpointsRequest.Unmarshal(m, b)
}
func (m *ListActiveBreakpointsRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListActiveBreakpointsRequest.Marshal(b, m, deterministic)
}
func (dst *ListActiveBreakpointsRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListActiveBreakpointsRequest.Merge(dst, src)
}
func (m *ListActiveBreakpointsRequest) XXX_Size() int {
	return xxx_messageInfo_ListActiveBreakpointsRequest.Size(m)
}
func (m *ListActiveBreakpointsRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ListActiveBreakpointsRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ListActiveBreakpointsRequest proto.InternalMessageInfo

func (m *ListActiveBreakpointsRequest) GetDebuggeeId() string {
	if m != nil {
		return m.DebuggeeId
	}
	return ""
}

func (m *ListActiveBreakpointsRequest) GetWaitToken() string {
	if m != nil {
		return m.WaitToken
	}
	return ""
}

func (m *ListActiveBreakpointsRequest) GetSuccessOnTimeout() bool {
	if m != nil {
		return m.SuccessOnTimeout
	}
	return false
}

// Response for listing active breakpoints.
type ListActiveBreakpointsResponse struct {
	// List of all active breakpoints.
	// The fields `id` and `location` are guaranteed to be set on each breakpoint.
	Breakpoints []*Breakpoint `protobuf:"bytes,1,rep,name=breakpoints,proto3" json:"breakpoints,omitempty"`
	// A token that can be used in the next method call to block until
	// the list of breakpoints changes.
	NextWaitToken string `protobuf:"bytes,2,opt,name=next_wait_token,json=nextWaitToken,proto3" json:"next_wait_token,omitempty"`
	// If set to `true`, indicates that there is no change to the
	// list of active breakpoints and the server-selected timeout has expired.
	// The `breakpoints` field would be empty and should be ignored.
	WaitExpired          bool     `protobuf:"varint,3,opt,name=wait_expired,json=waitExpired,proto3" json:"wait_expired,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListActiveBreakpointsResponse) Reset()         { *m = ListActiveBreakpointsResponse{} }
func (m *ListActiveBreakpointsResponse) String() string { return proto.CompactTextString(m) }
func (*ListActiveBreakpointsResponse) ProtoMessage()    {}
func (*ListActiveBreakpointsResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_controller_3156ecf89afb2d41, []int{3}
}
func (m *ListActiveBreakpointsResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListActiveBreakpointsResponse.Unmarshal(m, b)
}
func (m *ListActiveBreakpointsResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListActiveBreakpointsResponse.Marshal(b, m, deterministic)
}
func (dst *ListActiveBreakpointsResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListActiveBreakpointsResponse.Merge(dst, src)
}
func (m *ListActiveBreakpointsResponse) XXX_Size() int {
	return xxx_messageInfo_ListActiveBreakpointsResponse.Size(m)
}
func (m *ListActiveBreakpointsResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ListActiveBreakpointsResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ListActiveBreakpointsResponse proto.InternalMessageInfo

func (m *ListActiveBreakpointsResponse) GetBreakpoints() []*Breakpoint {
	if m != nil {
		return m.Breakpoints
	}
	return nil
}

func (m *ListActiveBreakpointsResponse) GetNextWaitToken() string {
	if m != nil {
		return m.NextWaitToken
	}
	return ""
}

func (m *ListActiveBreakpointsResponse) GetWaitExpired() bool {
	if m != nil {
		return m.WaitExpired
	}
	return false
}

// Request to update an active breakpoint.
type UpdateActiveBreakpointRequest struct {
	// Identifies the debuggee being debugged.
	DebuggeeId string `protobuf:"bytes,1,opt,name=debuggee_id,json=debuggeeId,proto3" json:"debuggee_id,omitempty"`
	// Updated breakpoint information.
	// The field `id` must be set.
	// The agent must echo all Breakpoint specification fields in the update.
	Breakpoint           *Breakpoint `protobuf:"bytes,2,opt,name=breakpoint,proto3" json:"breakpoint,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *UpdateActiveBreakpointRequest) Reset()         { *m = UpdateActiveBreakpointRequest{} }
func (m *UpdateActiveBreakpointRequest) String() string { return proto.CompactTextString(m) }
func (*UpdateActiveBreakpointRequest) ProtoMessage()    {}
func (*UpdateActiveBreakpointRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_controller_3156ecf89afb2d41, []int{4}
}
func (m *UpdateActiveBreakpointRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UpdateActiveBreakpointRequest.Unmarshal(m, b)
}
func (m *UpdateActiveBreakpointRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UpdateActiveBreakpointRequest.Marshal(b, m, deterministic)
}
func (dst *UpdateActiveBreakpointRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateActiveBreakpointRequest.Merge(dst, src)
}
func (m *UpdateActiveBreakpointRequest) XXX_Size() int {
	return xxx_messageInfo_UpdateActiveBreakpointRequest.Size(m)
}
func (m *UpdateActiveBreakpointRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateActiveBreakpointRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateActiveBreakpointRequest proto.InternalMessageInfo

func (m *UpdateActiveBreakpointRequest) GetDebuggeeId() string {
	if m != nil {
		return m.DebuggeeId
	}
	return ""
}

func (m *UpdateActiveBreakpointRequest) GetBreakpoint() *Breakpoint {
	if m != nil {
		return m.Breakpoint
	}
	return nil
}

// Response for updating an active breakpoint.
// The message is defined to allow future extensions.
type UpdateActiveBreakpointResponse struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *UpdateActiveBreakpointResponse) Reset()         { *m = UpdateActiveBreakpointResponse{} }
func (m *UpdateActiveBreakpointResponse) String() string { return proto.CompactTextString(m) }
func (*UpdateActiveBreakpointResponse) ProtoMessage()    {}
func (*UpdateActiveBreakpointResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_controller_3156ecf89afb2d41, []int{5}
}
func (m *UpdateActiveBreakpointResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UpdateActiveBreakpointResponse.Unmarshal(m, b)
}
func (m *UpdateActiveBreakpointResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UpdateActiveBreakpointResponse.Marshal(b, m, deterministic)
}
func (dst *UpdateActiveBreakpointResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateActiveBreakpointResponse.Merge(dst, src)
}
func (m *UpdateActiveBreakpointResponse) XXX_Size() int {
	return xxx_messageInfo_UpdateActiveBreakpointResponse.Size(m)
}
func (m *UpdateActiveBreakpointResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateActiveBreakpointResponse.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateActiveBreakpointResponse proto.InternalMessageInfo

func init() {
	proto.RegisterType((*RegisterDebuggeeRequest)(nil), "google.devtools.clouddebugger.v2.RegisterDebuggeeRequest")
	proto.RegisterType((*RegisterDebuggeeResponse)(nil), "google.devtools.clouddebugger.v2.RegisterDebuggeeResponse")
	proto.RegisterType((*ListActiveBreakpointsRequest)(nil), "google.devtools.clouddebugger.v2.ListActiveBreakpointsRequest")
	proto.RegisterType((*ListActiveBreakpointsResponse)(nil), "google.devtools.clouddebugger.v2.ListActiveBreakpointsResponse")
	proto.RegisterType((*UpdateActiveBreakpointRequest)(nil), "google.devtools.clouddebugger.v2.UpdateActiveBreakpointRequest")
	proto.RegisterType((*UpdateActiveBreakpointResponse)(nil), "google.devtools.clouddebugger.v2.UpdateActiveBreakpointResponse")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// Controller2Client is the client API for Controller2 service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type Controller2Client interface {
	// Registers the debuggee with the controller service.
	//
	// All agents attached to the same application must call this method with
	// exactly the same request content to get back the same stable `debuggee_id`.
	// Agents should call this method again whenever `google.rpc.Code.NOT_FOUND`
	// is returned from any controller method.
	//
	// This protocol allows the controller service to disable debuggees, recover
	// from data loss, or change the `debuggee_id` format. Agents must handle
	// `debuggee_id` value changing upon re-registration.
	RegisterDebuggee(ctx context.Context, in *RegisterDebuggeeRequest, opts ...grpc.CallOption) (*RegisterDebuggeeResponse, error)
	// Returns the list of all active breakpoints for the debuggee.
	//
	// The breakpoint specification (`location`, `condition`, and `expressions`
	// fields) is semantically immutable, although the field values may
	// change. For example, an agent may update the location line number
	// to reflect the actual line where the breakpoint was set, but this
	// doesn't change the breakpoint semantics.
	//
	// This means that an agent does not need to check if a breakpoint has changed
	// when it encounters the same breakpoint on a successive call.
	// Moreover, an agent should remember the breakpoints that are completed
	// until the controller removes them from the active list to avoid
	// setting those breakpoints again.
	ListActiveBreakpoints(ctx context.Context, in *ListActiveBreakpointsRequest, opts ...grpc.CallOption) (*ListActiveBreakpointsResponse, error)
	// Updates the breakpoint state or mutable fields.
	// The entire Breakpoint message must be sent back to the controller service.
	//
	// Updates to active breakpoint fields are only allowed if the new value
	// does not change the breakpoint specification. Updates to the `location`,
	// `condition` and `expressions` fields should not alter the breakpoint
	// semantics. These may only make changes such as canonicalizing a value
	// or snapping the location to the correct line of code.
	UpdateActiveBreakpoint(ctx context.Context, in *UpdateActiveBreakpointRequest, opts ...grpc.CallOption) (*UpdateActiveBreakpointResponse, error)
}

type controller2Client struct {
	cc *grpc.ClientConn
}

func NewController2Client(cc *grpc.ClientConn) Controller2Client {
	return &controller2Client{cc}
}

func (c *controller2Client) RegisterDebuggee(ctx context.Context, in *RegisterDebuggeeRequest, opts ...grpc.CallOption) (*RegisterDebuggeeResponse, error) {
	out := new(RegisterDebuggeeResponse)
	err := c.cc.Invoke(ctx, "/google.devtools.clouddebugger.v2.Controller2/RegisterDebuggee", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *controller2Client) ListActiveBreakpoints(ctx context.Context, in *ListActiveBreakpointsRequest, opts ...grpc.CallOption) (*ListActiveBreakpointsResponse, error) {
	out := new(ListActiveBreakpointsResponse)
	err := c.cc.Invoke(ctx, "/google.devtools.clouddebugger.v2.Controller2/ListActiveBreakpoints", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *controller2Client) UpdateActiveBreakpoint(ctx context.Context, in *UpdateActiveBreakpointRequest, opts ...grpc.CallOption) (*UpdateActiveBreakpointResponse, error) {
	out := new(UpdateActiveBreakpointResponse)
	err := c.cc.Invoke(ctx, "/google.devtools.clouddebugger.v2.Controller2/UpdateActiveBreakpoint", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Controller2Server is the server API for Controller2 service.
type Controller2Server interface {
	// Registers the debuggee with the controller service.
	//
	// All agents attached to the same application must call this method with
	// exactly the same request content to get back the same stable `debuggee_id`.
	// Agents should call this method again whenever `google.rpc.Code.NOT_FOUND`
	// is returned from any controller method.
	//
	// This protocol allows the controller service to disable debuggees, recover
	// from data loss, or change the `debuggee_id` format. Agents must handle
	// `debuggee_id` value changing upon re-registration.
	RegisterDebuggee(context.Context, *RegisterDebuggeeRequest) (*RegisterDebuggeeResponse, error)
	// Returns the list of all active breakpoints for the debuggee.
	//
	// The breakpoint specification (`location`, `condition`, and `expressions`
	// fields) is semantically immutable, although the field values may
	// change. For example, an agent may update the location line number
	// to reflect the actual line where the breakpoint was set, but this
	// doesn't change the breakpoint semantics.
	//
	// This means that an agent does not need to check if a breakpoint has changed
	// when it encounters the same breakpoint on a successive call.
	// Moreover, an agent should remember the breakpoints that are completed
	// until the controller removes them from the active list to avoid
	// setting those breakpoints again.
	ListActiveBreakpoints(context.Context, *ListActiveBreakpointsRequest) (*ListActiveBreakpointsResponse, error)
	// Updates the breakpoint state or mutable fields.
	// The entire Breakpoint message must be sent back to the controller service.
	//
	// Updates to active breakpoint fields are only allowed if the new value
	// does not change the breakpoint specification. Updates to the `location`,
	// `condition` and `expressions` fields should not alter the breakpoint
	// semantics. These may only make changes such as canonicalizing a value
	// or snapping the location to the correct line of code.
	UpdateActiveBreakpoint(context.Context, *UpdateActiveBreakpointRequest) (*UpdateActiveBreakpointResponse, error)
}

func RegisterController2Server(s *grpc.Server, srv Controller2Server) {
	s.RegisterService(&_Controller2_serviceDesc, srv)
}

func _Controller2_RegisterDebuggee_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterDebuggeeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(Controller2Server).RegisterDebuggee(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.clouddebugger.v2.Controller2/RegisterDebuggee",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(Controller2Server).RegisterDebuggee(ctx, req.(*RegisterDebuggeeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Controller2_ListActiveBreakpoints_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListActiveBreakpointsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(Controller2Server).ListActiveBreakpoints(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.clouddebugger.v2.Controller2/ListActiveBreakpoints",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(Controller2Server).ListActiveBreakpoints(ctx, req.(*ListActiveBreakpointsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Controller2_UpdateActiveBreakpoint_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateActiveBreakpointRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(Controller2Server).UpdateActiveBreakpoint(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.devtools.clouddebugger.v2.Controller2/UpdateActiveBreakpoint",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(Controller2Server).UpdateActiveBreakpoint(ctx, req.(*UpdateActiveBreakpointRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Controller2_serviceDesc = grpc.ServiceDesc{
	ServiceName: "google.devtools.clouddebugger.v2.Controller2",
	HandlerType: (*Controller2Server)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "RegisterDebuggee",
			Handler:    _Controller2_RegisterDebuggee_Handler,
		},
		{
			MethodName: "ListActiveBreakpoints",
			Handler:    _Controller2_ListActiveBreakpoints_Handler,
		},
		{
			MethodName: "UpdateActiveBreakpoint",
			Handler:    _Controller2_UpdateActiveBreakpoint_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "google/devtools/clouddebugger/v2/controller.proto",
}

func init() {
	proto.RegisterFile("google/devtools/clouddebugger/v2/controller.proto", fileDescriptor_controller_3156ecf89afb2d41)
}

var fileDescriptor_controller_3156ecf89afb2d41 = []byte{
	// 602 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xac, 0x54, 0xdd, 0x6a, 0xd4, 0x40,
	0x14, 0x66, 0x5a, 0x94, 0x76, 0xa2, 0xb4, 0x0c, 0xa8, 0x21, 0xb6, 0xba, 0x0d, 0x52, 0x96, 0x75,
	0xc9, 0x60, 0xf4, 0xc6, 0x15, 0xfc, 0xd9, 0xaa, 0x45, 0x68, 0xb5, 0x2c, 0xb5, 0x82, 0x2c, 0x2c,
	0xd9, 0xe4, 0x18, 0x86, 0x66, 0x67, 0x62, 0x66, 0xb2, 0x56, 0x4a, 0x6f, 0xbc, 0x55, 0xbc, 0xf1,
	0x2d, 0x7c, 0x01, 0xc1, 0x0b, 0x1f, 0xc0, 0x5b, 0x7d, 0x04, 0xaf, 0x7c, 0x0a, 0xc9, 0xdf, 0xfe,
	0xb4, 0xdd, 0xa6, 0x5d, 0xbc, 0xcc, 0x77, 0xe6, 0xfb, 0xce, 0xf7, 0x4d, 0xce, 0x1c, 0x7c, 0xcb,
	0x17, 0xc2, 0x0f, 0x80, 0x7a, 0xd0, 0x57, 0x42, 0x04, 0x92, 0xba, 0x81, 0x88, 0x3d, 0x0f, 0xba,
	0xb1, 0xef, 0x43, 0x44, 0xfb, 0x36, 0x75, 0x05, 0x57, 0x91, 0x08, 0x02, 0x88, 0xac, 0x30, 0x12,
	0x4a, 0x90, 0x4a, 0x46, 0xb1, 0x0a, 0x8a, 0x35, 0x46, 0xb1, 0xfa, 0xb6, 0xb1, 0x94, 0x8b, 0x3a,
	0x21, 0xa3, 0x0e, 0xe7, 0x42, 0x39, 0x8a, 0x09, 0x2e, 0x33, 0xbe, 0x71, 0xb3, 0xb4, 0xa5, 0xe7,
	0x28, 0x27, 0x3f, 0x7c, 0x35, 0x3f, 0x9c, 0x7e, 0x75, 0xe3, 0x37, 0x14, 0x7a, 0xa1, 0x7a, 0x9f,
	0x15, 0x4d, 0x07, 0x5f, 0x69, 0x81, 0xcf, 0xa4, 0x82, 0xe8, 0x71, 0x46, 0x87, 0x16, 0xbc, 0x8d,
	0x41, 0x2a, 0xf2, 0x14, 0xcf, 0xe5, 0x8a, 0xa0, 0xa3, 0x0a, 0xaa, 0x6a, 0x76, 0xcd, 0x2a, 0xf3,
	0x6d, 0x0d, 0x44, 0x06, 0x5c, 0xb3, 0x8b, 0xf5, 0xa3, 0x2d, 0x64, 0x28, 0xb8, 0x84, 0xff, 0xd6,
	0xe3, 0x13, 0xc2, 0x4b, 0x1b, 0x4c, 0xaa, 0x47, 0xae, 0x62, 0x7d, 0x68, 0x46, 0xe0, 0xec, 0x86,
	0x82, 0x71, 0x25, 0x8b, 0x30, 0xd7, 0xb1, 0x56, 0x1c, 0xee, 0x30, 0x2f, 0xed, 0x35, 0xdf, 0xc2,
	0x05, 0xf4, 0xcc, 0x23, 0xcb, 0x18, 0xbf, 0x73, 0x98, 0xea, 0x28, 0xb1, 0x0b, 0x5c, 0x9f, 0x49,
	0xeb, 0xf3, 0x09, 0xb2, 0x9d, 0x00, 0xa4, 0x8e, 0x89, 0x8c, 0x5d, 0x17, 0xa4, 0xec, 0x08, 0xde,
	0x51, 0xac, 0x07, 0x22, 0x56, 0xfa, 0x6c, 0x05, 0x55, 0xe7, 0x5a, 0x8b, 0x79, 0xe5, 0x05, 0xdf,
	0xce, 0x70, 0xf3, 0x3b, 0xc2, 0xcb, 0x13, 0xec, 0xe4, 0xc1, 0x9f, 0x63, 0xad, 0x3b, 0x84, 0x75,
	0x54, 0x99, 0xad, 0x6a, 0x76, 0xbd, 0x3c, 0xfb, 0x50, 0xab, 0x35, 0x2a, 0x40, 0x56, 0xf1, 0x02,
	0x87, 0x3d, 0xd5, 0x39, 0x92, 0xe1, 0x62, 0x02, 0xbf, 0x1a, 0xe4, 0x58, 0xc1, 0x17, 0xd2, 0x23,
	0xb0, 0x17, 0xb2, 0x08, 0xbc, 0x3c, 0x81, 0x96, 0x60, 0x4f, 0x32, 0xc8, 0xfc, 0x8c, 0xf0, 0xf2,
	0xcb, 0xd0, 0x73, 0x14, 0x1c, 0xb6, 0x7f, 0xea, 0xcb, 0xdc, 0xc0, 0x78, 0x68, 0x2e, 0x35, 0x72,
	0xd6, 0x70, 0x23, 0x7c, 0xb3, 0x82, 0xaf, 0x4d, 0xf2, 0x93, 0xdd, 0xa6, 0xfd, 0xf1, 0x1c, 0xd6,
	0xd6, 0x06, 0x8f, 0xcc, 0x26, 0xdf, 0x10, 0x5e, 0x3c, 0x3c, 0x73, 0xe4, 0x6e, 0xb9, 0x81, 0x09,
	0x4f, 0xc1, 0x68, 0x4c, 0x43, 0xcd, 0xbc, 0x99, 0xf5, 0x0f, 0xbf, 0xfe, 0x7c, 0x99, 0x59, 0x35,
	0x57, 0xc6, 0x37, 0x01, 0x2d, 0xae, 0x4b, 0xd2, 0x28, 0xa7, 0x36, 0x50, 0x8d, 0xfc, 0x46, 0xf8,
	0xd2, 0xb1, 0x93, 0x43, 0xee, 0x97, 0x7b, 0x38, 0xe9, 0x05, 0x18, 0x0f, 0xa6, 0xe6, 0xe7, 0x41,
	0x1a, 0x69, 0x90, 0x3b, 0xc4, 0x9e, 0x18, 0x64, 0x7f, 0x64, 0x2a, 0x0e, 0xe8, 0xe8, 0x78, 0xfe,
	0x45, 0xf8, 0xf2, 0xf1, 0xff, 0x90, 0x9c, 0xc2, 0xd7, 0x89, 0xd3, 0x68, 0x3c, 0x9c, 0x5e, 0x20,
	0x4f, 0xb6, 0x99, 0x26, 0x5b, 0x37, 0x9a, 0x67, 0x4f, 0x46, 0xf7, 0x87, 0x1f, 0x16, 0xf3, 0x0e,
	0x1a, 0xa8, 0xd6, 0xfc, 0x81, 0xf0, 0x0d, 0x57, 0xf4, 0x4a, 0x6d, 0x35, 0x17, 0x86, 0x33, 0xbb,
	0x95, 0x6c, 0xe3, 0x2d, 0xf4, 0x7a, 0x33, 0x27, 0xf9, 0x22, 0x70, 0xb8, 0x6f, 0x89, 0xc8, 0xa7,
	0x3e, 0xf0, 0x74, 0x57, 0xd3, 0xac, 0xe4, 0x84, 0x4c, 0x4e, 0x5e, 0xfc, 0xf7, 0xc6, 0x80, 0xaf,
	0x33, 0xfa, 0x7a, 0xa6, 0xb7, 0x96, 0xc0, 0xc5, 0xe6, 0x8c, 0xac, 0x1d, 0xfb, 0x67, 0x51, 0x6a,
	0xa7, 0xa5, 0x76, 0x51, 0x6a, 0xef, 0xd8, 0xdd, 0xf3, 0x69, 0xbf, 0xdb, 0xff, 0x02, 0x00, 0x00,
	0xff, 0xff, 0x54, 0xe1, 0x5c, 0x2a, 0xda, 0x06, 0x00, 0x00,
}
