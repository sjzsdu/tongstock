package protocol

import (
	"errors"

	"github.com/sjzsdu/tongstock/pkg/utils"
)

type Quote struct{}

func (q Quote) Frame(codes ...string) (*Frame, error) {
	header := []byte{0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	payload := []byte{uint8(len(codes)), uint8(len(codes) >> 8)}
	for _, code := range codes {
		market, num, err := utils.DecodeStockCode(code)
		if err != nil {
			return nil, err
		}
		var ex byte
		switch market {
		case "sh":
			ex = byte(ExchangeSH)
		case "bj":
			ex = byte(ExchangeBJ)
		default:
			ex = byte(ExchangeSZ)
		}
		payload = append(payload, ex)
		payload = append(payload, []byte(num)...)
	}
	return &Frame{
		Control: Control01,
		Type:    TypeQuote,
		Data:    append(header, payload...),
	}, nil
}

type QuoteItem struct {
	Code   string
	Name   string
	Open   float64
	High   float64
	Low    float64
	Price  float64
	Volume float64
	Amount float64
}

func (q Quote) Decode(bs []byte) ([]*QuoteItem, error) {
	if len(bs) < 4 {
		return nil, errors.New("数据长度不足")
	}

	bs = bs[2:]
	count := int(Uint16LE(bs[:2]))
	bs = bs[2:]

	result := make([]*QuoteItem, 0, count)
	for i := 0; i < count; i++ {
		if len(bs) < 9 {
			break
		}
		item := &QuoteItem{
			Code: string(bs[1:7]),
		}

		var k K
		bs, k = decodeK(bs[9:])

		item.Price = k.Close
		item.Open = k.Open
		item.High = k.High
		item.Low = k.Low

		bs, _ = varUint(bs)
		bs, _ = varUint(bs)
		var totalHand int
		bs, totalHand = varUint(bs)
		item.Volume = float64(totalHand)
		bs, _ = varUint(bs)

		if len(bs) >= 4 {
			item.Amount = volumeEncoded(Uint32LE(bs[:4])) / 10000
			bs = bs[4:]
		}
		bs, _ = varUint(bs)
		bs, _ = varUint(bs)
		bs, _ = varUint(bs)
		bs, _ = varUint(bs)

		for j := 0; j < 5; j++ {
			if len(bs) < 1 {
				break
			}
			bs, _ = varPrice(bs)
			bs, _ = varPrice(bs)
			bs, _ = varUint(bs)
			bs, _ = varUint(bs)
		}

		if len(bs) >= 2 {
			bs = bs[2:]
		}
		bs, _ = varUint(bs)
		bs, _ = varUint(bs)
		bs, _ = varUint(bs)
		bs, _ = varUint(bs)
		if len(bs) >= 4 {
			bs = bs[4:]
		}

		result = append(result, item)
	}
	return result, nil
}
