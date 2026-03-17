package protocol

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	Prefix     = 0x0C
	PrefixResp = 0xB1CB7400
)

type Message interface {
	Bytes() []byte
}

type Control uint8

func (c Control) Uint8() uint8 {
	return uint8(c)
}

type Frame struct {
	MsgID   uint32
	Control Control
	Type    uint16
	Data    []byte
}

func (f *Frame) Bytes() []byte {
	length := uint16(len(f.Data) + 2)
	data := make([]byte, 12+len(f.Data))
	data[0] = Prefix
	binary.LittleEndian.PutUint32(data[1:5], f.MsgID)
	data[5] = f.Control.Uint8()
	binary.LittleEndian.PutUint16(data[6:8], length)
	binary.LittleEndian.PutUint16(data[8:10], length) // TDX 协议要求 length 写两次
	binary.LittleEndian.PutUint16(data[10:12], f.Type)
	copy(data[12:], f.Data)
	return data
}

type Response struct {
	Prefix    uint32
	Control   uint8
	MsgID     uint32
	Unknown   uint8
	Type      uint16
	ZipLength uint16
	Length    uint16
	Data      []byte
}

func Uint32(bs []byte) uint32 {
	reversed := make([]byte, 4)
	for i := 0; i < 4; i++ {
		reversed[i] = bs[3-i]
	}
	return binary.BigEndian.Uint32(reversed)
}

func Uint16(bs []byte) uint16 {
	reversed := make([]byte, 2)
	for i := 0; i < 2; i++ {
		reversed[i] = bs[1-i]
	}
	return binary.BigEndian.Uint16(reversed)
}

func Decode(bs []byte) (*Response, error) {
	if len(bs) < 16 {
		return nil, errors.New("数据长度不足")
	}

	resp := &Response{
		Prefix:    binary.BigEndian.Uint32(bs[:4]),
		Control:   bs[4],
		MsgID:     Uint32(bs[5:9]),
		Unknown:   bs[9],
		Type:      Uint16(bs[10:12]),
		ZipLength: Uint16(bs[12:14]),
		Length:    Uint16(bs[14:16]),
		Data:      bs[16:],
	}

	if int(resp.ZipLength) != len(bs[16:]) {
		return nil, fmt.Errorf("压缩数据长度不匹配,预期%d,得到%d", resp.ZipLength+16, len(bs))
	}

	if resp.ZipLength != resp.Length {
		r, err := zlib.NewReader(bytes.NewReader(resp.Data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		resp.Data, err = io.ReadAll(r)
		if err != nil {
			return nil, err
		}
	}

	if int(resp.Length) != len(resp.Data) {
		return nil, fmt.Errorf("解压数据长度不匹配,预期%d,得到%d", resp.Length, len(resp.Data))
	}

	return resp, nil
}

func ReadFrom(r io.Reader) (result []byte, err error) {
	for {
		result = []byte(nil)

		prefix := make([]byte, 4)
		_, err := io.ReadFull(r, prefix)
		if err != nil {
			return nil, err
		}
		if binary.BigEndian.Uint32(prefix) != PrefixResp {
			continue
		}
		result = append(result, prefix...)

		buf := make([]byte, 12)
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return nil, err
		}
		result = append(result, buf...)

		length := uint16(result[13])<<8 + uint16(result[12])
		buf = make([]byte, length)
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return nil, err
		}
		result = append(result, buf...)

		return result, nil
	}
}
