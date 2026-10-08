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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/adapter/automationrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/dashboardrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/discoveryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/paradigmrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/positiondecisionrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/stockpoolrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/validationrepo"
	"github.com/sjzsdu/tongstock/internal/agentservice"
	"github.com/sjzsdu/tongstock/internal/app/discoveryapp"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/internal/automation"
	"github.com/sjzsdu/tongstock/internal/dashboard"
	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/factorlab"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodautomation"
	"github.com/sjzsdu/tongstock/internal/methodhealth"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methodseed"
	"github.com/sjzsdu/tongstock/internal/onboarding"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/internal/serviceproc"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/newsfeed"
	"github.com/sjzsdu/tongstock/pkg/newsfeed/sources"
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
	cfg         *config.Config
	storage     *storage.Storage
	executor    tdx.Executor
	data        *tdx.Service
	stockData   *stockdata.Service
	api         *server.Server
	newsfeed    *newsfeed.SQLiteStore
	newsService *newsfeed.Service
	httpServer  *http.Server
	listen      func(network, address string) (net.Listener, error)
	addr        string

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
		// 个股资讯服务：注入数据源与股票名录，并启动实体识别。
		// 没有它，/api/news/stock/:code 只能读库，永远拿不到新数据。
		newsSvc, svcErr := newsfeed.NewService(
			app.newsfeed, newsSources(), newsfeed.NewDBDirectory(app.storage.DB()))
		if svcErr != nil {
			log.Printf("newsfeed service degraded: %v", svcErr)
			app.setModule("newsfeed", "degraded", "newsfeed service init failed")
		} else {
			app.newsService = newsSvc
			newsHandler.SetStockNewsService(newsSvc)
			app.setModule("newsfeed", "ready", "")
			app.startNewsBackgroundSync()
		}
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

	// 内置示例方法：编译 → 冻结真实数据全池回测 → 按真实验证结论注册。
	seedEvidence, err := validationrepo.NewEvidenceRepository(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化验证证据仓库失败: %w", err)
	}
	methodSeedService, err := methodseed.NewService(methodseed.Deps{
		Registry:  methodRegistry,
		Snapshots: paradigm.NewDatasetSnapshotStore(app.storage),
		Bars:      validationrepo.New(app.storage),
		Benchmark: validationrepo.NewBenchmark(app.storage),
		Evidence:  seedEvidence,
		Universe:  validationrepo.NewSnapshotUniverse(app.storage),
	})
	if err != nil {
		return nil, fmt.Errorf("初始化内置示例方法服务失败: %w", err)
	}
	app.api.SetMethodSeed(methodSeedService)
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
	// 手动「用选中方法筛选」：与每日自动选股共用同一引擎，仅限定方法集。
	app.api.SetSelectionEngine(selectionEngine, marketSnapshots)
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

	// 首屏「今日状态」读模型 + 首次引导编排：都只读取真实事实或调用既有引擎。
	klineDates, err := dashboardrepo.NewKlineDateReader(app.storage)
	if err != nil {
		return nil, fmt.Errorf("初始化数据水位读取器失败: %w", err)
	}
	dashboardService, err := dashboard.NewService(dashboard.Deps{
		Snapshots: marketSnapshots, Methods: methodRepo, Selections: selectionRuns,
		Positions: positionRuns, Holdings: tradingStore, DataDates: klineDates,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化今日状态服务失败: %w", err)
	}
	app.api.SetDashboard(dashboardService)

	featureEngine := marketsnapshotrepo.NewSQLiteFeatureEngine(app.storage)
	snapshotBuilder := marketsnapshot.NewBuilder(
		marketsnapshotrepo.NewSQLiteUniverseProvider(app.storage),
		marketsnapshotrepo.NewSQLiteWatermarkProvider(app.storage),
		marketsnapshotrepo.NewSQLiteTradingCalendar(app.storage),
	)
	onboardingService, err := onboarding.NewService(onboarding.Deps{
		Builder: snapshotBuilder, Snapshots: marketSnapshots, Features: featureEngine,
		Selection: selectionEngine, Freshness: klineDates,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化首次引导服务失败: %w", err)
	}
	app.api.SetOnboarding(onboardingService)
	app.setModule("dashboard", "ready", "")

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

	// 可信方法自动化闭环（阶段 A）：定时把「规律发现 → 保留窗口全池验证 →
	// 按机器证据晋级/拒绝」跑起来，让方法库持续产出 verified 方法。
	discoveryResearcher, err := discovery.NewResearcher(discoveryrepo.New(app.storage))
	if err != nil {
		return nil, fmt.Errorf("初始化规律发现引擎失败: %w", err)
	}
	methodAutomation, err := methodautomation.New(methodautomation.Deps{
		Registry:   methodRegistry,
		Snapshots:  paradigm.NewDatasetSnapshotStore(app.storage),
		Universe:   validationrepo.NewSnapshotUniverse(app.storage),
		Bars:       validationrepo.New(app.storage),
		Benchmark:  validationrepo.NewBenchmark(app.storage),
		Evidence:   seedEvidence,
		Discoverer: discoveryResearcher,
		Traces:     discoverTraces,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化自动方法研究编排失败: %w", err)
	}
	app.api.SetMethodAutomation(methodAutomation)
	// 研究批次完全手动触发（用户点「启动自动研究」）：自动跑一轮要几十分钟，
	// 启动即跑 + 30 分钟一次的调度会让用户任何时候进页面都撞上「研究进行中」。
	// StartMethodResearchScheduler 保留，未来需要定时研究时显式启用。
	app.setModule("method_automation", "ready", "")

	// 横截面因子研究（factorlab）：与方法自动化共用同一组数据依赖
	// （冻结快照 + 股票池 + K 线），把选股问题重述为「预测未来 N 日收益的
	// 截面排序」。只读研究入口，产出因子 IC 证据与 TopN 名单，不改动选股引擎。
	factorLab, err := factorlab.New(methodautomation.Deps{
		Snapshots: paradigm.NewDatasetSnapshotStore(app.storage),
		Universe:  validationrepo.NewSnapshotUniverse(app.storage),
		Bars:      validationrepo.New(app.storage),
	})
	if err != nil {
		return nil, fmt.Errorf("初始化横截面因子研究失败: %w", err)
	}
	app.api.SetFactorLab(factorLab)
	// 因子通道持久化（最小生产闭环）：研究批次完成后，write_to_selection=true
	// 时显著因子 TopN 观察名单按截面日期幂等落库（factor_pick_run 表），
	// 经 GET /factors/picks/last 可复核。不进方法库 / selection 表。
	pickStore, err := factorlab.NewSQLitePickStore(app.storage.DB())
	if err != nil {
		return nil, fmt.Errorf("初始化因子通道产出仓库失败: %w", err)
	}
	app.api.SetFactorPicks(pickStore)
	// 因子候选通道：选股引擎读取最近落库名单，把新鲜（≤14 天）的因子 TopN
	// 以 watch 级候选并入每日选股产出（staleness 门控 + 贡献分解透出）。
	selectionEngine.SetFactorPicks(pickStore)
	app.setModule("factor_lab", "ready", "")

	// 前向健康闭环（阶段 D）：定时把前向账本的真实 paper-trade 表现回写到
	// 方法库，由 Policy.Health 触发 observing/degraded/retired 转移。
	healthEngine, err := methodhealth.New(methodRegistry, forwardLedger, true, nil)
	if err != nil {
		return nil, fmt.Errorf("初始化前向健康评估失败: %w", err)
	}
	app.api.SetMethodHealth(healthEngine)
	app.api.StartMethodHealthScheduler(app.runCtx, 24*time.Hour)
	app.setModule("method_health", "ready", "")

	app.configureOptionalModules()
	router := app.buildRouter()

	bind, _, err := server.ValidateBindSecurity(cfg.Server.BindAddress, cfg.Server.AccessToken)
	if err != nil {
		return nil, err
	}
	port := cfg.Server.Port
	if port == 0 {
		port = 8106
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
	if a.cfg.Agent.Enabled {
		// The shared agent service owns definition loading (embedded +
		// agent_paths), runtime construction, and home expansion; the server
		// only needs the resolved config.
		if err := a.api.InitAgentStateWithOptions(agentservice.FromAgentConfig(a.cfg.Agent)); err != nil {
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

func (a *App) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(server.RequestID())
	router.Use(server.AccessLog())
	router.Use(server.Recovery())
	router.Use(server.SecurityHeaders())
	router.Use(server.MaxRequestBody())
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
		// 静态资源（带内容 hash 的 js/css/字体等）缺失时必须 404，
		// 不能回退 index.html：旧标签页在重新部署后会请求旧 hash 的
		// chunk，返回 HTML 会让浏览器报「Failed to fetch dynamically
		// imported module」，页面直接渲染失败。404 则由前端捕获并
		// 自动刷新到新版本。仅无扩展名的 SPA 路由才回退 index.html。
		if hasFileExtension(path) {
			c.Data(http.StatusNotFound, "text/plain; charset=utf-8", []byte("static asset not found"))
			return
		}
		serveIndex(c)
	})
}

// hasFileExtension 判断请求路径是否像静态资源文件（含扩展名）。
// SPA 路由（如 /stock/600519、/news）不含扩展名，仍走 index.html 回退。
func hasFileExtension(path string) bool {
	last := path
	if idx := strings.LastIndexByte(path, '/'); idx >= 0 {
		last = path[idx+1:]
	}
	if last == "" {
		return false
	}
	dot := strings.LastIndexByte(last, '.')
	// 扩展名过长说明点号大概率属于路径本身（如 /v1.2/release）。
	return dot > 0 && dot < len(last)-1 && len(last)-dot-1 <= 8
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
	log.Printf("Web UI: http://%s/", webHostPort(app.addr))
	return app.Run(signalCtx)
}

// webHostPort turns a bound address into something a browser can open: a
// wildcard bind (0.0.0.0/::) is reachable locally via loopback.
func webHostPort(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), port)
}

// newsSources 构造启用的资讯数据源。东财负责个股新闻与研报，
// 财联社提供实时快讯。雪球与巨潮的个股接口不可用，默认不启用。
func newsSources() []newsfeed.Feed {
	feeds := make([]newsfeed.Feed, 0, 2)
	for _, f := range sources.NewAllSources() {
		if f != nil {
			feeds = append(feeds, f)
		}
	}
	return feeds
}

// startNewsBackgroundSync 定时抓取全局快讯。
// 之前聚合器的 StartBackgroundFetch 全仓库没有任何调用点，
// 库里的数据只能靠手动触发，很快就过期了。这里把它接到应用生命周期上。
func (a *App) startNewsBackgroundSync() {
	if a.newsService == nil {
		return
	}
	// 热点事件挂在同步循环上：每次抓取完成后重建，「热点TOP10」才有数据。
	a.newsService.SetClusterer(newsfeed.NewClusterer(a.newsfeed, newsfeed.DefaultClusterConfig()))
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-a.runCtx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(a.runCtx, 60*time.Second)
				n, degraded := a.newsService.GlobalSync(ctx)
				cancel()
				if n > 0 {
					log.Printf("news: 后台同步写入 %d 条", n)
				}
				for _, d := range degraded {
					log.Printf("news: 数据源 %s 降级 - %s", d.Source, d.Error)
				}
			}
		}
	}()
}
