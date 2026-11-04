package dao

import (
	"context"

	"github.com/Havens-blog/e-common-go/mongox"
)

// ChangeHistoryCollection 与 e-cam-service 共享的变更记录集合名。
const ChangeHistoryCollection = "ecam_change_history"

// ChangeRecord 变更记录 DAO 模型（与 e-cam 的 domain.ChangeRecord 同构）。
type ChangeRecord struct {
	ID           int64  `bson:"id"`
	AssetID      string `bson:"asset_id"`
	AssetName    string `bson:"asset_name"`
	ModelUID     string `bson:"model_uid"`
	TenantID     int64  `bson:"tenant_id"`
	AccountID    int64  `bson:"account_id"`
	Provider     string `bson:"provider"`
	Region       string `bson:"region"`
	FieldName    string `bson:"field_name"`
	OldValue     string `bson:"old_value"`
	NewValue     string `bson:"new_value"`
	ChangeSource string `bson:"change_source"`
	ChangeTaskID string `bson:"change_task_id"`
	Operator     string `bson:"operator,omitempty"`
	Ctime        int64  `bson:"ctime"`
}

// ChangeHistoryDAO 变更记录写入接口（仅写，查询走 e-cam 侧）。
type ChangeHistoryDAO interface {
	// BatchCreate 批量插入变更记录。
	BatchCreate(ctx context.Context, records []ChangeRecord) error
}

type changeHistoryDAO struct {
	db *mongox.Mongo
}

// NewChangeHistoryDAO 创建变更记录 DAO。
func NewChangeHistoryDAO(db *mongox.Mongo) ChangeHistoryDAO {
	return &changeHistoryDAO{db: db}
}

// BatchCreate 批量插入变更记录（与 e-cam 的 BatchCreate 同构）。
func (d *changeHistoryDAO) BatchCreate(ctx context.Context, records []ChangeRecord) error {
	if len(records) == 0 {
		return nil
	}
	docs := make([]interface{}, len(records))
	for i, r := range records {
		if r.ID == 0 {
			r.ID = d.db.GetIdGenerator(ChangeHistoryCollection)
		}
		docs[i] = r
	}
	_, err := d.db.Collection(ChangeHistoryCollection).InsertMany(ctx, docs)
	if err != nil {
		// 幂等性说明：InsertMany 偶发重复（如超时重试）可能写入重复记录，
		// 但与 e-cam 现状一致（无唯一键去重），且审计记录重复无害（可据 ctime 排查）。
		return err
	}
	return nil
}
