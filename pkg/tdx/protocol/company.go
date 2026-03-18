package protocol

import (
	"encoding/binary"
)

type companyCategoryStruct struct{}
type companyContentStruct struct{}

var MCompanyCategory = companyCategoryStruct{}
var MCompanyContent = companyContentStruct{}

type CompanyCategoryItem struct {
	Name     string
	Filename string
	Start    uint32
	Length   uint32
}

func (c companyCategoryStruct) Frame(code string) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 12)
	binary.LittleEndian.PutUint16(data[0:2], uint16(exchange))
	copy(data[2:8], []byte(number))
	binary.LittleEndian.PutUint32(data[8:12], 0)
	return &Frame{
		Control: Control01,
		Type:    TypeCompanyCategory,
		Data:    data,
	}, nil
}

func (c companyCategoryStruct) Decode(bs []byte) ([]*CompanyCategoryItem, error) {
	if len(bs) < 2 {
		return nil, ErrDataLength
	}
	count := int(Uint16LE(bs[:2]))
	bs = bs[2:]

	const recordSize = 152
	items := make([]*CompanyCategoryItem, 0, count)
	for i := 0; i < count && len(bs) >= recordSize; i++ {
		name := trimNull(bs[:64])
		filename := trimNull(bs[64:144])
		start := binary.LittleEndian.Uint32(bs[144:148])
		length := binary.LittleEndian.Uint32(bs[148:152])
		bs = bs[recordSize:]
		items = append(items, &CompanyCategoryItem{
			Name:     name,
			Filename: filename,
			Start:    start,
			Length:   length,
		})
	}
	return items, nil
}

func (c companyContentStruct) Frame(code, filename string, start, length uint32) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 102)
	binary.LittleEndian.PutUint16(data[0:2], uint16(exchange))
	copy(data[2:8], []byte(number))
	binary.LittleEndian.PutUint16(data[8:10], 0)
	copy(data[10:90], []byte(filename))
	binary.LittleEndian.PutUint32(data[90:94], start)
	binary.LittleEndian.PutUint32(data[94:98], length)
	return &Frame{
		Control: Control01,
		Type:    TypeCompanyContent,
		Data:    data,
	}, nil
}

func (c companyContentStruct) Decode(bs []byte) (string, error) {
	if len(bs) < 12 {
		return "", ErrDataLength
	}
	length := binary.LittleEndian.Uint16(bs[10:12])
	bs = bs[12:]
	if int(length) > len(bs) {
		length = uint16(len(bs))
	}
	content := UTF8ToGBK(bs[:length])
	return string(content), nil
}
