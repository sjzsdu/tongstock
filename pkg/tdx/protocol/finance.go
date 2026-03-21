package protocol

import (
	"encoding/binary"
	"math"
)

type financeStruct struct{}

var MFinance = financeStruct{}

func (f financeStruct) Frame(code string) (*Frame, error) {
	exchange, number, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	data := []byte{0x01, 0x00}
	data = append(data, exchange)
	data = append(data, []byte(number)...)
	return &Frame{
		Control: Control01,
		Type:    TypeFinance,
		Data:    data,
	}, nil
}

type FinanceInfo struct {
	LiuTongGuBen       float64 // 流通股本(股)
	Province           uint16
	Industry           uint16
	UpdatedDate        uint32
	IPODate            uint32
	ZongGuBen          float64 // 总股本(股)
	GuoJiaGu           float64 // 国家股(股)
	FaQiRenFaRenGu     float64 // 发起人法人股(股)
	FaRenGu            float64 // 法人股(股)
	BGu                float64 // B股(股)
	HGu                float64 // H股(股)
	ZhiGongGu          float64 // 职工股(股)
	ZongZiChan         float64 // 总资产(元)
	LiuDongZiChan      float64 // 流动资产(元)
	GuDingZiChan       float64 // 固定资产(元)
	WuXingZiChan       float64 // 无形资产(元)
	GuDongRenShu       float64 // 股东人数
	LiuDongFuZhai      float64 // 流动负债(元)
	ChangQiFuZhai      float64 // 长期负债(元)
	ZiBenGongJiJin     float64 // 资本公积金(元)
	JingZiChan         float64 // 净资产(元)
	ZhuYingShouRu      float64 // 主营收入(元)
	ZhuYingLiRun       float64 // 主营利润(元)
	YingShouZhangKuan  float64 // 应收帐款(元)
	YingYeLiRun        float64 // 营业利润(元)
	TouZiShouYi        float64 // 投资收益(元)
	JingYingXianJinLiu float64 // 经营现金流(元)
	ZongXianJinLiu     float64 // 总现金流(元)
	CunHuo             float64 // 存货(元)
	LiRunZongHe        float64 // 利润总额(元)
	ShuiHouLiRun       float64 // 税后利润(元)
	JingLiRun          float64 // 净利润(元)
	WeiFenPeiLiRun     float64 // 未分配利润(元)
	MeiGuJingZiChan    float64 // 每股净资产(元)
	BaoLiu2            float64
}

// pytdx struct: "<fHHIIffffffffffffffffffffffffffffff"
// float + H + H + I + I + 30*float = 4+2+2+4+4+30*4 = 136 bytes
const financeRecordSize = 136

func (f financeStruct) Decode(bs []byte) (*FinanceInfo, error) {
	if len(bs) < 2+7+financeRecordSize {
		return nil, ErrDataLength
	}

	bs = bs[2:]
	bs = bs[7:]

	info := &FinanceInfo{}
	info.LiuTongGuBen = float64(readFloat32(bs[0:4]))
	info.Province = binary.LittleEndian.Uint16(bs[4:6])
	info.Industry = binary.LittleEndian.Uint16(bs[6:8])
	info.UpdatedDate = binary.LittleEndian.Uint32(bs[8:12])
	info.IPODate = binary.LittleEndian.Uint32(bs[12:16])
	info.ZongGuBen = float64(readFloat32(bs[16:20]))
	info.GuoJiaGu = float64(readFloat32(bs[20:24]))
	info.FaQiRenFaRenGu = float64(readFloat32(bs[24:28]))
	info.FaRenGu = float64(readFloat32(bs[28:32]))
	info.BGu = float64(readFloat32(bs[32:36]))
	info.HGu = float64(readFloat32(bs[36:40]))
	info.ZhiGongGu = float64(readFloat32(bs[40:44]))
	info.ZongZiChan = float64(readFloat32(bs[44:48]))
	info.LiuDongZiChan = float64(readFloat32(bs[48:52]))
	info.GuDingZiChan = float64(readFloat32(bs[52:56]))
	info.WuXingZiChan = float64(readFloat32(bs[56:60]))
	info.GuDongRenShu = float64(readFloat32(bs[60:64]))
	info.LiuDongFuZhai = float64(readFloat32(bs[64:68]))
	info.ChangQiFuZhai = float64(readFloat32(bs[68:72]))
	info.ZiBenGongJiJin = float64(readFloat32(bs[72:76]))
	info.JingZiChan = float64(readFloat32(bs[76:80]))
	info.ZhuYingShouRu = float64(readFloat32(bs[80:84]))
	info.ZhuYingLiRun = float64(readFloat32(bs[84:88]))
	info.YingShouZhangKuan = float64(readFloat32(bs[88:92]))
	info.YingYeLiRun = float64(readFloat32(bs[92:96]))
	info.TouZiShouYi = float64(readFloat32(bs[96:100]))
	info.JingYingXianJinLiu = float64(readFloat32(bs[100:104]))
	info.ZongXianJinLiu = float64(readFloat32(bs[104:108]))
	info.CunHuo = float64(readFloat32(bs[108:112]))
	info.LiRunZongHe = float64(readFloat32(bs[112:116]))
	info.ShuiHouLiRun = float64(readFloat32(bs[116:120]))
	info.JingLiRun = float64(readFloat32(bs[120:124]))
	info.WeiFenPeiLiRun = float64(readFloat32(bs[124:128]))
	info.MeiGuJingZiChan = float64(readFloat32(bs[128:132]))
	info.BaoLiu2 = float64(readFloat32(bs[132:136]))

	return info, nil
}

func readFloat32(bs []byte) float32 {
	bits := binary.LittleEndian.Uint32(bs)
	return math.Float32frombits(bits)
}
