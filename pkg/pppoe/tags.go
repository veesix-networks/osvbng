package pppoe

import (
	"encoding/binary"
	"fmt"
)

const (
	TagEndOfList      uint16 = 0x0000
	TagServiceName    uint16 = 0x0101
	TagACName         uint16 = 0x0102
	TagHostUniq       uint16 = 0x0103
	TagACCookie       uint16 = 0x0104
	TagVendorSpecific uint16 = 0x0105
	TagRelaySessionID uint16 = 0x0110
	TagServiceNameErr uint16 = 0x0201
	TagACSystemErr    uint16 = 0x0202
	TagGenericErr     uint16 = 0x0203
)

type Tag struct {
	Type  uint16
	Value []byte
}

type Tags struct {
	ServiceName    string
	ACName         string
	HostUniq       []byte
	ACCookie       []byte
	RelaySessionID []byte
	VendorSpecific []byte
	Errors         []string
	Raw            []Tag
}

func ParseTags(payload []byte) (*Tags, error) {
	tags := &Tags{}
	offset := 0

	for offset+4 <= len(payload) {
		tagType := binary.BigEndian.Uint16(payload[offset : offset+2])
		tagLen := binary.BigEndian.Uint16(payload[offset+2 : offset+4])
		offset += 4

		if tagType == TagEndOfList {
			break
		}

		if offset+int(tagLen) > len(payload) {
			return nil, fmt.Errorf("tag length %d exceeds payload at offset %d", tagLen, offset)
		}

		value := payload[offset : offset+int(tagLen)]
		offset += int(tagLen)

		tags.Raw = append(tags.Raw, Tag{Type: tagType, Value: value})

		switch tagType {
		case TagServiceName:
			tags.ServiceName = string(value)
		case TagACName:
			tags.ACName = string(value)
		case TagHostUniq:
			tags.HostUniq = make([]byte, len(value))
			copy(tags.HostUniq, value)
		case TagACCookie:
			tags.ACCookie = make([]byte, len(value))
			copy(tags.ACCookie, value)
		case TagRelaySessionID:
			tags.RelaySessionID = make([]byte, len(value))
			copy(tags.RelaySessionID, value)
		case TagVendorSpecific:
			tags.VendorSpecific = make([]byte, len(value))
			copy(tags.VendorSpecific, value)
		case TagServiceNameErr:
			tags.Errors = append(tags.Errors, "service-name-error: "+string(value))
		case TagACSystemErr:
			tags.Errors = append(tags.Errors, "ac-system-error: "+string(value))
		case TagGenericErr:
			tags.Errors = append(tags.Errors, "generic-error: "+string(value))
		}
	}

	return tags, nil
}

type TagBuilder struct {
	buf []byte
}

func NewTagBuilder() *TagBuilder {
	return &TagBuilder{buf: make([]byte, 0, 256)}
}

func (b *TagBuilder) AddTag(tagType uint16, value []byte) *TagBuilder {
	b.buf = append(b.buf, byte(tagType>>8), byte(tagType))
	b.buf = append(b.buf, byte(len(value)>>8), byte(len(value)))
	b.buf = append(b.buf, value...)
	return b
}

func (b *TagBuilder) AddServiceName(name string) *TagBuilder {
	return b.AddTag(TagServiceName, []byte(name))
}

func (b *TagBuilder) AddACName(name string) *TagBuilder {
	return b.AddTag(TagACName, []byte(name))
}

func (b *TagBuilder) AddHostUniq(value []byte) *TagBuilder {
	return b.AddTag(TagHostUniq, value)
}

func (b *TagBuilder) AddACCookie(value []byte) *TagBuilder {
	return b.AddTag(TagACCookie, value)
}

func (b *TagBuilder) AddRelaySessionID(value []byte) *TagBuilder {
	return b.AddTag(TagRelaySessionID, value)
}

func (b *TagBuilder) AddServiceNameError(msg string) *TagBuilder {
	return b.AddTag(TagServiceNameErr, []byte(msg))
}

func (b *TagBuilder) AddACSystemError(msg string) *TagBuilder {
	return b.AddTag(TagACSystemErr, []byte(msg))
}

func (b *TagBuilder) Build() []byte {
	return b.buf
}
