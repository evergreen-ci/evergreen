// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: ssh.proto

package sshforward

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import bytes "bytes"

import strings "strings"
import reflect "reflect"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

// BytesMessage contains a chunk of byte data
type BytesMessage struct {
	Data []byte `protobuf:"bytes,1,opt,name=data,proto3" json:"data,omitempty"`
}

func (m *BytesMessage) Reset()      { *m = BytesMessage{} }
func (*BytesMessage) ProtoMessage() {}
func (*BytesMessage) Descriptor() ([]byte, []int) {
	return fileDescriptor_ssh_13bd2c34c031d472, []int{0}
}
func (m *BytesMessage) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *BytesMessage) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_BytesMessage.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *BytesMessage) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BytesMessage.Merge(dst, src)
}
func (m *BytesMessage) XXX_Size() int {
	return m.Size()
}
func (m *BytesMessage) XXX_DiscardUnknown() {
	xxx_messageInfo_BytesMessage.DiscardUnknown(m)
}

var xxx_messageInfo_BytesMessage proto.InternalMessageInfo

func (m *BytesMessage) GetData() []byte {
	if m != nil {
		return m.Data
	}
	return nil
}

type CheckAgentRequest struct {
	ID string `protobuf:"bytes,1,opt,name=ID,proto3" json:"ID,omitempty"`
}

func (m *CheckAgentRequest) Reset()      { *m = CheckAgentRequest{} }
func (*CheckAgentRequest) ProtoMessage() {}
func (*CheckAgentRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_ssh_13bd2c34c031d472, []int{1}
}
func (m *CheckAgentRequest) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *CheckAgentRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_CheckAgentRequest.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *CheckAgentRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CheckAgentRequest.Merge(dst, src)
}
func (m *CheckAgentRequest) XXX_Size() int {
	return m.Size()
}
func (m *CheckAgentRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_CheckAgentRequest.DiscardUnknown(m)
}

var xxx_messageInfo_CheckAgentRequest proto.InternalMessageInfo

func (m *CheckAgentRequest) GetID() string {
	if m != nil {
		return m.ID
	}
	return ""
}

type CheckAgentResponse struct {
}

func (m *CheckAgentResponse) Reset()      { *m = CheckAgentResponse{} }
func (*CheckAgentResponse) ProtoMessage() {}
func (*CheckAgentResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_ssh_13bd2c34c031d472, []int{2}
}
func (m *CheckAgentResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *CheckAgentResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_CheckAgentResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *CheckAgentResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CheckAgentResponse.Merge(dst, src)
}
func (m *CheckAgentResponse) XXX_Size() int {
	return m.Size()
}
func (m *CheckAgentResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_CheckAgentResponse.DiscardUnknown(m)
}

var xxx_messageInfo_CheckAgentResponse proto.InternalMessageInfo

func init() {
	proto.RegisterType((*BytesMessage)(nil), "moby.sshforward.v1.BytesMessage")
	proto.RegisterType((*CheckAgentRequest)(nil), "moby.sshforward.v1.CheckAgentRequest")
	proto.RegisterType((*CheckAgentResponse)(nil), "moby.sshforward.v1.CheckAgentResponse")
}
func (this *BytesMessage) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*BytesMessage)
	if !ok {
		that2, ok := that.(BytesMessage)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	if !bytes.Equal(this.Data, that1.Data) {
		return false
	}
	return true
}
func (this *CheckAgentRequest) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*CheckAgentRequest)
	if !ok {
		that2, ok := that.(CheckAgentRequest)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	if this.ID != that1.ID {
		return false
	}
	return true
}
func (this *CheckAgentResponse) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*CheckAgentResponse)
	if !ok {
		that2, ok := that.(CheckAgentResponse)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	return true
}
func (this *BytesMessage) GoString() string {
	if this == nil {
		return "nil"
	}
	s := make([]string, 0, 5)
	s = append(s, "&sshforward.BytesMessage{")
	s = append(s, "Data: "+fmt.Sprintf("%#v", this.Data)+",\n")
	s = append(s, "}")
	return strings.Join(s, "")
}
func (this *CheckAgentRequest) GoString() string {
	if this == nil {
		return "nil"
	}
	s := make([]string, 0, 5)
	s = append(s, "&sshforward.CheckAgentRequest{")
	s = append(s, "ID: "+fmt.Sprintf("%#v", this.ID)+",\n")
	s = append(s, "}")
	return strings.Join(s, "")
}
func (this *CheckAgentResponse) GoString() string {
	if this == nil {
		return "nil"
	}
	s := make([]string, 0, 4)
	s = append(s, "&sshforward.CheckAgentResponse{")
	s = append(s, "}")
	return strings.Join(s, "")
}
func valueToGoStringSsh(v interface{}, typ string) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("func(v %v) *%v { return &v } ( %#v )", typ, typ, pv)
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// SSHClient is the client API for SSH service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type SSHClient interface {
	CheckAgent(ctx context.Context, in *CheckAgentRequest, opts ...grpc.CallOption) (*CheckAgentResponse, error)
	ForwardAgent(ctx context.Context, opts ...grpc.CallOption) (SSH_ForwardAgentClient, error)
}

type sSHClient struct {
	cc *grpc.ClientConn
}

func NewSSHClient(cc *grpc.ClientConn) SSHClient {
	return &sSHClient{cc}
}

func (c *sSHClient) CheckAgent(ctx context.Context, in *CheckAgentRequest, opts ...grpc.CallOption) (*CheckAgentResponse, error) {
	out := new(CheckAgentResponse)
	err := c.cc.Invoke(ctx, "/moby.sshforward.v1.SSH/CheckAgent", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sSHClient) ForwardAgent(ctx context.Context, opts ...grpc.CallOption) (SSH_ForwardAgentClient, error) {
	stream, err := c.cc.NewStream(ctx, &_SSH_serviceDesc.Streams[0], "/moby.sshforward.v1.SSH/ForwardAgent", opts...)
	if err != nil {
		return nil, err
	}
	x := &sSHForwardAgentClient{stream}
	return x, nil
}

type SSH_ForwardAgentClient interface {
	Send(*BytesMessage) error
	Recv() (*BytesMessage, error)
	grpc.ClientStream
}

type sSHForwardAgentClient struct {
	grpc.ClientStream
}

func (x *sSHForwardAgentClient) Send(m *BytesMessage) error {
	return x.ClientStream.SendMsg(m)
}

func (x *sSHForwardAgentClient) Recv() (*BytesMessage, error) {
	m := new(BytesMessage)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// SSHServer is the server API for SSH service.
type SSHServer interface {
	CheckAgent(context.Context, *CheckAgentRequest) (*CheckAgentResponse, error)
	ForwardAgent(SSH_ForwardAgentServer) error
}

func RegisterSSHServer(s *grpc.Server, srv SSHServer) {
	s.RegisterService(&_SSH_serviceDesc, srv)
}

func _SSH_CheckAgent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CheckAgentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SSHServer).CheckAgent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/moby.sshforward.v1.SSH/CheckAgent",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SSHServer).CheckAgent(ctx, req.(*CheckAgentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SSH_ForwardAgent_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(SSHServer).ForwardAgent(&sSHForwardAgentServer{stream})
}

type SSH_ForwardAgentServer interface {
	Send(*BytesMessage) error
	Recv() (*BytesMessage, error)
	grpc.ServerStream
}

type sSHForwardAgentServer struct {
	grpc.ServerStream
}

func (x *sSHForwardAgentServer) Send(m *BytesMessage) error {
	return x.ServerStream.SendMsg(m)
}

func (x *sSHForwardAgentServer) Recv() (*BytesMessage, error) {
	m := new(BytesMessage)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var _SSH_serviceDesc = grpc.ServiceDesc{
	ServiceName: "moby.sshforward.v1.SSH",
	HandlerType: (*SSHServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CheckAgent",
			Handler:    _SSH_CheckAgent_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ForwardAgent",
			Handler:       _SSH_ForwardAgent_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "ssh.proto",
}

func (m *BytesMessage) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *BytesMessage) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Data) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintSsh(dAtA, i, uint64(len(m.Data)))
		i += copy(dAtA[i:], m.Data)
	}
	return i, nil
}

func (m *CheckAgentRequest) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *CheckAgentRequest) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.ID) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintSsh(dAtA, i, uint64(len(m.ID)))
		i += copy(dAtA[i:], m.ID)
	}
	return i, nil
}

func (m *CheckAgentResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *CheckAgentResponse) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	return i, nil
}

func encodeVarintSsh(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *BytesMessage) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Data)
	if l > 0 {
		n += 1 + l + sovSsh(uint64(l))
	}
	return n
}

func (m *CheckAgentRequest) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.ID)
	if l > 0 {
		n += 1 + l + sovSsh(uint64(l))
	}
	return n
}

func (m *CheckAgentResponse) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	return n
}

func sovSsh(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozSsh(x uint64) (n int) {
	return sovSsh(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (this *BytesMessage) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&BytesMessage{`,
		`Data:` + fmt.Sprintf("%v", this.Data) + `,`,
		`}`,
	}, "")
	return s
}
func (this *CheckAgentRequest) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&CheckAgentRequest{`,
		`ID:` + fmt.Sprintf("%v", this.ID) + `,`,
		`}`,
	}, "")
	return s
}
func (this *CheckAgentResponse) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&CheckAgentResponse{`,
		`}`,
	}, "")
	return s
}
func valueToStringSsh(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("*%v", pv)
}
func (m *BytesMessage) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowSsh
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: BytesMessage: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: BytesMessage: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Data", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSsh
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthSsh
			}
			postIndex := iNdEx + byteLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Data = append(m.Data[:0], dAtA[iNdEx:postIndex]...)
			if m.Data == nil {
				m.Data = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipSsh(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthSsh
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *CheckAgentRequest) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowSsh
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: CheckAgentRequest: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: CheckAgentRequest: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field ID", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSsh
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSsh
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.ID = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipSsh(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthSsh
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *CheckAgentResponse) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowSsh
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: CheckAgentResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: CheckAgentResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipSsh(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthSsh
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipSsh(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowSsh
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowSsh
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowSsh
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthSsh
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowSsh
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipSsh(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthSsh = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowSsh   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("ssh.proto", fileDescriptor_ssh_13bd2c34c031d472) }

var fileDescriptor_ssh_13bd2c34c031d472 = []byte{
	// 252 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0x2c, 0x2e, 0xce, 0xd0,
	0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0x12, 0xca, 0xcd, 0x4f, 0xaa, 0xd4, 0x2b, 0x2e, 0xce, 0x48,
	0xcb, 0x2f, 0x2a, 0x4f, 0x2c, 0x4a, 0xd1, 0x2b, 0x33, 0x54, 0x52, 0xe2, 0xe2, 0x71, 0xaa, 0x2c,
	0x49, 0x2d, 0xf6, 0x4d, 0x2d, 0x2e, 0x4e, 0x4c, 0x4f, 0x15, 0x12, 0xe2, 0x62, 0x49, 0x49, 0x2c,
	0x49, 0x94, 0x60, 0x54, 0x60, 0xd4, 0xe0, 0x09, 0x02, 0xb3, 0x95, 0x94, 0xb9, 0x04, 0x9d, 0x33,
	0x52, 0x93, 0xb3, 0x1d, 0xd3, 0x53, 0xf3, 0x4a, 0x82, 0x52, 0x0b, 0x4b, 0x53, 0x8b, 0x4b, 0x84,
	0xf8, 0xb8, 0x98, 0x3c, 0x5d, 0xc0, 0xca, 0x38, 0x83, 0x98, 0x3c, 0x5d, 0x94, 0x44, 0xb8, 0x84,
	0x90, 0x15, 0x15, 0x17, 0xe4, 0xe7, 0x15, 0xa7, 0x1a, 0xed, 0x62, 0xe4, 0x62, 0x0e, 0x0e, 0xf6,
	0x10, 0x8a, 0xe6, 0xe2, 0x42, 0xc8, 0x0a, 0xa9, 0xea, 0x61, 0xba, 0x44, 0x0f, 0xc3, 0x0a, 0x29,
	0x35, 0x42, 0xca, 0x20, 0x96, 0x08, 0x85, 0x71, 0xf1, 0xb8, 0x41, 0x14, 0x40, 0x8c, 0x57, 0xc0,
	0xa6, 0x0f, 0xd9, 0x97, 0x52, 0x04, 0x55, 0x68, 0x30, 0x1a, 0x30, 0x3a, 0x39, 0x5c, 0x78, 0x28,
	0xc7, 0x70, 0xe3, 0xa1, 0x1c, 0xc3, 0x87, 0x87, 0x72, 0x8c, 0x0d, 0x8f, 0xe4, 0x18, 0x57, 0x3c,
	0x92, 0x63, 0x3c, 0xf1, 0x48, 0x8e, 0xf1, 0xc2, 0x23, 0x39, 0xc6, 0x07, 0x8f, 0xe4, 0x18, 0x5f,
	0x3c, 0x92, 0x63, 0xf8, 0xf0, 0x48, 0x8e, 0x71, 0xc2, 0x63, 0x39, 0x86, 0x0b, 0x8f, 0xe5, 0x18,
	0x6e, 0x3c, 0x96, 0x63, 0x88, 0xe2, 0x42, 0x98, 0x9a, 0xc4, 0x06, 0x0e, 0x78, 0x63, 0x40, 0x00,
	0x00, 0x00, 0xff, 0xff, 0x6c, 0xe6, 0x6d, 0xb7, 0x85, 0x01, 0x00, 0x00,
}
