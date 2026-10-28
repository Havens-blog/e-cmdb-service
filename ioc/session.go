package ioc

import (
	"context"
	"fmt"
	"time"

	"github.com/ecodeclub/ginx/session"
	"github.com/ecodeclub/ginx/session/cookie"
	"github.com/ecodeclub/ginx/session/header"
	"github.com/ecodeclub/ginx/session/mixin"
	ginRedis "github.com/ecodeclub/ginx/session/redis"
	goRedis "github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// InitSessionProvider 构建平台共享会话提供者（eiam 签发会话，本服务只读校验）。
//
// 配置（与 eiam / e-cam-service 逐字节一致，否则解不开对方签发的会话）：
//   - session.session_encrypted_key：HS256 会话签名密钥
//   - session.cookie.name / domain：会话令牌 Cookie 载体（读取用 name；本服务不签发）
//   - redis.addr / password / db：会话 Redis（db 必须与 eiam 一致 = 1）
//
// fail-fast：密钥缺失或 Redis 不可达时 panic，宁可启动失败也不裸奔
// （没有会话校验的 CMDB 等于向全网开放跨租户读写）。
func InitSessionProvider() session.Provider {
	type Config struct {
		SessionEncryptedKey string `mapstructure:"session_encrypted_key"`
		Cookie              struct {
			Domain string `mapstructure:"domain"`
			Name   string `mapstructure:"name"`
		} `mapstructure:"cookie"`
	}
	var cfg Config
	if err := viper.UnmarshalKey("session", &cfg); err != nil {
		panic(fmt.Errorf("读取会话配置失败: %w", err))
	}
	if cfg.SessionEncryptedKey == "" {
		panic(fmt.Errorf("session.session_encrypted_key 未配置：无法本地校验平台会话"))
	}
	if cfg.Cookie.Name == "" {
		cfg.Cookie.Name = "ecmdb-token-key" // 平台默认会话 Cookie 名（eiam session.cookie.name）
	}

	addr := viper.GetString("redis.addr")
	if addr == "" {
		panic(fmt.Errorf("redis.addr 未配置：会话存储不可用"))
	}
	cmd := goRedis.NewClient(&goRedis.Options{
		Addr:     addr,
		Password: viper.GetString("redis.password"),
		DB:       viper.GetInt("redis.db"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cmd.Ping(ctx).Err(); err != nil {
		panic(fmt.Errorf("会话 Redis 连接失败 (%s): %w", addr, err))
	}

	const day = time.Hour * 24 * 30
	sp := ginRedis.NewSessionProvider(cmd, cfg.SessionEncryptedKey, day)
	// 令牌载体：Authorization Bearer 头 + Cookie 双通道（与 e-cam-service 同构），
	// 本服务只 Extract 不 Inject，cookie 的 Domain/Secure 仅影响签发侧。
	cookieC := &cookie.TokenCarrier{
		MaxAge:   int(day.Seconds()),
		Name:     cfg.Cookie.Name,
		Secure:   true,
		HttpOnly: false,
		Domain:   cfg.Cookie.Domain,
	}
	sp.TokenCarrier = mixin.NewTokenCarrier(header.NewTokenCarrier(), cookieC)
	return sp
}
