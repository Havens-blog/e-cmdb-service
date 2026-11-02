package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const SyncQualityIssueCollection = "ecam_sync_quality_issue"

// SyncQualityIssue 同步质量问题 DAO 模型（字段与 domain.SyncQualityIssue 对齐）。
type SyncQualityIssue struct {
	ID        int64  `bson:"id"`
	TenantID  int64  `bson:"tenant_id"`
	ModelUID  string `bson:"model_uid"`
	AssetID   string `bson:"asset_id"`
	AssetName string `bson:"asset_name"`
	IssueType string `bson:"issue_type"`
	FieldUID  string `bson:"field_uid"`
	Message   string `bson:"message"`
	LastSeen  int64  `bson:"last_seen"` // UnixMilli
	Ctime     int64  `bson:"ctime"`     // UnixMilli
}

// SyncQualityIssueFilter DAO 层过滤
type SyncQualityIssueFilter struct {
	TenantID  int64
	ModelUID  string
	IssueType string
	AssetID   string
	Offset    int64
	Limit     int64
}

// SyncQualityIssueDAO 同步质量问题数据访问接口。
type SyncQualityIssueDAO interface {
	// UpsertByKey 按自然键去重写入：已存在则刷新 last_seen/message/asset_name，
	// 不存在则插入。返回该记录 ID。
	UpsertByKey(ctx context.Context, issue SyncQualityIssue) (int64, error)
	List(ctx context.Context, filter SyncQualityIssueFilter) ([]SyncQualityIssue, int64, error)
	// DeleteByAssetID 删除某实例的全部问题记录（实例删除时级联清理）。
	DeleteByAssetID(ctx context.Context, tenantID int64, assetID string) error
	InitIndexes(ctx context.Context) error
}

type syncQualityIssueDAO struct {
	db *mongox.Mongo
}

// NewSyncQualityIssueDAO 创建同步质量问题 DAO。
func NewSyncQualityIssueDAO(db *mongox.Mongo) SyncQualityIssueDAO {
	return &syncQualityIssueDAO{db: db}
}

// naturalKey 去重键：同一租户下、同一实例、同一字段、同一类问题只留一条。
func syncQualityNaturalKey(issue SyncQualityIssue) bson.M {
	return bson.M{
		"tenant_id":  issue.TenantID,
		"model_uid":  issue.ModelUID,
		"asset_id":   issue.AssetID,
		"issue_type": issue.IssueType,
		"field_uid":  issue.FieldUID,
	}
}

// UpsertByKey 按自然键去重写入。
//
// 先查后写（而非直接 upsert），原因：直接 upsert 的 $setOnInsert.id 需要一个
// 预先分配的自增 ID，而每次调用 GetIdGenerator 都会消耗 ID，即使最终只做了更新。
// 问题记录通常是少量坏数据，先查后写的两次往返成本可忽略。
func (d *syncQualityIssueDAO) UpsertByKey(ctx context.Context, issue SyncQualityIssue) (int64, error) {
	now := time.Now().UnixMilli()
	key := syncQualityNaturalKey(issue)

	var existing SyncQualityIssue
	err := d.db.Collection(SyncQualityIssueCollection).FindOne(ctx, key).Decode(&existing)
	if err == nil {
		// 已存在：只刷新 last_seen / message / asset_name
		_, uerr := d.db.Collection(SyncQualityIssueCollection).UpdateOne(ctx, key, bson.M{
			"$set": bson.M{
				"last_seen":  now,
				"message":    issue.Message,
				"asset_name": issue.AssetName,
			},
		})
		if uerr != nil {
			return 0, uerr
		}
		return existing.ID, nil
	}
	if err != mongo.ErrNoDocuments {
		return 0, err
	}

	// 不存在：分配 ID 插入
	issue.ID = d.db.GetIdGenerator(SyncQualityIssueCollection)
	issue.LastSeen = now
	issue.Ctime = now
	if _, err := d.db.Collection(SyncQualityIssueCollection).InsertOne(ctx, issue); err != nil {
		return 0, err
	}
	return issue.ID, nil
}

// DeleteByAssetID 删除某实例的全部问题记录（实例删除时级联清理，避免悬空问题）。
func (d *syncQualityIssueDAO) DeleteByAssetID(ctx context.Context, tenantID int64, assetID string) error {
	_, err := d.db.Collection(SyncQualityIssueCollection).DeleteMany(ctx, bson.M{
		"tenant_id": tenantID,
		"asset_id":  assetID,
	})
	return err
}

// InitIndexes 建自然键唯一索引（防并发重复插入）。
func (d *syncQualityIssueDAO) InitIndexes(ctx context.Context) error {
	_, err := d.db.Collection(SyncQualityIssueCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "tenant_id", Value: 1},
			{Key: "model_uid", Value: 1},
			{Key: "asset_id", Value: 1},
			{Key: "issue_type", Value: 1},
			{Key: "field_uid", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	return err
}

// List 分页查询 + 计数。
func (d *syncQualityIssueDAO) List(ctx context.Context, filter SyncQualityIssueFilter) ([]SyncQualityIssue, int64, error) {
	query := bson.M{}
	if filter.TenantID != 0 {
		query["tenant_id"] = filter.TenantID
	}
	if filter.ModelUID != "" {
		query["model_uid"] = filter.ModelUID
	}
	if filter.IssueType != "" {
		query["issue_type"] = filter.IssueType
	}
	if filter.AssetID != "" {
		query["asset_id"] = filter.AssetID
	}

	total, err := d.db.Collection(SyncQualityIssueCollection).CountDocuments(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	opts := options.Find().
		SetSort(bson.M{"last_seen": -1}).
		SetSkip(filter.Offset).
		SetLimit(filter.Limit)
	cursor, err := d.db.Collection(SyncQualityIssueCollection).Find(ctx, query, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var rows []SyncQualityIssue
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
