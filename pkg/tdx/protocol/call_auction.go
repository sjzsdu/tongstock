package protocol

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

type CallAuctionResp struct {
	Count uint16
	List  []*CallAuction
}

type CallAuction struct {
	Time      time.Time `json:"time"`
	Price     float64   `json:"price"`
	Match     int64     `json:"match"`     // 匹配量
	Unmatched int64     `json:"unmatched"` // 未匹配量(绝对值)
	Flag      int8      `json:"flag"`      // 1=未匹配为买单, -1=未匹配为卖单
}

type callAuctionStruct struct{}

var MCallAuction = callAuctionStruct{}

func (c callAuctionStruct) Frame(code string) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	codeBs := []byte(number)
	codeBs = append(codeBs, []byte{
		0x00, 0x00, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
	}...)
	return &Frame{
		Control: Control01,
		Type:    TypeCallAuction,
		Data:    append([]byte{exchange, 0x00}, codeBs...),
	}, nil
}

func (c callAuctionStruct) Decode(bs []byte) (*CallAuctionResp, error) {
	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}

	count := Uint16LE(bs[:2])
	resp := &CallAuctionResp{
		Count: count,
		List:  make([]*CallAuction, 0, count),
	}

	bs = bs[2:]

	now := time.Now()
	for i := uint16(0); i < resp.Count; i++ {
		if len(bs) < 16 {
			break
		}

		// 每条记录固定 16 字节
		n := Uint16LE(bs[:2])
		hour := int(n / 60)
		minute := int(n % 60)

		price := math.Float32frombits(binary.LittleEndian.Uint32(bs[2:6]))
		match := int64(Uint32LE(bs[6:10]))
		unmatched := int64(int16(Uint16LE(bs[10:12])))
		second := int(bs[15])

		flag := int8(1)
		if unmatched < 0 {
			flag = -1
			unmatched = -unmatched
		}

		a := &CallAuction{
			Time:      time.Date(now.Year(), now.Month(), now.Day(), hour, minute, second, 0, now.Location()),
			Price:     float64(price),
			Match:     match,
			Unmatched: unmatched,
			Flag:      flag,
		}
		resp.List = append(resp.List, a)

		bs = bs[16:]
	}

	return resp, nil
}
