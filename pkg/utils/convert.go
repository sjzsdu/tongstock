package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func Reverse(bs []byte) []byte {
	x := make([]byte, len(bs))
	for i, v := range bs {
		x[len(bs)-i-1] = v
	}
	return x
}

func Uint16LE(bs []byte) uint16 {
	return binary.LittleEndian.Uint16(bs)
}

func Uint32LE(bs []byte) uint32 {
	return binary.LittleEndian.Uint32(bs)
}

func PutUint16LE(b []byte, v uint16) {
	binary.LittleEndian.PutUint16(b, v)
}

func PutUint32LE(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b, v)
}

func BytesToString(bs []byte) string {
	return string(Reverse(bs))
}

func BytesToUint32(bs []byte) uint32 {
	return Uint32LE(Reverse(bs))
}

func BytesToUint16(bs []byte) uint16 {
	return Uint16LE(Reverse(bs))
}

func UTF8ToGBK(text []byte) []byte {
	r := bytes.NewReader(text)
	decoder := transform.NewReader(r, simplifiedchinese.GBK.NewDecoder())
	content, _ := io.ReadAll(decoder)
	return bytes.ReplaceAll(content, []byte{0x00}, []byte{})
}

func DecodeStockCode(code string) (string, string, error) {
	code = AddPrefix(code)
	if len(code) != 8 {
		return "", "", fmt.Errorf("股票代码长度错误,例如:SZ000001")
	}
	switch strings.ToLower(code[:2]) {
	case "sh":
		return "sh", code[2:], nil
	case "sz":
		return "sz", code[2:], nil
	case "bj":
		return "bj", code[2:], nil
	default:
		return "", "", fmt.Errorf("股票代码错误,例如:SZ000001")
	}
}

func AddPrefix(code string) string {
	if len(code) == 6 {
		switch {
		case code[:1] == "6":
			code = "sh" + code
		case code[:1] == "0":
			code = "sz" + code
		case code[:2] == "30":
			code = "sz" + code
		case code[:3] == "510", code[:3] == "511", code[:3] == "512", code[:3] == "513", code[:3] == "515":
			code = "sh" + code
		case code[:3] == "159":
			code = "sz" + code
		case code[:1] == "8", code[:2] == "92", code[:2] == "43":
			code = "bj" + code
		case code[:2] == "39":
			code = "sz" + code
		case code[:1] == "9":
			code = "sh" + code
		}
	}
	return code
}

func GetHourMinute(bs []byte) string {
	n := binary.BigEndian.Uint16(bs)
	h := n / 60
	m := n % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func GetTimeFromBytes(bs []byte, klineType uint8) time.Time {
	switch klineType {
	case 7, 8, 0, 1, 2, 3:
		yearMonthDay := binary.LittleEndian.Uint16(bs[:2])
		hourMinute := binary.LittleEndian.Uint16(bs[2:4])
		year := int(yearMonthDay>>11 + 2004)
		month := int((yearMonthDay % 2048) / 100)
		day := int((yearMonthDay % 2048) % 100)
		hour := int(hourMinute / 60)
		minute := int(hourMinute % 60)
		return time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
	default:
		yearMonthDay := binary.LittleEndian.Uint32(bs[:4])
		year := int(yearMonthDay / 10000)
		month := int((yearMonthDay % 10000) / 100)
		day := int(yearMonthDay % 100)
		return time.Date(year, time.Month(month), day, 15, 0, 0, 0, time.Local)
	}
}

func GetVolume(val uint32) float64 {
	ivol := int32(val)
	logpoint := ivol >> 24
	hleax := (ivol >> 16) & 0xff
	lheax := (ivol >> 8) & 0xff
	lleax := ivol & 0xff

	dwEcx := logpoint*2 - 0x7f
	dwEdx := logpoint*2 - 0x86
	dwEsi := logpoint*2 - 0x8e
	dwEax := logpoint*2 - 0x96

	tmpEax := dwEcx
	if dwEcx < 0 {
		tmpEax = -dwEcx
	}

	dbl_xmm6 := math.Exp2(float64(tmpEax))
	if dwEcx < 0 {
		dbl_xmm6 = 1.0 / dbl_xmm6
	}

	var dbl_xmm4 float64
	if hleax > 0x80 {
		dbl_xmm0 := math.Exp2(float64(dwEdx+1))*64.0 + math.Exp2(float64(dwEdx))*float64(hleax&0x7f)
		dbl_xmm4 = dbl_xmm0
	} else {
		if dwEdx >= 0 {
			dbl_xmm0 := math.Exp2(float64(dwEdx)) * float64(hleax)
			dbl_xmm4 = dbl_xmm0
		} else {
			dbl_xmm0 := (1 / math.Exp2(float64(-dwEdx))) * float64(hleax)
			dbl_xmm4 = dbl_xmm0
		}
	}

	scale := 1.0
	if (hleax & 0x80) != 0 {
		scale = 2.0
	}

	dbl_xmm3 := math.Exp2(float64(dwEsi)) * float64(lheax) * scale
	dbl_xmm1 := math.Exp2(float64(dwEax)) * float64(lleax) * scale

	return dbl_xmm6 + dbl_xmm4 + dbl_xmm3 + dbl_xmm1
}

func IsStock(code string) bool {
	code = strings.ToLower(AddPrefix(code))
	return strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") || strings.HasPrefix(code, "bj")
}

func IsETF(code string) bool {
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	if (strings.HasPrefix(code, "sh") && (code[2:4] == "51" || code[2:4] == "56" || code[2:4] == "58")) ||
		(strings.HasPrefix(code, "sz") && (code[2:4] == "15" || code[2:4] == "16")) {
		return true
	}
	return false
}
