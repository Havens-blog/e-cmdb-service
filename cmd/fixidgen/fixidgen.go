// Package fixidgen 提供 fix-idgen 子命令：把 ecam_id_generator 计数器同步到各集合
// 实际的 max(id)，修复「计数器与数据脱节」导致的模型/分组创建重复键错误。
//
// 背景：ecam_model / ecam_model_group 等集合的数据自 e-cam-service 迁移而来，
// 但 ecam_id_generator 的 next_id 未同步，仍停留在很小的值；创建时 GetIdGenerator
// 返回的 id 与已有数据冲突（E11000 duplicate key），导致模型创建失败。
//
// 使用方式：
//
//	go run . fix-idgen -f config/prod.yaml
//	go run . fix-idgen -f config/prod.yaml --dry-run   # 只预览、不写库
//
// 幂等：用 $max 更新，只会把 next_id 抬高到 max(id)，不会降低；可安全重复执行。
package fixidgen

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/ioc"
	"github.com/Havens-blog/e-common-go/mongox"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const idGenCollection = "ecam_id_generator"

// collections 使用 GetIdGenerator 自增业务 id 的集合。
var collections = []string{
	"ecam_model",
	"ecam_model_group",
	"ecam_attribute",
	"ecam_attribute_group",
	"ecam_model_relation_type",
	"ecam_instance",
	"ecam_instance_relation",
	"ecam_api_token",
	"ecam_webhook",
}

var dryRun bool

// Cmd fix-idgen 子命令。
var Cmd = &cobra.Command{
	Use:   "fix-idgen",
	Short: "同步 ecam_id_generator 计数器到各集合实际最大 id",
	Long: "读取各使用自增 id 的集合，取 max(id)，并用 $max 幂等地把 ecam_id_generator 的\n" +
		"next_id 抬高到该值，修复迁移后计数器脱节导致的重复键错误。\n" +
		"建议先加 --dry-run 预览。",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(cmd.Context())
	},
}

func init() {
	Cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只读取并预览，不写入数据库")
}

func run(ctx context.Context) error {
	ioc.InitCustomLogger()
	logger := elog.DefaultLogger

	db := ioc.InitMongoDB()
	idGen := db.Collection(idGenCollection)

	fmt.Println("=== fix-idgen ===")
	for _, name := range collections {
		maxID, err := maxIDOf(ctx, db, name)
		if err != nil {
			logger.Error("查询集合最大 id 失败",
				elog.String("collection", name), elog.FieldErr(err))
			fmt.Printf("  [err] %-28s %v\n", name, err)
			continue
		}
		if maxID <= 0 {
			fmt.Printf("  [skip] %-28s 无数据（max_id=0）\n", name)
			continue
		}

		if dryRun {
			fmt.Printf("  [dry-run] %-28s 将同步 next_id -> %d\n", name, maxID)
			continue
		}

		if err := syncCounter(ctx, idGen, name, maxID); err != nil {
			logger.Error("同步计数器失败",
				elog.String("collection", name), elog.FieldErr(err))
			fmt.Printf("  [err] %-28s %v\n", name, err)
			continue
		}
		fmt.Printf("  [ok] %-28s next_id -> %d\n", name, maxID)
	}
	fmt.Println("=== 完成 ===")
	return nil
}

// maxIDOf 返回集合中业务 id 字段的最大值；空集合返回 0。
func maxIDOf(ctx context.Context, db *mongox.Mongo, name string) (int64, error) {
	var doc struct {
		ID int64 `bson:"id"`
	}
	err := db.Collection(name).FindOne(
		ctx,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "id", Value: -1}}),
	).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, err
	}
	return doc.ID, nil
}

// syncCounter 用 $max 幂等地把 next_id 抬高到 maxID（upsert）。
func syncCounter(ctx context.Context, idGen *mongo.Collection, name string, maxID int64) error {
	_, err := idGen.UpdateOne(
		ctx,
		bson.M{"name": name},
		bson.M{"$max": bson.M{"next_id": maxID}},
		options.Update().SetUpsert(true),
	)
	return err
}
