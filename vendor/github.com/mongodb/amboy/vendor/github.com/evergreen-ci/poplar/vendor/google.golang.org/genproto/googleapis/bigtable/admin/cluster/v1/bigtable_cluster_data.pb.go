// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/bigtable/admin/cluster/v1/bigtable_cluster_data.proto

package cluster // import "google.golang.org/genproto/googleapis/bigtable/admin/cluster/v1"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import _ "github.com/golang/protobuf/ptypes/timestamp"
import _ "google.golang.org/genproto/googleapis/api/annotations"
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

type StorageType int32

const (
	// The storage type used is unspecified.
	StorageType_STORAGE_UNSPECIFIED StorageType = 0
	// Data will be stored in SSD, providing low and consistent latencies.
	StorageType_STORAGE_SSD StorageType = 1
	// Data will be stored in HDD, providing high and less predictable
	// latencies.
	StorageType_STORAGE_HDD StorageType = 2
)

var StorageType_name = map[int32]string{
	0: "STORAGE_UNSPECIFIED",
	1: "STORAGE_SSD",
	2: "STORAGE_HDD",
}
var StorageType_value = map[string]int32{
	"STORAGE_UNSPECIFIED": 0,
	"STORAGE_SSD":         1,
	"STORAGE_HDD":         2,
}

func (x StorageType) String() string {
	return proto.EnumName(StorageType_name, int32(x))
}
func (StorageType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_cluster_data_5751b30eb8ec0701, []int{0}
}

// Possible states of a zone.
type Zone_Status int32

const (
	// The state of the zone is unknown or unspecified.
	Zone_UNKNOWN Zone_Status = 0
	// The zone is in a good state.
	Zone_OK Zone_Status = 1
	// The zone is down for planned maintenance.
	Zone_PLANNED_MAINTENANCE Zone_Status = 2
	// The zone is down for emergency or unplanned maintenance.
	Zone_EMERGENCY_MAINENANCE Zone_Status = 3
)

var Zone_Status_name = map[int32]string{
	0: "UNKNOWN",
	1: "OK",
	2: "PLANNED_MAINTENANCE",
	3: "EMERGENCY_MAINENANCE",
}
var Zone_Status_value = map[string]int32{
	"UNKNOWN":              0,
	"OK":                   1,
	"PLANNED_MAINTENANCE":  2,
	"EMERGENCY_MAINENANCE": 3,
}

func (x Zone_Status) String() string {
	return proto.EnumName(Zone_Status_name, int32(x))
}
func (Zone_Status) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_cluster_data_5751b30eb8ec0701, []int{0, 0}
}

// A physical location in which a particular project can allocate Cloud BigTable
// resources.
type Zone struct {
	// A permanent unique identifier for the zone.
	// Values are of the form projects/<project>/zones/[a-z][-a-z0-9]*
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The name of this zone as it appears in UIs.
	DisplayName string `protobuf:"bytes,2,opt,name=display_name,json=displayName,proto3" json:"display_name,omitempty"`
	// The current state of this zone.
	Status               Zone_Status `protobuf:"varint,3,opt,name=status,proto3,enum=google.bigtable.admin.cluster.v1.Zone_Status" json:"status,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *Zone) Reset()         { *m = Zone{} }
func (m *Zone) String() string { return proto.CompactTextString(m) }
func (*Zone) ProtoMessage()    {}
func (*Zone) Descriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_cluster_data_5751b30eb8ec0701, []int{0}
}
func (m *Zone) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Zone.Unmarshal(m, b)
}
func (m *Zone) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Zone.Marshal(b, m, deterministic)
}
func (dst *Zone) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Zone.Merge(dst, src)
}
func (m *Zone) XXX_Size() int {
	return xxx_messageInfo_Zone.Size(m)
}
func (m *Zone) XXX_DiscardUnknown() {
	xxx_messageInfo_Zone.DiscardUnknown(m)
}

var xxx_messageInfo_Zone proto.InternalMessageInfo

func (m *Zone) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Zone) GetDisplayName() string {
	if m != nil {
		return m.DisplayName
	}
	return ""
}

func (m *Zone) GetStatus() Zone_Status {
	if m != nil {
		return m.Status
	}
	return Zone_UNKNOWN
}

// An isolated set of Cloud BigTable resources on which tables can be hosted.
type Cluster struct {
	// A permanent unique identifier for the cluster. For technical reasons, the
	// zone in which the cluster resides is included here.
	// Values are of the form
	// projects/<project>/zones/<zone>/clusters/[a-z][-a-z0-9]*
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The operation currently running on the cluster, if any.
	// This cannot be set directly, only through CreateCluster, UpdateCluster,
	// or UndeleteCluster. Calls to these methods will be rejected if
	// "current_operation" is already set.
	CurrentOperation *longrunning.Operation `protobuf:"bytes,3,opt,name=current_operation,json=currentOperation,proto3" json:"current_operation,omitempty"`
	// The descriptive name for this cluster as it appears in UIs.
	// Must be unique per zone.
	DisplayName string `protobuf:"bytes,4,opt,name=display_name,json=displayName,proto3" json:"display_name,omitempty"`
	// The number of serve nodes allocated to this cluster.
	ServeNodes int32 `protobuf:"varint,5,opt,name=serve_nodes,json=serveNodes,proto3" json:"serve_nodes,omitempty"`
	// What storage type to use for tables in this cluster. Only configurable at
	// cluster creation time. If unspecified, STORAGE_SSD will be used.
	DefaultStorageType   StorageType `protobuf:"varint,8,opt,name=default_storage_type,json=defaultStorageType,proto3,enum=google.bigtable.admin.cluster.v1.StorageType" json:"default_storage_type,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *Cluster) Reset()         { *m = Cluster{} }
func (m *Cluster) String() string { return proto.CompactTextString(m) }
func (*Cluster) ProtoMessage()    {}
func (*Cluster) Descriptor() ([]byte, []int) {
	return fileDescriptor_bigtable_cluster_data_5751b30eb8ec0701, []int{1}
}
func (m *Cluster) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Cluster.Unmarshal(m, b)
}
func (m *Cluster) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Cluster.Marshal(b, m, deterministic)
}
func (dst *Cluster) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Cluster.Merge(dst, src)
}
func (m *Cluster) XXX_Size() int {
	return xxx_messageInfo_Cluster.Size(m)
}
func (m *Cluster) XXX_DiscardUnknown() {
	xxx_messageInfo_Cluster.DiscardUnknown(m)
}

var xxx_messageInfo_Cluster proto.InternalMessageInfo

func (m *Cluster) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Cluster) GetCurrentOperation() *longrunning.Operation {
	if m != nil {
		return m.CurrentOperation
	}
	return nil
}

func (m *Cluster) GetDisplayName() string {
	if m != nil {
		return m.DisplayName
	}
	return ""
}

func (m *Cluster) GetServeNodes() int32 {
	if m != nil {
		return m.ServeNodes
	}
	return 0
}

func (m *Cluster) GetDefaultStorageType() StorageType {
	if m != nil {
		return m.DefaultStorageType
	}
	return StorageType_STORAGE_UNSPECIFIED
}

func init() {
	proto.RegisterType((*Zone)(nil), "google.bigtable.admin.cluster.v1.Zone")
	proto.RegisterType((*Cluster)(nil), "google.bigtable.admin.cluster.v1.Cluster")
	proto.RegisterEnum("google.bigtable.admin.cluster.v1.StorageType", StorageType_name, StorageType_value)
	proto.RegisterEnum("google.bigtable.admin.cluster.v1.Zone_Status", Zone_Status_name, Zone_Status_value)
}

func init() {
	proto.RegisterFile("google/bigtable/admin/cluster/v1/bigtable_cluster_data.proto", fileDescriptor_bigtable_cluster_data_5751b30eb8ec0701)
}

var fileDescriptor_bigtable_cluster_data_5751b30eb8ec0701 = []byte{
	// 493 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x93, 0xd1, 0x6e, 0xd3, 0x3c,
	0x1c, 0xc5, 0x97, 0xae, 0xeb, 0xbe, 0xcf, 0x41, 0x10, 0xcc, 0x24, 0xa2, 0x09, 0xb4, 0x52, 0xb8,
	0xa8, 0x90, 0x70, 0xb4, 0x71, 0x09, 0x37, 0x6d, 0x63, 0xba, 0x32, 0xe6, 0x56, 0x49, 0x27, 0xc4,
	0x6e, 0x2c, 0xb7, 0xf5, 0xac, 0x48, 0xa9, 0x1d, 0xc5, 0x4e, 0xa5, 0x3e, 0x03, 0x12, 0x8f, 0xc7,
	0xf3, 0xa0, 0x3a, 0x6e, 0x55, 0x34, 0xd0, 0xb8, 0xb3, 0xcf, 0x39, 0x3f, 0xbb, 0xff, 0x53, 0x07,
	0x7c, 0x14, 0x4a, 0x89, 0x9c, 0x47, 0xb3, 0x4c, 0x18, 0x36, 0xcb, 0x79, 0xc4, 0x16, 0xcb, 0x4c,
	0x46, 0xf3, 0xbc, 0xd2, 0x86, 0x97, 0xd1, 0xea, 0x7c, 0xe7, 0x50, 0xa7, 0xd1, 0x05, 0x33, 0x0c,
	0x15, 0xa5, 0x32, 0x0a, 0xb6, 0x6b, 0x1a, 0x6d, 0x33, 0xc8, 0xd2, 0xc8, 0x25, 0xd1, 0xea, 0xfc,
	0xf4, 0x85, 0x3b, 0x9f, 0x15, 0x59, 0xc4, 0xa4, 0x54, 0x86, 0x99, 0x4c, 0x49, 0x5d, 0xf3, 0xa7,
	0xaf, 0x9d, 0x9b, 0x2b, 0x29, 0xca, 0x4a, 0xca, 0x4c, 0x8a, 0x48, 0x15, 0xbc, 0xfc, 0x2d, 0x74,
	0xe6, 0x42, 0x76, 0x37, 0xab, 0xee, 0x22, 0x93, 0x2d, 0xb9, 0x36, 0x6c, 0x59, 0xd4, 0x81, 0xce,
	0x4f, 0x0f, 0x34, 0x6f, 0x95, 0xe4, 0x10, 0x82, 0xa6, 0x64, 0x4b, 0x1e, 0x7a, 0x6d, 0xaf, 0xfb,
	0x7f, 0x62, 0xd7, 0xf0, 0x15, 0x78, 0xb4, 0xc8, 0x74, 0x91, 0xb3, 0x35, 0xb5, 0x5e, 0xc3, 0x7a,
	0xbe, 0xd3, 0xc8, 0x26, 0x82, 0x41, 0x4b, 0x1b, 0x66, 0x2a, 0x1d, 0x1e, 0xb6, 0xbd, 0xee, 0xe3,
	0x8b, 0x77, 0xe8, 0xa1, 0xb1, 0xd0, 0xe6, 0x3a, 0x94, 0x5a, 0x28, 0x71, 0x70, 0x67, 0x02, 0x5a,
	0xb5, 0x02, 0x7d, 0x70, 0x7c, 0x43, 0xae, 0xc8, 0xf8, 0x2b, 0x09, 0x0e, 0x60, 0x0b, 0x34, 0xc6,
	0x57, 0x81, 0x07, 0x9f, 0x83, 0x67, 0x93, 0x2f, 0x3d, 0x42, 0x70, 0x4c, 0xaf, 0x7b, 0x23, 0x32,
	0xc5, 0xa4, 0x47, 0x06, 0x38, 0x68, 0xc0, 0x10, 0x9c, 0xe0, 0x6b, 0x9c, 0x0c, 0x31, 0x19, 0x7c,
	0xb3, 0x96, 0x73, 0x0e, 0x3b, 0x3f, 0x1a, 0xe0, 0x78, 0x50, 0x5f, 0xfa, 0xc7, 0xd9, 0x3e, 0x83,
	0xa7, 0xf3, 0xaa, 0x2c, 0xb9, 0x34, 0x74, 0xd7, 0x9a, 0x9d, 0xc1, 0xbf, 0x78, 0xb9, 0x9d, 0x61,
	0xaf, 0x5a, 0x34, 0xde, 0x86, 0x92, 0xc0, 0x71, 0x3b, 0xe5, 0x5e, 0x4f, 0xcd, 0xfb, 0x3d, 0x9d,
	0x01, 0x5f, 0xf3, 0x72, 0xc5, 0xa9, 0x54, 0x0b, 0xae, 0xc3, 0xa3, 0xb6, 0xd7, 0x3d, 0x4a, 0x80,
	0x95, 0xc8, 0x46, 0x81, 0x14, 0x9c, 0x2c, 0xf8, 0x1d, 0xab, 0x72, 0x43, 0xb5, 0x51, 0x25, 0x13,
	0x9c, 0x9a, 0x75, 0xc1, 0xc3, 0xff, 0xfe, 0xb5, 0xd6, 0xb4, 0xa6, 0xa6, 0xeb, 0x82, 0x27, 0xd0,
	0x1d, 0xb5, 0xa7, 0xbd, 0xbd, 0x04, 0xfe, 0xde, 0x76, 0x53, 0x69, 0x3a, 0x1d, 0x27, 0xbd, 0x21,
	0xa6, 0x37, 0x24, 0x9d, 0xe0, 0xc1, 0xe8, 0xd3, 0x08, 0xc7, 0xc1, 0x01, 0x7c, 0x02, 0xfc, 0xad,
	0x91, 0xa6, 0x71, 0xe0, 0xed, 0x0b, 0x97, 0x71, 0x1c, 0x34, 0xfa, 0xdf, 0x3d, 0xf0, 0x66, 0xae,
	0x96, 0x0f, 0xfe, 0xa4, 0x7e, 0xd8, 0x77, 0x96, 0xfb, 0x23, 0x62, 0x66, 0xd8, 0x64, 0xf3, 0xec,
	0x26, 0xde, 0xed, 0xd0, 0xd1, 0x42, 0xe5, 0x4c, 0x0a, 0xa4, 0x4a, 0x11, 0x09, 0x2e, 0xed, 0xa3,
	0x8c, 0x6a, 0x8b, 0x15, 0x99, 0xfe, 0xfb, 0xb7, 0xf5, 0xc1, 0x2d, 0x67, 0x2d, 0xcb, 0xbc, 0xff,
	0x15, 0x00, 0x00, 0xff, 0xff, 0xc9, 0x27, 0x25, 0xa6, 0x8e, 0x03, 0x00, 0x00,
}
