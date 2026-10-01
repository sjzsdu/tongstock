package protocol

import (
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/utils"
)

type indexBarStruct struct{}

var MIndexBar = indexBarStruct{}

func (k indexBarStruct) Frame(ktype uint8, code string, start, count uint16) (*Frame, error) {
	ex, num, err := DecodeIndexCode(code)
	if err != nil {
		return nil, err
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

// DecodeIndexCode 解析指数代码的市场归属。
//
// 不能复用 DecodeStockCode：它按股票规则路由，0 开头归深市，
// 而 0 开头的六位数在通达信指数体系里是上证指数（000001 上证指数、
// 000300 沪深300 等），按股票路由会把上证指数当成深市股票查询，
// 返回完全错乱的数据（如负价格、30450 年的日期）。
// 指数市场规则：999xxx/000xxx → 上交所，399xxx → 深交所，
// 880xxx 是通达信自编板块指数（上交所通道查询）。
func DecodeIndexCode(code string) (byte, string, error) {
	code = strings.TrimSpace(code)
	// 去掉可能的市场前缀
	if len(code) == 8 {
		switch strings.ToLower(code[:2]) {
		case "sh", "sz", "bj":
			code = code[2:]
		}
	}
	if len(code) != 6 {
		return 0, "", fmt.Errorf("指数代码长度错误: %s", code)
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			return 0, "", fmt.Errorf("指数代码格式错误: %s", code)
		}
	}
	switch {
	case strings.HasPrefix(code, "399"):
		return byte(ExchangeSZ), code, nil
	default:
		// 000xxx / 999xxx / 880xxx 及其他均按上交所指数查询
		return byte(ExchangeSH), code, nil
	}
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
