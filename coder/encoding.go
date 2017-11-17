package coder

import (
	"bytes"
	"encoding/binary"
)

// Coder 编解码接口
type Coder interface {
	Encode(*bytes.Buffer)
	Decode(*bytes.Buffer)
}

func Decodestring(reader *bytes.Buffer) string {
	var count int32
	binary.Read(reader, binary.LittleEndian, &count)
	buff := make([]byte, count)
	reader.Read(buff)
	return string(buff)
}

func Encode(coder Coder) []byte {
	writer := bytes.NewBuffer([]byte{})
	coder.Encode(writer)
	return writer.Bytes()
}

func Decode(coder Coder, data []byte) {
	coder.Decode(bytes.NewBuffer(data))
}
