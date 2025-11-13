package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
)

type TagValue struct {
	Tag    byte
	Length uint16
	Value  []byte
}

type TagVO struct {
	CommandId byte
	message   []TagValue
}

// Add a 1-byte value
func (t *TagVO) AddByteValue(tag int, value byte) {
	val := []byte{value}
	t.message = append(t.message, TagValue{
		Tag:    byte(tag),
		Length: uint16(len(val)),
		Value:  val,
	})
}

// Add a 4-byte int value (BigEndian)
func (t *TagVO) AddIntValue(tag int, value int32) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, value)
	t.message = append(t.message, TagValue{
		Tag:    byte(tag),
		Length: 4,
		Value:  buf.Bytes(),
	})
}

// Add an 8-byte float value (to match Java's allocate(8))
func (t *TagVO) AddFloatValue(tag int, value float32) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], math.Float32bits(value))
	// remaining 4 bytes stay zero, matching Java ByteBuffer.allocate(8).putFloat()
	t.message = append(t.message, TagValue{
		Tag:    byte(tag),
		Length: 8,
		Value:  buf,
	})
}

// Add a string (raw bytes)
func (t *TagVO) AddStringValue(tag int, value string) {
	val := []byte(value)
	t.message = append(t.message, TagValue{
		Tag:    byte(tag),
		Length: uint16(len(val)),
		Value:  val,
	})
}

// CreateRequestMessage — builds the same packet as Java’s createRequestMessage()
func (t *TagVO) CreateRequestMessage() []byte {
	var messageBuf bytes.Buffer

	// Build TLV segments
	for _, tv := range t.message {
		messageBuf.WriteByte(tv.Tag)
		binary.Write(&messageBuf, binary.BigEndian, tv.Length)
		messageBuf.Write(tv.Value)
	}

	messageBytes := messageBuf.Bytes()

	// Total length (2 bytes)
	totalLen := uint16(len(messageBytes))
	var finalBuf bytes.Buffer

	// Write fixed header [1][1][1]
	finalBuf.WriteByte(1)
	finalBuf.WriteByte(1)
	finalBuf.WriteByte(1)

	// Write command ID
	finalBuf.WriteByte(t.CommandId)

	// Write total message length (2 bytes)
	binary.Write(&finalBuf, binary.BigEndian, totalLen)

	// Write TLVs
	finalBuf.Write(messageBytes)

	return finalBuf.Bytes()
}

// Hex dump for debugging
func (t *TagVO) HexDump() string {
	return hex.EncodeToString(t.CreateRequestMessage())
}
