// Code generated by protoc-gen-gogo.
// source: wire.proto
// DO NOT EDIT!

package fsutil

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import strconv "strconv"

import bytes "bytes"

import strings "strings"
import reflect "reflect"

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

type Packet_PacketType int32

const (
	PACKET_STAT Packet_PacketType = 0
	PACKET_REQ  Packet_PacketType = 1
	PACKET_DATA Packet_PacketType = 2
	PACKET_FIN  Packet_PacketType = 3
	PACKET_ERR  Packet_PacketType = 4
)

var Packet_PacketType_name = map[int32]string{
	0: "PACKET_STAT",
	1: "PACKET_REQ",
	2: "PACKET_DATA",
	3: "PACKET_FIN",
	4: "PACKET_ERR",
}
var Packet_PacketType_value = map[string]int32{
	"PACKET_STAT": 0,
	"PACKET_REQ":  1,
	"PACKET_DATA": 2,
	"PACKET_FIN":  3,
	"PACKET_ERR":  4,
}

func (Packet_PacketType) EnumDescriptor() ([]byte, []int) { return fileDescriptorWire, []int{0, 0} }

type Packet struct {
	Type Packet_PacketType `protobuf:"varint,1,opt,name=type,proto3,enum=fsutil.Packet_PacketType" json:"type,omitempty"`
	Stat *Stat             `protobuf:"bytes,2,opt,name=stat" json:"stat,omitempty"`
	ID   uint32            `protobuf:"varint,3,opt,name=ID,proto3" json:"ID,omitempty"`
	Data []byte            `protobuf:"bytes,4,opt,name=data,proto3" json:"data,omitempty"`
}

func (m *Packet) Reset()                    { *m = Packet{} }
func (*Packet) ProtoMessage()               {}
func (*Packet) Descriptor() ([]byte, []int) { return fileDescriptorWire, []int{0} }

func (m *Packet) GetType() Packet_PacketType {
	if m != nil {
		return m.Type
	}
	return PACKET_STAT
}

func (m *Packet) GetStat() *Stat {
	if m != nil {
		return m.Stat
	}
	return nil
}

func (m *Packet) GetID() uint32 {
	if m != nil {
		return m.ID
	}
	return 0
}

func (m *Packet) GetData() []byte {
	if m != nil {
		return m.Data
	}
	return nil
}

func init() {
	proto.RegisterType((*Packet)(nil), "fsutil.Packet")
	proto.RegisterEnum("fsutil.Packet_PacketType", Packet_PacketType_name, Packet_PacketType_value)
}
func (x Packet_PacketType) String() string {
	s, ok := Packet_PacketType_name[int32(x)]
	if ok {
		return s
	}
	return strconv.Itoa(int(x))
}
func (this *Packet) Equal(that interface{}) bool {
	if that == nil {
		if this == nil {
			return true
		}
		return false
	}

	that1, ok := that.(*Packet)
	if !ok {
		that2, ok := that.(Packet)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		if this == nil {
			return true
		}
		return false
	} else if this == nil {
		return false
	}
	if this.Type != that1.Type {
		return false
	}
	if !this.Stat.Equal(that1.Stat) {
		return false
	}
	if this.ID != that1.ID {
		return false
	}
	if !bytes.Equal(this.Data, that1.Data) {
		return false
	}
	return true
}
func (this *Packet) GoString() string {
	if this == nil {
		return "nil"
	}
	s := make([]string, 0, 8)
	s = append(s, "&fsutil.Packet{")
	s = append(s, "Type: "+fmt.Sprintf("%#v", this.Type)+",\n")
	if this.Stat != nil {
		s = append(s, "Stat: "+fmt.Sprintf("%#v", this.Stat)+",\n")
	}
	s = append(s, "ID: "+fmt.Sprintf("%#v", this.ID)+",\n")
	s = append(s, "Data: "+fmt.Sprintf("%#v", this.Data)+",\n")
	s = append(s, "}")
	return strings.Join(s, "")
}
func valueToGoStringWire(v interface{}, typ string) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("func(v %v) *%v { return &v } ( %#v )", typ, typ, pv)
}
func (m *Packet) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Packet) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Type != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintWire(dAtA, i, uint64(m.Type))
	}
	if m.Stat != nil {
		dAtA[i] = 0x12
		i++
		i = encodeVarintWire(dAtA, i, uint64(m.Stat.Size()))
		n1, err := m.Stat.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n1
	}
	if m.ID != 0 {
		dAtA[i] = 0x18
		i++
		i = encodeVarintWire(dAtA, i, uint64(m.ID))
	}
	if len(m.Data) > 0 {
		dAtA[i] = 0x22
		i++
		i = encodeVarintWire(dAtA, i, uint64(len(m.Data)))
		i += copy(dAtA[i:], m.Data)
	}
	return i, nil
}

func encodeFixed64Wire(dAtA []byte, offset int, v uint64) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	dAtA[offset+4] = uint8(v >> 32)
	dAtA[offset+5] = uint8(v >> 40)
	dAtA[offset+6] = uint8(v >> 48)
	dAtA[offset+7] = uint8(v >> 56)
	return offset + 8
}
func encodeFixed32Wire(dAtA []byte, offset int, v uint32) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	return offset + 4
}
func encodeVarintWire(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Packet) Size() (n int) {
	var l int
	_ = l
	if m.Type != 0 {
		n += 1 + sovWire(uint64(m.Type))
	}
	if m.Stat != nil {
		l = m.Stat.Size()
		n += 1 + l + sovWire(uint64(l))
	}
	if m.ID != 0 {
		n += 1 + sovWire(uint64(m.ID))
	}
	l = len(m.Data)
	if l > 0 {
		n += 1 + l + sovWire(uint64(l))
	}
	return n
}

func sovWire(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozWire(x uint64) (n int) {
	return sovWire(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (this *Packet) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&Packet{`,
		`Type:` + fmt.Sprintf("%v", this.Type) + `,`,
		`Stat:` + strings.Replace(fmt.Sprintf("%v", this.Stat), "Stat", "Stat", 1) + `,`,
		`ID:` + fmt.Sprintf("%v", this.ID) + `,`,
		`Data:` + fmt.Sprintf("%v", this.Data) + `,`,
		`}`,
	}, "")
	return s
}
func valueToStringWire(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("*%v", pv)
}
func (m *Packet) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowWire
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
			return fmt.Errorf("proto: Packet: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Packet: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			m.Type = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowWire
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Type |= (Packet_PacketType(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Stat", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowWire
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthWire
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Stat == nil {
				m.Stat = &Stat{}
			}
			if err := m.Stat.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ID", wireType)
			}
			m.ID = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowWire
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.ID |= (uint32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Data", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowWire
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
				return ErrInvalidLengthWire
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
			skippy, err := skipWire(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthWire
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
func skipWire(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowWire
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
					return 0, ErrIntOverflowWire
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
					return 0, ErrIntOverflowWire
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
				return 0, ErrInvalidLengthWire
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowWire
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
				next, err := skipWire(dAtA[start:])
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
	ErrInvalidLengthWire = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowWire   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("wire.proto", fileDescriptorWire) }

var fileDescriptorWire = []byte{
	// 259 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0xe2, 0xe2, 0x2a, 0xcf, 0x2c, 0x4a,
	0xd5, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0x62, 0x4b, 0x2b, 0x2e, 0x2d, 0xc9, 0xcc, 0x91, 0xe2,
	0x2a, 0x2e, 0x49, 0x2c, 0x81, 0x88, 0x29, 0xdd, 0x65, 0xe4, 0x62, 0x0b, 0x48, 0x4c, 0xce, 0x4e,
	0x2d, 0x11, 0xd2, 0xe5, 0x62, 0x29, 0xa9, 0x2c, 0x48, 0x95, 0x60, 0x54, 0x60, 0xd4, 0xe0, 0x33,
	0x92, 0xd4, 0x83, 0xa8, 0xd6, 0x83, 0xc8, 0x42, 0xa9, 0x90, 0xca, 0x82, 0xd4, 0x20, 0xb0, 0x32,
	0x21, 0x05, 0x2e, 0x16, 0x90, 0x39, 0x12, 0x4c, 0x0a, 0x8c, 0x1a, 0xdc, 0x46, 0x3c, 0x30, 0xe5,
	0xc1, 0x25, 0x89, 0x25, 0x41, 0x60, 0x19, 0x21, 0x3e, 0x2e, 0x26, 0x4f, 0x17, 0x09, 0x66, 0x05,
	0x46, 0x0d, 0xde, 0x20, 0x26, 0x4f, 0x17, 0x21, 0x21, 0x2e, 0x96, 0x94, 0xc4, 0x92, 0x44, 0x09,
	0x16, 0x05, 0x46, 0x0d, 0x9e, 0x20, 0x30, 0x5b, 0x29, 0x8e, 0x8b, 0x0b, 0x61, 0xb2, 0x10, 0x3f,
	0x17, 0x77, 0x80, 0xa3, 0xb3, 0xb7, 0x6b, 0x48, 0x7c, 0x70, 0x88, 0x63, 0x88, 0x00, 0x83, 0x10,
	0x1f, 0x17, 0x17, 0x54, 0x20, 0xc8, 0x35, 0x50, 0x80, 0x11, 0x49, 0x81, 0x8b, 0x63, 0x88, 0xa3,
	0x00, 0x13, 0x92, 0x02, 0x37, 0x4f, 0x3f, 0x01, 0x66, 0x24, 0xbe, 0x6b, 0x50, 0x90, 0x00, 0x8b,
	0x93, 0xce, 0x85, 0x87, 0x72, 0x0c, 0x37, 0x1e, 0xca, 0x31, 0x7c, 0x78, 0x28, 0xc7, 0xd8, 0xf0,
	0x48, 0x8e, 0x71, 0xc5, 0x23, 0x39, 0xc6, 0x13, 0x8f, 0xe4, 0x18, 0x2f, 0x3c, 0x92, 0x63, 0x7c,
	0xf0, 0x48, 0x8e, 0xf1, 0xc5, 0x23, 0x39, 0x86, 0x0f, 0x8f, 0xe4, 0x18, 0x27, 0x3c, 0x96, 0x63,
	0x48, 0x62, 0x03, 0x07, 0x8a, 0x31, 0x20, 0x00, 0x00, 0xff, 0xff, 0x8b, 0xce, 0x55, 0x3b, 0x36,
	0x01, 0x00, 0x00,
}
