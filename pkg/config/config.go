package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port string `json:"port" yaml:"port"`
	} `json:"server" yaml:"server"`

	Redis struct {
		Addr     string `json:"addr" yaml:"addr"`
		Password string `json:"password" yaml:"password"`
		DB       int    `json:"db" yaml:"db"`
	} `json:"redis" yaml:"redis"`

	MySQL struct {
		DSN string `json:"dsn" yaml:"dsn"`
	} `json:"mysql" yaml:"mysql"`

	IDGen struct {
		MachineID int64 `json:"machine_id" yaml:"machine_id"`
	} `json:"idgen" yaml:"idgen"`

	RateLimit RateLimitConfig `json:"rate_limit" yaml:"rate_limit"`
}

type RateLimitConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"` // 总开关
	Type    string `yaml:"type" json:"type"`       // "memory", "redis", "noop"
	Memory  struct {
		Rate  float64 `yaml:"rate" json:"rate"`
		Burst int     `yaml:"burst" json:"burst"`
	} `yaml:"memory" json:"memory"`
	Redis struct {
		Rate int `yaml:"rate" json:"rate"` // 每秒请求数
	} `yaml:"redis" json:"redis"`
}

// Load 从文件加载配置，支持 .json 或 .yaml/.yml，并允许环境变量覆盖
func Load(path string) (*Config, error) {
	cfg := &Config{}
	// 默认值
	cfg.Server.Port = "8080"
	cfg.Redis.Addr = "localhost:6379"
	cfg.Redis.Password = ""
	cfg.Redis.DB = 0
	cfg.MySQL.DSN = ""
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.Type = "memory"
	cfg.RateLimit.Memory.Rate = 20
	cfg.RateLimit.Memory.Burst = 30
	cfg.RateLimit.Redis.Rate = 20

	// 如果指定了配置文件，尝试读取
	if path != "" {
		data, err := os.ReadFile(filepath.Clean(path))
		if err == nil {
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".json":
				err = json.Unmarshal(data, cfg)
			case ".yaml", ".yml":
				err = yaml.Unmarshal(data, cfg)
			}
			if err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	// 环境变量覆盖（优先级最高）
	if port := os.Getenv("SERVER_PORT"); port != "" {
		cfg.Server.Port = port
	}
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		cfg.Redis.Addr = redisAddr
	}
	if redisPwd := os.Getenv("REDIS_PASSWORD"); redisPwd != "" {
		cfg.Redis.Password = redisPwd
	}
	if mysqlDSN := os.Getenv("MYSQL_DSN"); mysqlDSN != "" {
		cfg.MySQL.DSN = mysqlDSN
	}
	if machineID := os.Getenv("MACHINE_ID"); machineID != "" {
		if id, err := strconv.ParseInt(machineID, 10, 64); err == nil {
			cfg.IDGen.MachineID = id
		}
	}
	// ========== 限流环境变量覆盖 ==========
	if v := os.Getenv("RATE_LIMIT_ENABLED"); v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			cfg.RateLimit.Enabled = true
		case "false", "0", "no", "off":
			cfg.RateLimit.Enabled = false
		}
	}
	if v := os.Getenv("RATE_LIMIT_TYPE"); v != "" {
		cfg.RateLimit.Type = v
	}
	if v := os.Getenv("RATE_LIMIT_MEMORY_RATE"); v != "" {
		if rate, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RateLimit.Memory.Rate = rate
		}
	}
	if v := os.Getenv("RATE_LIMIT_MEMORY_BURST"); v != "" {
		if burst, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.Memory.Burst = burst
		}
	}
	if v := os.Getenv("RATE_LIMIT_REDIS_RATE"); v != "" {
		if rate, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.Redis.Rate = rate
		}
	}
	return cfg, nil
}
