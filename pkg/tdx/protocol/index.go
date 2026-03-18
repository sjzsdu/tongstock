package protocol

import (
	"time"

	"github.com/sjzsdu/tongstock/pkg/utils"
)

type indexBarStruct struct{}

var MIndexBar = indexBarStruct{}

func (k indexBarStruct) Frame(ktype uint8, code string, start, count uint16) (*Frame, error) {
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

	data := []byte{ex, 0x0}
	data = append(data, []byte(num)...)
	data = append(data, ktype, 0x0)
	data = append(data, 0x01, 0x0)
	data = append(data, uint8(start), uint8(start>>8))
	data = append(data, uint8(count), uint8(count>>8))
	data = append(data, make([]byte, 10)...)
	return &Frame{
		Control: Control01,
		Type:    TypeKline,
		Data:    data,
	}, nil
}

type IndexBar struct {
	Time      time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Amount    float64
	UpCount   uint16 // 上涨家数
	DownCount uint16 // 下跌家数
}

func (k indexBarStruct) Decode(bs []byte, ktype uint8) ([]*IndexBar, error) {
	if len(bs) < 2 {
		return nil, ErrDataLength
	}

	count := int(Uint16LE(bs[:2]))
	bs = bs[2:]

	var lastClose float64
	items := make([]*IndexBar, 0, count)
	for i := 0; i < count && len(bs) >= 12; i++ {
		t := utils.GetTimeFromBytes(bs[:4], ktype)
		bs = bs[4:]

		var openRaw, closeRaw, highRaw, lowRaw int64
		bs, openRaw = varPrice(bs)
		bs, closeRaw = varPrice(bs)
		bs, highRaw = varPrice(bs)
		bs, lowRaw = varPrice(bs)

		open := lastClose + float64(openRaw)/1000
		close := open + float64(closeRaw)/1000
		high := open + float64(highRaw)/1000
		low := open + float64(lowRaw)/1000
		lastClose = close

		if len(bs) < 12 {
			break
		}
		vol := volumeEncoded(Uint32LE(bs[:4]))
		amount := volumeEncoded(Uint32LE(bs[4:8])) / 100
		bs = bs[8:]

		upCount := Uint16LE(bs[:2])
		downCount := Uint16LE(bs[2:4])
		bs = bs[4:]

		items = append(items, &IndexBar{
			Time:      t,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    vol,
			Amount:    amount,
			UpCount:   upCount,
			DownCount: downCount,
		})
	}
	return items, nil
}
