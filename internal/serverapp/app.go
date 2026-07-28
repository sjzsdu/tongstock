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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/agents"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/serviceproc"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/history"
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

// Run starts the TongStock HTTP server and blocks until it exits.
func Run() error {
	runCtx, cancelRun := context.WithCancel(context.Background())

	// Initialize config
	if err := config.Init(); err != nil {
		cancelRun()
		return fmt.Errorf("初始化配置失败: %w", err)
	}
	cfg := config.Get()

	// Initialize param config
	if err := param.AutoInit(); err != nil {
		log.Printf("初始化指标参数配置失败: %v", err)
	}

	// Create TDX executor (single client or Pool). The executor owns the
	// actual connection(s) and must outlive everything that uses them.
	hosts := cfg.TDX.Hosts
	if len(hosts) == 0 {
		hosts = tdx.DefaultHosts
	}
	executor, err := tdx.NewExecutor(func() (*tdx.Client, error) {
		return tdx.DialHosts(hosts, tdx.WithRedial(true))
	}, 3)
	if err != nil {
		return fmt.Errorf("创建 TDX 执行器失败: %w", err)
	}
	defer func() {
		if err := executor.Close(); err != nil {
			log.Printf("关闭 TDX 执行器失败: %v", err)
		}
	}()

	// Initialize unified storage
	s, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return fmt.Errorf("初始化存储失败: %w", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			log.Printf("关闭存储失败: %v", err)
		}
	}()

	// Create service backed by the executor
	svc, err := tdx.NewServiceWithExecutor(executor, s)
	if err != nil {
		return fmt.Errorf("创建服务失败: %w", err)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			log.Printf("关闭服务失败: %v", err)
		}
	}()

	// Initialize history store with same storage
	historyStore, err := history.New(s)
	if err != nil {
		return fmt.Errorf("打开历史数据库失败: %w", err)
	}

	// Initialize watchlist store with same storage
	watchlistStore, err := watchlist.New(s)
	if err != nil {
		return fmt.Errorf("打开自选股数据库失败: %w", err)
	}

	// Initialize trading store with same storage
	tradingStore, err := trading.New(s)
	if err != nil {
		return fmt.Errorf("打开交易数据库失败: %w", err)
	}

	// Initialize stockpool store with same storage
	stockpoolStore, err := stockpool.New(s)
	if err != nil {
		return fmt.Errorf("打开股票池数据库失败: %w", err)
	}

	// Initialize stockinfo store with same storage
	stockinfoStore, err := stockinfo.New(s)
	if err != nil {
		return fmt.Errorf("打开股票信息数据库失败: %w", err)
	}

	// Create HTTP server
	httpServer := server.NewServer(svc, historyStore, watchlistStore, tradingStore, stockpoolStore, stockinfoStore)
	defer func() {
		cancelRun()
		httpServer.WaitForBackgroundTasks()
	}()

	// Initialize Agent (picoclaw) if enabled
	if cfg.Agent.Enabled {
		log.Println("Initializing AI Agent (picoclaw)...")
		agentLister := func() ([]server.EmbeddedAgent, error) {
			allAgents, err := agents.All()
			if err != nil {
				return nil, err
			}
			result := make([]server.EmbeddedAgent, len(allAgents))
			for i, a := range allAgents {
				result[i] = server.EmbeddedAgent{
					ID:          a.ID,
					Name:        a.Name,
					Description: a.Description,
					Prompt:      a.Prompt,
					Soul:        a.Soul,
					Skills:      a.Skills,
					Tools:       a.Tools,
					NoHistory:   a.NoHistory,
				}
			}
			return result, nil
		}
		httpServer.SetAgentLister(agentLister)
		// Expand ~ in home and config paths
		agentHome := cfg.Agent.Home
		if strings.HasPrefix(agentHome, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				agentHome = filepath.Join(home, strings.TrimPrefix(agentHome, "~"))
			}
		}
		agentConfig := cfg.Agent.Config
		if strings.HasPrefix(agentConfig, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				agentConfig = filepath.Join(home, strings.TrimPrefix(agentConfig, "~"))
			}
		}
		if err := httpServer.InitAgentState(agentHome, agentConfig, cfg.Agent.Model, cfg.Agent.Agent, cfg.Agent.StockAgent, ""); err != nil {
			log.Printf("Warning: Failed to initialize agent: %v", err)
		} else {
			log.Println("AI Agent initialized successfully")
		}
	}

	// Initialize chat store for session persistence
	chatStore, err := server.NewChatStoreWithStorage("", s)
	if err != nil {
		log.Printf("Warning: Failed to initialize chat store: %v", err)
	} else {
		httpServer.SetChatStore(chatStore)
		log.Printf("Chat store initialized")
	}

	// Initialize paradigm store
	paradigmStore, err := paradigms.NewStoreWithStorage("", s)
	if err != nil {
		log.Printf("Warning: Failed to initialize paradigm store: %v", err)
	} else {
		httpServer.SetParadigmStore(paradigmStore)
		log.Printf("Paradigm store initialized: %d paradigms loaded", paradigmStore.Count())
		httpServer.StartParadigmAlertScanner(runCtx, 5*time.Minute)
	}

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(server.SecurityHeaders())
	r.Use(server.MaxRequestBody())

	// Protect API traffic for non-loopback binds while keeping the embedded
	// SPA and health endpoint reachable for browser token bootstrap.
	httpServer.SetupRoutes(r, server.AccessTokenAuth(cfg.Server.BindAddress, cfg.Server.AccessToken))

	// Serve static files for SPA
	r.GET("/", func(c *gin.Context) {
		f, err := web.DistFS().Open("index.html")
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to open index.html"})
			return
		}
		defer f.Close()

		data, err := io.ReadAll(f)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to read index.html"})
			return
		}

		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/") {
			path = path[1:]
		}

		if web.Exists(path) {
			c.FileFromFS(path, web.DistFS())
			return
		}

		f, err := web.DistFS().Open("index.html")
		if err != nil {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		defer f.Close()

		data, err := io.ReadAll(f)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to read index.html"})
			return
		}

		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// Start server
	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}

	// Security rule: non-loopback binds MUST have an access token set.
	bind, isLoopback, err := server.ValidateBindSecurity(cfg.Server.BindAddress, cfg.Server.AccessToken)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(bind, fmt.Sprintf("%d", port))
	log.Printf("TongStock server starting on %s (loopback=%v, token=%v)", addr, isLoopback, cfg.Server.AccessToken != "")

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("启动服务器失败: %w", err)
	}

	record := serviceproc.CurrentRecord()
	if err := serviceproc.Write(record); err != nil {
		_ = listener.Close()
		return fmt.Errorf("记录服务进程失败: %w", err)
	}
	defer serviceproc.RemoveIfPID(record.PID)

	httpDaemon := &http.Server{Addr: addr, Handler: r}
	serverErr := make(chan error, 1)
	go func() {
		err := httpDaemon.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErr <- err
	}()

	signalCtx, stopSignals := signal.NotifyContext(runCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	select {
	case err := <-serverErr:
		return err
	case <-signalCtx.Done():
		log.Println("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := httpDaemon.Shutdown(shutdownCtx); err != nil {
			_ = httpDaemon.Close()
			return fmt.Errorf("关闭 HTTP 服务失败: %w", err)
		}
		return <-serverErr
	}
}
