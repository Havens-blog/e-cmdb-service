package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// SyncQualityIssueRepository 同步质量问题仓储接口。
type SyncQualityIssueRepository interface {
	// UpsertByKey 按自然键去重写入，返回记录 ID。
	UpsertByKey(ctx context.Context, issue domain.SyncQualityIssue) (int64, error)
	List(ctx context.Context, filter domain.SyncQualityIssueFilter) ([]domain.SyncQualityIssue, int64, error)
	// DeleteByAssetID 删除某实例的全部问题记录（实例删除时级联清理）。
	DeleteByAssetID(ctx context.Context, tenantID int64, assetID string) error
}

type syncQualityIssueRepository struct {
	dao dao.SyncQualityIssueDAO
}

// NewSyncQualityIssueRepository 创建同步质量问题仓储。
func NewSyncQualityIssueRepository(d dao.SyncQualityIssueDAO) SyncQualityIssueRepository {
	return &syncQualityIssueRepository{dao: d}
}

func (r *syncQualityIssueRepository) UpsertByKey(ctx context.Context, issue domain.SyncQualityIssue) (int64, error) {
	return r.dao.UpsertByKey(ctx, r.toDAO(issue))
}

func (r *syncQualityIssueRepository) DeleteByAssetID(ctx context.Context, tenantID int64, assetID string) error {
	return r.dao.DeleteByAssetID(ctx, tenantID, assetID)
}

func (r *syncQualityIssueRepository) List(ctx context.Context, filter domain.SyncQualityIssueFilter) ([]domain.SyncQualityIssue, int64, error) {
	rows, total, err := r.dao.List(ctx, dao.SyncQualityIssueFilter{
		TenantID:  filter.TenantID,
		ModelUID:  filter.ModelUID,
		IssueType: filter.IssueType,
		AssetID:   filter.AssetID,
		Offset:    filter.Offset,
		Limit:     filter.Limit,
	})
	if err != nil {
		return nil, 0, err
	}

	result := make([]domain.SyncQualityIssue, len(rows))
	for i, row := range rows {
		result[i] = domain.SyncQualityIssue{
			ID:         row.ID,
			TenantID:   row.TenantID,
			ModelUID:   row.ModelUID,
			AssetID:    row.AssetID,
			AssetName:  row.AssetName,
			IssueType:  row.IssueType,
			FieldUID:   row.FieldUID,
			Message:    row.Message,
			LastSeen:   row.LastSeen,
			CreateTime: time.UnixMilli(row.Ctime),
		}
	}
	return result, total, nil
}

func (r *syncQualityIssueRepository) toDAO(issue domain.SyncQualityIssue) dao.SyncQualityIssue {
	return dao.SyncQualityIssue{
		ID:        issue.ID,
		TenantID:  issue.TenantID,
		ModelUID:  issue.ModelUID,
		AssetID:   issue.AssetID,
		AssetName: issue.AssetName,
		IssueType: issue.IssueType,
		FieldUID:  issue.FieldUID,
		Message:   issue.Message,
		LastSeen:  issue.LastSeen,
	}
}
