package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	ReadTimeout  string `mapstructure:"read_timeout"`
	WriteTimeout string `mapstructure:"write_timeout"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	MaxConns int    `mapstructure:"max_conns"`
	MaxIdle  int    `mapstructure:"max_idle"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

// AccountConfig 模拟交易费用配置（与 quant-engine 共用同一份数值，禁止单独改动）
type AccountConfig struct {
	InitialCash     float64 `mapstructure:"initial_cash"`
	CommissionRate  float64 `mapstructure:"commission_rate"`
	MinCommission   float64 `mapstructure:"min_commission"`
	StampTaxRate    float64 `mapstructure:"stamp_tax_rate"`
	Slippage        float64 `mapstructure:"slippage"`
}

// Config 总配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
	Account  AccountConfig  `mapstructure:"account"`
}

// Load 加载配置文件，敏感项优先取环境变量（DB_PASSWORD 等）
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// 容器内配置路径 /app/configs，本地开发用 ./configs
	for _, dir := range []string{"/app/configs", "configs"} {
		if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
			v.AddConfigPath(dir)
			break
		}
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 环境变量覆盖（Docker Compose 注入）
	cfg.Database.Host = getEnv("DB_HOST", cfg.Database.Host)
	cfg.Database.Port = getEnvInt("DB_PORT", cfg.Database.Port)
	cfg.Database.User = getEnv("DB_USER", cfg.Database.User)
	cfg.Database.Password = getEnv("DB_PASSWORD", cfg.Database.Password)
	cfg.Database.Name = getEnv("DB_NAME", cfg.Database.Name)

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
