package protocol

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestKlineDecodeConsumesRejectedRecordTail(t *testing.T) {
	payload := []byte{3, 0}
	payload = appendTestKline(payload, 20260730, 10000, 100, 200, 0)
	// high < low: this record must be rejected without shifting the cursor.
	payload = appendTestKline(payload, 20260731, 0, 0, 0, 100)
	payload = appendTestKline(payload, 20260803, 0, 100, 200, 0)

	items, err := MKline.Decode(payload, TypeKlineDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("decoded rows = %d, want 2", len(items))
	}
	if got := items[1].Time.Format("20060102"); got != "20260803" {
		t.Fatalf("last date = %s, want 20260803", got)
	}
	if math.Abs(items[1].Close-10.2) > 1e-9 {
		t.Fatalf("last close = %v, want 10.2", items[1].Close)
	}
}

func TestKlineDecodeRejectsUnterminatedPrice(t *testing.T) {
	payload := []byte{1, 0}
	date := make([]byte, 4)
	binary.LittleEndian.PutUint32(date, 20260803)
	payload = append(payload, date...)
	payload = append(payload, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80)
	payload = append(payload, make([]byte, 8)...)

	if _, err := MKline.Decode(payload, TypeKlineDay); err == nil {
		t.Fatal("unterminated variable price was accepted")
	}
}

func appendTestKline(dst []byte, date uint32, prices ...int64) []byte {
	rawDate := make([]byte, 4)
	binary.LittleEndian.PutUint32(rawDate, date)
	dst = append(dst, rawDate...)
	for _, price := range prices {
		dst = append(dst, encodeTestPrice(price)...)
	}
	return append(dst, make([]byte, 8)...)
}

func encodeTestPrice(value int64) []byte {
	negative := value < 0
	if negative {
		value = -value
	}
	first := byte(value & 0x3f)
	value >>= 6
	if negative {
		first |= 0x40
	}
	if value == 0 {
		return []byte{first}
	}
	result := []byte{first | 0x80}
	for value > 0 {
		current := byte(value & 0x7f)
		value >>= 7
		if value > 0 {
			current |= 0x80
		}
		result = append(result, current)
	}
	return result
}
