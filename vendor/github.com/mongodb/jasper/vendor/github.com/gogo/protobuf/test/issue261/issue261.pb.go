// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: issue261.proto

package issue261

import (
	fmt "fmt"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	github_com_gogo_protobuf_sortkeys "github.com/gogo/protobuf/sortkeys"
	_ "github.com/gogo/protobuf/types"
	github_com_gogo_protobuf_types "github.com/gogo/protobuf/types"
	io "io"
	math "math"
	reflect "reflect"
	strings "strings"
	time "time"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf
var _ = time.Kitchen

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type MapStdTypes struct {
	NullableDuration map[int32]*time.Duration `protobuf:"bytes,3,rep,name=nullableDuration,proto3,stdduration" json:"nullableDuration,omitempty" protobuf_key:"varint,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (m *MapStdTypes) Reset()      { *m = MapStdTypes{} }
func (*MapStdTypes) ProtoMessage() {}
func (*MapStdTypes) Descriptor() ([]byte, []int) {
	return fileDescriptor_35fa31fa2023935a, []int{0}
}
func (m *MapStdTypes) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *MapStdTypes) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_MapStdTypes.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *MapStdTypes) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MapStdTypes.Merge(m, src)
}
func (m *MapStdTypes) XXX_Size() int {
	return m.Size()
}
func (m *MapStdTypes) XXX_DiscardUnknown() {
	xxx_messageInfo_MapStdTypes.DiscardUnknown(m)
}

var xxx_messageInfo_MapStdTypes proto.InternalMessageInfo

func (m *MapStdTypes) GetNullableDuration() map[int32]*time.Duration {
	if m != nil {
		return m.NullableDuration
	}
	return nil
}

func init() {
	proto.RegisterType((*MapStdTypes)(nil), "issue261.MapStdTypes")
	proto.RegisterMapType((map[int32]*time.Duration)(nil), "issue261.MapStdTypes.NullableDurationEntry")
}

func init() { proto.RegisterFile("issue261.proto", fileDescriptor_35fa31fa2023935a) }

var fileDescriptor_35fa31fa2023935a = []byte{
	// 275 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0xcb, 0x2c, 0x2e, 0x2e,
	0x4d, 0x35, 0x32, 0x33, 0xd4, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2, 0x80, 0xf1, 0xa5, 0x74,
	0xd3, 0x33, 0x4b, 0x32, 0x4a, 0x93, 0xf4, 0x92, 0xf3, 0x73, 0xf5, 0xd3, 0xf3, 0xd3, 0xf3, 0xf5,
	0xc1, 0x0a, 0x92, 0x4a, 0xd3, 0xc0, 0x3c, 0x30, 0x07, 0xcc, 0x82, 0x68, 0x94, 0x92, 0x4b, 0xcf,
	0xcf, 0x4f, 0xcf, 0x49, 0x45, 0xa8, 0x4a, 0x29, 0x2d, 0x4a, 0x2c, 0xc9, 0xcc, 0xcf, 0x83, 0xc8,
	0x2b, 0x9d, 0x61, 0xe4, 0xe2, 0xf6, 0x4d, 0x2c, 0x08, 0x2e, 0x49, 0x09, 0xa9, 0x2c, 0x48, 0x2d,
	0x16, 0x8a, 0xe5, 0x12, 0xc8, 0x2b, 0xcd, 0xc9, 0x49, 0x4c, 0xca, 0x49, 0x75, 0x81, 0xaa, 0x94,
	0x60, 0x56, 0x60, 0xd6, 0xe0, 0x36, 0xd2, 0xd6, 0x83, 0xbb, 0x09, 0x49, 0x83, 0x9e, 0x1f, 0x9a,
	0x6a, 0xd7, 0xbc, 0x92, 0xa2, 0x4a, 0x27, 0x96, 0x19, 0xf7, 0xe5, 0x19, 0x83, 0x30, 0x8c, 0x92,
	0x8a, 0xe3, 0x12, 0xc5, 0xaa, 0x41, 0x48, 0x80, 0x8b, 0x39, 0x3b, 0xb5, 0x52, 0x82, 0x51, 0x81,
	0x51, 0x83, 0x35, 0x08, 0xc4, 0x14, 0xd2, 0xe7, 0x62, 0x2d, 0x4b, 0xcc, 0x29, 0x4d, 0x95, 0x60,
	0x52, 0x60, 0xd4, 0xe0, 0x36, 0x92, 0xd4, 0x83, 0xf8, 0x44, 0x0f, 0xe6, 0x13, 0x3d, 0x98, 0x01,
	0x41, 0x10, 0x75, 0x56, 0x4c, 0x16, 0x8c, 0x4e, 0x26, 0x17, 0x1e, 0xca, 0x31, 0xdc, 0x78, 0x28,
	0xc7, 0xf0, 0xe1, 0xa1, 0x1c, 0x63, 0xc3, 0x23, 0x39, 0xc6, 0x15, 0x8f, 0xe4, 0x18, 0x4f, 0x3c,
	0x92, 0x63, 0xbc, 0xf0, 0x48, 0x8e, 0xf1, 0xc1, 0x23, 0x39, 0xc6, 0x17, 0x8f, 0xe4, 0x18, 0x3e,
	0x3c, 0x92, 0x63, 0x9c, 0xf0, 0x58, 0x8e, 0xe1, 0xc2, 0x63, 0x39, 0x86, 0x1b, 0x8f, 0xe5, 0x18,
	0x92, 0xd8, 0xc0, 0x66, 0x1a, 0x03, 0x02, 0x00, 0x00, 0xff, 0xff, 0xa0, 0x4d, 0xcb, 0xf5, 0x76,
	0x01, 0x00, 0x00,
}

func (this *MapStdTypes) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*MapStdTypes)
	if !ok {
		that2, ok := that.(MapStdTypes)
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
	if len(this.NullableDuration) != len(that1.NullableDuration) {
		return false
	}
	for i := range this.NullableDuration {
		if dthis, dthat := this.NullableDuration[i], that1.NullableDuration[i]; (dthis != nil && dthat != nil && *dthis != *dthat) || (dthis != nil && dthat == nil) || (dthis == nil && dthat != nil) {
			return false
		}
	}
	return true
}
func (this *MapStdTypes) GoString() string {
	if this == nil {
		return "nil"
	}
	s := make([]string, 0, 5)
	s = append(s, "&issue261.MapStdTypes{")
	keysForNullableDuration := make([]int32, 0, len(this.NullableDuration))
	for k := range this.NullableDuration {
		keysForNullableDuration = append(keysForNullableDuration, k)
	}
	github_com_gogo_protobuf_sortkeys.Int32s(keysForNullableDuration)
	mapStringForNullableDuration := "map[int32]*time.Duration{"
	for _, k := range keysForNullableDuration {
		mapStringForNullableDuration += fmt.Sprintf("%#v: %#v,", k, this.NullableDuration[k])
	}
	mapStringForNullableDuration += "}"
	if this.NullableDuration != nil {
		s = append(s, "NullableDuration: "+mapStringForNullableDuration+",\n")
	}
	s = append(s, "}")
	return strings.Join(s, "")
}
func valueToGoStringIssue261(v interface{}, typ string) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("func(v %v) *%v { return &v } ( %#v )", typ, typ, pv)
}
func (m *MapStdTypes) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *MapStdTypes) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.NullableDuration) > 0 {
		for k := range m.NullableDuration {
			dAtA[i] = 0x1a
			i++
			v := m.NullableDuration[k]
			msgSize := 0
			if v != nil {
				msgSize = github_com_gogo_protobuf_types.SizeOfStdDuration(*v)
				msgSize += 1 + sovIssue261(uint64(msgSize))
			}
			mapSize := 1 + sovIssue261(uint64(k)) + msgSize
			i = encodeVarintIssue261(dAtA, i, uint64(mapSize))
			dAtA[i] = 0x8
			i++
			i = encodeVarintIssue261(dAtA, i, uint64(k))
			if v != nil {
				dAtA[i] = 0x12
				i++
				i = encodeVarintIssue261(dAtA, i, uint64(github_com_gogo_protobuf_types.SizeOfStdDuration(*v)))
				n1, err1 := github_com_gogo_protobuf_types.StdDurationMarshalTo(*v, dAtA[i:])
				if err1 != nil {
					return 0, err1
				}
				i += n1
			}
		}
	}
	return i, nil
}

func encodeVarintIssue261(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *MapStdTypes) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if len(m.NullableDuration) > 0 {
		for k, v := range m.NullableDuration {
			_ = k
			_ = v
			l = 0
			if v != nil {
				l = github_com_gogo_protobuf_types.SizeOfStdDuration(*v)
				l += 1 + sovIssue261(uint64(l))
			}
			mapEntrySize := 1 + sovIssue261(uint64(k)) + l
			n += mapEntrySize + 1 + sovIssue261(uint64(mapEntrySize))
		}
	}
	return n
}

func sovIssue261(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozIssue261(x uint64) (n int) {
	return sovIssue261(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (this *MapStdTypes) String() string {
	if this == nil {
		return "nil"
	}
	keysForNullableDuration := make([]int32, 0, len(this.NullableDuration))
	for k := range this.NullableDuration {
		keysForNullableDuration = append(keysForNullableDuration, k)
	}
	github_com_gogo_protobuf_sortkeys.Int32s(keysForNullableDuration)
	mapStringForNullableDuration := "map[int32]*time.Duration{"
	for _, k := range keysForNullableDuration {
		mapStringForNullableDuration += fmt.Sprintf("%v: %v,", k, this.NullableDuration[k])
	}
	mapStringForNullableDuration += "}"
	s := strings.Join([]string{`&MapStdTypes{`,
		`NullableDuration:` + mapStringForNullableDuration + `,`,
		`}`,
	}, "")
	return s
}
func valueToStringIssue261(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("*%v", pv)
}
func (m *MapStdTypes) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowIssue261
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: MapStdTypes: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MapStdTypes: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field NullableDuration", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowIssue261
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthIssue261
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthIssue261
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.NullableDuration == nil {
				m.NullableDuration = make(map[int32]*time.Duration)
			}
			var mapkey int32
			mapvalue := new(time.Duration)
			for iNdEx < postIndex {
				entryPreIndex := iNdEx
				var wire uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return ErrIntOverflowIssue261
					}
					if iNdEx >= l {
						return io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					wire |= uint64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				fieldNum := int32(wire >> 3)
				if fieldNum == 1 {
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowIssue261
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						mapkey |= int32(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
				} else if fieldNum == 2 {
					var mapmsglen int
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowIssue261
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						mapmsglen |= int(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					if mapmsglen < 0 {
						return ErrInvalidLengthIssue261
					}
					postmsgIndex := iNdEx + mapmsglen
					if postmsgIndex < 0 {
						return ErrInvalidLengthIssue261
					}
					if postmsgIndex > l {
						return io.ErrUnexpectedEOF
					}
					if err := github_com_gogo_protobuf_types.StdDurationUnmarshal(mapvalue, dAtA[iNdEx:postmsgIndex]); err != nil {
						return err
					}
					iNdEx = postmsgIndex
				} else {
					iNdEx = entryPreIndex
					skippy, err := skipIssue261(dAtA[iNdEx:])
					if err != nil {
						return err
					}
					if skippy < 0 {
						return ErrInvalidLengthIssue261
					}
					if (iNdEx + skippy) > postIndex {
						return io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}
			m.NullableDuration[mapkey] = mapvalue
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipIssue261(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthIssue261
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthIssue261
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
func skipIssue261(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowIssue261
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
					return 0, ErrIntOverflowIssue261
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
					return 0, ErrIntOverflowIssue261
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
			if length < 0 {
				return 0, ErrInvalidLengthIssue261
			}
			iNdEx += length
			if iNdEx < 0 {
				return 0, ErrInvalidLengthIssue261
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowIssue261
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
				next, err := skipIssue261(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
				if iNdEx < 0 {
					return 0, ErrInvalidLengthIssue261
				}
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
	ErrInvalidLengthIssue261 = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowIssue261   = fmt.Errorf("proto: integer overflow")
)
