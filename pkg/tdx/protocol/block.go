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
	const fileHeader = 384
	const recordSize = 2813
	const nameOffset = 2
	const nameSize = 9
	const countOffset = 11
	const typeOffset = 13
	const codeOffset = 15
	const codeSize = 7
	const maxCodes = (recordSize - codeOffset) / codeSize

	if len(bs) < fileHeader+recordSize {
		return nil, ErrDataLength
	}

	numRecords := (len(bs) - fileHeader) / recordSize
	items := make([]*BlockItem, 0, numRecords*50)

	for i := 0; i < numRecords; i++ {
		base := fileHeader + i*recordSize
		if base+codeOffset > len(bs) {
			break
		}

		blockName := trimNull(bs[base+nameOffset : base+nameOffset+nameSize])
		if blockName == "" {
			continue
		}
		blockType := Uint16LE(bs[base+typeOffset : base+typeOffset+2])
		stockCount := int(Uint16LE(bs[base+countOffset : base+countOffset+2]))
		if stockCount > maxCodes {
			stockCount = maxCodes
		}

		for j := 0; j < stockCount; j++ {
			off := base + codeOffset + j*codeSize
			if off+codeSize > len(bs) {
				break
			}
			code := trimNull(bs[off : off+codeSize])
			if code == "" {
				break
			}
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
