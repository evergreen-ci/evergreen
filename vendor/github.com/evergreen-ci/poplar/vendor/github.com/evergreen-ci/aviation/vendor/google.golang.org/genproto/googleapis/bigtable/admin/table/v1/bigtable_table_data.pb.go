// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/bigtable/admin/table/v1/bigtable_table_data.proto

package table // import "google.golang.org/genproto/googleapis/bigtable/admin/table/v1"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import duration "github.com/golang/protobuf/ptypes/duration"
import longrunning "google.golang.org/genproto/googleapis/longrunning"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type Table_TimestampGranularity int32

const (
	Table_MILLIS Table_TimestampGranularity = 0
)

var Table_TimestampGranularity_name = map[int32]string{
	0: "MILLIS",
}
var Table_TimestampGranularity_value = map[string]int32{
	"MILLIS": 0,
}

func (x Table_TimestampGranularity) String() string {
	return proto.EnumName(Table_TimestampGranularity_name, int32(x))
}
func (Table_TimestampGranularity) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_table_data_021ea70d03a8dd36, []int{0, 0}
}

// A collection of user data indexed by row, column, and timestamp.
// Each table is served using the resources of its parent cluster.
type Table struct {
	// A unique identifier of the form
	// <cluster_name>/tables/[_a-zA-Z0-9][-_.a-zA-Z0-9]*
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// If this Table is in the process of being created, the Operation used to
	// track its progress. As long as this operation is present, the Table will
	// not accept any Table Admin or Read/Write requests.
	CurrentOperation *longrunning.Operation `protobuf:"bytes,2,opt,name=current_operation,json=currentOperation,proto3" json:"current_operation,omitempty"`
	// The column families configured for this table, mapped by column family id.
	ColumnFamilies map[string]*ColumnFamily `protobuf:"bytes,3,rep,name=column_families,json=columnFamilies,proto3" json:"column_families,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// The granularity (e.g. MILLIS, MICROS) at which timestamps are stored in
	// this table. Timestamps not matching the granularity will be rejected.
	// Cannot be changed once the table is created.
	Granularity          Table_TimestampGranularity `protobuf:"varint,4,opt,name=granularity,proto3,enum=google.bigtable.admin.table.v1.Table_TimestampGranularity" json:"granularity,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                   `json:"-"`
	XXX_unrecognized     []byte                     `json:"-"`
	XXX_sizecache        int32                      `json:"-"`
}

func (m *Table) Reset()         { *m = Table{} }
func (m *Table) String() string { return proto.CompactTextString(m) }
func (*Table) ProtoMessage()    {}
func (*Table) Descriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_table_data_021ea70d03a8dd36, []int{0}
}
func (m *Table) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Table.Unmarshal(m, b)
}
func (m *Table) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Table.Marshal(b, m, deterministic)
}
func (dst *Table) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Table.Merge(dst, src)
}
func (m *Table) XXX_Size() int {
	return xxx_messageInfo_Table.Size(m)
}
func (m *Table) XXX_DiscardUnknown() {
	xxx_messageInfo_Table.DiscardUnknown(m)
}

var xxx_messageInfo_Table proto.InternalMessageInfo

func (m *Table) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Table) GetCurrentOperation() *longrunning.Operation {
	if m != nil {
		return m.CurrentOperation
	}
	return nil
}

func (m *Table) GetColumnFamilies() map[string]*ColumnFamily {
	if m != nil {
		return m.ColumnFamilies
	}
	return nil
}

func (m *Table) GetGranularity() Table_TimestampGranularity {
	if m != nil {
		return m.Granularity
	}
	return Table_MILLIS
}

// A set of columns within a table which share a common configuration.
type ColumnFamily struct {
	// A unique identifier of the form <table_name>/columnFamilies/[-_.a-zA-Z0-9]+
	// The last segment is the same as the "name" field in
	// google.bigtable.v1.Family.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Garbage collection expression specified by the following grammar:
	//   GC = EXPR
	//      | "" ;
	//   EXPR = EXPR, "||", EXPR              (* lowest precedence *)
	//        | EXPR, "&&", EXPR
	//        | "(", EXPR, ")"                (* highest precedence *)
	//        | PROP ;
	//   PROP = "version() >", NUM32
	//        | "age() >", NUM64, [ UNIT ] ;
	//   NUM32 = non-zero-digit { digit } ;    (* # NUM32 <= 2^32 - 1 *)
	//   NUM64 = non-zero-digit { digit } ;    (* # NUM64 <= 2^63 - 1 *)
	//   UNIT =  "d" | "h" | "m"  (* d=days, h=hours, m=minutes, else micros *)
	// GC expressions can be up to 500 characters in length
	//
	// The different types of PROP are defined as follows:
	//   version() - cell index, counting from most recent and starting at 1
	//   age() - age of the cell (current time minus cell timestamp)
	//
	// Example: "version() > 3 || (age() > 3d && version() > 1)"
	//   drop cells beyond the most recent three, and drop cells older than three
	//   days unless they're the most recent cell in the row/column
	//
	// Garbage collection executes opportunistically in the background, and so
	// it's possible for reads to return a cell even if it matches the active GC
	// expression for its family.
	GcExpression string `protobuf:"bytes,2,opt,name=gc_expression,json=gcExpression,proto3" json:"gc_expression,omitempty"`
	// Garbage collection rule specified as a protobuf.
	// Supersedes `gc_expression`.
	// Must serialize to at most 500 bytes.
	//
	// NOTE: Garbage collection executes opportunistically in the background, and
	// so it's possible for reads to return a cell even if it matches the active
	// GC expression for its family.
	GcRule               *GcRule  `protobuf:"bytes,3,opt,name=gc_rule,json=gcRule,proto3" json:"gc_rule,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ColumnFamily) Reset()         { *m = ColumnFamily{} }
func (m *ColumnFamily) String() string { return proto.CompactTextString(m) }
func (*ColumnFamily) ProtoMessage()    {}
func (*ColumnFamily) Descriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_table_data_021ea70d03a8dd36, []int{1}
}
func (m *ColumnFamily) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ColumnFamily.Unmarshal(m, b)
}
func (m *ColumnFamily) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ColumnFamily.Marshal(b, m, deterministic)
}
func (dst *ColumnFamily) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ColumnFamily.Merge(dst, src)
}
func (m *ColumnFamily) XXX_Size() int {
	return xxx_messageInfo_ColumnFamily.Size(m)
}
func (m *ColumnFamily) XXX_DiscardUnknown() {
	xxx_messageInfo_ColumnFamily.DiscardUnknown(m)
}

var xxx_messageInfo_ColumnFamily proto.InternalMessageInfo

func (m *ColumnFamily) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *ColumnFamily) GetGcExpression() string {
	if m != nil {
		return m.GcExpression
	}
	return ""
}

func (m *ColumnFamily) GetGcRule() *GcRule {
	if m != nil {
		return m.GcRule
	}
	return nil
}

// Rule for determining which cells to delete during garbage collection.
type GcRule struct {
	// Types that are valid to be assigned to Rule:
	//	*GcRule_MaxNumVersions
	//	*GcRule_MaxAge
	//	*GcRule_Intersection_
	//	*GcRule_Union_
	Rule                 isGcRule_Rule `protobuf_oneof:"rule"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *GcRule) Reset()         { *m = GcRule{} }
func (m *GcRule) String() string { return proto.CompactTextString(m) }
func (*GcRule) ProtoMessage()    {}
func (*GcRule) Descriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_table_data_021ea70d03a8dd36, []int{2}
}
func (m *GcRule) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GcRule.Unmarshal(m, b)
}
func (m *GcRule) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GcRule.Marshal(b, m, deterministic)
}
func (dst *GcRule) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GcRule.Merge(dst, src)
}
func (m *GcRule) XXX_Size() int {
	return xxx_messageInfo_GcRule.Size(m)
}
func (m *GcRule) XXX_DiscardUnknown() {
	xxx_messageInfo_GcRule.DiscardUnknown(m)
}

var xxx_messageInfo_GcRule proto.InternalMessageInfo

type isGcRule_Rule interface {
	isGcRule_Rule()
}

type GcRule_MaxNumVersions struct {
	MaxNumVersions int32 `protobuf:"varint,1,opt,name=max_num_versions,json=maxNumVersions,proto3,oneof"`
}

type GcRule_MaxAge struct {
	MaxAge *duration.Duration `protobuf:"bytes,2,opt,name=max_age,json=maxAge,proto3,oneof"`
}

type GcRule_Intersection_ struct {
	Intersection *GcRule_Intersection `protobuf:"bytes,3,opt,name=intersection,proto3,oneof"`
}

type GcRule_Union_ struct {
	Union *GcRule_Union `protobuf:"bytes,4,opt,name=union,proto3,oneof"`
}

func (*GcRule_MaxNumVersions) isGcRule_Rule() {}

func (*GcRule_MaxAge) isGcRule_Rule() {}

func (*GcRule_Intersection_) isGcRule_Rule() {}

func (*GcRule_Union_) isGcRule_Rule() {}

func (m *GcRule) GetRule() isGcRule_Rule {
	if m != nil {
		return m.Rule
	}
	return nil
}

func (m *GcRule) GetMaxNumVersions() int32 {
	if x, ok := m.GetRule().(*GcRule_MaxNumVersions); ok {
		return x.MaxNumVersions
	}
	return 0
}

func (m *GcRule) GetMaxAge() *duration.Duration {
	if x, ok := m.GetRule().(*GcRule_MaxAge); ok {
		return x.MaxAge
	}
	return nil
}

func (m *GcRule) GetIntersection() *GcRule_Intersection {
	if x, ok := m.GetRule().(*GcRule_Intersection_); ok {
		return x.Intersection
	}
	return nil
}

func (m *GcRule) GetUnion() *GcRule_Union {
	if x, ok := m.GetRule().(*GcRule_Union_); ok {
		return x.Union
	}
	return nil
}

// XXX_OneofFuncs is for the internal use of the proto package.
func (*GcRule) XXX_OneofFuncs() (func(msg proto.Message, b *proto.Buffer) error, func(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error), func(msg proto.Message) (n int), []interface{}) {
	return _GcRule_OneofMarshaler, _GcRule_OneofUnmarshaler, _GcRule_OneofSizer, []interface{}{
		(*GcRule_MaxNumVersions)(nil),
		(*GcRule_MaxAge)(nil),
		(*GcRule_Intersection_)(nil),
		(*GcRule_Union_)(nil),
	}
}

func _GcRule_OneofMarshaler(msg proto.Message, b *proto.Buffer) error {
	m := msg.(*GcRule)
	// rule
	switch x := m.Rule.(type) {
	case *GcRule_MaxNumVersions:
		b.EncodeVarint(1<<3 | proto.WireVarint)
		b.EncodeVarint(uint64(x.MaxNumVersions))
	case *GcRule_MaxAge:
		b.EncodeVarint(2<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.MaxAge); err != nil {
			return err
		}
	case *GcRule_Intersection_:
		b.EncodeVarint(3<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Intersection); err != nil {
			return err
		}
	case *GcRule_Union_:
		b.EncodeVarint(4<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Union); err != nil {
			return err
		}
	case nil:
	default:
		return fmt.Errorf("GcRule.Rule has unexpected type %T", x)
	}
	return nil
}

func _GcRule_OneofUnmarshaler(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error) {
	m := msg.(*GcRule)
	switch tag {
	case 1: // rule.max_num_versions
		if wire != proto.WireVarint {
			return true, proto.ErrInternalBadWireType
		}
		x, err := b.DecodeVarint()
		m.Rule = &GcRule_MaxNumVersions{int32(x)}
		return true, err
	case 2: // rule.max_age
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(duration.Duration)
		err := b.DecodeMessage(msg)
		m.Rule = &GcRule_MaxAge{msg}
		return true, err
	case 3: // rule.intersection
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(GcRule_Intersection)
		err := b.DecodeMessage(msg)
		m.Rule = &GcRule_Intersection_{msg}
		return true, err
	case 4: // rule.union
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(GcRule_Union)
		err := b.DecodeMessage(msg)
		m.Rule = &GcRule_Union_{msg}
		return true, err
	default:
		return false, nil
	}
}

func _GcRule_OneofSizer(msg proto.Message) (n int) {
	m := msg.(*GcRule)
	// rule
	switch x := m.Rule.(type) {
	case *GcRule_MaxNumVersions:
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(x.MaxNumVersions))
	case *GcRule_MaxAge:
		s := proto.Size(x.MaxAge)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *GcRule_Intersection_:
		s := proto.Size(x.Intersection)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *GcRule_Union_:
		s := proto.Size(x.Union)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case nil:
	default:
		panic(fmt.Sprintf("proto: unexpected type %T in oneof", x))
	}
	return n
}

// A GcRule which deletes cells matching all of the given rules.
type GcRule_Intersection struct {
	// Only delete cells which would be deleted by every element of `rules`.
	Rules                []*GcRule `protobuf:"bytes,1,rep,name=rules,proto3" json:"rules,omitempty"`
	XXX_NoUnkeyedLiteral struct{}  `json:"-"`
	XXX_unrecognized     []byte    `json:"-"`
	XXX_sizecache        int32     `json:"-"`
}

func (m *GcRule_Intersection) Reset()         { *m = GcRule_Intersection{} }
func (m *GcRule_Intersection) String() string { return proto.CompactTextString(m) }
func (*GcRule_Intersection) ProtoMessage()    {}
func (*GcRule_Intersection) Descriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_table_data_021ea70d03a8dd36, []int{2, 0}
}
func (m *GcRule_Intersection) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GcRule_Intersection.Unmarshal(m, b)
}
func (m *GcRule_Intersection) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GcRule_Intersection.Marshal(b, m, deterministic)
}
func (dst *GcRule_Intersection) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GcRule_Intersection.Merge(dst, src)
}
func (m *GcRule_Intersection) XXX_Size() int {
	return xxx_messageInfo_GcRule_Intersection.Size(m)
}
func (m *GcRule_Intersection) XXX_DiscardUnknown() {
	xxx_messageInfo_GcRule_Intersection.DiscardUnknown(m)
}

var xxx_messageInfo_GcRule_Intersection proto.InternalMessageInfo

func (m *GcRule_Intersection) GetRules() []*GcRule {
	if m != nil {
		return m.Rules
	}
	return nil
}

// A GcRule which deletes cells matching any of the given rules.
type GcRule_Union struct {
	// Delete cells which would be deleted by any element of `rules`.
	Rules                []*GcRule `protobuf:"bytes,1,rep,name=rules,proto3" json:"rules,omitempty"`
	XXX_NoUnkeyedLiteral struct{}  `json:"-"`
	XXX_unrecognized     []byte    `json:"-"`
	XXX_sizecache        int32     `json:"-"`
}

func (m *GcRule_Union) Reset()         { *m = GcRule_Union{} }
func (m *GcRule_Union) String() string { return proto.CompactTextString(m) }
func (*GcRule_Union) ProtoMessage()    {}
func (*GcRule_Union) Descriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_table_data_021ea70d03a8dd36, []int{2, 1}
}
func (m *GcRule_Union) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GcRule_Union.Unmarshal(m, b)
}
func (m *GcRule_Union) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GcRule_Union.Marshal(b, m, deterministic)
}
func (dst *GcRule_Union) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GcRule_Union.Merge(dst, src)
}
func (m *GcRule_Union) XXX_Size() int {
	return xxx_messageInfo_GcRule_Union.Size(m)
}
func (m *GcRule_Union) XXX_DiscardUnknown() {
	xxx_messageInfo_GcRule_Union.DiscardUnknown(m)
}

var xxx_messageInfo_GcRule_Union proto.InternalMessageInfo

func (m *GcRule_Union) GetRules() []*GcRule {
	if m != nil {
		return m.Rules
	}
	return nil
}

func init() {
	proto.RegisterType((*Table)(nil), "google.bigtable.admin.table.v1.Table")
	proto.RegisterMapType((map[string]*ColumnFamily)(nil), "google.bigtable.admin.table.v1.Table.ColumnFamiliesEntry")
	proto.RegisterType((*ColumnFamily)(nil), "google.bigtable.admin.table.v1.ColumnFamily")
	proto.RegisterType((*GcRule)(nil), "google.bigtable.admin.table.v1.GcRule")
	proto.RegisterType((*GcRule_Intersection)(nil), "google.bigtable.admin.table.v1.GcRule.Intersection")
	proto.RegisterType((*GcRule_Union)(nil), "google.bigtable.admin.table.v1.GcRule.Union")
	proto.RegisterEnum("google.bigtable.admin.table.v1.Table_TimestampGranularity", Table_TimestampGranularity_name, Table_TimestampGranularity_value)
}

func init() {
	proto.RegisterFile("google/bigtable/admin/table/v1/bigtable_table_data.proto", fileDescriptor_bigtable_table_data_021ea70d03a8dd36)
}

var fileDescriptor_bigtable_table_data_021ea70d03a8dd36 = []byte{
	// 579 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xa4, 0x94, 0x61, 0x6b, 0xd3, 0x40,
	0x18, 0xc7, 0x9b, 0xa5, 0xed, 0xd8, 0xb3, 0x3a, 0xeb, 0x29, 0x52, 0x0b, 0x4a, 0xc9, 0x40, 0x8a,
	0xc8, 0x85, 0x6d, 0xbe, 0x98, 0x53, 0x10, 0xbb, 0xcd, 0x6d, 0x32, 0x75, 0xc4, 0x29, 0x28, 0x42,
	0xb8, 0x66, 0xb7, 0x23, 0x98, 0xbb, 0x2b, 0x97, 0x5c, 0x69, 0x5f, 0xfb, 0xc6, 0x8f, 0xe2, 0xa7,
	0xf0, 0xb3, 0x49, 0xee, 0x2e, 0x35, 0x83, 0xe9, 0x26, 0xbe, 0x49, 0x9e, 0x3c, 0xf7, 0xfc, 0x7f,
	0xf7, 0xcf, 0xf3, 0x5c, 0x02, 0xdb, 0x4c, 0x4a, 0x96, 0xd1, 0x70, 0x9c, 0xb2, 0x82, 0x8c, 0x33,
	0x1a, 0x92, 0x33, 0x9e, 0x8a, 0xd0, 0xc6, 0xd3, 0x8d, 0x45, 0x3e, 0xb6, 0xd7, 0x33, 0x52, 0x10,
	0x3c, 0x51, 0xb2, 0x90, 0xe8, 0x81, 0x55, 0xe2, 0xaa, 0x02, 0x1b, 0x25, 0xb6, 0xf1, 0x74, 0xa3,
	0xbf, 0xee, 0xc8, 0x99, 0x14, 0x4c, 0x69, 0x21, 0x52, 0xc1, 0x42, 0x39, 0xa1, 0x8a, 0x14, 0xa9,
	0x14, 0xb9, 0x85, 0xf4, 0x1d, 0x24, 0x34, 0x4f, 0x63, 0x7d, 0x1e, 0x9e, 0x69, 0x5b, 0x60, 0xd7,
	0x83, 0x9f, 0x3e, 0xb4, 0x4e, 0x4b, 0x22, 0x42, 0xd0, 0x14, 0x84, 0xd3, 0x9e, 0x37, 0xf0, 0x86,
	0x2b, 0x91, 0x89, 0xd1, 0x6b, 0xb8, 0x95, 0x68, 0xa5, 0xa8, 0x28, 0xe2, 0x05, 0xb9, 0xb7, 0x34,
	0xf0, 0x86, 0xab, 0x9b, 0xf7, 0xb1, 0xb3, 0x57, 0xdb, 0x1e, 0xbf, 0xab, 0x8a, 0xa2, 0xae, 0xd3,
	0x2d, 0x32, 0x68, 0x0c, 0x37, 0x13, 0x99, 0x69, 0x2e, 0xe2, 0x73, 0xc2, 0xd3, 0x2c, 0xa5, 0x79,
	0xcf, 0x1f, 0xf8, 0xc3, 0xd5, 0xcd, 0xa7, 0xf8, 0xef, 0x2f, 0x8a, 0x8d, 0x3f, 0xbc, 0x6b, 0xc4,
	0xaf, 0x9c, 0x76, 0x5f, 0x14, 0x6a, 0x1e, 0xad, 0x25, 0x17, 0x92, 0xe8, 0x0b, 0xac, 0x32, 0x45,
	0x84, 0xce, 0x88, 0x4a, 0x8b, 0x79, 0xaf, 0x39, 0xf0, 0x86, 0x6b, 0x9b, 0x3b, 0xd7, 0xe3, 0x9f,
	0xa6, 0x9c, 0xe6, 0x05, 0xe1, 0x93, 0x83, 0xdf, 0x84, 0xa8, 0x8e, 0xeb, 0x4b, 0xb8, 0x7d, 0x89,
	0x09, 0xd4, 0x05, 0xff, 0x2b, 0x9d, 0xbb, 0xbe, 0x95, 0x21, 0x1a, 0x41, 0x6b, 0x4a, 0x32, 0x4d,
	0x5d, 0xab, 0x1e, 0x5f, 0x65, 0xa0, 0x46, 0x9d, 0x47, 0x56, 0xba, 0xb3, 0xb4, 0xed, 0x05, 0x01,
	0xdc, 0xb9, 0xcc, 0x15, 0x02, 0x68, 0xbf, 0x39, 0x3a, 0x3e, 0x3e, 0x7a, 0xdf, 0x6d, 0x04, 0xdf,
	0x3d, 0xe8, 0xd4, 0xf5, 0x97, 0xce, 0x71, 0x1d, 0x6e, 0xb0, 0x24, 0xa6, 0xb3, 0x89, 0xa2, 0x79,
	0x5e, 0xcd, 0x70, 0x25, 0xea, 0xb0, 0x64, 0x7f, 0x91, 0x43, 0x2f, 0x60, 0x99, 0x25, 0xb1, 0xd2,
	0x19, 0xed, 0xf9, 0xc6, 0xf7, 0xc3, 0xab, 0x7c, 0x1f, 0x24, 0x91, 0xce, 0x68, 0xd4, 0x66, 0xe6,
	0x1e, 0xfc, 0xf0, 0xa1, 0x6d, 0x53, 0xe8, 0x11, 0x74, 0x39, 0x99, 0xc5, 0x42, 0xf3, 0x78, 0x4a,
	0x55, 0x89, 0xcf, 0x8d, 0xa1, 0xd6, 0x61, 0x23, 0x5a, 0xe3, 0x64, 0xf6, 0x56, 0xf3, 0x8f, 0x2e,
	0x8f, 0x9e, 0xc0, 0x72, 0x59, 0x4b, 0x58, 0xd5, 0xaf, 0x7b, 0xd5, 0xbe, 0xd5, 0xa1, 0xc5, 0x7b,
	0xee, 0xd0, 0x1e, 0x36, 0xa2, 0x36, 0x27, 0xb3, 0x97, 0x8c, 0xa2, 0x4f, 0xd0, 0x49, 0x45, 0x41,
	0x55, 0x4e, 0x13, 0x73, 0x2a, 0xad, 0xe5, 0xad, 0xeb, 0x59, 0xc6, 0x47, 0x35, 0xe9, 0x61, 0x23,
	0xba, 0x80, 0x42, 0x7b, 0xd0, 0xd2, 0xa2, 0x64, 0x36, 0xaf, 0x37, 0x3e, 0xc7, 0xfc, 0x20, 0x2c,
	0xcc, 0x8a, 0xfb, 0xc7, 0xd0, 0xa9, 0xef, 0x82, 0x9e, 0x43, 0xab, 0xec, 0x6d, 0xd9, 0x07, 0xff,
	0x1f, 0x9a, 0x6b, 0x45, 0xfd, 0x7d, 0x68, 0x19, 0xfe, 0xff, 0x61, 0x46, 0x6d, 0x68, 0x96, 0xc1,
	0xe8, 0x9b, 0x07, 0x41, 0x22, 0xf9, 0x15, 0xe2, 0xd1, 0xdd, 0x91, 0x5b, 0x30, 0x9f, 0xc8, 0x1e,
	0x29, 0xc8, 0x49, 0x39, 0x92, 0x13, 0xef, 0xf3, 0xae, 0x53, 0x32, 0x99, 0x11, 0xc1, 0xb0, 0x54,
	0x2c, 0x64, 0x54, 0x98, 0x81, 0x85, 0x76, 0x89, 0x4c, 0xd2, 0xfc, 0x4f, 0x7f, 0xbd, 0x67, 0x26,
	0x18, 0xb7, 0x4d, 0xfd, 0xd6, 0xaf, 0x00, 0x00, 0x00, 0xff, 0xff, 0xd7, 0x80, 0x76, 0xdc, 0x24,
	0x05, 0x00, 0x00,
}
