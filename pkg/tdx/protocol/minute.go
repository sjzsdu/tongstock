package protocol

import (
	"errors"
	"fmt"
	"time"
)

type MinuteResp struct {
	Count uint16
	List  []PriceNumber
}

type PriceNumber struct {
	Time   string
	Price  float64
	Number int
}

type minuteStruct struct{}

var MMinute = minuteStruct{}

func (m minuteStruct) Frame(code string) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	codeBs := []byte(number)
	codeBs = append(codeBs, 0x0, 0x0, 0x0, 0x0)
	return &Frame{
		Control: Control01,
		Type:    TypeMinute,
		Data:    append([]byte{exchange, 0x0}, codeBs...),
	}, nil
}

func (m minuteStruct) Decode(bs []byte) (*MinuteResp, error) {
	if len(bs) < 6 {
		return nil, errors.New("数据长度不足")
	}

	resp := &MinuteResp{
		Count: Uint16LE(bs[:2]),
	}

	bs = bs[6:]
	price := float64(0)

	t := time.Date(0, 0, 0, 9, 0, 0, 0, time.Local)
	for i := uint16(0); i < resp.Count; i++ {
		var priceRaw int64
		bs, priceRaw = varPrice(bs)
		bs, _ = varUint(bs)
		price = float64(priceRaw) / 1000
		var number int
		bs, number = varUint(bs)
		if i == 120 {
			t = t.Add(time.Hour * 2)
		}
		resp.List = append(resp.List, PriceNumber{
			Time:   t.Add(time.Minute * time.Duration(i)).Format("15:04"),
			Price:  price,
			Number: number,
		})
	}

	return resp, nil
}

type historyMinuteStruct struct{}

var MHistoryMinute = historyMinuteStruct{}

func (m historyMinuteStruct) Frame(date, code string) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	year, month, day, err := parseDateStr(date)
	if err != nil {
		return nil, err
	}
	dateBs := []byte{byte(year >> 8), byte(year), byte(month), byte(day)}
	dataBs := dateBs
	dataBs = append(dataBs, exchange)
	dataBs = append(dataBs, []byte(number)...)
	return &Frame{
		Control: Control01,
		Type:    TypeHistoryMinute,
		Data:    dataBs,
	}, nil
}

func (m historyMinuteStruct) Decode(bs []byte) (*MinuteResp, error) {
	if len(bs) < 6 {
		return nil, errors.New("数据长度不足")
	}

	resp := &MinuteResp{
		Count: Uint16LE(bs[:2]),
	}

	bs = bs[6:]

	lastPrice := float64(0)
	t := time.Date(0, 0, 0, 9, 30, 0, 0, time.Local)
	for i := uint16(0); i < resp.Count; i++ {
		bs, price := varPrice(bs)
		bs, _ = varPrice(bs)
		lastPrice += float64(price) / 1000
		var number int
		bs, number = varUint(bs)

		if i == 120 {
			t = t.Add(time.Minute * 90)
		}
		resp.List = append(resp.List, PriceNumber{
			Time:   t.Add(time.Minute * time.Duration(i+1)).Format("15:04"),
			Price:  lastPrice,
			Number: number,
		})
	}
	return resp, nil
}

func decodeCode(code string) (byte, string, error) {
	if len(code) == 8 {
		var market byte
		switch code[:2] {
		case "sh":
			market = byte(ExchangeSH)
		case "sz":
			market = byte(ExchangeSZ)
		case "bj":
			market = byte(ExchangeBJ)
		default:
			market = byte(ExchangeSZ)
		}
		return market, code[2:], nil
	}
	if len(code) == 6 {
		var market byte
		switch code[0] {
		case '6':
			market = byte(ExchangeSH)
		case '0', '3':
			market = byte(ExchangeSZ)
		default:
			market = byte(ExchangeSZ)
		}
		return market, code, nil
	}
	return 0, "", errors.New("invalid code format")
}

func parseInts(s string, fn func([]int)) ([]byte, error) {
	var vals []int
	var cur int
	var hasDigit bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			cur = cur*10 + int(c-'0')
			hasDigit = true
		} else if hasDigit {
			vals = append(vals, cur)
			cur = 0
			hasDigit = false
		}
	}
	if hasDigit {
		vals = append(vals, cur)
	}
	fn(vals)
	return nil, nil
}

func parseDateStr(s string) (int, int, int, error) {
	if len(s) != 8 {
		return 0, 0, 0, errors.New("invalid date format, expected YYYYMMDD")
	}
	var year, month, day int
	_, err := fmt.Sscanf(s, "%4d%2d%2d", &year, &month, &day)
	if err != nil {
		return 0, 0, 0, errors.New("invalid date format")
	}
	return year, month, day, nil
}
