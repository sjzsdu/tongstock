package serverapp

import (
	"fmt"
	"io"
	"log"
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
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/server"
	"github.com/sjzsdu/tongstock/pkg/stockpool"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
	"github.com/sjzsdu/tongstock/pkg/web"
)

// Run starts the TongStock HTTP server and blocks until it exits.
func Run() {
	// Initialize config
	if err := config.Init(); err != nil {
		log.Fatalf("初始化配置失败: %v", err)
	}
	cfg := config.Get()

	// Initialize param config
	if err := param.AutoInit(); err != nil {
		log.Printf("初始化指标参数配置失败: %v", err)
	}

	// Create TDX client pool
	hosts := cfg.TDX.Hosts
	if len(hosts) == 0 {
		hosts = tdx.DefaultHosts
	}
	pool, err := tdx.NewPool(func() (*tdx.Client, error) {
		return tdx.DialHosts(hosts, tdx.WithRedial(true))
	}, 3)
	if err != nil {
		log.Fatalf("创建连接池失败: %v", err)
	}

	// Get a client from pool to create service
	client, err := pool.Get()
	if err != nil {
		log.Fatalf("获取连接失败: %v", err)
	}

	// Initialize unified storage
	s, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		log.Fatalf("初始化存储失败: %v", err)
	}

	// Create service with shared storage
	svc, err := tdx.NewService(client, s)
	if err != nil {
		log.Fatalf("创建服务失败: %v", err)
	}

	// Initialize history store with same storage
	historyStore, err := history.New(s)
	if err != nil {
		log.Fatalf("打开历史数据库失败: %v", err)
	}

	// Initialize watchlist store with same storage
	watchlistStore, err := watchlist.New(s)
	if err != nil {
		log.Fatalf("打开自选股数据库失败: %v", err)
	}

	// Initialize trading store with same storage
	tradingStore, err := trading.New(s)
	if err != nil {
		log.Fatalf("打开交易数据库失败: %v", err)
	}

	// Initialize stockpool store with same storage
	stockpoolStore, err := stockpool.New(s)
	if err != nil {
		log.Fatalf("打开股票池数据库失败: %v", err)
	}

	// Create HTTP server
	httpServer := server.NewServer(svc, historyStore, watchlistStore, tradingStore, stockpoolStore)

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
		server.RegisterAgentLister(agentLister)
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
		server.ParadigmStore = paradigmStore
		log.Printf("Paradigm store initialized: %d paradigms loaded", paradigmStore.Count())
		httpServer.StartParadigmAlertScanner(5 * time.Minute)
	}

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Setup API routes
	httpServer.SetupRoutes(r)

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

	addr := fmt.Sprintf(":%d", port)
	log.Printf("TongStock server starting on %s", addr)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down server...")

		// Close resources
		if err := svc.Close(); err != nil {
			log.Printf("关闭服务失败: %v", err)
		}
		if err := s.Close(); err != nil {
			log.Printf("关闭存储失败: %v", err)
		}

		os.Exit(0)
	}()

	if err := r.Run(addr); err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}
}
