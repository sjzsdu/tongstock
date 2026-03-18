package protocol

import (
	"encoding/binary"
)

type blockInfoMetaStruct struct{}
type blockInfoStruct struct{}

var MBlockInfoMeta = blockInfoMetaStruct{}
var MBlockInfo = blockInfoStruct{}

type BlockInfoMeta struct {
	Size      uint32
	HashValue string
}

func (b blockInfoMetaStruct) Frame(blockFile string) *Frame {
	fileBytes := make([]byte, 0x2a-2)
	copy(fileBytes, []byte(blockFile))
	return &Frame{
		Control: Control01,
		Type:    TypeBlockInfoMeta,
		Data:    fileBytes,
	}
}

func (b blockInfoMetaStruct) Decode(bs []byte) (*BlockInfoMeta, error) {
	if len(bs) < 38 {
		return nil, ErrDataLength
	}
	size := binary.LittleEndian.Uint32(bs[:4])
	hashValue := string(bs[5:37])
	return &BlockInfoMeta{
		Size:      size,
		HashValue: hashValue,
	}, nil
}

func (b blockInfoStruct) Frame(blockFile string, start, size uint32) *Frame {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], start)
	binary.LittleEndian.PutUint32(data[4:8], size)
	fileBytes := make([]byte, 0x6e-10)
	copy(fileBytes, []byte(blockFile))
	data = append(data, fileBytes...)
	return &Frame{
		Control: Control01,
		Type:    TypeBlockInfo,
		Data:    data,
	}
}

func (b blockInfoStruct) Decode(bs []byte) ([]byte, error) {
	if len(bs) < 4 {
		return nil, ErrDataLength
	}
	return bs[4:], nil
}

type BlockItem struct {
	BlockName string
	BlockType uint16
	StockCode string
}

func ParseBlockData(bs []byte) ([]*BlockItem, error) {
	if len(bs) < 4 {
		return nil, ErrDataLength
	}

	items := make([]*BlockItem, 0)
	pos := 0
	for pos+64 <= len(bs) {
		blockName := trimNull(bs[pos : pos+9])
		blockType := Uint16LE(bs[pos+9 : pos+11])
		stockCount := Uint16LE(bs[pos+11 : pos+13])
		pos += 13

		for i := uint16(0); i < stockCount && pos+7 <= len(bs); i++ {
			code := trimNull(bs[pos : pos+7])
			pos += 7
			items = append(items, &BlockItem{
				BlockName: blockName,
				BlockType: blockType,
				StockCode: code,
			})
		}
	}
	return items, nil
}

func trimNull(bs []byte) string {
	for i, b := range bs {
		if b == 0 {
			return string(UTF8ToGBK(bs[:i]))
		}
	}
	return string(UTF8ToGBK(bs))
}
