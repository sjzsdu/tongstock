package serverapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/adapter/automationrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/discoveryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/paradigmrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/positiondecisionrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/stockpoolrepo"
	"github.com/sjzsdu/tongstock/internal/agents"
	"github.com/sjzsdu/tongstock/internal/app/discoveryapp"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/internal/automation"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/internal/serviceproc"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/newsfeed"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/server"
	"github.com/sjzsdu/tongstock/pkg/stockinfo"
	"github.com/sjzsdu/tongstock/pkg/stockpool"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
	"github.com/sjzsdu/tongstock/pkg/web"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 60 * time.Second
	// WriteTimeout stays disabled because Agent SSE and long-running sync
	// responses are controlled through request contexts instead.
	writeTimeout = 0
)

type Options struct {
	ExecutorFactory func() (tdx.Executor, error)
	Listen          func(network, address string) (net.Listener, error)
	SkipProcessFile bool
}

// App is the composition root and sole owner of long-lived resources.
// Stores borrow Storage and never close it. Shutdown is safe to call multiple
// times and closes resources in the reverse order of construction.
type App struct {
	cfg        *config.Config
	storage    *storage.Storage
	executor   tdx.Executor
	data       *tdx.Service
	stockData  *stockdata.Service
	api        *server.Server
	newsfeed   *newsfeed.SQLiteStore
	httpServer *http.Server
	listen     func(network, address string) (net.Listener, error)
	addr       string

	runCtx context.Context
	cancel context.CancelFunc

	moduleMu sync.RWMutex
	modules  map[string]server.ModuleHealth

	runMu       sync.Mutex
	running     bool
	listener    net.Listener
	serverDone  chan error
	processPID  int
	skipProcess bool
	shutdown    sync.Once
	shutdownErr error
}

func NewApp(cfg *config.Config, opts Options) (_ *App, err error) {
	if cfg == nil {
		return nil, errors.New("nil config")
	}
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{
		cfg:         cfg,
		runCtx:      ctx,
		cancel:      cancel,
		listen:      opts.Listen,
		skipProcess: opts.SkipProcessFile,
		modules:     make(map[string]server.ModuleHealth),
	}
	if app.listen == nil {
		app.listen = net.Listen
	}
	cleanup := true
	defer func() {
		if cleanup {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = app.Shutdown(shutdownCtx)
		}
	}()

	if err := param.AutoInit(); err != nil {
		app.setModule("parameters", "degraded", err.Error())
	} else {
		app.setModule("parameters", "ready", "")
	}

	app.storage, err = storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, fmt.Errorf("初始化存储失败: %w", err)
	}
	app.setModule("database", "ready", "")

	if opts.ExecutorFactory != nil {
		app.executor, err = opts.ExecutorFactory()
	} else {
		hosts := cfg.TDX.Hosts
		if len(hosts) == 0 {
			hosts = tdx.DefaultHosts
		}
		app.executor, err = tdx.NewExecutor(func() (*tdx.Client, error) {
			return tdx.DialHosts(hosts, tdx.WithRedial(true))
		}, 3)
	}
	if err != nil {
		return nil, fmt.Errorf("创建 TDX 执行器失败: %w", err)
	}
	app.setModule("tdx", "ready", "")

	app.data, err = tdx.NewServiceWithExecutor(app.executor, app.storage)
	if err != nil {
		return nil, fmt.Errorf("创建股票数据服务失败: %w", err)
	}
	repository, err := stockdata.NewSQLiteRepository(app.storage)
	if err != nil {
		return nil, fmt.Errorf("创建股票数据仓库失败: %w", err)
	}
	provider, err := stockdata.NewTDXProvider(app.data)
	if err != nil {
		return nil, fmt.Errorf("创建 TDX 数据 Provider 失败: %w", err)
	}
	calendar, err := stockdata.NewSQLiteTradingCalendar(app.storage)
	if err != nil {
		return nil, fmt.Errorf("创建交易日历失败: %w", err)
	}
	app.stockData, err = stockdata.NewServiceWithContext(
		app.runCtx,
		repository,
		provider,
		stockdata.NewMarketFreshnessPolicy(calendar, time.Local),
		stockdata.SystemClock{},
	)
	if err != nil {
		return nil, fmt.Errorf("创建统一股票数据服务失败: %w", err)
	}

	historyStore, err := history.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化历史记录失败: %w", err)
	}
	watchlistStore, err := watchlist.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化自选股失败: %w", err)
	}
	tradingStore, err := trading.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化交易记录失败: %w", err)
	}
	stockpoolStore, err := stockpool.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化股票池失败: %w", err)
	}
	stockinfoStore, err := stockinfo.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化股票信息失败: %w", err)
	}

	app.newsfeed, err = newsfeed.NewStoreWithStorage(app.storage)
	var newsHandler *server.NewsfeedHandler
	if err != nil {
		log.Printf("newsfeed initialization degraded: %v", err)
		app.setModule("newsfeed", "degraded", "newsfeed initialization failed")
	} else {
		newsHandler = server.NewNewsfeedHandler(app.newsfeed)
		app.setModule("newsfeed", "ready", "")
	}

	forwardLedger, err := ledger.NewSQLiteSignalLedger(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化持久化前向账本失败: %w", err)
	}
	app.api = server.NewServer(server.Dependencies{
		StockData: app.data, UnifiedData: app.stockData, History: historyStore, Watchlist: watchlistStore,
		Trading: tradingStore, StockPool: stockpoolStore, StockInfo: stockinfoStore,
		Newsfeed: newsHandler, Diagnostics: server.DiagnosticsFunc(app.Diagnostics),
		Storage: app.storage, Ledger: forwardLedger,
	})
	methodRepo, err := methodregistryrepo.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化投资方法仓库失败: %w", err)
	}
	methodRegistry, err := methodregistry.New(methodRepo)
	if err != nil {
		return nil, fmt.Errorf("初始化投资方法库失败: %w", err)
	}
	app.api.SetMethodRegistry(methodRegistry)
	app.setModule("method_registry", "ready", "")
	selectionRuns, err := selectionrepo.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化每日选股仓库失败: %w", err)
	}
	app.api.SetSelectionRuns(selectionRuns)
	app.setModule("daily_selection", "ready", "")
	positionRuns, err := positiondecisionrepo.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化持仓判断仓库失败: %w", err)
	}
	marketSnapshots, err := marketsnapshotrepo.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化市场快照仓库失败: %w", err)
	}
	positionEngine, err := positiondecision.NewEngine(marketSnapshots, tradingStore, methodRepo, positionRuns)
	if err != nil {
		return nil, fmt.Errorf("初始化持仓判断引擎失败: %w", err)
	}
	app.api.SetPositionDecision(positionEngine, positionRuns)
	app.setModule("position_decision", "ready", "")
	selectionEngine, err := selection.NewEngine(marketSnapshots, methodRepo, selectionRuns)
	if err != nil {
		return nil, fmt.Errorf("初始化每日选股引擎失败: %w", err)
	}
	automationRuns, err := automationrepo.New(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化自动任务仓库失败: %w", err)
	}
	automationEngine, err := automation.New(selectionEngine, positionEngine, marketSnapshots, forwardLedger, automationRuns)
	if err != nil {
		return nil, fmt.Errorf("初始化自动任务引擎失败: %w", err)
	}
	app.api.SetAutomation(automationEngine, automationRuns)
	app.api.StartAutomationScheduler(app.runCtx, marketSnapshots)
	app.setModule("daily_automation", "ready", "")

	// 规律发现应用服务：CLI 与 HTTP 共用同一 Runner。
	discoverResolver, err := stockpoolrepo.NewResolver(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化股票池解析器失败: %w", err)
	}
	discoverTraces, err := discoveryrepo.NewTraceRepository(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化发现轨迹仓库失败: %w", err)
	}
	discoverRunner := discoveryapp.NewRunner(app.storage, discoverResolver, app.data)
	app.api.SetDiscoverRunner(discoverRunner, discoverTraces)
	app.setModule("discovery", "ready", "")

	app.configureOptionalModules()
	router := app.buildRouter()

	bind, _, err := server.ValidateBindSecurity(cfg.Server.BindAddress, cfg.Server.AccessToken)
	if err != nil {
		return nil, err
	}
	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	app.addr = net.JoinHostPort(bind, fmt.Sprintf("%d", port))
	app.httpServer = &http.Server{
		Addr:              app.addr,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	cleanup = false
	return app, nil
}

func (a *App) configureOptionalModules() {
	agentPaths := make([]string, 0, len(a.cfg.Agent.AgentPaths))
	for _, path := range a.cfg.Agent.AgentPaths {
		agentPaths = append(agentPaths, expandHome(path))
	}
	a.api.SetAgentLister(func() ([]server.EmbeddedAgent, error) {
		return configuredAgentLister(agentPaths)
	})
	if a.cfg.Agent.Enabled {
		home := expandHome(a.cfg.Agent.Home)
		configPath := expandHome(a.cfg.Agent.Config)
		if err := a.api.InitAgentStateWithOptions(server.AgentRuntimeOptions{
			Backend: a.cfg.Agent.EffectiveBackend(), Home: home, ConfigPath: configPath,
			Provider: a.cfg.Agent.Provider, APIBase: a.cfg.Agent.APIBase, APIKeyEnv: a.cfg.Agent.APIKeyEnv,
			Model: a.cfg.Agent.Model, Agent: a.cfg.Agent.Agent, Session: a.cfg.Agent.Session,
			StockAgent: a.cfg.Agent.StockAgent,
		}); err != nil {
			log.Printf("agent initialization degraded: %v", err)
			a.setModule("agent", "degraded", err.Error())
		} else {
			a.setModule("agent", "ready", "")
		}
	} else {
		a.setModule("agent", "disabled", "")
	}

	chatStore, err := server.NewChatStoreWithStorage("", a.storage)
	if err != nil {
		log.Printf("chat initialization degraded: %v", err)
		a.setModule("chat", "degraded", "chat initialization failed")
	} else {
		a.api.SetChatStore(chatStore)
		a.setModule("chat", "ready", "")
	}

	paradigmRepo, err := paradigmrepo.NewSQLiteRepository(a.storage)
	if err != nil {
		log.Printf("paradigm initialization degraded: %v", err)
		a.setModule("paradigm", "degraded", "paradigm repository init failed")
		return
	}
	paradigmStore, err := paradigms.NewStoreWithRepository(paradigmRepo)
	if err != nil {
		log.Printf("paradigm initialization degraded: %v", err)
		a.setModule("paradigm", "degraded", "paradigm initialization failed")
	} else {
		a.api.SetParadigmStore(paradigmStore)
		a.api.StartParadigmAlertScanner(a.runCtx, 5*time.Minute)
		a.setModule("paradigm", "ready", "")
	}
}

func embeddedAgentLister() ([]server.EmbeddedAgent, error) {
	return configuredAgentLister(nil)
}

func configuredAgentLister(paths []string) ([]server.EmbeddedAgent, error) {
	allAgents, err := agents.ListWithPaths(paths)
	if err != nil {
		return nil, err
	}
	result := make([]server.EmbeddedAgent, len(allAgents))
	for i, agent := range allAgents {
		result[i] = server.EmbeddedAgent{
			ID: agent.ID, Name: agent.Name, Description: agent.Description,
			Prompt: agent.Prompt, Soul: agent.Soul, Aliases: agent.Aliases, Skills: agent.Skills,
			Tools: agent.Tools, NoHistory: agent.NoHistory,
		}
	}
	return result, nil
}

func (a *App) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(server.RequestID())
	router.Use(server.AccessLog())
	router.Use(server.Recovery())
	router.Use(server.SecurityHeaders())
	router.Use(server.MaxRequestBody())
	router.Use(server.ErrorEnvelopeMiddleware())
	a.api.SetupRoutes(router, server.AccessTokenAuth(a.cfg.Server.BindAddress, a.cfg.Server.AccessToken))
	setupStaticRoutes(router)
	return router
}

func setupStaticRoutes(router *gin.Engine) {
	serveIndex := func(c *gin.Context) {
		file, err := web.DistFS().Open("index.html")
		if err != nil {
			server.WriteError(c, http.StatusInternalServerError, "static_unavailable", "页面资源不可用")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			server.WriteError(c, http.StatusInternalServerError, "static_unavailable", "页面资源不可用")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}
	router.GET("/", serveIndex)
	router.NoRoute(func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "api" || strings.HasPrefix(path, "api/") {
			server.WriteError(c, http.StatusNotFound, "not_found", "请求的 API 不存在")
			return
		}
		if web.Exists(path) {
			c.FileFromFS(path, web.DistFS())
			return
		}
		serveIndex(c)
	})
}

func (a *App) Run(ctx context.Context) error {
	a.runMu.Lock()
	if a.running {
		a.runMu.Unlock()
		return errors.New("app is already running")
	}
	listener, err := a.listen("tcp", a.addr)
	if err != nil {
		a.runMu.Unlock()
		return fmt.Errorf("启动服务器失败: %w", err)
	}
	a.listener = listener
	a.running = true
	a.serverDone = make(chan error, 1)
	a.processPID = os.Getpid()
	a.runMu.Unlock()

	if !a.skipProcess {
		record := serviceproc.CurrentRecord()
		if err := serviceproc.Write(record); err != nil {
			_ = listener.Close()
			return fmt.Errorf("记录服务进程失败: %w", err)
		}
		defer serviceproc.RemoveIfPID(record.PID)
	}

	go func() {
		err := a.httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		a.serverDone <- err
	}()

	select {
	case err := <-a.serverDone:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := a.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-a.serverDone
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	a.shutdown.Do(func() {
		a.cancel()
		if a.httpServer != nil {
			if err := a.httpServer.Shutdown(ctx); err != nil {
				_ = a.httpServer.Close()
				a.shutdownErr = errors.Join(a.shutdownErr, fmt.Errorf("关闭 HTTP 服务: %w", err))
			}
		}
		if a.api != nil {
			a.api.WaitForBackgroundTasks()
			a.shutdownErr = errors.Join(a.shutdownErr, a.api.Close())
		}
		if a.data != nil {
			a.shutdownErr = errors.Join(a.shutdownErr, a.data.Close())
		}
		if a.executor != nil {
			a.shutdownErr = errors.Join(a.shutdownErr, a.executor.Close())
		}
		if a.storage != nil {
			a.shutdownErr = errors.Join(a.shutdownErr, a.storage.Close())
		}
	})
	return a.shutdownErr
}

func (a *App) Diagnostics(ctx context.Context) server.Diagnostics {
	result := server.Diagnostics{
		Status: "ready", Service: "tongstock", CheckedAt: time.Now(),
		Modules: make(map[string]server.ModuleHealth),
	}
	a.moduleMu.RLock()
	for key, value := range a.modules {
		result.Modules[key] = value
		if value.Status == "degraded" && result.Status == "ready" {
			result.Status = "degraded"
		}
	}
	a.moduleMu.RUnlock()

	if a.storage == nil || a.storage.Ping(ctx) != nil {
		result.Modules["database"] = server.ModuleHealth{Status: "unavailable", Message: "database ping failed"}
		result.Status = "unavailable"
	} else if version, err := a.storage.SchemaVersion(ctx); err == nil {
		result.SchemaVersion = version
	}
	if a.executor == nil || !a.executor.Status().Open {
		result.Modules["tdx"] = server.ModuleHealth{Status: "unavailable", Message: "TDX executor is closed"}
		result.Status = "unavailable"
	}
	return result
}

func (a *App) setModule(name, status, message string) {
	a.moduleMu.Lock()
	a.modules[name] = server.ModuleHealth{Status: status, Message: message}
	a.moduleMu.Unlock()
}

func expandHome(value string) string {
	if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	return value
}

// Run loads configuration, constructs App, and handles OS cancellation.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("初始化配置失败: %w", err)
	}
	app, err := NewApp(cfg, Options{})
	if err != nil {
		return err
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := app.Shutdown(ctx); err != nil {
			log.Printf("关闭应用失败: %v", err)
		}
	}()
	log.Printf("TongStock server starting on %s", app.addr)
	return app.Run(signalCtx)
}
