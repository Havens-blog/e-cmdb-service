package dao

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MissingFieldInstance 缺字段实例的最小字段视图（数据质量明细用）。
type MissingFieldInstance struct {
	ID        int64  `bson:"id"`
	AssetID   string `bson:"asset_id"`
	AssetName string `bson:"asset_name"`
	ModelUID  string `bson:"model_uid"`
}

// missingFieldQuery 构造「某模型缺少指定属性」的查询。
func missingFieldQuery(tenantID int64, modelUID, fieldUID string) bson.M {
	return bson.M{
		"tenant_id":              tenantID,
		"model_uid":              modelUID,
		"attributes." + fieldUID: bson.M{"$exists": false},
	}
}

// CountMissingAttributeForModel 统计某 (模型, 字段) 组合下缺该属性的实例数。
// 命名与既有 CountMissingAttribute 区分：后者已存在，此处显式按组合计数。
func (d *instanceDAO) CountMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	if modelUID == "" || fieldUID == "" {
		return 0, nil
	}
	return d.db.Collection(InstanceCollection).CountDocuments(ctx, missingFieldQuery(tenantID, modelUID, fieldUID))
}

// ListMissingFieldGroup 分页拉取某 (模型, 字段) 组合下缺该属性的实例。
//
// 排序固定按 id 升序，保证同一页多次请求结果稳定。
func (d *instanceDAO) ListMissingFieldGroup(
	ctx context.Context,
	tenantID int64,
	modelUID, fieldUID string,
	offset, limit int64,
) ([]MissingFieldInstance, error) {
	if modelUID == "" || fieldUID == "" || limit <= 0 {
		return nil, nil
	}
	if offset < 0 {
		offset = 0
	}

	opts := options.Find().
		SetProjection(bson.M{"id": 1, "asset_id": 1, "asset_name": 1, "model_uid": 1}).
		SetSort(bson.M{"id": 1}).
		SetSkip(offset).
		SetLimit(limit)

	cursor, err := d.db.Collection(InstanceCollection).Find(ctx, missingFieldQuery(tenantID, modelUID, fieldUID), opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []MissingFieldInstance
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
