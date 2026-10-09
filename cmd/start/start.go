package start

import (
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/ioc"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Cmd start 子命令：启动 CMDB API 服务。
var Cmd = &cobra.Command{
	Use:   "start",
	Short: "CMDB API 服务",
	Long:  "启动 CMDB 配置管理服务，对外暴露资产模型/实例/关系/拓扑接口",
	RunE: func(cmd *cobra.Command, args []string) error {
		ioc.InitCustomLogger()
		logger := elog.DefaultLogger
		logger.Info("开始启动 CMDB 服务")

		logger.Info("初始化 MongoDB")
		db := ioc.InitMongoDB()

		logger.Info("初始化 Web 服务器")
		server := ioc.InitWebServer(db)

		type Config struct {
			Port string `mapstructure:"port"`
		}
		var cfg Config
		if err := viper.UnmarshalKey("e-cmdb-service", &cfg); err != nil {
			logger.Error("读取服务配置失败", elog.FieldErr(err))
			panic(fmt.Errorf("failed to read service config: %w", err))
		}
		if cfg.Port == "" {
			cfg.Port = "8002"
		}

		logger.Info("启动 HTTP 服务器", elog.String("port", cfg.Port))
		if err := server.Run(fmt.Sprintf(":%s", cfg.Port)); err != nil {
			logger.Error("HTTP服务器启动失败", elog.FieldErr(err))
			panic(fmt.Errorf("HTTP server failed to start: %w", err))
		}
		return nil
	},
}
