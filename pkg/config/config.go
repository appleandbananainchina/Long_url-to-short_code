package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
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
		DSN string `json:"dsn" yaml:"dsn"` // "user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"
	} `json:"mysql" yaml:"mysql"`

	IDGen struct {
		MachineID int64 `json:"machine_id" yaml:"machine_id"` // 雪花算法机器ID（0-1023）
	} `json:"idgen" yaml:"idgen"`
}

// Load 从文件或环境变量加载配置，优先级：环境变量 > 文件 > 默认值
func Load(path string) (*Config, error) {
	cfg := &Config{}
	// 默认值
	cfg.Server.Port = "8080"
	cfg.Redis.Addr = "localhost:6379"
	cfg.Redis.Password = ""
	cfg.Redis.DB = 0
	cfg.MySQL.DSN = ""

	// 尝试读取配置文件
	if path != "" {
		data, err := ioutil.ReadFile(filepath.Clean(path))
		if err == nil {
			// 简单支持JSON，实际可使用yaml库
			_ = json.Unmarshal(data, cfg)
		}
	}

	// 环境变量覆盖
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
	// ... 其他环境变量

	return cfg, nil
}
