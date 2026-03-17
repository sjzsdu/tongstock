package protocol

import "errors"

type Code struct{}

func (c Code) Frame(exchange Exchange, start uint16) *Frame {
	return &Frame{
		Control: Control01,
		Type:    TypeCode,
		Data:    []byte{exchange.Uint8(), 0x0, uint8(start), uint8(start >> 8)},
	}
}

type CodeItem struct {
	Code string
	Name string
}

type CodeResp struct {
	Items []CodeItem
}

func (c Code) Decode(bs []byte) (*CodeResp, error) {
	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}

	count := int(Uint16LE(bs[:2]))
	bs = bs[2:]

	const recordLen = 29
	items := make([]CodeItem, 0, count)
	for i := 0; i < count && len(bs) >= recordLen; i++ {
		items = append(items, CodeItem{
			Code: string(bs[:6]),
			Name: string(UTF8ToGBK(bs[8:16])),
		})
		bs = bs[recordLen:]
	}
	return &CodeResp{Items: items}, nil
}
