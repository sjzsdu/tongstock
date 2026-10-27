package serverapp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/server"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/web"
)

type fakeExecutor struct {
	open atomic.Bool
}

func TestUnknownAPIRouteReturnsStableErrorInsteadOfSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var envelope server.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "not_found" || envelope.Error.RequestID == "" {
		t.Fatalf("envelope = %+v", envelope)
	}
}

// 回归：缺失的静态资源（旧 hash chunk）必须 404 而不是回退 index.html。
// 返回 HTML 会让浏览器把 HTML 当 JS 模块解析，旧标签页在重新部署后
// 切换菜单时报「Failed to fetch dynamically imported module」。
//
// 自托管同域布局下应用在 /app/ 下，所以这一段既覆盖 /app/assets/**，
// 也覆盖根路径下不属于官网的带扩展名请求。
func TestMissingStaticAssetReturns404InsteadOfSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	for _, path := range []string{
		"/app/assets/OvernightArbitrage-DaJ8vyzY.js",
		"/app/assets/index-OLDHASH.css",
		"/app/fonts/foo.woff2",
		"/assets/index-OLDHASH.js",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET %s: status = %d, want 404", path, response.Code)
		}
		if ct := response.Header().Get("Content-Type"); strings.Contains(ct, "text/html") {
			t.Fatalf("GET %s: content-type = %s, must not be html", path, ct)
		}
	}
}

// 回归：/app/assets/<真实存在的文件> 必须 200。
// FS 的根是 web/dist，而 URL 带着 /app/ 前缀；少一层路径变换就会出现
// 「文件明明嵌进二进制了，请求却是 404」——这个坑实测踩到过。
func TestAppStaticAssetServed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	// 从内嵌 FS 里挑一个真实的 assets 文件，避免把构建 hash 写进测试。
	dir, err := web.DistFS().Open("assets")
	if err != nil {
		t.Skipf("web 产物缺少 assets 目录（先构建前端）：%v", err)
	}
	entries, err := dir.Readdir(0)
	if err != nil {
		t.Fatalf("读取 assets 目录失败：%v", err)
	}
	var name string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") {
			name = entry.Name()
			break
		}
	}
	if name == "" {
		t.Skip("assets 目录里没有 .js 文件")
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/assets/"+name, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /app/assets/%s: status = %d, want 200", name, response.Code)
	}
	if ct := response.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("GET /app/assets/%s: content-type = %s, want javascript", name, ct)
	}
}

// 回归：/app/<目录>（无尾斜杠）必须 301 到 /app/<目录>/，不能丢 /app 前缀。
// FileServer 基于改写后的 URL.Path 拼 Location，直接放行会跳到 /assets/（白页）。
func TestAppDirectoryRedirectKeepsPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/assets", nil))
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /app/assets: status = %d, want 301", response.Code)
	}
	if loc := response.Header().Get("Location"); loc != "/app/assets/" {
		t.Fatalf("GET /app/assets: Location = %q, want /app/assets/（不能丢 /app 前缀）", loc)
	}

	// query 要跟着走，旧链接里的筛选参数才不会丢。
	withQuery := httptest.NewRecorder()
	router.ServeHTTP(withQuery, httptest.NewRequest(http.MethodGet, "/app/assets?x=1", nil))
	if loc := withQuery.Header().Get("Location"); loc != "/app/assets/?x=1" {
		t.Fatalf("GET /app/assets?x=1: Location = %q, want /app/assets/?x=1", loc)
	}
}

// 目录型请求带尾斜杠时：没有 index.html 就 404 文本，绝不列目录内容。
func TestAppDirectoryWithoutIndexReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/assets/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /app/assets/: status = %d, want 404", response.Code)
	}
	if ct := response.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("GET /app/assets/: content-type = %s, want text/plain（不能是目录列表）", ct)
	}
}

// 应用内的 SPA 路由（无扩展名）仍回退 app/index.html —— 深链直接进应用，
// 不再依赖静态托管的 404 脚本。根路径下的同名旧路由则 302 到 /app/。
func TestSPARoutesStillFallBackToIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	for _, path := range []string{"/app/watchlist", "/app/stock/600519", "/app/news/event/abc", "/app/screen"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, response.Code)
		}
		if ct := response.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("GET %s: content-type = %s, want text/html", path, ct)
		}
	}
}

// 自托管同域布局：`/` 出官网、`/docs/**` 出文档、`/app/**` 出应用。
// 这条同时守住「一条命令全起」的六个入口里的四个（API 与 /health 走的是真实路由）。
func TestSelfHostedSameOriginLayout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	cases := []struct {
		path   string
		expect string // 页面里必须出现的特征串，用来区分官网 / 文档 / 应用
	}{
		{"/", "TongStock"},
		{"/docs/api/endpoints/", "端点参考"},
		{"/app/", `id="root"`},
		{"/app/stock/000001", `id="root"`}, // 深链直接进应用，不再依赖 404 脚本
	}

	for _, tc := range cases {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", tc.path, response.Code)
		}
		if ct := response.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("GET %s: content-type = %s, want text/html", tc.path, ct)
		}
		if !strings.Contains(response.Body.String(), tc.expect) {
			t.Fatalf("GET %s: body 里缺少特征串 %q", tc.path, tc.expect)
		}
	}
}

// 旧 SPA 深链（应用搬到 /app/ 之前的外链与书签）必须 302 到 /app/<原路径>，并保留 query。
func TestLegacySPADeepLinksRedirectToApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	cases := []struct {
		request string
		want    string
	}{
		{"/stock/600519", "/app/stock/600519"},
		{"/index/000001/day", "/app/index/000001/day"},
		{"/methods/advanced", "/app/methods/advanced"},
		{"/method-does-not-exist", ""}, // 非旧路由：不是 302
		{"/news", "/app/news"},
	}

	for _, tc := range cases {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.request, nil))
		if tc.want == "" {
			if response.Code == http.StatusFound {
				t.Fatalf("GET %s: 不该 302（%s）", tc.request, response.Header().Get("Location"))
			}
			continue
		}
		if response.Code != http.StatusFound {
			t.Fatalf("GET %s: status = %d, want 302", tc.request, response.Code)
		}
		if loc := response.Header().Get("Location"); loc != tc.want {
			t.Fatalf("GET %s: Location = %q, want %q", tc.request, loc, tc.want)
		}
	}

	// query 必须跟着走，否则旧链接里的 tab/筛选条件会丢。
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/stock/600519?tab=finance", nil))
	if loc := response.Header().Get("Location"); loc != "/app/stock/600519?tab=finance" {
		t.Fatalf("Location = %q, want /app/stock/600519?tab=finance", loc)
	}
}

// /app（少尾斜杠）要补一次跳转：应用以 /app/ 为 base，否则相对资源会解析错位。
func TestAppPrefixWithoutSlashRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app", nil))
	if response.Code != http.StatusFound {
		t.Fatalf("GET /app: status = %d, want 302", response.Code)
	}
	if loc := response.Header().Get("Location"); loc != "/app/" {
		t.Fatalf("GET /app: Location = %q, want /app/", loc)
	}
}

// 官网静态资源公开可取；文档目录少的尾斜杠由 FileServer 补跳转。
func TestSiteAssetsAndDocsDirectoryRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	asset := httptest.NewRecorder()
	router.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/favicon.svg", nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("GET /favicon.svg: status = %d, want 200", asset.Code)
	}
	if ct := asset.Header().Get("Content-Type"); !strings.Contains(ct, "svg") {
		t.Fatalf("GET /favicon.svg: content-type = %s, want svg", ct)
	}

	// /docs/api/endpoints 是目录：应 301 到带尾斜杠的地址，而不是 404 或直接吐 HTML。
	redirect := httptest.NewRecorder()
	router.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/docs/api/endpoints", nil))
	if redirect.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /docs/api/endpoints: status = %d, want 301", redirect.Code)
	}
	if loc := redirect.Header().Get("Location"); !strings.HasSuffix(loc, "/") {
		t.Fatalf("GET /docs/api/endpoints: Location = %q, want 以 / 结尾", loc)
	}
}

// 未命中的页面返回官网 404 页面，状态仍是 404（不用 200 假装正常）。
func TestUnknownPageReturnsSiteNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/no-such-page", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if response.Body.Len() == 0 {
		t.Fatal("404 响应体为空，应该返回官网 404 页面")
	}
}

// legacySPARoutes 必须与 App.tsx 同源：这条守住「改了路由却没重新生成」。
func TestLegacySPARoutesGenerated(t *testing.T) {
	if len(legacySPARoutes) == 0 {
		t.Fatal("legacySPARoutes 为空，说明生成器没跑或 App.tsx 解析失败")
	}
	for _, route := range legacySPARoutes {
		if !strings.HasPrefix(route, "/") {
			t.Fatalf("路由 %q 必须以 / 开头", route)
		}
	}
	// 抽样确认动态段路由也在表里（App.tsx 里存在 /stock/:code 与 /news/event/:id）。
	if !matchLegacySPARoute("stock/600519") {
		t.Fatal("matchLegacySPARoute 未命中 /stock/:code")
	}
	if !matchLegacySPARoute("news/event/42") {
		t.Fatal("matchLegacySPARoute 未命中 /news/event/:id")
	}
	if matchLegacySPARoute("docs/api/endpoints") {
		t.Fatal("官网文档路径不该被当作旧 SPA 路由")
	}
}

type blockingListener struct {
	done chan struct{}
	once sync.Once
}

func newBlockingListener() *blockingListener {
	return &blockingListener{done: make(chan struct{})}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	<-l.done
	return nil, net.ErrClosed
}

func (l *blockingListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *blockingListener) Addr() net.Addr { return fakeAddr("memory") }

type fakeAddr string

func (a fakeAddr) Network() string { return "memory" }
func (a fakeAddr) String() string  { return string(a) }

func newFakeExecutor() *fakeExecutor {
	executor := &fakeExecutor{}
	executor.open.Store(true)
	return executor
}

func (e *fakeExecutor) Do(fn func(*tdx.Client) error) error {
	return e.DoContext(context.Background(), fn)
}

func (e *fakeExecutor) DoContext(ctx context.Context, fn func(*tdx.Client) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(nil)
}

func (e *fakeExecutor) Close() error {
	e.open.Store(false)
	return nil
}

func (e *fakeExecutor) Len() int {
	if e.open.Load() {
		return 1
	}
	return 0
}

func (e *fakeExecutor) Status() tdx.ExecutorStatus {
	return tdx.ExecutorStatus{Size: 1, Idle: e.Len(), Open: e.open.Load()}
}

func TestAppRepeatedRunAndIdempotentShutdown(t *testing.T) {
	for iteration := 0; iteration < 3; iteration++ {
		executor := newFakeExecutor()
		cfg := config.DefaultConfig()
		cfg.Database.DSN = filepath.Join(t.TempDir(), "app.db")
		cfg.Server.BindAddress = "127.0.0.1"
		cfg.Agent.Enabled = false
		listener := newBlockingListener()

		app, err := NewApp(cfg, Options{
			ExecutorFactory: func() (tdx.Executor, error) { return executor, nil },
			Listen: func(_, _ string) (net.Listener, error) {
				return listener, nil
			},
			SkipProcessFile: true,
		})
		if err != nil {
			t.Fatalf("iteration %d NewApp() error = %v", iteration, err)
		}
		if app.httpServer.ReadHeaderTimeout != readHeaderTimeout ||
			app.httpServer.ReadTimeout != readTimeout ||
			app.httpServer.IdleTimeout != idleTimeout ||
			app.httpServer.WriteTimeout != writeTimeout {
			t.Fatalf("unexpected HTTP timeout configuration: %+v", app.httpServer)
		}

		runCtx, cancelRun := context.WithCancel(context.Background())
		runDone := make(chan error, 1)
		go func() { runDone <- app.Run(runCtx) }()

		var address string
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case err := <-runDone:
				t.Fatalf("App stopped before listening: %v", err)
			default:
			}
			app.runMu.Lock()
			if app.listener != nil {
				address = app.listener.Addr().String()
			}
			app.runMu.Unlock()
			if address != "" {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if address == "" {
			t.Fatal("App did not start listening")
		}

		diagnostics := app.Diagnostics(context.Background())
		if diagnostics.Status == "unavailable" {
			t.Fatalf("diagnostics unexpectedly unavailable: %+v", diagnostics)
		}

		cancelRun()
		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not stop after cancellation")
		}
		if err := app.Shutdown(context.Background()); err != nil {
			t.Fatalf("second Shutdown() error = %v", err)
		}
		if executor.open.Load() {
			t.Fatal("executor remained open after shutdown")
		}
	}
}

func TestNewAppFailureRollsBackConstructedResources(t *testing.T) {
	executor := newFakeExecutor()
	cfg := config.DefaultConfig()
	cfg.Database.DSN = filepath.Join(t.TempDir(), "rollback.db")
	cfg.Server.BindAddress = "0.0.0.0"
	cfg.Server.AccessToken = ""

	app, err := NewApp(cfg, Options{
		ExecutorFactory: func() (tdx.Executor, error) { return executor, nil },
		SkipProcessFile: true,
	})
	if err == nil || app != nil {
		t.Fatalf("NewApp() = (%v, %v), want bind security failure", app, err)
	}
	if executor.open.Load() {
		t.Fatal("executor remained open after construction rollback")
	}
}

func TestAppListenerFailureCanBeShutDownIdempotently(t *testing.T) {
	executor := newFakeExecutor()
	cfg := config.DefaultConfig()
	cfg.Database.DSN = filepath.Join(t.TempDir(), "listen-failure.db")
	listenErr := errors.New("address unavailable")
	app, err := NewApp(cfg, Options{
		ExecutorFactory: func() (tdx.Executor, error) { return executor, nil },
		Listen: func(_, _ string) (net.Listener, error) {
			return nil, listenErr
		},
		SkipProcessFile: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background()); !errors.Is(err, listenErr) {
		t.Fatalf("Run() error = %v", err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executor.open.Load() {
		t.Fatal("executor remained open after listener failure")
	}
}
