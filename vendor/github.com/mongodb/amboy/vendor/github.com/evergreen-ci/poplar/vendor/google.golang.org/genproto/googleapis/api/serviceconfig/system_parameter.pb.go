// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/api/system_parameter.proto

package serviceconfig // import "google.golang.org/genproto/googleapis/api/serviceconfig"

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

// ### System parameter configuration
//
// A system parameter is a special kind of parameter defined by the API
// system, not by an individual API. It is typically mapped to an HTTP header
// and/or a URL query parameter. This configuration specifies which methods
// change the names of the system parameters.
type SystemParameters struct {
	// Define system parameters.
	//
	// The parameters defined here will override the default parameters
	// implemented by the system. If this field is missing from the service
	// config, default system parameters will be used. Default system parameters
	// and names is implementation-dependent.
	//
	// Example: define api key for all methods
	//
	//     system_parameters
	//       rules:
	//         - selector: "*"
	//           parameters:
	//             - name: api_key
	//               url_query_parameter: api_key
	//
	//
	// Example: define 2 api key names for a specific method.
	//
	//     system_parameters
	//       rules:
	//         - selector: "/ListShelves"
	//           parameters:
	//             - name: api_key
	//               http_header: Api-Key1
	//             - name: api_key
	//               http_header: Api-Key2
	//
	// **NOTE:** All service configuration rules follow "last one wins" order.
	Rules                []*SystemParameterRule `protobuf:"bytes,1,rep,name=rules,proto3" json:"rules,omitempty"`
	XXX_NoUnkeyedLiteral struct{}               `json:"-"`
	XXX_unrecognized     []byte                 `json:"-"`
	XXX_sizecache        int32                  `json:"-"`
}

func (m *SystemParameters) Reset()         { *m = SystemParameters{} }
func (m *SystemParameters) String() string { return proto.CompactTextString(m) }
func (*SystemParameters) ProtoMessage()    {}
func (*SystemParameters) Descriptor() ([]byte, []int) {
	return fileDescriptor_system_parameter_260dda33a71a8c82, []int{0}
}
func (m *SystemParameters) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SystemParameters.Unmarshal(m, b)
}
func (m *SystemParameters) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SystemParameters.Marshal(b, m, deterministic)
}
func (dst *SystemParameters) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SystemParameters.Merge(dst, src)
}
func (m *SystemParameters) XXX_Size() int {
	return xxx_messageInfo_SystemParameters.Size(m)
}
func (m *SystemParameters) XXX_DiscardUnknown() {
	xxx_messageInfo_SystemParameters.DiscardUnknown(m)
}

var xxx_messageInfo_SystemParameters proto.InternalMessageInfo

func (m *SystemParameters) GetRules() []*SystemParameterRule {
	if m != nil {
		return m.Rules
	}
	return nil
}

// Define a system parameter rule mapping system parameter definitions to
// methods.
type SystemParameterRule struct {
	// Selects the methods to which this rule applies. Use '*' to indicate all
	// methods in all APIs.
	//
	// Refer to [selector][google.api.DocumentationRule.selector] for syntax details.
	Selector string `protobuf:"bytes,1,opt,name=selector,proto3" json:"selector,omitempty"`
	// Define parameters. Multiple names may be defined for a parameter.
	// For a given method call, only one of them should be used. If multiple
	// names are used the behavior is implementation-dependent.
	// If none of the specified names are present the behavior is
	// parameter-dependent.
	Parameters           []*SystemParameter `protobuf:"bytes,2,rep,name=parameters,proto3" json:"parameters,omitempty"`
	XXX_NoUnkeyedLiteral struct{}           `json:"-"`
	XXX_unrecognized     []byte             `json:"-"`
	XXX_sizecache        int32              `json:"-"`
}

func (m *SystemParameterRule) Reset()         { *m = SystemParameterRule{} }
func (m *SystemParameterRule) String() string { return proto.CompactTextString(m) }
func (*SystemParameterRule) ProtoMessage()    {}
func (*SystemParameterRule) Descriptor() ([]byte, []int) {
	return fileDescriptor_system_parameter_260dda33a71a8c82, []int{1}
}
func (m *SystemParameterRule) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SystemParameterRule.Unmarshal(m, b)
}
func (m *SystemParameterRule) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SystemParameterRule.Marshal(b, m, deterministic)
}
func (dst *SystemParameterRule) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SystemParameterRule.Merge(dst, src)
}
func (m *SystemParameterRule) XXX_Size() int {
	return xxx_messageInfo_SystemParameterRule.Size(m)
}
func (m *SystemParameterRule) XXX_DiscardUnknown() {
	xxx_messageInfo_SystemParameterRule.DiscardUnknown(m)
}

var xxx_messageInfo_SystemParameterRule proto.InternalMessageInfo

func (m *SystemParameterRule) GetSelector() string {
	if m != nil {
		return m.Selector
	}
	return ""
}

func (m *SystemParameterRule) GetParameters() []*SystemParameter {
	if m != nil {
		return m.Parameters
	}
	return nil
}

// Define a parameter's name and location. The parameter may be passed as either
// an HTTP header or a URL query parameter, and if both are passed the behavior
// is implementation-dependent.
type SystemParameter struct {
	// Define the name of the parameter, such as "api_key" . It is case sensitive.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Define the HTTP header name to use for the parameter. It is case
	// insensitive.
	HttpHeader string `protobuf:"bytes,2,opt,name=http_header,json=httpHeader,proto3" json:"http_header,omitempty"`
	// Define the URL query parameter name to use for the parameter. It is case
	// sensitive.
	UrlQueryParameter    string   `protobuf:"bytes,3,opt,name=url_query_parameter,json=urlQueryParameter,proto3" json:"url_query_parameter,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SystemParameter) Reset()         { *m = SystemParameter{} }
func (m *SystemParameter) String() string { return proto.CompactTextString(m) }
func (*SystemParameter) ProtoMessage()    {}
func (*SystemParameter) Descriptor() ([]byte, []int) {
	return fileDescriptor_system_parameter_260dda33a71a8c82, []int{2}
}
func (m *SystemParameter) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SystemParameter.Unmarshal(m, b)
}
func (m *SystemParameter) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SystemParameter.Marshal(b, m, deterministic)
}
func (dst *SystemParameter) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SystemParameter.Merge(dst, src)
}
func (m *SystemParameter) XXX_Size() int {
	return xxx_messageInfo_SystemParameter.Size(m)
}
func (m *SystemParameter) XXX_DiscardUnknown() {
	xxx_messageInfo_SystemParameter.DiscardUnknown(m)
}

var xxx_messageInfo_SystemParameter proto.InternalMessageInfo

func (m *SystemParameter) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *SystemParameter) GetHttpHeader() string {
	if m != nil {
		return m.HttpHeader
	}
	return ""
}

func (m *SystemParameter) GetUrlQueryParameter() string {
	if m != nil {
		return m.UrlQueryParameter
	}
	return ""
}

func init() {
	proto.RegisterType((*SystemParameters)(nil), "google.api.SystemParameters")
	proto.RegisterType((*SystemParameterRule)(nil), "google.api.SystemParameterRule")
	proto.RegisterType((*SystemParameter)(nil), "google.api.SystemParameter")
}

func init() {
	proto.RegisterFile("google/api/system_parameter.proto", fileDescriptor_system_parameter_260dda33a71a8c82)
}

var fileDescriptor_system_parameter_260dda33a71a8c82 = []byte{
	// 286 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x91, 0xbf, 0x4e, 0xc3, 0x30,
	0x10, 0x87, 0x95, 0xb6, 0x20, 0xb8, 0x4a, 0xfc, 0x71, 0x19, 0x22, 0x18, 0x5a, 0x3a, 0x75, 0x72,
	0x24, 0x10, 0x53, 0x27, 0x2a, 0x21, 0xe8, 0x16, 0xca, 0xc6, 0x12, 0x99, 0x70, 0xb8, 0x91, 0x9c,
	0xd8, 0x9c, 0x9d, 0x48, 0x7d, 0x1d, 0x9e, 0x14, 0xc5, 0x29, 0x69, 0x89, 0x10, 0x9b, 0xef, 0xbe,
	0xcf, 0xfa, 0x9d, 0xee, 0xe0, 0x5a, 0x6a, 0x2d, 0x15, 0x46, 0xc2, 0x64, 0x91, 0xdd, 0x58, 0x87,
	0x79, 0x62, 0x04, 0x89, 0x1c, 0x1d, 0x12, 0x37, 0xa4, 0x9d, 0x66, 0xd0, 0x28, 0x5c, 0x98, 0x6c,
	0xba, 0x84, 0xb3, 0x17, 0x6f, 0xc5, 0x3f, 0x92, 0x65, 0x77, 0x70, 0x40, 0xa5, 0x42, 0x1b, 0x06,
	0x93, 0xfe, 0x6c, 0x78, 0x33, 0xe6, 0x3b, 0x9f, 0x77, 0xe4, 0x55, 0xa9, 0x70, 0xd5, 0xd8, 0xd3,
	0x02, 0x46, 0x7f, 0x50, 0x76, 0x09, 0x47, 0x16, 0x15, 0xa6, 0x4e, 0x53, 0x18, 0x4c, 0x82, 0xd9,
	0xf1, 0xaa, 0xad, 0xd9, 0x1c, 0xa0, 0x1d, 0xce, 0x86, 0x3d, 0x1f, 0x77, 0xf5, 0x5f, 0xdc, 0x9e,
	0x3e, 0xad, 0xe0, 0xb4, 0x83, 0x19, 0x83, 0x41, 0x21, 0x72, 0xdc, 0xe6, 0xf8, 0x37, 0x1b, 0xc3,
	0x70, 0xed, 0x9c, 0x49, 0xd6, 0x28, 0xde, 0x91, 0xc2, 0x9e, 0x47, 0x50, 0xb7, 0x9e, 0x7c, 0x87,
	0x71, 0x18, 0x95, 0xa4, 0x92, 0xcf, 0x12, 0x69, 0xb3, 0xdb, 0x55, 0xd8, 0xf7, 0xe2, 0x79, 0x49,
	0xea, 0xb9, 0x26, 0x6d, 0xc8, 0xa2, 0x82, 0x93, 0x54, 0xe7, 0x7b, 0x53, 0x2e, 0x2e, 0x3a, 0x73,
	0xc4, 0xf5, 0x9a, 0xe3, 0xe0, 0xf5, 0x61, 0xeb, 0x48, 0xad, 0x44, 0x21, 0xb9, 0x26, 0x19, 0x49,
	0x2c, 0xfc, 0x11, 0xa2, 0x06, 0x09, 0x93, 0xd9, 0xe6, 0x54, 0x48, 0x55, 0x96, 0x62, 0xaa, 0x8b,
	0x8f, 0x4c, 0xce, 0x7f, 0x55, 0x5f, 0xbd, 0xc1, 0xe3, 0x7d, 0xbc, 0x7c, 0x3b, 0xf4, 0x1f, 0x6f,
	0xbf, 0x03, 0x00, 0x00, 0xff, 0xff, 0x5e, 0xdf, 0x2e, 0x09, 0xe2, 0x01, 0x00, 0x00,
}
