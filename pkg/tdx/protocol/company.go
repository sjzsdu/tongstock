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
	// Start is the byte offset of this block inside the file named Filename.
	// The document is GBK encoded, so Start/Length count GBK bytes — not the
	// bytes of the UTF-8 string the CLI ultimately prints (which is typically
	// 17~30% longer for the same content). Never slice a UTF-8 string with
	// these values.
	Start uint32
	// Length is the block size in GBK bytes; see Start. Note that the file is
	// regenerated daily while this catalogue is cached for much longer, so
	// both fields can go stale — never trust them without checking the block
	// header line (CompanyBlockMarker) against the document.
	Length uint32
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

// CompanyBlockMarker returns the raw GBK bytes that open a block of the F10
// document, for example "财务分析☆ ◇600000 ..." for block 财务分析 of 600000.
// Every block starts with such a line (and the header also carries
// "★本栏包括【…】", the list of the sections inside the block), which makes it
// the natural separator: block boundaries derived from it stay correct even
// when the cached byte offsets no longer match the current document.
func CompanyBlockMarker(code, name string) ([]byte, error) {
	_, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	return UTF8ToGBK(name + "☆ ◇" + number)
}

// DecodeRaw returns the still GBK-encoded payload of a company-content reply.
// Callers that stitch several replies together must concatenate these bytes
// first and decode only once at the end, otherwise a reply boundary that
// lands inside a GBK character produces mojibake.
func (c companyContentStruct) DecodeRaw(bs []byte) ([]byte, error) {
	if len(bs) < 12 {
		return nil, ErrDataLength
	}
	length := binary.LittleEndian.Uint16(bs[10:12])
	bs = bs[12:]
	if int(length) > len(bs) {
		length = uint16(len(bs))
	}
	return bs[:length], nil
}

func (c companyContentStruct) Decode(bs []byte) (string, error) {
	raw, err := c.DecodeRaw(bs)
	if err != nil {
		return "", err
	}
	return string(GBKToUTF8(raw)), nil
}
