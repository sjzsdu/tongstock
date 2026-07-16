package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/server"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
	"github.com/sjzsdu/tongstock/pkg/web"
)

func main() {
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
		return tdx.DialHosts(hosts)
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

	// Create HTTP server
	httpServer := server.NewServer(svc, historyStore, watchlistStore)

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
