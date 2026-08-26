package methodresearchai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/methodresearch"
	"github.com/sjzsdu/tongstock/internal/picoclaw"
)

type RunFunc func(context.Context, picoclaw.RunOptions) (string, error)

type Provider struct {
	run                   RunFunc
	agent, model, session string
	embedded              []picoclaw.EmbeddedAgent
	client                *http.Client
}

func New(run RunFunc, agent, model, session string, embedded []picoclaw.EmbeddedAgent) (*Provider, error) {
	if run == nil {
		return nil, fmt.Errorf("AI research runner is required")
	}
	p := &Provider{run: run, agent: agent, model: model, session: session, embedded: embedded}
	p.client = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many source redirects")
		}
		return ensurePublicURL(req.Context(), req.URL)
	}}
	return p, nil
}

func (p *Provider) Research(ctx context.Context, input methodresearch.ResearchInput) (*methodresearch.ResearchDraft, error) {
	b, _ := json.Marshal(input)
	prompt := `研究下面的投资方法输入，并只返回一个 JSON 对象，不要 Markdown。必须实际使用 web_search/web_fetch（URL/文本入口也要核验来源）。
JSON 必须严格匹配 ResearchDraft：method_name,summary,sources,citations,claims,variants。
sources 每项必须含 id,url,title,publisher,retrieved_at(RFC3339),tier(primary|secondary),content_hash(SHA-256；基于实际抓取正文)，可选 author,published_at。
citations 每项含 id,source_id,locator,excerpt；excerpt 只能是最多500字的短摘录或忠实转述。
claims 每项含 id,field(entry|exit|position|exception|invalidation|market),key,value,provenance(primary_source|secondary_source|ai_inference|user_input),citation_ids。
同一 key 的来源冲突必须保留为不同 claim。variants 必须显式选择 claim_ids，不得把冲突值合并；candidate 必须符合 methods.Candidate JSON 及其 AST。
可执行 variant 必须包含 entry、exit、position 三类 claim；来源没有仓位规则时要写明缺失，不能让编译器默认值冒充原方法。
candidate 使用这些 JSON 字段：name,description,source_kind,source_text,universe,board_filter,market_state,feature_deps,max_positions,entry_rule,exit_rule,invalid_rule,position_mode,position_pct,position_lots,holding_max_days,holding_min_days,stop_loss_pct,take_profit_pct,trailing_stop_pct。
规则 AST 节点只允许：indicator {type,indicator,params}；constant {type,value}；compare {type,left,right,op(gt|lt|gte|lte|eq)}；and/or {type,children}；not {type,children}；cross {type,left,right,cross(above|below)}；in_window {type,window_days,window_mode,children}；无法表达时 ambiguous {type,ambiguous_source,ambiguous_reasons}。
找不到可靠规则时 sources/claims/variants 保持为空或只返回能证实的部分，不得补造阈值、作者、时间、URL、引用或回测结果。
日内规则若当前 DSL 不能表达，candidate 中使用 ambiguous 节点，不得用日线规则冒充。
输入：` + string(b)
	resp, err := p.run(ctx, picoclaw.RunOptions{Message: prompt, Agent: p.agent, Model: p.model, Session: p.session, Quiet: true, EmbeddedAgents: p.embedded})
	if err != nil {
		return nil, err
	}
	raw := extractJSONObject(resp)
	if raw == "" {
		return nil, fmt.Errorf("AI response contains no JSON object")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var draft methodresearch.ResearchDraft
	if err := dec.Decode(&draft); err != nil {
		return nil, fmt.Errorf("decode AI research draft: %w", err)
	}
	if err := p.verifySources(ctx, &draft); err != nil {
		return nil, err
	}
	return &draft, nil
}

func (p *Provider) verifySources(ctx context.Context, draft *methodresearch.ResearchDraft) error {
	for i := range draft.Sources {
		source := &draft.Sources[i]
		u, err := url.Parse(strings.TrimSpace(source.URL))
		if err != nil {
			return fmt.Errorf("invalid source URL %s: %w", source.ID, err)
		}
		if err := ensurePublicURL(ctx, u); err != nil {
			return fmt.Errorf("unsafe source URL %s: %w", source.ID, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "TongStock-MethodResearch/1.0")
		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("fetch source %s: %w", source.ID, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read source %s: %w", source.ID, readErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("fetch source %s: HTTP %d", source.ID, resp.StatusCode)
		}
		if len(body) == 0 {
			return fmt.Errorf("fetch source %s: empty body", source.ID)
		}
		if len(body) > 4<<20 {
			return fmt.Errorf("fetch source %s: body exceeds 4 MiB evidence limit", source.ID)
		}
		sum := sha256.Sum256(body)
		source.ContentHash = fmt.Sprintf("%x", sum)
		source.RetrievedAt = time.Now().UTC()
	}
	return nil
}

func ensurePublicURL(ctx context.Context, u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("only public HTTP(S) URLs are allowed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("local host is forbidden")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return fmt.Errorf("host has no address")
	}
	for _, address := range addresses {
		ip := address.IP
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("private or local address is forbidden")
		}
	}
	return nil
}

func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

var _ methodresearch.SourceProvider = (*Provider)(nil)
