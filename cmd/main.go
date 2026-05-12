package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"short-url-service/pkg/bloom"
	"short-url-service/pkg/statistics"
	"syscall"
	"time"

	"short-url-service/internal/handler"
	"short-url-service/internal/middleware"
	"short-url-service/internal/repository"
	"short-url-service/internal/service"
	"short-url-service/pkg/cache"
	"short-url-service/pkg/config"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
)

func init() {
	// JSON 格式输出，便于日志系统采集
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	traceHandler := middleware.NewTraceHandler(jsonHandler)
	logger := slog.New(traceHandler).With("service", "short-url")
	slog.SetDefault(logger)
}

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Info("No .env file found, relying on system env")
	}
	// 加载配置
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml" // 或 config.json
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("load config error", "error", err)
		os.Exit(1)
	}

	// 初始化MySQL
	mysqlRepo, err := repository.NewMySQLRepo(cfg.MySQL.DSN)
	if err != nil {
		slog.Error("connect mysql error", "error", err)
		os.Exit(1)
	}
	defer mysqlRepo.Close()

	// 初始化Redis
	redisCli := cache.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)

	// 初始化布隆过滤器
	if err := bloom.InitBloom(redisCli.Raw(), 10_000_000, 0.0001); err != nil {
		slog.Error("init bloom error", "error", err)
		os.Exit(1)
	}

	// 初始化业务服务
	shortenerService := service.NewShortenerService(cfg.IDGen.MachineID, mysqlRepo, redisCli)

	statistics.InitStatisticsWorker(10, 10000, redisCli)

	//预热布隆过滤器
	go func() {
		if err := bloom.Warmup(redisCli.Raw(), mysqlRepo, 200, context.Background()); err != nil {
			slog.Error("bloom warmup error", "error", err)
		}
	}()

	var l middleware.Limiter

	if !cfg.RateLimit.Enabled {
		l = middleware.NoopLimiter{}
	} else {
		switch cfg.RateLimit.Type {
		case "redis":
			// 需要传入 redis 客户端
			l = middleware.NewRedisLimiter(redisCli.Raw(), cfg.RateLimit.Redis.Rate)
		case "memory":
			// 使用原有的内存限流器
			l = middleware.NewIPRateLimiter(
				rate.Limit(cfg.RateLimit.Memory.Rate),
				cfg.RateLimit.Memory.Burst,
				5*time.Minute, // 清理间隔可配置
			)
		default:
			l = middleware.NoopLimiter{}
		}
	}

	// 设置路由
	mux := http.NewServeMux()
	mux.HandleFunc("/shorten", handler.ShortenHandler(shortenerService))
	mux.HandleFunc("/", handler.RedirectHandler(shortenerService))
	mux.Handle("/metrics", promhttp.Handler())
	// 应用中间件
	handlerWithMiddleware := middleware.TraceMiddleware(middleware.Recover(l.Middleware(mux)))

	// 启动HTTP服务器
	srv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: handlerWithMiddleware,
	}

	// 优雅关闭
	go func() {
		slog.Info("Server starting", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	statistics.StopWorker(time.Duration(10) * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("Server exited")
}
