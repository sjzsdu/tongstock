package protocol

var (
	MConnect = Connect{}
	MHeart   = Heart{}
	MCount   = Count{}
	MQuote   = Quote{}
	MCode    = Code{}
	MKline   = klineStruct{}
)

type ConnectResp struct {
	Info string
}

type Connect struct{}

func (c Connect) Frame() *Frame {
	return &Frame{
		Control: Control01,
		Type:    TypeConnect,
		Data:    []byte{0x01},
	}
}

func (c Connect) Decode(bs []byte) (*ConnectResp, error) {
	if len(bs) < 68 {
		return nil, ErrDataLength
	}
	return &ConnectResp{Info: string(UTF8ToGBK(bs[68:]))}, nil
}

type Heart struct{}

func (h Heart) Frame() *Frame {
	return &Frame{
		Control: Control01,
		Type:    TypeHeart,
	}
}

type Count struct{}

func (c Count) Frame() *Frame {
	return &Frame{
		Control: Control01,
		Type:    TypeCount,
		Data:    []byte{0x01},
	}
}

type CountResp struct {
	Count int
}

func (c Count) Decode(bs []byte) (*CountResp, error) {
	if len(bs) < 4 {
		return nil, ErrDataLength
	}
	return &CountResp{Count: int(Uint16LE(bs[2:4]))}, nil
}

var ErrDataLength = Err("数据长度不足")

func Err(msg string) error {
	return &ProtocolError{Message: msg}
}

type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string {
	return e.Message
}
