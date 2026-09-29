package tdx

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// ErrCompanyBlockNotFound reports that the F10 catalogue holds no block with
// the requested name.
var ErrCompanyBlockNotFound = errors.New("未找到 F10 信息块")

// blockPeekSize is how many bytes past Length are read when a block is
// fetched. They are what makes both ends of the window checkable: the bytes
// right after Length must open the next block's header line (or be nothing at
// all, when this is the last block). It has to cover a header line — around
// 40 GBK bytes — plus any separator the document puts between blocks.
const blockPeekSize = 512

// f10Source is the slice of *Service the block reader needs. Keeping it a small
// interface lets the reader be exercised against a catalogue and a document
// that deliberately disagree, which is exactly the situation that produced
// misaligned blocks in production.
type f10Source interface {
	FetchCompanyCategory(code string) ([]*protocol.CompanyCategoryItem, error)
	RefreshCompanyCategory(code string) ([]*protocol.CompanyCategoryItem, error)
	fetchCompanyRaw(code, filename string, start, length uint32) ([]byte, error)
	fetchCompanyDocument(code, filename string, refresh bool) ([]byte, error)
}

// companyBlockReader turns an F10 block name into the text of that block.
//
// The naive route — trust the catalogue's Start/Length and slice the document —
// silently returns the wrong text whenever the catalogue is older than the
// document: the file is regenerated daily, the catalogue is cached for much
// longer, and every offset after the first changed byte lands somewhere else.
// So every window is verified against the document before it is returned, and
// when verification fails the boundaries are recomputed from the block header
// lines, which the document itself carries.
type companyBlockReader struct {
	source f10Source
}

func (r *companyBlockReader) fetch(code, block string) (string, error) {
	cats, err := r.source.FetchCompanyCategory(code)
	if err != nil {
		return "", err
	}
	idx, err := blockIndex(cats, block)
	if err != nil {
		return "", err
	}

	// Fast path: the catalogue's own offsets, verified against the document.
	// Verification is free — the bytes past Length are read anyway to see
	// where the next block begins.
	raw, ok, err := r.readBlock(code, cats, idx)
	if err != nil {
		return "", err
	}
	if ok {
		return decodeBlock(raw), nil
	}

	// The catalogue no longer describes this document: refetch it (it is a
	// single small reply) and try the verified slice once more.
	fresh, err := r.source.RefreshCompanyCategory(code)
	if err == nil {
		if freshIdx, ferr := blockIndex(fresh, block); ferr == nil {
			raw, ok, err = r.readBlock(code, fresh, freshIdx)
			if err != nil {
				return "", err
			}
			if ok {
				return decodeBlock(raw), nil
			}
			cats, idx = fresh, freshIdx
		}
	}

	// Still wrong: drop the offsets entirely and cut the block out of the
	// document between its own header line and the next one.
	raw, ok, err = r.readBlockByMarkers(code, cats, idx)
	if err != nil {
		return "", err
	}
	if ok {
		return decodeBlock(raw), nil
	}

	// The document carries no recognisable header lines, so there is nothing
	// left to verify against. Fall back to the catalogue window rather than
	// failing a stock that used to work, but make the loss of the guarantee
	// visible instead of returning silently misaligned text.
	cat := cats[idx]
	log.Printf("警告: %s 的 F10 文档缺少块头行，%q 只能按目录偏移返回，边界未经校验", code, block)
	raw, err = r.source.fetchCompanyRaw(code, cat.Filename, cat.Start, cat.Length)
	if err != nil {
		return "", err
	}
	return decodeBlock(raw), nil
}

// readBlock fetches [Start, Start+Length+blockPeekSize) and checks that the
// window really is the block: it must open with the block's own header line
// and end where the next block's header line starts (or at end of file for the
// last block). A window that fails either end is returned as not ok, with no
// error — the caller is expected to refresh the catalogue and retry.
func (r *companyBlockReader) readBlock(code string, cats []*protocol.CompanyCategoryItem, idx int) ([]byte, bool, error) {
	cat := cats[idx]
	var next *protocol.CompanyCategoryItem
	if idx+1 < len(cats) {
		next = cats[idx+1]
	}

	raw, err := r.source.fetchCompanyRaw(code, cat.Filename, cat.Start, cat.Length+blockPeekSize)
	if err != nil {
		return nil, false, err
	}
	ok, err := validBlockWindow(raw, code, cat, next)
	if err != nil || !ok {
		return nil, false, err
	}
	return raw[:cat.Length], true, nil
}

// readBlockByMarkers ignores the catalogue's byte offsets and splits the whole
// document on the header lines the document itself carries, so it stays correct
// no matter how stale the catalogue is. The document is read live here: this is
// the correctness fallback, and serving a cached copy would only move the
// staleness one level down.
func (r *companyBlockReader) readBlockByMarkers(code string, cats []*protocol.CompanyCategoryItem, idx int) ([]byte, bool, error) {
	cat := cats[idx]
	doc, err := r.source.fetchCompanyDocument(code, cat.Filename, true)
	if err != nil {
		return nil, false, err
	}
	starts, ok := blockBoundaries(doc, code, cats)
	if !ok {
		return nil, false, nil
	}
	if total := catalogueLength(cats); total != len(doc) {
		// The report-style invariant: the catalogue must tile the document.
		// It is what tells us the catalogue and the document came from the
		// same update, so worth logging when they do not.
		log.Printf("警告: %s 的 F10 目录总长 %d 与文档长度 %d 不一致", code, total, len(doc))
	}
	end := len(doc)
	if idx+1 < len(starts) {
		end = starts[idx+1]
	}
	if starts[idx] > end {
		return nil, false, nil
	}
	return doc[starts[idx]:end], true, nil
}

// validBlockWindow checks both ends of a window against the document.
func validBlockWindow(raw []byte, code string, cat, next *protocol.CompanyCategoryItem) (bool, error) {
	if uint32(len(raw)) < cat.Length {
		// The document ends inside the window: its offsets are from an older,
		// longer version of the file.
		return false, nil
	}
	marker, err := protocol.CompanyBlockMarker(code, cat.Name)
	if err != nil {
		return false, err
	}
	body := bytes.TrimLeft(raw[:cat.Length], " \t\r\n")
	if !bytes.HasPrefix(body, marker) {
		return false, nil
	}

	rest := bytes.TrimLeft(raw[cat.Length:], " \t\r\n")
	if next == nil {
		// Last block: it has to end exactly at end of file, otherwise Length
		// is from an older version of the file (too short truncates the block,
		// too long swallows whatever follows it).
		return uint32(len(raw)) == cat.Length, nil
	}
	nextMarker, err := protocol.CompanyBlockMarker(code, next.Name)
	if err != nil {
		return false, err
	}
	return bytes.HasPrefix(rest, nextMarker), nil
}

// blockBoundaries returns the offset of every block's header line inside doc,
// or ok=false when a header cannot be located (a document format we do not
// recognise, or a catalogue whose blocks no longer exist in the document).
func blockBoundaries(doc []byte, code string, cats []*protocol.CompanyCategoryItem) ([]int, bool) {
	starts := make([]int, 0, len(cats))
	from := 0
	for _, cat := range cats {
		marker, err := protocol.CompanyBlockMarker(code, cat.Name)
		if err != nil {
			return nil, false
		}
		at := bytes.Index(doc[from:], marker)
		if at < 0 {
			return nil, false
		}
		start := from + at
		starts = append(starts, start)
		from = start + len(marker)
	}
	return starts, true
}

// blockIndex resolves a block name to its position in the catalogue.
func blockIndex(cats []*protocol.CompanyCategoryItem, name string) (int, error) {
	for i, cat := range cats {
		if cat.Name == name {
			return i, nil
		}
	}
	names := make([]string, 0, len(cats))
	for _, cat := range cats {
		names = append(names, cat.Name)
	}
	return -1, fmt.Errorf("%w: %s（可用的块: %s）", ErrCompanyBlockNotFound, name, strings.Join(names, "、"))
}

func catalogueLength(cats []*protocol.CompanyCategoryItem) int {
	total := 0
	for _, cat := range cats {
		total += int(cat.Length)
	}
	return total
}

// decodeBlock turns one block of raw GBK bytes into UTF-8 text. The window is
// known to start on a character boundary (it opens with the header line) and
// covers a whole block, so decoding it in one piece cannot split a character.
func decodeBlock(raw []byte) string {
	return string(protocol.GBKToUTF8(raw))
}

// FetchCompanyBlock returns the text of a single F10 block such as 公司概况 or
// 财务分析. Block boundaries are verified against the current document, so a
// stale catalogue can never produce text that silently belongs to a different
// block.
func (s *Service) FetchCompanyBlock(code, block string) (string, error) {
	return (&companyBlockReader{source: s}).fetch(code, block)
}

func (s *Service) fetchCompanyRaw(code, filename string, start, length uint32) ([]byte, error) {
	var raw []byte
	err := s.withClient(func(c *Client) error {
		var e error
		raw, e = c.GetCompanyInfoContentRawRange(code, filename, start, length)
		return e
	})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// fetchCompanyDocument returns the whole raw F10 document of a stock. With
// refresh it bypasses the cache and reads the file live — the path taken when a
// misaligned block has to be repaired, where serving a cached document would
// only reintroduce the staleness being fixed.
func (s *Service) fetchCompanyDocument(code, filename string, refresh bool) ([]byte, error) {
	if !refresh && s.company != nil {
		if doc, err := s.company.GetContent(code, filename); err == nil && len(doc) > 0 {
			return doc, nil
		}
	}
	var doc []byte
	err := s.withClient(func(c *Client) error {
		var e error
		doc, e = c.GetCompanyInfoDocument(code, filename, 0)
		return e
	})
	if err != nil {
		return nil, err
	}
	if s.company != nil {
		_ = s.company.SaveContent(code, filename, doc)
	}
	return doc, nil
}
