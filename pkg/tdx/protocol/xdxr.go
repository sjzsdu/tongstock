package protocol

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/sjzsdu/tongstock/pkg/utils"
)

type xdxrStruct struct{}

var MXdXr = xdxrStruct{}

func (x xdxrStruct) Frame(code string) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	data := []byte{0x01, 0x00}
	data = append(data, exchange)
	data = append(data, []byte(number)...)
	return &Frame{
		Control: Control01,
		Type:    TypeXdXr,
		Data:    data,
	}, nil
}

type XdXrCategory uint8

const (
	XdXrChuQuanChuXi         XdXrCategory = 1  // 除权除息
	XdXrSongPeiGuShangShi    XdXrCategory = 2  // 送配股上市
	XdXrFeiLiuTongGuShangShi XdXrCategory = 3  // 非流通股上市
	XdXrGuBenBianDong        XdXrCategory = 4  // 未知股本变动
	XdXrGuBenBianHua         XdXrCategory = 5  // 股本变化
	XdXrZengFaXinGu          XdXrCategory = 6  // 增发新股
	XdXrGuFenHuiGou          XdXrCategory = 7  // 股份回购
	XdXrZengFaShangShi       XdXrCategory = 8  // 增发新股上市
	XdXrZhuanPeiGuShangShi   XdXrCategory = 9  // 转配股上市
	XdXrKeZhuanZhaiShangShi  XdXrCategory = 10 // 可转债上市
	XdXrKuoSuoGu             XdXrCategory = 11 // 扩缩股
	XdXrFeiLiuTongGuSuoGu    XdXrCategory = 12 // 非流通股缩股
	XdXrSongRenGouQuanZheng  XdXrCategory = 13 // 送认购权证
	XdXrSongRenGuQuanZheng   XdXrCategory = 14 // 送认沽权证
)

func (c XdXrCategory) String() string {
	switch c {
	case XdXrChuQuanChuXi:
		return "除权除息"
	case XdXrSongPeiGuShangShi:
		return "送配股上市"
	case XdXrFeiLiuTongGuShangShi:
		return "非流通股上市"
	case XdXrGuBenBianDong:
		return "未知股本变动"
	case XdXrGuBenBianHua:
		return "股本变化"
	case XdXrZengFaXinGu:
		return "增发新股"
	case XdXrGuFenHuiGou:
		return "股份回购"
	case XdXrZengFaShangShi:
		return "增发新股上市"
	case XdXrZhuanPeiGuShangShi:
		return "转配股上市"
	case XdXrKeZhuanZhaiShangShi:
		return "可转债上市"
	case XdXrKuoSuoGu:
		return "扩缩股"
	case XdXrFeiLiuTongGuSuoGu:
		return "非流通股缩股"
	case XdXrSongRenGouQuanZheng:
		return "送认购权证"
	case XdXrSongRenGuQuanZheng:
		return "送认沽权证"
	default:
		return "未知"
	}
}

type XdXrItem struct {
	Date     time.Time
	Category XdXrCategory

	FenHong     float32 // 分红(每股,元)
	PeiGuJia    float32 // 配股价
	SongZhuanGu float32 // 送转股(每10股)
	PeiGu       float32 // 配股(每10股)

	SuoGu float32 // 缩股比例

	XingQuanJia float32 // 行权价
	FenShu      float32 // 份数

	PanQianLiuTong float64 // 盘前流通股本(万股)
	PanHouLiuTong  float64 // 盘后流通股本(万股)
	QianZongGuBen  float64 // 前总股本(万股)
	HouZongGuBen   float64 // 后总股本(万股)
}

func (x xdxrStruct) Decode(bs []byte) ([]*XdXrItem, error) {
	if len(bs) < 11 {
		return nil, ErrDataLength
	}

	bs = bs[9:]
	if len(bs) < 2 {
		return nil, ErrDataLength
	}
	count := int(Uint16LE(bs[:2]))
	bs = bs[2:]

	items := make([]*XdXrItem, 0, count)
	for i := 0; i < count; i++ {
		if len(bs) < 29 {
			break
		}
		bs = bs[8:]

		dateVal := Uint32LE(bs[:4])
		year := int(dateVal / 10000)
		month := int((dateVal % 10000) / 100)
		day := int(dateVal % 100)
		bs = bs[4:]

		category := XdXrCategory(bs[0])
		bs = bs[1:]

		item := &XdXrItem{
			Date:     time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local),
			Category: category,
		}

		if len(bs) < 16 {
			break
		}

		switch category {
		case XdXrChuQuanChuXi: // 1
			item.FenHong = readFloat32LE(bs[0:4])
			item.PeiGuJia = readFloat32LE(bs[4:8])
			item.SongZhuanGu = readFloat32LE(bs[8:12])
			item.PeiGu = readFloat32LE(bs[12:16])
		case XdXrKuoSuoGu, XdXrFeiLiuTongGuSuoGu: // 11, 12
			item.SuoGu = readFloat32LE(bs[8:12])
		case XdXrSongRenGouQuanZheng, XdXrSongRenGuQuanZheng: // 13, 14
			item.XingQuanJia = readFloat32LE(bs[0:4])
			item.FenShu = readFloat32LE(bs[8:12])
		default: // 2~10: 股本变动
			item.PanQianLiuTong = float64(utils.GetVolume(Uint32LE(bs[0:4])))
			item.QianZongGuBen = float64(utils.GetVolume(Uint32LE(bs[4:8])))
			item.PanHouLiuTong = float64(utils.GetVolume(Uint32LE(bs[8:12])))
			item.HouZongGuBen = float64(utils.GetVolume(Uint32LE(bs[12:16])))
		}

		bs = bs[16:]
		items = append(items, item)
	}

	return items, nil
}

func readFloat32LE(bs []byte) float32 {
	bits := binary.LittleEndian.Uint32(bs)
	return math.Float32frombits(bits)
}
