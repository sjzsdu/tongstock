package protocol

import (
	"encoding/binary"
	"testing"
)

// buildBlockFile 构造一个最小但真实的板块文件缓冲区：384 字节文件头加
// 若干 2813 字节步长的记录。每条记录带自己的 GBK 板块名；代码按 7 字节槽
// 从记录偏移 15 处依次写入——包括跨过记录边界的第 400 只，与真实文件布局
// 一致（第 400 只的最后 2 字节落进下一条记录的保留区）。
func buildBlockFile(t *testing.T, records []blockRecord) []byte {
	t.Helper()

	const fileHeader = 384
	const recordSize = 2813

	buf := make([]byte, fileHeader+recordSize*len(records))
	for i, rec := range records {
		base := fileHeader + i*recordSize
		if len(rec.codes) > 400 {
			t.Fatalf("record %d has %d codes, max is 400", i, len(rec.codes))
		}
		copy(buf[base+2:base+11], rec.gbkName)
		binary.LittleEndian.PutUint16(buf[base+11:base+13], uint16(len(rec.codes)))
		binary.LittleEndian.PutUint16(buf[base+13:base+15], 2)
		for j, code := range rec.codes {
			off := base + 15 + j*7
			if off+7 > len(buf) {
				t.Fatalf("code %d of record %d overruns the buffer", j, i)
			}
			copy(buf[off:off+7], code+"\x00")
		}
	}
	return buf
}

type blockRecord struct {
	gbkName []byte
	codes   []string
}

func sequentialCodes(n int) []string {
	codes := make([]string, n)
	for i := range codes {
		codes[i] = "600000" + string(rune('A'+i%26))
	}
	return codes
}

func TestParseBlockDataReads400thCodeAcrossRecordBoundary(t *testing.T) {
	// 两条记录：第一条满 400 只（第 400 只的槽跨入第二条的保留区），
	// 第二条 2 只。
	file := buildBlockFile(t, []blockRecord{
		{gbkName: []byte{0xb0, 0xe5, 0xbf, 0xd9, 0xd2, 0xbb, 0x00, 0x00, 0x00}, codes: sequentialCodes(400)},         // 板块一
		{gbkName: []byte{0xb0, 0xe5, 0xbf, 0xd9, 0xb6, 0xfe, 0x00, 0x00, 0x00}, codes: []string{"000001", "000002"}}, // 板块二
	})

	items, err := ParseBlockData(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 402 {
		t.Fatalf("total items = %d, want 402", len(items))
	}

	// 用与解析器相同的 GBK 转换派生板块名 key，测试不依赖手拼编码的正确性。
	name1 := string(GBKToUTF8([]byte{0xb0, 0xe5, 0xbf, 0xd9, 0xd2, 0xbb, 0x00, 0x00, 0x00}))
	name2 := string(GBKToUTF8([]byte{0xb0, 0xe5, 0xbf, 0xd9, 0xb6, 0xfe, 0x00, 0x00, 0x00}))

	byBlock := map[string][]string{}
	for _, item := range items {
		byBlock[item.BlockName] = append(byBlock[item.BlockName], item.StockCode)
	}
	first := byBlock[name1]
	if len(first) != 400 {
		t.Fatalf("first block (%q) has %d codes, want 400", name1, len(first))
	}
	want := sequentialCodes(400)
	if first[399] != want[399] {
		t.Fatalf("400th code = %q, want %q", first[399], want[399])
	}
	second := byBlock[name2]
	if len(second) != 2 || second[0] != "000001" || second[1] != "000002" {
		t.Fatalf("second block (%q) codes = %v, want [000001 000002]", name2, second)
	}
}

func TestParseBlockDataCapsCountAt400(t *testing.T) {
	// count 字段若被脏数据污染超过 400，按 400 截断而不是越界读取。
	file := buildBlockFile(t, []blockRecord{
		{gbkName: []byte{0xb0, 0xe5, 0xbf, 0xd9, 0x00, 0x00, 0x00, 0x00, 0x00}, codes: sequentialCodes(10)},
	})
	base := 384
	binary.LittleEndian.PutUint16(file[base+11:base+13], 1000)

	items, err := ParseBlockData(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 {
		t.Fatalf("items = %d, want 10 (empty slots stop the loop)", len(items))
	}
}

func TestParseBlockDataRejectsShortPayload(t *testing.T) {
	if _, err := ParseBlockData(make([]byte, 100)); err == nil {
		t.Fatal("short payload accepted")
	}
}
