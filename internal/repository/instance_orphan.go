package repository

import (
	"context"

	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// OrphanInstanceView 孤儿实例的只读视图（数据质量明细用）。
type OrphanInstanceView struct {
	ID        int64
	AssetID   string
	AssetName string
	ModelUID  string
}

// CountOrphanInstances 统计孤儿实例数量（model_uid 不在已知模型名单）。
func (r *instanceRepository) CountOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string) (int64, error) {
	return r.dao.CountOrphanInstances(ctx, tenantID, knownModelUIDs)
}

// ListOrphanInstances 分页拉取孤儿实例。
func (r *instanceRepository) ListOrphanInstances(
	ctx context.Context,
	tenantID int64,
	knownModelUIDs []string,
	offset, limit int64,
) ([]OrphanInstanceView, int64, error) {
	rows, total, err := r.dao.ListOrphanInstances(ctx, tenantID, knownModelUIDs, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]OrphanInstanceView, len(rows))
	for i, row := range rows {
		out[i] = toOrphanView(row)
	}
	return out, total, nil
}

func toOrphanView(r dao.OrphanInstance) OrphanInstanceView {
	return OrphanInstanceView{
		ID:        r.ID,
		AssetID:   r.AssetID,
		AssetName: r.AssetName,
		ModelUID:  r.ModelUID,
	}
}
