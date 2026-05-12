package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 全局变量，方便其他包直接使用
var (
	// HTTP 请求总数（Counter）
	HTTPRequestsTotal *prometheus.CounterVec
	// HTTP 请求耗时（Histogram）
	HTTPRequestDuration *prometheus.HistogramVec

	// 熔断器状态（Gauge）
	CircuitBreakerState *prometheus.GaugeVec

	// 布隆过滤器计数器
	BloomFilterHits   prometheus.Counter
	BloomFilterMisses prometheus.Counter

	// 统计队列指标（Gauge）
	StatisticsQueueLength  prometheus.Gauge
	StatisticsQueueDropped prometheus.Counter
)

func init() {
	// 1. HTTP 请求指标
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"}, // 三个标签
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets, // 默认桶: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		},
		[]string{"method", "path"},
	)

	// 2. 熔断器状态指标 (示例：针对 "bloom-breaker")
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"breaker_name"}, // 标签：熔断器名称
	)

	// 3. 布隆过滤器指标
	BloomFilterHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "bloom_filter_hits_total",
			Help: "Total number of bloom filter hits (short code found)",
		},
	)

	BloomFilterMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "bloom_filter_misses_total",
			Help: "Total number of bloom filter misses (short code not found, fallback triggered)",
		},
	)

	// 4. 统计队列指标 (假设 StatisticsWorker 是 *StatisticsWorker)
	// 这些是示例，实际需要通过你的 StatisticsWorker 来更新
	StatisticsQueueLength = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "statistics_queue_length",
			Help: "Current length of the statistics task queue",
		},
	)

	StatisticsQueueDropped = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "statistics_queue_dropped_total",
			Help: "Total number of tasks dropped due to full queue",
		},
	)
}
