package ioc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// InitMongoDB 初始化 MongoDB 连接。
// 与 e-cam-service 共用同一套 mongodb 配置段与 ecam 库，CMDB 集合
// （ecam_instance / ecam_model 等）不变，拆分后数据零迁移。
func InitMongoDB() *mongox.Mongo {
	logger := elog.DefaultLogger
	logger.Info("开始初始化MongoDB连接")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type Config struct {
		DSN      string `mapstructure:"dsn"`
		DB       string `mapstructure:"db"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}

	var cfg Config
	if err := viper.UnmarshalKey("mongodb", &cfg); err != nil {
		logger.Error("读取MongoDB配置失败", elog.FieldErr(err))
		panic(fmt.Errorf("failed to read mongodb config: %w", err))
	}

	if cfg.DSN == "" || cfg.DB == "" || cfg.Username == "" || cfg.Password == "" {
		panic(fmt.Errorf("mongodb config incomplete: dsn/db/username/password are all required"))
	}

	dsn := strings.Split(cfg.DSN, "//")
	if len(dsn) != 2 {
		panic(fmt.Errorf("invalid mongodb dsn format: %s", cfg.DSN))
	}

	uri := fmt.Sprintf("%s//%s:%s@%s", dsn[0], cfg.Username, cfg.Password, dsn[1])
	logger.Info("构建MongoDB连接URI", elog.String("db", cfg.DB))

	opts := options.Client().
		ApplyURI(uri).
		SetCompressors([]string{"zstd", "snappy"}).
		SetMaxPoolSize(100).
		SetMinPoolSize(10).
		SetMaxConnecting(10).
		SetServerSelectionTimeout(5 * time.Second).
		SetConnectTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		panic(fmt.Errorf("failed to connect to mongodb: %w", err))
	}
	if err = client.Ping(ctx, nil); err != nil {
		panic(fmt.Errorf("failed to ping mongodb server: %w", err))
	}

	logger.Info("MongoDB连接初始化完成", elog.String("database", cfg.DB))
	return mongox.NewMongo(client, cfg.DB)
}
