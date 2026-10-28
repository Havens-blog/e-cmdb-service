package ioc

import (
	"github.com/Havens-blog/e-cmdb-service/internal"
	"github.com/Havens-blog/e-common-go/mongox"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/viper"
)

// InitWebServer 初始化 Web 服务器并挂载 CMDB 路由。
func InitWebServer(db *mongox.Mongo) *gin.Engine {
	logger := elog.DefaultLogger
	logger.Info("开始初始化Web服务器")

	gin.SetMode(gin.ReleaseMode)
	server := gin.Default()

	// gin.Context 可作为 context.Context 透传
	server.ContextWithFallback = true

	// 健康检查（不需要认证，供 K8s 探针与网关健康检查）
	server.GET("/api/v1/cmdb/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": 0, "msg": "ok"})
	})

	// 装配 CMDB 模块并注册路由
	// 用户路由经 SessionAuth 本地校验平台共享会话（不再信任网关透传头），
	// /internal 路由经 S2SAuth 验 eiam 签发的服务令牌。
	sp := InitSessionProvider()
	module := internal.InitModule(db)
	module.RegisterRoutes(server, sp, viper.GetString("eiam.public_key"))

	logger.Info("Web服务器初始化完成")
	return server
}
