package newsfeed

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// 新闻与股票的关联来源。区分「数据源原生携带」与「本地推测」是刻意的：
// 系统不允许把推测结果伪装成确定事实，调用方必须能分辨二者。
const (
	MatchNative  = "native"   // 数据源原生携带（如财联社 stock_list）
	MatchCodeHit = "code_hit" // 正文命中 6 位股票代码
	MatchNameHit = "name_hit" // 正文命中股票简称
)

// StockRef 描述一条新闻与一只股票的关联。
type StockRef struct {
	Code       string  `json:"code"`
	MatchType  string  `json:"match_type"`
	Confidence float64 `json:"confidence"`
}

// StockEntity 是实体识别依赖的最小股票信息。
type StockEntity struct {
	Code string
	Name string
}

var (
	htmlTagPattern  = regexp.MustCompile(`<[^>]*>`)
	stockCodeRun    = regexp.MustCompile(`\d{6}`)
	stPrefixPattern = regexp.MustCompile(`^\*?ST`)
)

// minNameLen 是参与匹配的最短简称字数。低于此值的别名不注册：
// 一两个字的片段在中文里歧义过大，只会制造噪声。
const minNameLen = 2

// StripHTML 移除 HTML 标签并压缩空白。
// 必须在使用 Matcher 之前调用：东财返回的高亮标签会把「贵州<em>茅台</em>」
// 切断，导致简称匹配失效。
func StripHTML(s string) string {
	s = htmlTagPattern.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

// normalizeName 规范化股票简称：去掉 ST/*ST 前缀与空白。
// 「*ST皇庭」在新闻里通常被写成「皇庭」或「*ST皇庭」，两种都要能命中。
func normalizeName(name string) string {
	n := strings.TrimSpace(name)
	n = stPrefixPattern.ReplaceAllString(n, "")
	n = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, n)
	return n
}

// EntityMatcher 在文本中识别股票实体。
type EntityMatcher struct {
	byName map[string][]string // 规范化简称 -> 代码（可能重名）
	names  []string            // 按长度降序，保证最长匹配优先
	known  map[string]bool     // 合法代码集合
}

// NewEntityMatcher 用股票名录构建匹配器。entities 为空时匹配器始终返回空结果，
// 绝不猜测。
func NewEntityMatcher(entities []StockEntity) *EntityMatcher {
	m := &EntityMatcher{
		byName: make(map[string][]string),
		known:  make(map[string]bool, len(entities)),
	}
	seen := make(map[string]bool)
	addAlias := func(alias, code string) {
		if len([]rune(alias)) < minNameLen {
			return
		}
		if !seen[alias] {
			m.names = append(m.names, alias)
			seen[alias] = true
		}
		m.byName[alias] = append(m.byName[alias], code)
	}
	for _, e := range entities {
		code := strings.TrimSpace(e.Code)
		if len(code) != 6 {
			continue
		}
		m.known[code] = true
		// 全称（含 ST 前缀）与剥离前缀后的简称都要注册：
		// 新闻里两种写法都常见，且全称歧义更小。
		full := strings.TrimSpace(e.Name)
		addAlias(full, code)
		if norm := normalizeName(e.Name); norm != full {
			addAlias(norm, code)
		}
	}
	sort.Slice(m.names, func(i, j int) bool {
		return len([]rune(m.names[i])) > len([]rune(m.names[j]))
	})
	return m
}

// Empty 报告匹配器是否没有任何股票名录。没有名录时不允许推测关联。
func (m *EntityMatcher) Empty() bool {
	return m == nil || len(m.names) == 0
}

// Match 在 text 中识别股票。返回按置信度降序的关联列表，无命中返回 nil。
// native 关联由数据源提供，不经过本方法。
func (m *EntityMatcher) Match(text string) []StockRef {
	if m.Empty() {
		return nil
	}
	text = StripHTML(text)
	if text == "" {
		return nil
	}

	found := make(map[string]StockRef)

	// 1) 代码命中：高精度。
	for _, code := range stockCodeRun.FindAllString(text, -1) {
		if m.known[code] {
			found[code] = StockRef{Code: code, MatchType: MatchCodeHit, Confidence: 1.0}
		}
	}

	// 2) 简称命中：最长优先，命中后用占位符遮蔽，避免「招商银行」被
	//    「招商轮船」之类的短名重复消费。
	runes := []rune(text)
	for _, name := range m.names {
		nr := []rune(name)
		for i := 0; i+len(nr) <= len(runes); i++ {
			if !equalRunes(runes[i:i+len(nr)], nr) {
				continue
			}
			for _, code := range m.byName[name] {
				if prev, ok := found[code]; ok && prev.MatchType == MatchCodeHit {
					continue // 代码命中优先，不降级
				}
				found[code] = StockRef{Code: code, MatchType: MatchNameHit, Confidence: nameConfidence(name)}
			}
			mask(runes[i : i+len(nr)])
		}
	}

	refs := make([]StockRef, 0, len(found))
	for _, r := range found {
		refs = append(refs, r)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Confidence != refs[j].Confidence {
			return refs[i].Confidence > refs[j].Confidence
		}
		return refs[i].Code < refs[j].Code
	})
	return refs
}

func nameConfidence(name string) float64 {
	switch l := len([]rune(name)); {
	case l >= 4:
		return 0.9
	case l == 3:
		return 0.75
	default:
		return 0.5
	}
}

func equalRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mask(runes []rune) {
	for i := range runes {
		runes[i] = '\x00'
	}
}
