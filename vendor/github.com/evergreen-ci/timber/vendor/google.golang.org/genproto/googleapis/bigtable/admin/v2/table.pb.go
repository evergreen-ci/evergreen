// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/bigtable/admin/v2/table.proto

package admin // import "google.golang.org/genproto/googleapis/bigtable/admin/v2"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import duration "github.com/golang/protobuf/ptypes/duration"
import timestamp "github.com/golang/protobuf/ptypes/timestamp"
import _ "google.golang.org/genproto/googleapis/api/annotations"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Possible timestamp granularities to use when keeping multiple versions
// of data in a table.
type Table_TimestampGranularity int32

const (
	// The user did not specify a granularity. Should not be returned.
	// When specified during table creation, MILLIS will be used.
	Table_TIMESTAMP_GRANULARITY_UNSPECIFIED Table_TimestampGranularity = 0
	// The table keeps data versioned at a granularity of 1ms.
	Table_MILLIS Table_TimestampGranularity = 1
)

var Table_TimestampGranularity_name = map[int32]string{
	0: "TIMESTAMP_GRANULARITY_UNSPECIFIED",
	1: "MILLIS",
}
var Table_TimestampGranularity_value = map[string]int32{
	"TIMESTAMP_GRANULARITY_UNSPECIFIED": 0,
	"MILLIS":                            1,
}

func (x Table_TimestampGranularity) String() string {
	return proto.EnumName(Table_TimestampGranularity_name, int32(x))
}
func (Table_TimestampGranularity) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{0, 0}
}

// Defines a view over a table's fields.
type Table_View int32

const (
	// Uses the default view for each method as documented in its request.
	Table_VIEW_UNSPECIFIED Table_View = 0
	// Only populates `name`.
	Table_NAME_ONLY Table_View = 1
	// Only populates `name` and fields related to the table's schema.
	Table_SCHEMA_VIEW Table_View = 2
	// Only populates `name` and fields related to the table's
	// replication state.
	Table_REPLICATION_VIEW Table_View = 3
	// Populates all fields.
	Table_FULL Table_View = 4
)

var Table_View_name = map[int32]string{
	0: "VIEW_UNSPECIFIED",
	1: "NAME_ONLY",
	2: "SCHEMA_VIEW",
	3: "REPLICATION_VIEW",
	4: "FULL",
}
var Table_View_value = map[string]int32{
	"VIEW_UNSPECIFIED": 0,
	"NAME_ONLY":        1,
	"SCHEMA_VIEW":      2,
	"REPLICATION_VIEW": 3,
	"FULL":             4,
}

func (x Table_View) String() string {
	return proto.EnumName(Table_View_name, int32(x))
}
func (Table_View) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{0, 1}
}

// Table replication states.
type Table_ClusterState_ReplicationState int32

const (
	// The replication state of the table is unknown in this cluster.
	Table_ClusterState_STATE_NOT_KNOWN Table_ClusterState_ReplicationState = 0
	// The cluster was recently created, and the table must finish copying
	// over pre-existing data from other clusters before it can begin
	// receiving live replication updates and serving Data API requests.
	Table_ClusterState_INITIALIZING Table_ClusterState_ReplicationState = 1
	// The table is temporarily unable to serve Data API requests from this
	// cluster due to planned internal maintenance.
	Table_ClusterState_PLANNED_MAINTENANCE Table_ClusterState_ReplicationState = 2
	// The table is temporarily unable to serve Data API requests from this
	// cluster due to unplanned or emergency maintenance.
	Table_ClusterState_UNPLANNED_MAINTENANCE Table_ClusterState_ReplicationState = 3
	// The table can serve Data API requests from this cluster. Depending on
	// replication delay, reads may not immediately reflect the state of the
	// table in other clusters.
	Table_ClusterState_READY Table_ClusterState_ReplicationState = 4
)

var Table_ClusterState_ReplicationState_name = map[int32]string{
	0: "STATE_NOT_KNOWN",
	1: "INITIALIZING",
	2: "PLANNED_MAINTENANCE",
	3: "UNPLANNED_MAINTENANCE",
	4: "READY",
}
var Table_ClusterState_ReplicationState_value = map[string]int32{
	"STATE_NOT_KNOWN":       0,
	"INITIALIZING":          1,
	"PLANNED_MAINTENANCE":   2,
	"UNPLANNED_MAINTENANCE": 3,
	"READY":                 4,
}

func (x Table_ClusterState_ReplicationState) String() string {
	return proto.EnumName(Table_ClusterState_ReplicationState_name, int32(x))
}
func (Table_ClusterState_ReplicationState) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{0, 0, 0}
}

// Possible states of a snapshot.
type Snapshot_State int32

const (
	// The state of the snapshot could not be determined.
	Snapshot_STATE_NOT_KNOWN Snapshot_State = 0
	// The snapshot has been successfully created and can serve all requests.
	Snapshot_READY Snapshot_State = 1
	// The snapshot is currently being created, and may be destroyed if the
	// creation process encounters an error. A snapshot may not be restored to a
	// table while it is being created.
	Snapshot_CREATING Snapshot_State = 2
)

var Snapshot_State_name = map[int32]string{
	0: "STATE_NOT_KNOWN",
	1: "READY",
	2: "CREATING",
}
var Snapshot_State_value = map[string]int32{
	"STATE_NOT_KNOWN": 0,
	"READY":           1,
	"CREATING":        2,
}

func (x Snapshot_State) String() string {
	return proto.EnumName(Snapshot_State_name, int32(x))
}
func (Snapshot_State) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{3, 0}
}

// A collection of user data indexed by row, column, and timestamp.
// Each table is served using the resources of its parent cluster.
type Table struct {
	// (`OutputOnly`)
	// The unique name of the table. Values are of the form
	// `projects/<project>/instances/<instance>/tables/[_a-zA-Z0-9][-_.a-zA-Z0-9]*`.
	// Views: `NAME_ONLY`, `SCHEMA_VIEW`, `REPLICATION_VIEW`, `FULL`
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// (`OutputOnly`)
	// Map from cluster ID to per-cluster table state.
	// If it could not be determined whether or not the table has data in a
	// particular cluster (for example, if its zone is unavailable), then
	// there will be an entry for the cluster with UNKNOWN `replication_status`.
	// Views: `REPLICATION_VIEW`, `FULL`
	ClusterStates map[string]*Table_ClusterState `protobuf:"bytes,2,rep,name=cluster_states,json=clusterStates,proto3" json:"cluster_states,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// (`CreationOnly`)
	// The column families configured for this table, mapped by column family ID.
	// Views: `SCHEMA_VIEW`, `FULL`
	ColumnFamilies map[string]*ColumnFamily `protobuf:"bytes,3,rep,name=column_families,json=columnFamilies,proto3" json:"column_families,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// (`CreationOnly`)
	// The granularity (i.e. `MILLIS`) at which timestamps are stored in
	// this table. Timestamps not matching the granularity will be rejected.
	// If unspecified at creation time, the value will be set to `MILLIS`.
	// Views: `SCHEMA_VIEW`, `FULL`
	Granularity          Table_TimestampGranularity `protobuf:"varint,4,opt,name=granularity,proto3,enum=google.bigtable.admin.v2.Table_TimestampGranularity" json:"granularity,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                   `json:"-"`
	XXX_unrecognized     []byte                     `json:"-"`
	XXX_sizecache        int32                      `json:"-"`
}

func (m *Table) Reset()         { *m = Table{} }
func (m *Table) String() string { return proto.CompactTextString(m) }
func (*Table) ProtoMessage()    {}
func (*Table) Descriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{0}
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

func (m *Table) GetClusterStates() map[string]*Table_ClusterState {
	if m != nil {
		return m.ClusterStates
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
	return Table_TIMESTAMP_GRANULARITY_UNSPECIFIED
}

// The state of a table's data in a particular cluster.
type Table_ClusterState struct {
	// (`OutputOnly`)
	// The state of replication for the table in this cluster.
	ReplicationState     Table_ClusterState_ReplicationState `protobuf:"varint,1,opt,name=replication_state,json=replicationState,proto3,enum=google.bigtable.admin.v2.Table_ClusterState_ReplicationState" json:"replication_state,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                            `json:"-"`
	XXX_unrecognized     []byte                              `json:"-"`
	XXX_sizecache        int32                               `json:"-"`
}

func (m *Table_ClusterState) Reset()         { *m = Table_ClusterState{} }
func (m *Table_ClusterState) String() string { return proto.CompactTextString(m) }
func (*Table_ClusterState) ProtoMessage()    {}
func (*Table_ClusterState) Descriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{0, 0}
}
func (m *Table_ClusterState) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Table_ClusterState.Unmarshal(m, b)
}
func (m *Table_ClusterState) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Table_ClusterState.Marshal(b, m, deterministic)
}
func (dst *Table_ClusterState) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Table_ClusterState.Merge(dst, src)
}
func (m *Table_ClusterState) XXX_Size() int {
	return xxx_messageInfo_Table_ClusterState.Size(m)
}
func (m *Table_ClusterState) XXX_DiscardUnknown() {
	xxx_messageInfo_Table_ClusterState.DiscardUnknown(m)
}

var xxx_messageInfo_Table_ClusterState proto.InternalMessageInfo

func (m *Table_ClusterState) GetReplicationState() Table_ClusterState_ReplicationState {
	if m != nil {
		return m.ReplicationState
	}
	return Table_ClusterState_STATE_NOT_KNOWN
}

// A set of columns within a table which share a common configuration.
type ColumnFamily struct {
	// Garbage collection rule specified as a protobuf.
	// Must serialize to at most 500 bytes.
	//
	// NOTE: Garbage collection executes opportunistically in the background, and
	// so it's possible for reads to return a cell even if it matches the active
	// GC expression for its family.
	GcRule               *GcRule  `protobuf:"bytes,1,opt,name=gc_rule,json=gcRule,proto3" json:"gc_rule,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ColumnFamily) Reset()         { *m = ColumnFamily{} }
func (m *ColumnFamily) String() string { return proto.CompactTextString(m) }
func (*ColumnFamily) ProtoMessage()    {}
func (*ColumnFamily) Descriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{1}
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

func (m *ColumnFamily) GetGcRule() *GcRule {
	if m != nil {
		return m.GcRule
	}
	return nil
}

// Rule for determining which cells to delete during garbage collection.
type GcRule struct {
	// Garbage collection rules.
	//
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
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{2}
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
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{2, 0}
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
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{2, 1}
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

// A snapshot of a table at a particular time. A snapshot can be used as a
// checkpoint for data restoration or a data source for a new table.
//
// Note: This is a private alpha release of Cloud Bigtable snapshots. This
// feature is not currently available to most Cloud Bigtable customers. This
// feature might be changed in backward-incompatible ways and is not recommended
// for production use. It is not subject to any SLA or deprecation policy.
type Snapshot struct {
	// (`OutputOnly`)
	// The unique name of the snapshot.
	// Values are of the form
	// `projects/<project>/instances/<instance>/clusters/<cluster>/snapshots/<snapshot>`.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// (`OutputOnly`)
	// The source table at the time the snapshot was taken.
	SourceTable *Table `protobuf:"bytes,2,opt,name=source_table,json=sourceTable,proto3" json:"source_table,omitempty"`
	// (`OutputOnly`)
	// The size of the data in the source table at the time the snapshot was
	// taken. In some cases, this value may be computed asynchronously via a
	// background process and a placeholder of 0 will be used in the meantime.
	DataSizeBytes int64 `protobuf:"varint,3,opt,name=data_size_bytes,json=dataSizeBytes,proto3" json:"data_size_bytes,omitempty"`
	// (`OutputOnly`)
	// The time when the snapshot is created.
	CreateTime *timestamp.Timestamp `protobuf:"bytes,4,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
	// (`OutputOnly`)
	// The time when the snapshot will be deleted. The maximum amount of time a
	// snapshot can stay active is 365 days. If 'ttl' is not specified,
	// the default maximum of 365 days will be used.
	DeleteTime *timestamp.Timestamp `protobuf:"bytes,5,opt,name=delete_time,json=deleteTime,proto3" json:"delete_time,omitempty"`
	// (`OutputOnly`)
	// The current state of the snapshot.
	State Snapshot_State `protobuf:"varint,6,opt,name=state,proto3,enum=google.bigtable.admin.v2.Snapshot_State" json:"state,omitempty"`
	// (`OutputOnly`)
	// Description of the snapshot.
	Description          string   `protobuf:"bytes,7,opt,name=description,proto3" json:"description,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Snapshot) Reset()         { *m = Snapshot{} }
func (m *Snapshot) String() string { return proto.CompactTextString(m) }
func (*Snapshot) ProtoMessage()    {}
func (*Snapshot) Descriptor() ([]byte, []int) {
	return fileDescriptor_table_ed23c9c8618cc2b8, []int{3}
}
func (m *Snapshot) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Snapshot.Unmarshal(m, b)
}
func (m *Snapshot) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Snapshot.Marshal(b, m, deterministic)
}
func (dst *Snapshot) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Snapshot.Merge(dst, src)
}
func (m *Snapshot) XXX_Size() int {
	return xxx_messageInfo_Snapshot.Size(m)
}
func (m *Snapshot) XXX_DiscardUnknown() {
	xxx_messageInfo_Snapshot.DiscardUnknown(m)
}

var xxx_messageInfo_Snapshot proto.InternalMessageInfo

func (m *Snapshot) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Snapshot) GetSourceTable() *Table {
	if m != nil {
		return m.SourceTable
	}
	return nil
}

func (m *Snapshot) GetDataSizeBytes() int64 {
	if m != nil {
		return m.DataSizeBytes
	}
	return 0
}

func (m *Snapshot) GetCreateTime() *timestamp.Timestamp {
	if m != nil {
		return m.CreateTime
	}
	return nil
}

func (m *Snapshot) GetDeleteTime() *timestamp.Timestamp {
	if m != nil {
		return m.DeleteTime
	}
	return nil
}

func (m *Snapshot) GetState() Snapshot_State {
	if m != nil {
		return m.State
	}
	return Snapshot_STATE_NOT_KNOWN
}

func (m *Snapshot) GetDescription() string {
	if m != nil {
		return m.Description
	}
	return ""
}

func init() {
	proto.RegisterType((*Table)(nil), "google.bigtable.admin.v2.Table")
	proto.RegisterMapType((map[string]*Table_ClusterState)(nil), "google.bigtable.admin.v2.Table.ClusterStatesEntry")
	proto.RegisterMapType((map[string]*ColumnFamily)(nil), "google.bigtable.admin.v2.Table.ColumnFamiliesEntry")
	proto.RegisterType((*Table_ClusterState)(nil), "google.bigtable.admin.v2.Table.ClusterState")
	proto.RegisterType((*ColumnFamily)(nil), "google.bigtable.admin.v2.ColumnFamily")
	proto.RegisterType((*GcRule)(nil), "google.bigtable.admin.v2.GcRule")
	proto.RegisterType((*GcRule_Intersection)(nil), "google.bigtable.admin.v2.GcRule.Intersection")
	proto.RegisterType((*GcRule_Union)(nil), "google.bigtable.admin.v2.GcRule.Union")
	proto.RegisterType((*Snapshot)(nil), "google.bigtable.admin.v2.Snapshot")
	proto.RegisterEnum("google.bigtable.admin.v2.Table_TimestampGranularity", Table_TimestampGranularity_name, Table_TimestampGranularity_value)
	proto.RegisterEnum("google.bigtable.admin.v2.Table_View", Table_View_name, Table_View_value)
	proto.RegisterEnum("google.bigtable.admin.v2.Table_ClusterState_ReplicationState", Table_ClusterState_ReplicationState_name, Table_ClusterState_ReplicationState_value)
	proto.RegisterEnum("google.bigtable.admin.v2.Snapshot_State", Snapshot_State_name, Snapshot_State_value)
}

func init() {
	proto.RegisterFile("google/bigtable/admin/v2/table.proto", fileDescriptor_table_ed23c9c8618cc2b8)
}

var fileDescriptor_table_ed23c9c8618cc2b8 = []byte{
	// 965 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x56, 0xff, 0x6e, 0xdb, 0x54,
	0x18, 0xad, 0xe3, 0x38, 0x6d, 0xbf, 0xa4, 0xad, 0xb9, 0x1d, 0x22, 0x8b, 0xa6, 0x2d, 0x44, 0x30,
	0x45, 0x08, 0x1c, 0xc9, 0x1b, 0x08, 0x18, 0x1b, 0x72, 0x52, 0xb7, 0xb5, 0x48, 0xdc, 0xc8, 0x71,
	0x32, 0x75, 0x8a, 0x64, 0xdd, 0x38, 0x77, 0xc6, 0xe0, 0x1f, 0x91, 0x7f, 0x94, 0x66, 0x4f, 0xc1,
	0x0b, 0xf0, 0x37, 0x12, 0x12, 0x2f, 0xc2, 0xf3, 0xf0, 0x00, 0xc8, 0xf7, 0xda, 0x5b, 0xda, 0x26,
	0x84, 0xf1, 0x57, 0xaf, 0xbf, 0x7b, 0xce, 0xf9, 0xfc, 0x9d, 0x7b, 0x7d, 0x1a, 0xf8, 0xc4, 0x09,
	0x43, 0xc7, 0x23, 0x9d, 0x99, 0xeb, 0x24, 0x78, 0xe6, 0x91, 0x0e, 0x9e, 0xfb, 0x6e, 0xd0, 0xb9,
	0x92, 0x3b, 0xf4, 0x51, 0x5a, 0x44, 0x61, 0x12, 0xa2, 0x3a, 0x43, 0x49, 0x05, 0x4a, 0xa2, 0x28,
	0xe9, 0x4a, 0x6e, 0x3c, 0xc8, 0xf9, 0x78, 0xe1, 0x76, 0x70, 0x10, 0x84, 0x09, 0x4e, 0xdc, 0x30,
	0x88, 0x19, 0xaf, 0xf1, 0x30, 0xdf, 0xa5, 0x4f, 0xb3, 0xf4, 0x75, 0x67, 0x9e, 0x46, 0x14, 0x90,
	0xef, 0x3f, 0xba, 0xbd, 0x9f, 0xb8, 0x3e, 0x89, 0x13, 0xec, 0x2f, 0x18, 0xa0, 0xf5, 0xfb, 0x2e,
	0x08, 0x66, 0xd6, 0x11, 0x21, 0x28, 0x07, 0xd8, 0x27, 0x75, 0xae, 0xc9, 0xb5, 0xf7, 0x0d, 0xba,
	0x46, 0x97, 0x70, 0x68, 0x7b, 0x69, 0x9c, 0x90, 0xc8, 0x8a, 0x13, 0x9c, 0x90, 0xb8, 0x5e, 0x6a,
	0xf2, 0xed, 0xaa, 0x2c, 0x4b, 0x9b, 0xde, 0x57, 0xa2, 0x62, 0x52, 0x8f, 0xb1, 0x46, 0x94, 0xa4,
	0x06, 0x49, 0xb4, 0x34, 0x0e, 0xec, 0xd5, 0x1a, 0x9a, 0xc2, 0x91, 0x1d, 0x7a, 0xa9, 0x1f, 0x58,
	0xaf, 0xb1, 0xef, 0x7a, 0x2e, 0x89, 0xeb, 0x3c, 0xd5, 0x7e, 0xb2, 0x55, 0x9b, 0xd2, 0x4e, 0x73,
	0x16, 0x13, 0x3f, 0xb4, 0x6f, 0x14, 0xd1, 0x04, 0xaa, 0x4e, 0x84, 0x83, 0xd4, 0xc3, 0x91, 0x9b,
	0x2c, 0xeb, 0xe5, 0x26, 0xd7, 0x3e, 0x94, 0x9f, 0x6e, 0x53, 0x36, 0x0b, 0x73, 0xce, 0xde, 0x71,
	0x8d, 0x55, 0xa1, 0xc6, 0xdf, 0x1c, 0xd4, 0x56, 0x67, 0x43, 0x3f, 0xc1, 0x07, 0x11, 0x59, 0x78,
	0xae, 0x4d, 0x5d, 0x67, 0x2e, 0x51, 0x0b, 0x0f, 0xe5, 0xe7, 0xef, 0x63, 0x92, 0x64, 0xbc, 0x53,
	0xa1, 0x05, 0x43, 0x8c, 0x6e, 0x55, 0x5a, 0xd7, 0x20, 0xde, 0x46, 0xa1, 0x63, 0x38, 0x1a, 0x99,
	0x8a, 0xa9, 0x5a, 0xfa, 0x85, 0x69, 0xfd, 0xa0, 0x5f, 0xbc, 0xd4, 0xc5, 0x1d, 0x24, 0x42, 0x4d,
	0xd3, 0x35, 0x53, 0x53, 0xfa, 0xda, 0x2b, 0x4d, 0x3f, 0x13, 0x39, 0xf4, 0x11, 0x1c, 0x0f, 0xfb,
	0x8a, 0xae, 0xab, 0x27, 0xd6, 0x40, 0xd1, 0x74, 0x53, 0xd5, 0x15, 0xbd, 0xa7, 0x8a, 0x25, 0x74,
	0x1f, 0x3e, 0x1c, 0xeb, 0xeb, 0xb6, 0x78, 0xb4, 0x0f, 0x82, 0xa1, 0x2a, 0x27, 0x97, 0x62, 0xb9,
	0x11, 0x00, 0xba, 0x7b, 0xa2, 0x48, 0x04, 0xfe, 0x67, 0xb2, 0xcc, 0x2f, 0x4c, 0xb6, 0x44, 0x5d,
	0x10, 0xae, 0xb0, 0x97, 0x92, 0x7a, 0xa9, 0xc9, 0xb5, 0xab, 0xf2, 0xe7, 0xef, 0xe3, 0x80, 0xc1,
	0xa8, 0xdf, 0x96, 0xbe, 0xe6, 0x1a, 0x2e, 0x1c, 0xaf, 0x39, 0xe5, 0x35, 0x0d, 0xbf, 0xbb, 0xd9,
	0xf0, 0xf1, 0xe6, 0x86, 0x2b, 0x7a, 0xcb, 0x95, 0x56, 0x2d, 0x0d, 0xee, 0xad, 0x3b, 0x76, 0xf4,
	0x29, 0x7c, 0x6c, 0x6a, 0x03, 0x75, 0x64, 0x2a, 0x83, 0xa1, 0x75, 0x66, 0x28, 0xfa, 0xb8, 0xaf,
	0x18, 0x9a, 0x79, 0x69, 0x8d, 0xf5, 0xd1, 0x50, 0xed, 0x69, 0xa7, 0x9a, 0x7a, 0x22, 0xee, 0x20,
	0x80, 0xca, 0x40, 0xeb, 0xf7, 0xb5, 0x91, 0xc8, 0xb5, 0xa6, 0x50, 0x9e, 0xb8, 0xe4, 0x17, 0x74,
	0x0f, 0xc4, 0x89, 0xa6, 0xbe, 0xbc, 0x85, 0x3c, 0x80, 0x7d, 0x5d, 0x19, 0xa8, 0xd6, 0x85, 0xde,
	0xbf, 0x14, 0x39, 0x74, 0x04, 0xd5, 0x51, 0xef, 0x5c, 0x1d, 0x28, 0x56, 0x86, 0x15, 0x4b, 0x19,
	0xcb, 0x50, 0x87, 0x7d, 0xad, 0xa7, 0x98, 0xda, 0x85, 0xce, 0xaa, 0x3c, 0xda, 0x83, 0xf2, 0xe9,
	0xb8, 0xdf, 0x17, 0xcb, 0x2d, 0x0d, 0x6a, 0xab, 0x33, 0xa0, 0x6f, 0x60, 0xd7, 0xb1, 0xad, 0x28,
	0xf5, 0xd8, 0x7d, 0xab, 0xca, 0xcd, 0xcd, 0xc3, 0x9f, 0xd9, 0x46, 0xea, 0x11, 0xa3, 0xe2, 0xd0,
	0xbf, 0xad, 0x5f, 0x79, 0xa8, 0xb0, 0x12, 0xfa, 0x0c, 0x44, 0x1f, 0x5f, 0x5b, 0x41, 0xea, 0x5b,
	0x57, 0x24, 0x8a, 0xb3, 0x68, 0xa1, 0x72, 0xc2, 0xf9, 0x8e, 0x71, 0xe8, 0xe3, 0x6b, 0x3d, 0xf5,
	0x27, 0x79, 0x1d, 0x3d, 0x85, 0xdd, 0x0c, 0x8b, 0x9d, 0xc2, 0xee, 0xfb, 0x45, 0xc7, 0x22, 0x5e,
	0xa4, 0x93, 0x3c, 0x7e, 0xce, 0x77, 0x8c, 0x8a, 0x8f, 0xaf, 0x15, 0x87, 0xa0, 0x11, 0xd4, 0xdc,
	0x20, 0x21, 0x51, 0x4c, 0xec, 0x6c, 0xa7, 0xce, 0x53, 0xea, 0x17, 0xdb, 0x5e, 0x56, 0xd2, 0x56,
	0x48, 0xe7, 0x3b, 0xc6, 0x0d, 0x11, 0xf4, 0x02, 0x84, 0x34, 0xc8, 0xd4, 0xca, 0xdb, 0xce, 0x3d,
	0x57, 0x1b, 0x07, 0x4c, 0x86, 0xd1, 0x1a, 0xa7, 0x50, 0x5b, 0xd5, 0x47, 0x5f, 0x81, 0x90, 0x39,
	0x99, 0xcd, 0xce, 0xff, 0x27, 0x2b, 0x19, 0xbc, 0xf1, 0x3d, 0x08, 0x54, 0xf9, 0xff, 0x0a, 0x74,
	0x2b, 0x50, 0xce, 0x16, 0xad, 0xdf, 0x78, 0xd8, 0x1b, 0x05, 0x78, 0x11, 0xff, 0x18, 0x26, 0x6b,
	0xa3, 0xb8, 0x0b, 0xb5, 0x38, 0x4c, 0x23, 0x9b, 0x58, 0x54, 0x2f, 0x3f, 0x81, 0x47, 0x5b, 0xbe,
	0x30, 0xa3, 0xca, 0x48, 0x2c, 0xe2, 0x1f, 0xc3, 0xd1, 0x1c, 0x27, 0xd8, 0x8a, 0xdd, 0x37, 0xc4,
	0x9a, 0x2d, 0x13, 0x9a, 0xb9, 0x5c, 0x9b, 0x37, 0x0e, 0xb2, 0xf2, 0xc8, 0x7d, 0x43, 0xba, 0x59,
	0x11, 0x3d, 0x83, 0xaa, 0x1d, 0x11, 0x9c, 0x10, 0x2b, 0xfb, 0x77, 0x91, 0x7b, 0xdc, 0xb8, 0x73,
	0xd8, 0x6f, 0xbf, 0x1b, 0x03, 0x18, 0x3c, 0x2b, 0x64, 0xe4, 0x39, 0xf1, 0x48, 0x41, 0x16, 0xb6,
	0x93, 0x19, 0x9c, 0x92, 0x5f, 0x80, 0xc0, 0x22, 0xb4, 0x42, 0x23, 0xb4, 0xbd, 0x79, 0xbc, 0xc2,
	0x2c, 0x29, 0x0f, 0x0f, 0x4a, 0x43, 0xcd, 0xac, 0x79, 0x6c, 0x47, 0xee, 0x82, 0xde, 0xb5, 0x5d,
	0x6a, 0xe0, 0x6a, 0xa9, 0xf5, 0x25, 0x08, 0xff, 0x92, 0x9c, 0x6f, 0x33, 0x8f, 0x43, 0x35, 0xd8,
	0xeb, 0x19, 0xaa, 0x62, 0x66, 0x01, 0x5a, 0xea, 0xfe, 0xc9, 0xc1, 0x03, 0x3b, 0xf4, 0x37, 0xbe,
	0x4f, 0x17, 0xa8, 0xc5, 0xc3, 0x6c, 0xbc, 0x21, 0xf7, 0xea, 0x79, 0x8e, 0x73, 0x42, 0x0f, 0x07,
	0x8e, 0x14, 0x46, 0x4e, 0xc7, 0x21, 0x01, 0x1d, 0xbe, 0xc3, 0xb6, 0xf0, 0xc2, 0x8d, 0xef, 0xfe,
	0x28, 0x78, 0x46, 0x17, 0x7f, 0x94, 0x1e, 0x9e, 0x31, 0x7e, 0xcf, 0x0b, 0xd3, 0xb9, 0xd4, 0x2d,
	0xba, 0x29, 0xb4, 0xdb, 0x44, 0xfe, 0xab, 0x00, 0x4c, 0x29, 0x60, 0x5a, 0x00, 0xa6, 0x14, 0x30,
	0x9d, 0xc8, 0xb3, 0x0a, 0xed, 0xf5, 0xe4, 0x9f, 0x00, 0x00, 0x00, 0xff, 0xff, 0x56, 0x59, 0xa7,
	0xc1, 0x7f, 0x08, 0x00, 0x00,
}
