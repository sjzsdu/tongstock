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

// ParseBlockData 解析通达信板块文件（如 block_gn.dat）的二进制内容。
//
// 文件为 384 字节文件头加固定 2813 字节步长的记录流。每条记录：
//
//	[0:2)   保留区（当前记录为某板块的最后一条时才有效，见下）
//	[2:11)  板块名（GBK）
//	[11:13) 成分股数量（uint16 LE，最大 400）
//	[13:15) 板块类型（uint16 LE）
//	[15:15+count*7) 成分股代码，每只 7 字节
//
// 关键细节：记录步长 2813 字节，但代码区按 count 最多可占 15+400*7=2815
// 字节——第 400 只代码的最后 2 字节会写进下一条记录的保留区。因此读取代码
// 时不能按记录边界截断，必须从整个文件缓冲区里按偏移切片，否则恰好满 400
// 只的板块会丢掉第 400 只（历史上所有这类板块都显示成 399 只）。
func ParseBlockData(bs []byte) ([]*BlockItem, error) {
	const fileHeader = 384
	const recordSize = 2813
	const nameOffset = 2
	const nameSize = 9
	const countOffset = 11
	const typeOffset = 13
	const codeOffset = 15
	const codeSize = 7
	const maxCodes = 400

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
			return string(GBKToUTF8(bs[:i]))
		}
	}
	return string(GBKToUTF8(bs))
}
