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

type BidAsk struct {
	BidPrice  float64
	AskPrice  float64
	BidVolume int
	AskVolume int
}

type QuoteItem struct {
	Code      string
	Name      string
	Open      float64
	High      float64
	Low       float64
	Price     float64
	LastClose float64
	Volume    float64
	Amount    float64
	SVol      int // 内盘(主动卖)
	BVol      int // 外盘(主动买)
	BidAsk    [5]BidAsk
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
		item.LastClose = k.Last
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

		var sVol, bVol int
		bs, sVol = varUint(bs)
		bs, bVol = varUint(bs)
		item.SVol = sVol
		item.BVol = bVol
		bs, _ = varUint(bs)
		bs, _ = varUint(bs)

		for j := 0; j < 5; j++ {
			if len(bs) < 1 {
				break
			}
			var bidRaw, askRaw int64
			var bidVol, askVol int
			bs, bidRaw = varPrice(bs)
			bs, askRaw = varPrice(bs)
			bs, bidVol = varUint(bs)
			bs, askVol = varUint(bs)
			item.BidAsk[j] = BidAsk{
				BidPrice:  float64(int64(item.Price*100)+bidRaw) / 100,
				AskPrice:  float64(int64(item.Price*100)+askRaw) / 100,
				BidVolume: bidVol,
				AskVolume: askVol,
			}
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
