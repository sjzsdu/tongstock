package tdx

import (
	"errors"
	"strings"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

const (
	testCode     = "600000"
	testFilename = "600000.txt"
)

// testBlocks describes a synthetic F10 document. Every block opens with the
// header line TDX writes ("公司概况☆ ◇600000 ..."), which is what the block
// reader uses to recover boundaries.
var testBlocks = []struct {
	name string
	body string
}{
	{"最新提示", "★本栏包括【1.最新提示】\r\n最新提示正文，包含多字节汉字内容。"},
	{"公司概况", "★本栏包括【1.公司概况】【2.经营范围】\r\n│法人代表│张进│总经理│张进│\r\n├───────┼───────┤"},
	{"财务分析", "★本栏包括【1.主要财务指标】【2.偿债能力指标】\r\n│财务指标│2026-03-31│\r\n├────────┼──────────┤"},
	{"研报评级", "★本栏包括【1.研报评级】\r\n评级汇总，最后一块。"},
}

type testDocument struct {
	raw  []byte
	cats []*protocol.CompanyCategoryItem
}

func buildTestDocument(t *testing.T) testDocument {
	t.Helper()

	var doc testDocument
	for _, b := range testBlocks {
		text := b.name + "☆ ◇" + testCode + " 某某股份 更新日期：2026-09-29◇ 通达信沪深京F10\r\n" +
			b.body + "\r\n"
		raw, err := protocol.UTF8ToGBK(text)
		if err != nil {
			t.Fatalf("gbk encode %s: %v", b.name, err)
		}
		doc.cats = append(doc.cats, &protocol.CompanyCategoryItem{
			Name:     b.name,
			Filename: testFilename,
			Start:    uint32(len(doc.raw)),
			Length:   uint32(len(raw)),
		})
		doc.raw = append(doc.raw, raw...)
	}
	return doc
}

// staleCatalogue mimics a document that shrank after the catalogue was cached:
// the first block got 50 bytes shorter, so every later Start is 50 bytes too
// late while the first block's own Length is 50 bytes too long. That is the
// shape of the production incident — 公司概况 lost its header line and its
// first sections, and 研报评级 was truncated.
func staleCatalogue(cats []*protocol.CompanyCategoryItem) []*protocol.CompanyCategoryItem {
	const drift = 50
	stale := make([]*protocol.CompanyCategoryItem, 0, len(cats))
	for i, cat := range cats {
		next := *cat
		if i > 0 {
			next.Start = cat.Start + drift
		}
		if i == 0 {
			next.Length = cat.Length + drift
		}
		stale = append(stale, &next)
	}
	return stale
}

// fakeF10 serves a fixed document through the f10Source interface.
type fakeF10 struct {
	doc       []byte
	cached    []*protocol.CompanyCategoryItem
	fresh     []*protocol.CompanyCategoryItem
	refreshes int
	docReads  int
}

func (f *fakeF10) FetchCompanyCategory(string) ([]*protocol.CompanyCategoryItem, error) {
	return f.cached, nil
}

func (f *fakeF10) RefreshCompanyCategory(string) ([]*protocol.CompanyCategoryItem, error) {
	f.refreshes++
	return f.fresh, nil
}

func (f *fakeF10) fetchCompanyRaw(_ string, _ string, start, length uint32) ([]byte, error) {
	return sliceWindow(f.doc, start, length), nil
}

func (f *fakeF10) fetchCompanyDocument(_ string, _ string, _ bool) ([]byte, error) {
	f.docReads++
	return f.doc, nil
}

// sliceWindow behaves like the server: it returns the requested window,
// clamped at end of file.
func sliceWindow(doc []byte, start, length uint32) []byte {
	from := int(start)
	if from > len(doc) {
		return nil
	}
	to := from + int(length)
	if to > len(doc) {
		to = len(doc)
	}
	return doc[from:to]
}

func wantBlock(t *testing.T, doc testDocument, index int) string {
	t.Helper()
	text := doc.raw
	start := doc.cats[index].Start
	end := uint32(len(text))
	if index+1 < len(doc.cats) {
		end = doc.cats[index+1].Start
	}
	return string(protocol.GBKToUTF8(text[start:end]))
}

// TestValidBlockWindowAcceptsCurrentCatalogue is the assertion the incident
// report asked for: a window is only served when it opens with the block's own
// header line and ends where the next block's header line starts.
func TestValidBlockWindowAcceptsCurrentCatalogue(t *testing.T) {
	doc := buildTestDocument(t)

	for i, cat := range doc.cats {
		var next *protocol.CompanyCategoryItem
		if i+1 < len(doc.cats) {
			next = doc.cats[i+1]
		}
		window := sliceWindow(doc.raw, cat.Start, cat.Length+blockPeekSize)

		ok, err := validBlockWindow(window, testCode, cat, next)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", cat.Name, err)
		}
		if !ok {
			t.Errorf("%s: current catalogue window rejected", cat.Name)
		}
		if uint32(len(window)) < cat.Length {
			t.Errorf("%s: window shorter than Length: %d < %d", cat.Name, len(window), cat.Length)
		}
	}
}

// TestValidBlockWindowRejectsStaleOffsets covers both ways a stale catalogue
// goes wrong: a Start that has slid forward, and a Length that no longer
// matches the block it measures.
func TestValidBlockWindowRejectsStaleOffsets(t *testing.T) {
	doc := buildTestDocument(t)
	stale := staleCatalogue(doc.cats)

	for i, cat := range stale {
		var next *protocol.CompanyCategoryItem
		if i+1 < len(stale) {
			next = stale[i+1]
		}
		window := sliceWindow(doc.raw, cat.Start, cat.Length+blockPeekSize)

		ok, err := validBlockWindow(window, testCode, cat, next)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", cat.Name, err)
		}
		if ok {
			t.Errorf("%s: stale window (start=%d length=%d) accepted", cat.Name, cat.Start, cat.Length)
		}
	}
}

// TestValidBlockWindowRejectsTruncatedLastBlock covers the report's third
// symptom: the last block asked for more bytes than the document has left.
func TestValidBlockWindowRejectsTruncatedLastBlock(t *testing.T) {
	doc := buildTestDocument(t)
	cat := doc.cats[len(doc.cats)-1]
	grown := *cat
	grown.Length = cat.Length + 1024 // catalogue from a longer document

	window := sliceWindow(doc.raw, grown.Start, grown.Length+blockPeekSize)
	ok, err := validBlockWindow(window, testCode, &grown, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("truncated last block accepted")
	}
}

func TestBlockBoundariesFollowHeaderLines(t *testing.T) {
	doc := buildTestDocument(t)

	starts, ok := blockBoundaries(doc.raw, testCode, doc.cats)
	if !ok {
		t.Fatal("blockBoundaries failed on a well formed document")
	}
	for i, cat := range doc.cats {
		if starts[i] != int(cat.Start) {
			t.Errorf("%s: boundary %d, want %d", cat.Name, starts[i], cat.Start)
		}
	}
}

func TestBlockBoundariesRejectUnknownDocument(t *testing.T) {
	doc := buildTestDocument(t)

	// A header line we cannot find means the document is not laid out the way
	// the catalogue says; guessing would be worse than falling back.
	broken := append([]byte(nil), doc.raw...)
	copy(broken[doc.cats[2].Start:], []byte("## 分隔符 ##"))

	if _, ok := blockBoundaries(broken, testCode, doc.cats); ok {
		t.Fatal("blockBoundaries accepted a document without the 财务分析 header")
	}
}

// TestFetchCompanyBlockRepairsStaleCatalogue is the regression test for the
// reported bug: with a catalogue cached a day earlier, 公司概况 came back
// without its own header line and with the tail of a neighbouring block.
func TestFetchCompanyBlockRepairsStaleCatalogue(t *testing.T) {
	doc := buildTestDocument(t)
	src := &fakeF10{doc: doc.raw, cached: staleCatalogue(doc.cats), fresh: doc.cats}
	reader := &companyBlockReader{source: src}

	got, err := reader.fetch(testCode, "公司概况")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := wantBlock(t, doc, 1)
	if got != want {
		t.Fatalf("misaligned block:\n got = %q\nwant = %q", head(got), head(want))
	}
	if src.refreshes != 1 {
		t.Errorf("catalogue refreshes = %d, want 1", src.refreshes)
	}
	if src.docReads != 0 {
		t.Errorf("full document reads = %d, want 0 (the refreshed catalogue is enough)", src.docReads)
	}
}

// TestFetchCompanyBlockSplitsDocumentWhenCatalogueCannotBeRepaired covers the
// fallback: even a freshly fetched catalogue that still disagrees with the
// document must not produce a misaligned window.
func TestFetchCompanyBlockSplitsDocumentWhenCatalogueCannotBeRepaired(t *testing.T) {
	doc := buildTestDocument(t)
	stale := staleCatalogue(doc.cats)
	src := &fakeF10{doc: doc.raw, cached: stale, fresh: stale}
	reader := &companyBlockReader{source: src}

	got, err := reader.fetch(testCode, "财务分析")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := wantBlock(t, doc, 2)
	if got != want {
		t.Fatalf("misaligned block:\n got = %q\nwant = %q", head(got), head(want))
	}
	if src.docReads != 1 {
		t.Errorf("full document reads = %d, want 1", src.docReads)
	}
}

// TestFetchCompanyBlockUsesFreshCatalogueDirectly pins the fast path: when the
// catalogue is current, no extra work is needed.
func TestFetchCompanyBlockUsesFreshCatalogueDirectly(t *testing.T) {
	doc := buildTestDocument(t)
	src := &fakeF10{doc: doc.raw, cached: doc.cats, fresh: doc.cats}
	reader := &companyBlockReader{source: src}

	got, err := reader.fetch(testCode, "最新提示")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got != wantBlock(t, doc, 0) {
		t.Fatalf("wrong content: %q", head(got))
	}
	if src.refreshes != 0 || src.docReads != 0 {
		t.Errorf("refreshes=%d docReads=%d, want both 0", src.refreshes, src.docReads)
	}
}

func TestFetchCompanyBlockUnknownNameListsBlocks(t *testing.T) {
	doc := buildTestDocument(t)
	reader := &companyBlockReader{source: &fakeF10{doc: doc.raw, cached: doc.cats, fresh: doc.cats}}

	_, err := reader.fetch(testCode, "股东研究")
	if !errors.Is(err, ErrCompanyBlockNotFound) {
		t.Fatalf("err = %v, want ErrCompanyBlockNotFound", err)
	}
	for _, name := range []string{"公司概况", "财务分析", "研报评级"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not list %s: %v", name, err)
		}
	}
}

// TestReadF10RangeKeepsCharactersIntact pins the other half of the incident:
// chunk replies were decoded one by one, and a reply boundary that landed
// inside a GBK character turned "─" into "ぉ" and produced U+FFFD. The range
// reader must hand back the original bytes, character or not.
func TestReadF10RangeKeepsCharactersIntact(t *testing.T) {
	// An odd number of ASCII bytes before the CJK run puts a multi-byte
	// character exactly across the first companyChunkSize boundary.
	text := strings.Repeat("a", int(companyChunkSize)-1) +
		"├────┼────┤│财务指标│2026-03-31│" +
		strings.Repeat("尾部中文", 4000)
	raw, err := protocol.UTF8ToGBK(text)
	if err != nil {
		t.Fatalf("gbk encode: %v", err)
	}

	var reads int
	fetch := func(start, length uint32) ([]byte, error) {
		reads++
		return sliceWindow(raw, start, length), nil
	}

	got, err := readF10Range(fetch, 0, uint32(len(raw)))
	if err != nil {
		t.Fatalf("readF10Range: %v", err)
	}
	if reads < 2 {
		t.Fatalf("reads = %d, want the document to span several chunks", reads)
	}
	if string(got) != string(raw) {
		t.Fatal("assembled bytes differ from the document")
	}

	decoded := string(protocol.GBKToUTF8(got))
	if decoded != text {
		t.Fatalf("decoded text corrupted at chunk boundary:\n got = %q\nwant = %q",
			decoded[len(decoded)-64:], text[len(text)-64:])
	}
	if strings.Contains(decoded, "ぉ") || strings.Contains(decoded, "\uFFFD") {
		t.Fatalf("decoded text contains mojibake: ぉ=%d �=%d",
			strings.Count(decoded, "ぉ"), strings.Count(decoded, "\uFFFD"))
	}
}

// TestReadF10RangeStopsAtEndOfFile: a short reply means the document ended, so
// the reader must not keep asking for bytes that are not there.
func TestReadF10RangeStopsAtEndOfFile(t *testing.T) {
	raw, err := protocol.UTF8ToGBK("公司概况☆ ◇" + testCode + " 某某股份\r\n正文内容。")
	if err != nil {
		t.Fatalf("gbk encode: %v", err)
	}

	var offsets []uint32
	fetch := func(start, length uint32) ([]byte, error) {
		offsets = append(offsets, start)
		return sliceWindow(raw, start, length), nil
	}

	got, err := readF10Range(fetch, 0, uint32(len(raw))+companyChunkSize)
	if err != nil {
		t.Fatalf("readF10Range: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %d bytes, want %d", len(got), len(raw))
	}
	if len(offsets) != 1 {
		t.Errorf("reads = %d, want 1 (the reply was short)", len(offsets))
	}

	// Fully past the end: no bytes, no busy loop.
	got, err = readF10Range(fetch, uint32(len(raw)), 4096)
	if err != nil {
		t.Fatalf("readF10Range past EOF: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("past EOF returned %d bytes, want 0", len(got))
	}
	if len(offsets) != 2 {
		t.Errorf("past EOF reads = %d, want 2 (a single probe then stop)", len(offsets))
	}
}

func TestReadF10DocumentReadsUntilEndOfFile(t *testing.T) {
	raw, err := protocol.UTF8ToGBK(strings.Repeat("财务分析正文", 3000))
	if err != nil {
		t.Fatalf("gbk encode: %v", err)
	}

	var lastStart uint32
	fetch := func(start, length uint32) ([]byte, error) {
		lastStart = start
		return sliceWindow(raw, start, length), nil
	}

	got, err := readF10Document(fetch, 0)
	if err != nil {
		t.Fatalf("readF10Document: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %d bytes, want %d", len(got), len(raw))
	}
	if lastStart%companyChunkSize != 0 {
		t.Errorf("final probe at %d, want a chunk aligned offset", lastStart)
	}
}

func head(s string) string {
	const limit = 96
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
