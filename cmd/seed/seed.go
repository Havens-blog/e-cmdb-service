// Package seed 提供 seed 子命令：为 dev MongoDB 中已存在的模型（Model）批量生成
// 示例实例（Instance）数据，打通 dev 库「模型已就绪、实例为空」导致的 E2E 空数据问题。
//
// 使用方式：
//
//	go run . seed -f config/prod.yaml
//	go run . seed -f config/prod.yaml --dry-run          # 只预览、不写库
//	go run . seed -f config/prod.yaml --tenant-id 1 --per-model 3
//
// 说明：
//   - 复用 ioc.InitMongoDB() 读取 config/prod.yaml 的 mongodb 段（dsn/db/username/password），
//     不在代码里硬编码任何凭据。
//   - 从 ecam_model 集合读出全部模型，为每个模型生成确定性 AssetID 的示例实例，
//     「先查后插」保证幂等：重复执行跳过已存在实例，不重复插入。
//   - 只写 ecam_instance 集合，不改模型/属性/关系等其它集合。
package seed

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"github.com/Havens-blog/e-cmdb-service/internal/seedgen"
	"github.com/Havens-blog/e-cmdb-service/ioc"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
)

var (
	tenantID    int64
	accountID   int64
	perModel    int
	assetPrefix string
	dryRun      bool
)

// Cmd seed 子命令：为已有模型生成示例实例数据。
var Cmd = &cobra.Command{
	Use:   "seed",
	Short: "为已有模型生成示例实例数据",
	Long: "从 MongoDB 读出所有模型（ecam_model），为每个模型生成若干条符合约束的示例实例（ecam_instance）。\n" +
		"AssetID 使用确定性前缀（默认 seed-），先查后插，幂等可重复执行。\n" +
		"建议先加 --dry-run 预览将影响的模型与实例数量。",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(cmd.Context())
	},
}

func init() {
	Cmd.Flags().Int64Var(&tenantID, "tenant-id", 1, "造数实例所属租户ID（Instance.Validate 要求非 0）")
	Cmd.Flags().Int64Var(&accountID, "account-id", 1, "造数实例所属云账号ID")
	Cmd.Flags().IntVar(&perModel, "per-model", seedgen.DefaultPerModel, "每个模型生成的示例实例数量")
	Cmd.Flags().StringVar(&assetPrefix, "asset-prefix", seedgen.DefaultAssetPrefix, "示例实例 AssetID 前缀（幂等键的一部分）")
	Cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只读取并预览，不写入数据库")
}

func run(ctx context.Context) error {
	ioc.InitCustomLogger()
	logger := elog.DefaultLogger

	logger.Info("初始化 MongoDB（读取 config 的 mongodb 段）")
	db := ioc.InitMongoDB()

	modelDAO := dao.NewModelDAO(db)
	instanceDAO := dao.NewInstanceDAO(db)
	modelRepo := repository.NewModelRepository(modelDAO)
	instanceRepo := repository.NewInstanceRepository(instanceDAO)

	// 1. 读出全部模型。
	models, err := modelRepo.List(ctx, domain.ModelFilter{})
	if err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	if len(models) == 0 {
		fmt.Println("⚠️  ecam_model 集合为空，没有可造数的模型。请先初始化模型数据。")
		return nil
	}

	fmt.Printf("发现模型数量: %d\n", len(models))

	var (
		inserted   int
		skipped    int
		failed     int
		modelsHit  int
		modelsSkip int
	)

	for _, m := range models {
		meta := seedgen.Model{UID: m.UID, Name: m.Name, Provider: m.Provider}
		instances := seedgen.GenerateInstances(meta, tenantID, accountID, perModel, assetPrefix)

		if dryRun {
			fmt.Printf("  [dry-run] model=%s name=%q provider=%q -> %d 条示例实例\n",
				m.UID, m.Name, m.Provider, len(instances))
			for _, inst := range instances {
				fmt.Printf("      asset_id=%s asset_name=%q\n", inst.AssetID, inst.AssetName)
			}
			modelsHit++
			continue
		}

		// 2. 幂等：逐条「先查后插」。
		created := 0
		exists := 0
		for _, inst := range instances {
			if err := inst.Validate(); err != nil {
				logger.Error("生成的实例未通过校验",
					elog.String("model_uid", inst.ModelUID),
					elog.FieldErr(err))
				failed++
				continue
			}

			existing, err := instanceRepo.GetByAssetID(ctx, inst.TenantID, inst.ModelUID, inst.AssetID)
			if err != nil {
				logger.Error("查询已有实例失败",
					elog.String("model_uid", inst.ModelUID),
					elog.String("asset_id", inst.AssetID),
					elog.FieldErr(err))
				failed++
				continue
			}
			if existing.ID > 0 {
				exists++
				continue
			}

			if _, err := instanceRepo.Create(ctx, inst); err != nil {
				logger.Error("插入实例失败",
					elog.String("model_uid", inst.ModelUID),
					elog.String("asset_id", inst.AssetID),
					elog.FieldErr(err))
				failed++
				continue
			}
			created++
		}

		inserted += created
		skipped += exists
		if created > 0 {
			modelsHit++
			fmt.Printf("  model=%s 新增 %d 条", m.UID, created)
			if exists > 0 {
				fmt.Printf("（跳过已存在 %d 条）", exists)
			}
			fmt.Println()
		} else if exists > 0 {
			modelsSkip++
			fmt.Printf("  model=%s 已存在 %d 条示例实例，跳过\n", m.UID, exists)
		}
	}

	fmt.Println()
	fmt.Println("=== seed 汇总 ===")
	fmt.Printf("模型总数: %d\n", len(models))
	if dryRun {
		fmt.Printf("预览模型数: %d（未写库）\n", modelsHit)
		return nil
	}
	fmt.Printf("新增实例: %d\n", inserted)
	fmt.Printf("跳过(已存在): %d\n", skipped)
	fmt.Printf("失败: %d\n", failed)
	fmt.Printf("新增的模型数: %d / 全部跳过的模型数: %d\n", modelsHit, modelsSkip)
	return nil
}
