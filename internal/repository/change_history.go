package repository

import (
	"context"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// ChangeHistoryRepository 变更记录仓储接口（仅写，查询走 e-cam 侧）。
type ChangeHistoryRepository interface {
	// BatchCreate 批量写入字段变更记录。
	BatchCreate(ctx context.Context, records []domain.ChangeRecord) error
}

type changeHistoryRepository struct {
	dao dao.ChangeHistoryDAO
}

// NewChangeHistoryRepository 创建变更记录仓储。
func NewChangeHistoryRepository(d dao.ChangeHistoryDAO) ChangeHistoryRepository {
	return &changeHistoryRepository{dao: d}
}

func (r *changeHistoryRepository) BatchCreate(ctx context.Context, records []domain.ChangeRecord) error {
	daos := make([]dao.ChangeRecord, len(records))
	for i, rec := range records {
		daos[i] = dao.ChangeRecord{
			ID:           rec.ID,
			AssetID:      rec.AssetID,
			AssetName:    rec.AssetName,
			ModelUID:     rec.ModelUID,
			TenantID:     rec.TenantID,
			AccountID:    rec.AccountID,
			Provider:     rec.Provider,
			Region:       rec.Region,
			FieldName:    rec.FieldName,
			OldValue:     rec.OldValue,
			NewValue:     rec.NewValue,
			ChangeSource: rec.ChangeSource,
			ChangeTaskID: rec.ChangeTaskID,
			Operator:     rec.Operator,
			Ctime:        rec.Ctime,
		}
	}
	return r.dao.BatchCreate(ctx, daos)
}
