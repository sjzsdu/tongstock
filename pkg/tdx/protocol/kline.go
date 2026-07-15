package protocol

import (
	"errors"
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
	const maxPrice = 1000000 // 单价上限100万元，A股不可能超过
	const maxPriceChange = 5.0 // 单日价格变动上限500%

	for i := 0; i < count && len(bs) >= 12; i++ {
		t := utils.GetTimeFromBytes(bs[:4], ktype)
		bs = bs[4:]

		var openRaw, closeRaw, highRaw, lowRaw int64
		bs, openRaw = varPrice(bs)
		bs, closeRaw = varPrice(bs)
		bs, highRaw = varPrice(bs)
		bs, lowRaw = varPrice(bs)

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

		// 校验解码后的价格是否合理
		if open <= 0 || close <= 0 || high <= 0 || low <= 0 {
			lastClose = close
			continue
		}
		if open > maxPrice || close > maxPrice {
			lastClose = close
			continue
		}
		if high < low {
			lastClose = close
			continue
		}

		// 检查与前一条K线的价格变动是否合理
		if i > 0 && lastClose > 0 {
			changeRatio := close / lastClose
			if changeRatio > maxPriceChange || changeRatio < 1.0/maxPriceChange {
				// 价格变动超过500%，数据可能损坏，跳过
				lastClose = close
				continue
			}
		}

		lastClose = close

		if len(bs) < 8 {
			break
		}
		vol := volumeEncoded(Uint32LE(bs[:4]))
		if ktype <= 6 || ktype == 8 {
			vol /= 100
		}
		amount := volumeEncoded(Uint32LE(bs[4:8])) / 100
		bs = bs[8:]

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
