// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/api/expr/v1beta1/decl.proto

package expr // import "google.golang.org/genproto/googleapis/api/expr/v1beta1"

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

// A declaration.
type Decl struct {
	// The id of the declaration.
	Id int32 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	// The name of the declaration.
	Name string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	// The documentation string for the declaration.
	Doc string `protobuf:"bytes,3,opt,name=doc,proto3" json:"doc,omitempty"`
	// The kind of declaration.
	//
	// Types that are valid to be assigned to Kind:
	//	*Decl_Ident
	//	*Decl_Function
	Kind                 isDecl_Kind `protobuf_oneof:"kind"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *Decl) Reset()         { *m = Decl{} }
func (m *Decl) String() string { return proto.CompactTextString(m) }
func (*Decl) ProtoMessage()    {}
func (*Decl) Descriptor() ([]byte, []int) {
	return fileDescriptor_decl_6647d3ad822811d0, []int{0}
}
func (m *Decl) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Decl.Unmarshal(m, b)
}
func (m *Decl) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Decl.Marshal(b, m, deterministic)
}
func (dst *Decl) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Decl.Merge(dst, src)
}
func (m *Decl) XXX_Size() int {
	return xxx_messageInfo_Decl.Size(m)
}
func (m *Decl) XXX_DiscardUnknown() {
	xxx_messageInfo_Decl.DiscardUnknown(m)
}

var xxx_messageInfo_Decl proto.InternalMessageInfo

func (m *Decl) GetId() int32 {
	if m != nil {
		return m.Id
	}
	return 0
}

func (m *Decl) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Decl) GetDoc() string {
	if m != nil {
		return m.Doc
	}
	return ""
}

type isDecl_Kind interface {
	isDecl_Kind()
}

type Decl_Ident struct {
	Ident *IdentDecl `protobuf:"bytes,4,opt,name=ident,proto3,oneof"`
}

type Decl_Function struct {
	Function *FunctionDecl `protobuf:"bytes,5,opt,name=function,proto3,oneof"`
}

func (*Decl_Ident) isDecl_Kind() {}

func (*Decl_Function) isDecl_Kind() {}

func (m *Decl) GetKind() isDecl_Kind {
	if m != nil {
		return m.Kind
	}
	return nil
}

func (m *Decl) GetIdent() *IdentDecl {
	if x, ok := m.GetKind().(*Decl_Ident); ok {
		return x.Ident
	}
	return nil
}

func (m *Decl) GetFunction() *FunctionDecl {
	if x, ok := m.GetKind().(*Decl_Function); ok {
		return x.Function
	}
	return nil
}

// XXX_OneofFuncs is for the internal use of the proto package.
func (*Decl) XXX_OneofFuncs() (func(msg proto.Message, b *proto.Buffer) error, func(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error), func(msg proto.Message) (n int), []interface{}) {
	return _Decl_OneofMarshaler, _Decl_OneofUnmarshaler, _Decl_OneofSizer, []interface{}{
		(*Decl_Ident)(nil),
		(*Decl_Function)(nil),
	}
}

func _Decl_OneofMarshaler(msg proto.Message, b *proto.Buffer) error {
	m := msg.(*Decl)
	// kind
	switch x := m.Kind.(type) {
	case *Decl_Ident:
		b.EncodeVarint(4<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Ident); err != nil {
			return err
		}
	case *Decl_Function:
		b.EncodeVarint(5<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Function); err != nil {
			return err
		}
	case nil:
	default:
		return fmt.Errorf("Decl.Kind has unexpected type %T", x)
	}
	return nil
}

func _Decl_OneofUnmarshaler(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error) {
	m := msg.(*Decl)
	switch tag {
	case 4: // kind.ident
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(IdentDecl)
		err := b.DecodeMessage(msg)
		m.Kind = &Decl_Ident{msg}
		return true, err
	case 5: // kind.function
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(FunctionDecl)
		err := b.DecodeMessage(msg)
		m.Kind = &Decl_Function{msg}
		return true, err
	default:
		return false, nil
	}
}

func _Decl_OneofSizer(msg proto.Message) (n int) {
	m := msg.(*Decl)
	// kind
	switch x := m.Kind.(type) {
	case *Decl_Ident:
		s := proto.Size(x.Ident)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Decl_Function:
		s := proto.Size(x.Function)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case nil:
	default:
		panic(fmt.Sprintf("proto: unexpected type %T in oneof", x))
	}
	return n
}

// The declared type of a variable.
//
// Extends runtime type values with extra information used for type checking
// and dispatching.
type DeclType struct {
	// The expression id of the declared type, if applicable.
	Id int32 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	// The type name, e.g. 'int', 'my.type.Type' or 'T'
	Type string `protobuf:"bytes,2,opt,name=type,proto3" json:"type,omitempty"`
	// An ordered list of type parameters, e.g. `<string, int>`.
	// Only applies to a subset of types, e.g. `map`, `list`.
	TypeParams           []*DeclType `protobuf:"bytes,4,rep,name=type_params,json=typeParams,proto3" json:"type_params,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *DeclType) Reset()         { *m = DeclType{} }
func (m *DeclType) String() string { return proto.CompactTextString(m) }
func (*DeclType) ProtoMessage()    {}
func (*DeclType) Descriptor() ([]byte, []int) {
	return fileDescriptor_decl_6647d3ad822811d0, []int{1}
}
func (m *DeclType) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DeclType.Unmarshal(m, b)
}
func (m *DeclType) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DeclType.Marshal(b, m, deterministic)
}
func (dst *DeclType) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DeclType.Merge(dst, src)
}
func (m *DeclType) XXX_Size() int {
	return xxx_messageInfo_DeclType.Size(m)
}
func (m *DeclType) XXX_DiscardUnknown() {
	xxx_messageInfo_DeclType.DiscardUnknown(m)
}

var xxx_messageInfo_DeclType proto.InternalMessageInfo

func (m *DeclType) GetId() int32 {
	if m != nil {
		return m.Id
	}
	return 0
}

func (m *DeclType) GetType() string {
	if m != nil {
		return m.Type
	}
	return ""
}

func (m *DeclType) GetTypeParams() []*DeclType {
	if m != nil {
		return m.TypeParams
	}
	return nil
}

// An identifier declaration.
type IdentDecl struct {
	// Optional type of the identifier.
	Type *DeclType `protobuf:"bytes,3,opt,name=type,proto3" json:"type,omitempty"`
	// Optional value of the identifier.
	Value                *Expr    `protobuf:"bytes,4,opt,name=value,proto3" json:"value,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *IdentDecl) Reset()         { *m = IdentDecl{} }
func (m *IdentDecl) String() string { return proto.CompactTextString(m) }
func (*IdentDecl) ProtoMessage()    {}
func (*IdentDecl) Descriptor() ([]byte, []int) {
	return fileDescriptor_decl_6647d3ad822811d0, []int{2}
}
func (m *IdentDecl) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_IdentDecl.Unmarshal(m, b)
}
func (m *IdentDecl) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_IdentDecl.Marshal(b, m, deterministic)
}
func (dst *IdentDecl) XXX_Merge(src proto.Message) {
	xxx_messageInfo_IdentDecl.Merge(dst, src)
}
func (m *IdentDecl) XXX_Size() int {
	return xxx_messageInfo_IdentDecl.Size(m)
}
func (m *IdentDecl) XXX_DiscardUnknown() {
	xxx_messageInfo_IdentDecl.DiscardUnknown(m)
}

var xxx_messageInfo_IdentDecl proto.InternalMessageInfo

func (m *IdentDecl) GetType() *DeclType {
	if m != nil {
		return m.Type
	}
	return nil
}

func (m *IdentDecl) GetValue() *Expr {
	if m != nil {
		return m.Value
	}
	return nil
}

// A function declaration.
type FunctionDecl struct {
	// The function arguments.
	Args []*IdentDecl `protobuf:"bytes,1,rep,name=args,proto3" json:"args,omitempty"`
	// Optional declared return type.
	ReturnType *DeclType `protobuf:"bytes,2,opt,name=return_type,json=returnType,proto3" json:"return_type,omitempty"`
	// If the first argument of the function is the receiver.
	ReceiverFunction     bool     `protobuf:"varint,3,opt,name=receiver_function,json=receiverFunction,proto3" json:"receiver_function,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *FunctionDecl) Reset()         { *m = FunctionDecl{} }
func (m *FunctionDecl) String() string { return proto.CompactTextString(m) }
func (*FunctionDecl) ProtoMessage()    {}
func (*FunctionDecl) Descriptor() ([]byte, []int) {
	return fileDescriptor_decl_6647d3ad822811d0, []int{3}
}
func (m *FunctionDecl) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_FunctionDecl.Unmarshal(m, b)
}
func (m *FunctionDecl) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_FunctionDecl.Marshal(b, m, deterministic)
}
func (dst *FunctionDecl) XXX_Merge(src proto.Message) {
	xxx_messageInfo_FunctionDecl.Merge(dst, src)
}
func (m *FunctionDecl) XXX_Size() int {
	return xxx_messageInfo_FunctionDecl.Size(m)
}
func (m *FunctionDecl) XXX_DiscardUnknown() {
	xxx_messageInfo_FunctionDecl.DiscardUnknown(m)
}

var xxx_messageInfo_FunctionDecl proto.InternalMessageInfo

func (m *FunctionDecl) GetArgs() []*IdentDecl {
	if m != nil {
		return m.Args
	}
	return nil
}

func (m *FunctionDecl) GetReturnType() *DeclType {
	if m != nil {
		return m.ReturnType
	}
	return nil
}

func (m *FunctionDecl) GetReceiverFunction() bool {
	if m != nil {
		return m.ReceiverFunction
	}
	return false
}

func init() {
	proto.RegisterType((*Decl)(nil), "google.api.expr.v1beta1.Decl")
	proto.RegisterType((*DeclType)(nil), "google.api.expr.v1beta1.DeclType")
	proto.RegisterType((*IdentDecl)(nil), "google.api.expr.v1beta1.IdentDecl")
	proto.RegisterType((*FunctionDecl)(nil), "google.api.expr.v1beta1.FunctionDecl")
}

func init() {
	proto.RegisterFile("google/api/expr/v1beta1/decl.proto", fileDescriptor_decl_6647d3ad822811d0)
}

var fileDescriptor_decl_6647d3ad822811d0 = []byte{
	// 398 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x92, 0xcd, 0x4a, 0xeb, 0x40,
	0x14, 0xc7, 0xef, 0x34, 0x49, 0x69, 0x4f, 0x2f, 0x97, 0xde, 0xd9, 0xdc, 0x70, 0x45, 0x88, 0x01,
	0x21, 0x20, 0x24, 0xb4, 0x45, 0x17, 0x76, 0x17, 0x3f, 0xd0, 0x5d, 0x09, 0xae, 0xdc, 0x94, 0x69,
	0x32, 0x86, 0xd1, 0x74, 0x66, 0x98, 0xa6, 0xb5, 0x7d, 0x32, 0x9f, 0xc0, 0x77, 0x72, 0x29, 0x33,
	0x49, 0x83, 0xa0, 0x81, 0xae, 0x7a, 0x9a, 0xf3, 0xff, 0x9d, 0x8f, 0xff, 0x1c, 0xf0, 0x73, 0x21,
	0xf2, 0x82, 0x46, 0x44, 0xb2, 0x88, 0x6e, 0xa5, 0x8a, 0x36, 0xa3, 0x05, 0x2d, 0xc9, 0x28, 0xca,
	0x68, 0x5a, 0x84, 0x52, 0x89, 0x52, 0xe0, 0x7f, 0x95, 0x26, 0x24, 0x92, 0x85, 0x5a, 0x13, 0xd6,
	0x9a, 0xff, 0xad, 0xb0, 0x51, 0x19, 0xd8, 0x7f, 0x47, 0x60, 0x5f, 0xd3, 0xb4, 0xc0, 0x7f, 0xa0,
	0xc3, 0x32, 0x17, 0x79, 0x28, 0x70, 0x92, 0x0e, 0xcb, 0x30, 0x06, 0x9b, 0x93, 0x25, 0x75, 0x3b,
	0x1e, 0x0a, 0xfa, 0x89, 0x89, 0xf1, 0x10, 0xac, 0x4c, 0xa4, 0xae, 0x65, 0x3e, 0xe9, 0x10, 0x5f,
	0x82, 0xc3, 0x32, 0xca, 0x4b, 0xd7, 0xf6, 0x50, 0x30, 0x18, 0xfb, 0x61, 0xcb, 0x2c, 0xe1, 0xbd,
	0x56, 0xe9, 0x46, 0x77, 0xbf, 0x92, 0x0a, 0xc1, 0x57, 0xd0, 0x7b, 0x5a, 0xf3, 0xb4, 0x64, 0x82,
	0xbb, 0x8e, 0xc1, 0x4f, 0x5b, 0xf1, 0xdb, 0x5a, 0x58, 0x57, 0x68, 0xc0, 0xb8, 0x0b, 0xf6, 0x0b,
	0xe3, 0x99, 0xaf, 0xa0, 0xa7, 0x73, 0x0f, 0x3b, 0x49, 0x7f, 0x5a, 0xa5, 0xdc, 0xc9, 0x66, 0x15,
	0x1d, 0xe3, 0x18, 0x06, 0xfa, 0x77, 0x2e, 0x89, 0x22, 0xcb, 0x95, 0x6b, 0x7b, 0x56, 0x30, 0x18,
	0x9f, 0xb4, 0xf6, 0xdf, 0xd7, 0x4e, 0x40, 0x53, 0x33, 0x03, 0xf9, 0xaf, 0xd0, 0x6f, 0xd6, 0xc2,
	0xe7, 0x75, 0x13, 0xcb, 0x6c, 0x72, 0x40, 0xa5, 0x6a, 0x8e, 0x09, 0x38, 0x1b, 0x52, 0xac, 0x69,
	0x6d, 0xe0, 0x71, 0x2b, 0x77, 0xb3, 0x95, 0x2a, 0xa9, 0xb4, 0xfe, 0x1b, 0x82, 0xdf, 0x5f, 0x1d,
	0xc1, 0x17, 0x60, 0x13, 0x95, 0xaf, 0x5c, 0x64, 0xd6, 0x38, 0xe0, 0x15, 0x12, 0xa3, 0xd7, 0x2e,
	0x28, 0x5a, 0xae, 0x15, 0x9f, 0x37, 0x06, 0x1d, 0xe6, 0x42, 0x45, 0x19, 0xb7, 0xcf, 0xe0, 0xaf,
	0xa2, 0x29, 0x65, 0x1b, 0xaa, 0xe6, 0xcd, 0x7b, 0x6a, 0x17, 0x7a, 0xc9, 0x70, 0x9f, 0xd8, 0x0f,
	0x1b, 0x3f, 0xc3, 0x51, 0x2a, 0x96, 0x6d, 0x0d, 0xe2, 0xbe, 0xee, 0x30, 0xd3, 0x87, 0x39, 0x43,
	0x8f, 0xd3, 0x5a, 0x95, 0x8b, 0x82, 0xf0, 0x3c, 0x14, 0x2a, 0x8f, 0x72, 0xca, 0xcd, 0xd9, 0x46,
	0x55, 0x8a, 0x48, 0xb6, 0xfa, 0x76, 0xdd, 0x53, 0xfd, 0xe7, 0x03, 0xa1, 0x45, 0xd7, 0x48, 0x27,
	0x9f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x10, 0x20, 0xb6, 0xbc, 0x44, 0x03, 0x00, 0x00,
}
