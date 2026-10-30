package repository

import (
	"context"

	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// MissingFieldInstanceView 缺字段实例的只读视图（数据质量明细用）。
type MissingFieldInstanceView struct {
	ID        int64
	AssetID   string
	AssetName string
	ModelUID  string
}

// CountMissingFieldGroup 统计某 (模型, 字段) 组合缺该属性的实例数。
func (r *instanceRepository) CountMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	return r.dao.CountMissingFieldGroup(ctx, tenantID, modelUID, fieldUID)
}

// ListMissingFieldGroup 分页拉取某 (模型, 字段) 组合缺该属性的实例。
func (r *instanceRepository) ListMissingFieldGroup(
	ctx context.Context,
	tenantID int64,
	modelUID, fieldUID string,
	offset, limit int64,
) ([]MissingFieldInstanceView, error) {
	rows, err := r.dao.ListMissingFieldGroup(ctx, tenantID, modelUID, fieldUID, offset, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MissingFieldInstanceView, len(rows))
	for i, row := range rows {
		out[i] = toMissingFieldView(row)
	}
	return out, nil
}

func toMissingFieldView(r dao.MissingFieldInstance) MissingFieldInstanceView {
	return MissingFieldInstanceView{
		ID:        r.ID,
		AssetID:   r.AssetID,
		AssetName: r.AssetName,
		ModelUID:  r.ModelUID,
	}
}

// BatchCountMissingFieldGroups 一次聚合算出多个 (模型,字段) 组合的缺失实例数。
// 返回：model_uid -> (field_uid -> 缺失数)，仅含 count > 0 的字段。
func (r *instanceRepository) BatchCountMissingFieldGroups(
	ctx context.Context,
	tenantID int64,
	fieldsByModel map[string][]string,
) (map[string]map[string]int64, error) {
	raw, err := r.dao.BatchCountMissingFieldGroups(ctx, tenantID, fieldsByModel)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]int64, len(raw))
	for modelUID, item := range raw {
		out[modelUID] = item.Counts
	}
	return out, nil
}