// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// source: internal/testprotos/legacy/bug1052/bug1052.proto

package bug1052

import (
	fmt "fmt"
	math "math"

	proto "google.golang.org/protobuf/internal/protolegacy"
	descriptor "google.golang.org/protobuf/types/descriptorpb"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type Enum int32

const (
	Enum_ZERO Enum = 0
)

var Enum_name = map[int32]string{
	0: "ZERO",
}

var Enum_value = map[string]int32{
	"ZERO": 0,
}

func (x Enum) Enum() *Enum {
	p := new(Enum)
	*p = x
	return p
}

func (x Enum) String() string {
	return proto.EnumName(Enum_name, int32(x))
}

func (x *Enum) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(Enum_value, data, "Enum")
	if err != nil {
		return err
	}
	*x = Enum(value)
	return nil
}

func (Enum) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_2d34d4f47c51af27, []int{0}
}

var E_ExtensionEnum = &proto.ExtensionDesc{
	ExtendedType:  (*descriptor.MethodOptions)(nil),
	ExtensionType: (*Enum)(nil),
	Field:         5000,
	Name:          "goproto.proto.legacy.extension_enum",
	Tag:           "varint,5000,opt,name=extension_enum,enum=goproto.proto.legacy.Enum",
	Filename:      "internal/testprotos/legacy/bug1052/bug1052.proto",
}

func init() {
	proto.RegisterEnum("goproto.proto.legacy.Enum", Enum_name, Enum_value)
	proto.RegisterExtension(E_ExtensionEnum)
}

func init() {
	proto.RegisterFile("internal/testprotos/legacy/bug1052/bug1052.proto", fileDescriptor_2d34d4f47c51af27)
}

var fileDescriptor_2d34d4f47c51af27 = []byte{
	// 200 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x32, 0xc8, 0xcc, 0x2b, 0x49,
	0x2d, 0xca, 0x4b, 0xcc, 0xd1, 0x2f, 0x49, 0x2d, 0x2e, 0x29, 0x28, 0xca, 0x2f, 0xc9, 0x2f, 0xd6,
	0xcf, 0x49, 0x4d, 0x4f, 0x4c, 0xae, 0xd4, 0x4f, 0x2a, 0x4d, 0x37, 0x34, 0x30, 0x35, 0x82, 0xd1,
	0x7a, 0x60, 0x59, 0x21, 0x91, 0xf4, 0x7c, 0x30, 0x03, 0xc2, 0xd5, 0x83, 0xa8, 0x95, 0x52, 0x48,
	0xcf, 0xcf, 0x4f, 0xcf, 0x49, 0xd5, 0x07, 0x0b, 0x26, 0x95, 0xa6, 0xe9, 0xa7, 0xa4, 0x16, 0x27,
	0x17, 0x65, 0x16, 0x94, 0xe4, 0x17, 0x41, 0x14, 0x6a, 0x09, 0x70, 0xb1, 0xb8, 0xe6, 0x95, 0xe6,
	0x0a, 0x71, 0x70, 0xb1, 0x44, 0xb9, 0x06, 0xf9, 0x0b, 0x30, 0x58, 0x25, 0x71, 0xf1, 0xa5, 0x56,
	0x94, 0xa4, 0xe6, 0x15, 0x67, 0xe6, 0xe7, 0xc5, 0xa7, 0x82, 0xe4, 0xe4, 0xf4, 0x20, 0xc6, 0xe8,
	0xc1, 0x8c, 0xd1, 0xf3, 0x4d, 0x2d, 0xc9, 0xc8, 0x4f, 0xf1, 0x2f, 0x28, 0xc9, 0xcc, 0xcf, 0x2b,
	0x96, 0xe8, 0x50, 0x57, 0x60, 0xd4, 0xe0, 0x33, 0x92, 0xd2, 0xc3, 0xe6, 0x06, 0x3d, 0x90, 0xf1,
	0x41, 0xbc, 0x70, 0x23, 0x41, 0x5c, 0x27, 0xfb, 0x28, 0x5b, 0xa8, 0x91, 0xe9, 0xf9, 0x39, 0x89,
	0x79, 0xe9, 0x7a, 0xf9, 0x45, 0xe9, 0x08, 0x47, 0x12, 0xf6, 0x3c, 0x20, 0x00, 0x00, 0xff, 0xff,
	0x1d, 0x9e, 0x09, 0x6e, 0x21, 0x01, 0x00, 0x00,
}
