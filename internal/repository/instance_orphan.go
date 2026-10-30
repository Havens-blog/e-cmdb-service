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

// OrphanModelGroup 按缺失模型聚合的孤儿实例数（数据质量页「孤儿来源」用）。
type OrphanModelGroup struct {
	ModelUID string
	Count    int64
}

// GroupOrphanInstancesByModel 按 model_uid 聚合孤儿实例数量，按数量降序。
func (r *instanceRepository) GroupOrphanInstancesByModel(
	ctx context.Context,
	tenantID int64,
	knownModelUIDs []string,
) ([]OrphanModelGroup, error) {
	rows, err := r.dao.GroupOrphanInstancesByModel(ctx, tenantID, knownModelUIDs)
	if err != nil {
		return nil, err
	}
	out := make([]OrphanModelGroup, len(rows))
	for i, row := range rows {
		out[i] = OrphanModelGroup{ModelUID: row.ModelUID, Count: row.Count}
	}
	return out, nil
}
