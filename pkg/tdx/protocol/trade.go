package protocol

import (
	"errors"
	"time"
)

type TradeResp struct {
	Count uint16
	List  []*Trade
}

type Trade struct {
	Time   time.Time
	Price  float64
	Volume int
	Status int
}

type TradeCache struct {
	Date string
	Code string
}

type tradeStruct struct{}

var MTrade = tradeStruct{}

func (m tradeStruct) Frame(code string, start, count uint16) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	dataBs := []byte{exchange, 0x0}
	dataBs = append(dataBs, []byte(number)...)
	dataBs = append(dataBs, byte(start), byte(start>>8))
	dataBs = append(dataBs, byte(count), byte(count>>8))
	return &Frame{
		Control: Control01,
		Type:    TypeMinuteTrade,
		Data:    dataBs,
	}, nil
}

func (m tradeStruct) Decode(bs []byte, c TradeCache) (*TradeResp, error) {
	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}

	resp := &TradeResp{
		Count: Uint16LE(bs[:2]),
	}

	bs = bs[6:]

	lastPrice := float64(0)
	loc := time.FixedZone("CST", 8*3600)
	for i := uint16(0); i < resp.Count && len(bs) >= 8; i++ {
		timeBytes := [2]byte{bs[0], bs[1]}
		bs = bs[2:]
		t := parseHourMinute(timeBytes)
		dateStr := c.Date
		if dateStr == "" {
			dateStr = time.Now().Format("20060102")
		}
		dt, err := time.ParseInLocation("2006010215:04", dateStr+t, loc)
		if err != nil {
			dt = time.Now()
		}
		var priceRaw int64
		bs, priceRaw = varPrice(bs)
		lastPrice += float64(priceRaw) / 100
		var volume int
		bs, volume = varUint(bs)
		var status int
		bs, status = varUint(bs)
		resp.List = append(resp.List, &Trade{
			Time:   dt,
			Price:  lastPrice,
			Volume: volume,
			Status: status,
		})
	}

	return resp, nil
}

type historyTradeStruct struct{}

var MHistoryTrade = historyTradeStruct{}

func (m historyTradeStruct) Frame(date, code string, start, count uint16) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	year, month, day, err := parseDateStr(date)
	if err != nil {
		return nil, err
	}
	dateNum := uint32(year*10000 + month*100 + day)
	dataBs := []byte{byte(dateNum), byte(dateNum >> 8), byte(dateNum >> 16), byte(dateNum >> 24)}
	dataBs = append(dataBs, exchange, 0x0)
	dataBs = append(dataBs, []byte(number)...)
	dataBs = append(dataBs, byte(start), byte(start>>8))
	dataBs = append(dataBs, byte(count), byte(count>>8))
	return &Frame{
		Control: Control01,
		Type:    TypeHistoryMinuteTrade,
		Data:    dataBs,
	}, nil
}

func (m historyTradeStruct) Decode(bs []byte, c TradeCache) (*TradeResp, error) {
	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}

	resp := &TradeResp{
		Count: Uint16LE(bs[:2]),
	}

	bs = bs[6:]

	lastPrice := float64(0)
	loc := time.FixedZone("CST", 8*3600)
	for i := uint16(0); i < resp.Count && len(bs) >= 8; i++ {
		timeBytes := [2]byte{bs[0], bs[1]}
		bs = bs[2:]
		t := parseHourMinute(timeBytes)
		dateStr := c.Date
		if dateStr == "" {
			dateStr = time.Now().Format("20060102")
		}
		dt, err := time.ParseInLocation("2006010215:04", dateStr+t, loc)
		if err != nil {
			dt = time.Now()
		}
		var priceRaw int64
		bs, priceRaw = varPrice(bs)
		lastPrice += float64(priceRaw) / 100
		var volume int
		bs, volume = varUint(bs)
		var status int
		bs, status = varUint(bs)
		resp.List = append(resp.List, &Trade{
			Time:   dt,
			Price:  lastPrice,
			Volume: volume,
			Status: status,
		})
	}

	return resp, nil
}

func parseHourMinute(b [2]byte) string {
	h := ((b[0] >> 4) * 10) + (b[0] & 0x0F)
	m := ((b[1] >> 4) * 10) + (b[1] & 0x0F)
	return string('0'+h/10) + string('0'+h%10) + ":" + string('0'+m/10) + string('0'+m%10)
}
