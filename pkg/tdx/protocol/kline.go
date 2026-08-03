package protocol

import (
	"errors"
	"math"
	"time"

	"github.com/sjzsdu/tongstock/pkg/utils"
)

type Kline struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64
}

type klineStruct struct{}

func (k klineStruct) Frame(ktype uint8, code string, start, count uint16) (*Frame, error) {
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

func (k klineStruct) Decode(bs []byte, ktype uint8) ([]*Kline, error) {
	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}

	count := int(Uint16LE(bs[:2]))
	bs = bs[2:]

	var lastClose float64
	items := make([]*Kline, 0, count)

	// TDX协议: 第一条K线使用绝对价格，后续使用增量编码
	// 增量单位是厘 (0.001元)
	const maxPrice = 1000000   // 单价上限100万元，A股不可能超过
	const maxPriceChange = 5.0 // 单日价格变动上限500%

	for i := 0; i < count && len(bs) >= 12; i++ {
		t := utils.GetTimeFromBytes(bs[:4], ktype)
		bs = bs[4:]

		var openRaw, closeRaw, highRaw, lowRaw int64
		var ok bool
		if bs, openRaw, ok = readKlinePrice(bs); !ok {
			return nil, ErrDataLength
		}
		if bs, closeRaw, ok = readKlinePrice(bs); !ok {
			return nil, ErrDataLength
		}
		if bs, highRaw, ok = readKlinePrice(bs); !ok {
			return nil, ErrDataLength
		}
		if bs, lowRaw, ok = readKlinePrice(bs); !ok {
			return nil, ErrDataLength
		}

		var open, close, high, low float64
		if i == 0 {
			// 第一条K线: 绝对价格 (除以1000得到元)
			open = float64(openRaw) / 1000
			close = open + float64(closeRaw)/1000
			high = open + float64(highRaw)/1000
			low = open + float64(lowRaw)/1000
		} else {
			// 后续K线: 增量编码
			open = lastClose + float64(openRaw)/1000
			close = open + float64(closeRaw)/1000
			high = open + float64(highRaw)/1000
			low = open + float64(lowRaw)/1000
		}

		// 成交量和成交额是每条记录的固定尾部。必须在任何校验分支前消费，
		// 否则一条坏记录会让后续记录从错误的字节边界开始解码。
		if len(bs) < 8 {
			return nil, ErrDataLength
		}
		vol := volumeEncoded(Uint32LE(bs[:4]))
		if ktype <= 6 || ktype == 8 {
			vol /= 100
		}
		amount := volumeEncoded(Uint32LE(bs[4:8])) / 100
		bs = bs[8:]

		previousClose := lastClose
		// 后续记录的增量以协议中的前一条收盘价为基准，即使当前记录
		// 最终因质量校验被丢弃，也必须推进该解码状态。
		lastClose = close

		maxDate := time.Now().AddDate(0, 0, 1)
		minDate := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)
		invalidNumber := func(value float64) bool {
			return value <= 0 || math.IsNaN(value) || math.IsInf(value, 0)
		}
		if t.IsZero() || t.Before(minDate) || t.After(maxDate) ||
			invalidNumber(open) || invalidNumber(close) || invalidNumber(high) || invalidNumber(low) ||
			open > maxPrice || close > maxPrice || high > maxPrice || low > maxPrice ||
			high < low || high < open || high < close || low > open || low > close {
			continue
		}

		// 检查与前一条K线的价格变动是否合理
		if i > 0 && previousClose > 0 {
			changeRatio := close / previousClose
			if changeRatio > maxPriceChange || changeRatio < 1.0/maxPriceChange {
				continue
			}
		}

		items = append(items, &Kline{
			Time:   t,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: vol,
			Amount: amount,
		})
	}
	return items, nil
}

// readKlinePrice is the bounds-checked variant used by the paged K-line
// decoder. The legacy varPrice helper cannot distinguish an unterminated
// value from a decoded zero, which would make the record boundary ambiguous.
func readKlinePrice(bs []byte) ([]byte, int64, bool) {
	var value int64
	for i, current := range bs {
		if i == 0 {
			value += int64(current & 0x3f)
		} else {
			value += int64(current&0x7f) << uint8(6+(i-1)*7)
		}
		if current&0x80 == 0 {
			if bs[0]&0x40 != 0 {
				value = -value
			}
			return bs[i+1:], value, true
		}
		// int64 cannot represent additional payload bits safely.
		if i >= 8 {
			return nil, 0, false
		}
	}
	return nil, 0, false
}
