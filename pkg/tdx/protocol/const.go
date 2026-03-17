package protocol

import "time"

const (
	TypeConnect = 0x000D
	TypeHeart   = 0x0004
	TypeCount   = 0x044E
	TypeCode    = 0x0450
	TypeQuote   = 0x053E
	TypeMinute  = 0x051D

	TypeMinuteTrade        = 0x0FC5
	TypeHistoryMinute      = 0x0FB4
	TypeHistoryMinuteTrade = 0x0FB5
	TypeKline              = 0x052D
)

const (
	Control01 Control = 0x01
)

var ExchangeEstablish = time.Date(1990, 12, 19, 0, 0, 0, 0, time.Local)

type Exchange uint8

func (e Exchange) Uint8() uint8 { return uint8(e) }

const (
	ExchangeSZ Exchange = iota
	ExchangeSH
	ExchangeBJ
)

func (e Exchange) String() string {
	switch e {
	case ExchangeSH:
		return "sh"
	case ExchangeSZ:
		return "sz"
	case ExchangeBJ:
		return "bj"
	default:
		return ""
	}
}

func ParseExchange(s string) Exchange {
	switch s {
	case "sh", "shanghai", "上海":
		return ExchangeSH
	case "sz", "shenzhen", "深圳":
		return ExchangeSZ
	case "bj", "beijing", "北京":
		return ExchangeBJ
	default:
		return ExchangeSZ
	}
}
