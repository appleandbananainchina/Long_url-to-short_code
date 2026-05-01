package main

import (
	"context"
	"log"
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
	"golang.org/x/time/rate"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system env")
	}
	// 加载配置
	cfg, err := config.Load("config.json") // 可改为环境变量或命令行参数
	if err != nil {
		log.Fatalf("load config error: %v", err)
	}

	// 初始化MySQL
	mysqlRepo, err := repository.NewMySQLRepo(cfg.MySQL.DSN)
	if err != nil {
		log.Fatalf("connect mysql error: %v", err)
	}
	defer mysqlRepo.Close()

	// 初始化Redis
	redisCli := cache.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)

	// 初始化布隆过滤器
	if err := bloom.InitBloom(redisCli.Raw(), 10_000_000, 0.0001); err != nil {
		log.Fatalf("init bloom error: %v", err)
	}

	// 初始化业务服务
	shortenerService := service.NewShortenerService(cfg.IDGen.MachineID, mysqlRepo, redisCli)

	statistics.InitStatisticsWorker(10, 10000, redisCli)

	//预热布隆过滤器
	go func() {
		if err := bloom.Warmup(redisCli.Raw(), mysqlRepo, 200); err != nil {
			log.Printf("bloom warmup error: %v", err)
		}
	}()

	limiter := middleware.NewIPRateLimiter(rate.Limit(1), 3, 5*time.Minute)

	// 设置路由
	mux := http.NewServeMux()
	mux.HandleFunc("/shorten", handler.ShortenHandler(shortenerService))
	mux.HandleFunc("/", handler.RedirectHandler(shortenerService))

	// 应用中间件
	handlerWithMiddleware := middleware.Recover(limiter.RateLimitMiddleWare(mux))

	// 启动HTTP服务器
	srv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: handlerWithMiddleware,
	}

	// 优雅关闭
	go func() {
		log.Printf("Server starting on :%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	statistics.StopWorker()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	log.Println("Server exited")
}
