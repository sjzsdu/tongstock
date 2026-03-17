package protocol

import (
	"math"
	"time"

	"github.com/sjzsdu/tongstock/pkg/utils"
)

func UTF8ToGBK(text []byte) []byte {
	return utils.UTF8ToGBK(text)
}

func Uint16LE(bs []byte) uint16 {
	return utils.Uint16LE(bs)
}

func Uint32LE(bs []byte) uint32 {
	return utils.Uint32LE(bs)
}

func GetTimeFromBytes(bs []byte, klineType uint8) time.Time {
	return utils.GetTimeFromBytes(bs, klineType)
}

func GetVolume(val uint32) float64 {
	return utils.GetVolume(val)
}

// varPrice decodes a TDX variable-length signed integer (units: 厘 = 0.001 yuan).
func varPrice(bs []byte) ([]byte, int64) {
	var val int64
	for i := range bs {
		if i == 0 {
			val += int64(bs[0] & 0x3F)
		} else {
			val += int64(bs[i]&0x7F) << uint8(6+(i-1)*7)
		}
		if bs[i]&0x80 == 0 {
			if len(bs) > 0 && bs[0]&0x40 > 0 {
				val = -val
			}
			return bs[i+1:], val
		}
	}
	return bs, 0
}

// varUint decodes a TDX variable-length unsigned integer.
func varUint(bs []byte) ([]byte, int) {
	var val int
	for i := range bs {
		if i == 0 {
			val += int(bs[0] & 0x3F)
		} else {
			val += int(bs[i]&0x7F) << uint8(6+(i-1)*7)
		}
		if bs[i]&0x80 == 0 {
			if len(bs) > 0 && bs[0]&0x40 > 0 {
				val = -val
			}
			return bs[i+1:], val
		}
	}
	return bs, 0
}

type K struct {
	Last, Open, High, Low, Close float64
}

// decodeK parses the variable-length intraday K price block (typically ~6 bytes).
// Prices returned in yuan.
func decodeK(bs []byte) ([]byte, K) {
	var closeRaw, lastDiff, openDiff, highDiff, lowDiff int64
	bs, closeRaw = varPrice(bs)
	bs, lastDiff = varPrice(bs)
	bs, openDiff = varPrice(bs)
	bs, highDiff = varPrice(bs)
	bs, lowDiff = varPrice(bs)

	c := float64(closeRaw) / 100
	return bs, K{
		Close: c,
		Last:  float64(closeRaw+lastDiff) / 100,
		Open:  float64(closeRaw+openDiff) / 100,
		High:  float64(closeRaw+highDiff) / 100,
		Low:   float64(closeRaw+lowDiff) / 100,
	}
}

// volumeEncoded decodes TDX floating-point volume encoding.
func volumeEncoded(val uint32) float64 {
	ivol := int32(val)
	logpoint := ivol >> 24
	hleax := (ivol >> 16) & 0xff
	lheax := (ivol >> 8) & 0xff
	lleax := ivol & 0xff

	dwEcx := logpoint*2 - 0x7f
	base := math.Exp2(float64(dwEcx))

	var mid float64
	if hleax > 0x80 {
		mid = base * (64.0 + float64(hleax&0x7f)) / 64.0
	} else {
		mid = base * float64(hleax) / 128.0
	}

	scale := 1.0
	if hleax&0x80 != 0 {
		scale = 2.0
	}
	return base + mid + base*float64(lheax)/32768.0*scale + base*float64(lleax)/8388608.0*scale
}
