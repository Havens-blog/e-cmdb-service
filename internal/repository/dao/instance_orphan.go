package dao

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// OrphanInstance 孤儿实例的最小字段视图（数据质量明细用）。
type OrphanInstance struct {
	ID        int64  `bson:"id"`
	AssetID   string `bson:"asset_id"`
	AssetName string `bson:"asset_name"`
	ModelUID  string `bson:"model_uid"`
}

// orphanQuery 构造孤儿实例查询：租户内 model_uid 不在给定模型名单。
//
// 注意：$nin 不会匹配「字段缺失」的文档，因此额外用 $or 覆盖 model_uid
// 为空字符串或字段不存在的情况，保证与 analyzeQuality 的口径一致
// （后者把 model_uid 为空也判为孤儿）。
func orphanQuery(tenantID int64, knownModelUIDs []string) bson.M {
	q := bson.M{"tenant_id": tenantID}
	if len(knownModelUIDs) == 0 {
		// 没有已知模型时，所有实例都是孤儿
		return q
	}
	q["$or"] = bson.A{
		bson.M{"model_uid": bson.M{"$nin": knownModelUIDs}},
		bson.M{"model_uid": ""},
		bson.M{"model_uid": bson.M{"$exists": false}},
	}
	return q
}

// CountOrphanInstances 统计孤儿实例数量（服务端过滤）。
func (d *instanceDAO) CountOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string) (int64, error) {
	return d.db.Collection(InstanceCollection).CountDocuments(ctx, orphanQuery(tenantID, knownModelUIDs))
}

// OrphanGroupCount 按 model_uid 聚合的孤儿实例数。
type OrphanGroupCount struct {
	ModelUID string `bson:"_id"`
	Count    int64  `bson:"count"`
}

// GroupOrphanInstancesByModel 按 model_uid 聚合统计孤儿实例数量。
//
// 用途：数据质量页当前只展示「孤儿实例总数」，用户无法得知这些孤儿
// 究竟是因为哪些模型缺失造成的。实测（tenant 3，2026-10-31）：
// 孤儿 12760 条，来自 58 个未定义的 model_uid，
// 其中 volcengine_disk 1783、volcengine_snapshot 1491、aliyun_disk 1314 等。
// 把总数拆成「按缺失模型分组」后，数字才变得可执行（知道该补哪些模型）。
//
// 复用 orphanQuery，保证与 CountOrphanInstances / ListOrphanInstances 口径一致。
// 按数量降序，便于前端直接展示 TopN。
func (d *instanceDAO) GroupOrphanInstancesByModel(
	ctx context.Context,
	tenantID int64,
	knownModelUIDs []string,
) ([]OrphanGroupCount, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: orphanQuery(tenantID, knownModelUIDs)}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$model_uid"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}
	cur, err := d.db.Collection(InstanceCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []OrphanGroupCount
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListOrphanInstances 分页拉取孤儿实例，供数据质量明细接口使用。
//
// 相比把租户下全部实例拉进内存逐条判定（实测约 5.4s），该查询实测：
// count 约 40ms、首页 20 条约 70ms、skip 1000 约 75ms。
// 排序固定按 id 升序，保证分页稳定、同一页多次请求结果一致。
func (d *instanceDAO) ListOrphanInstances(
	ctx context.Context,
	tenantID int64,
	knownModelUIDs []string,
	offset, limit int64,
) ([]OrphanInstance, int64, error) {
	query := orphanQuery(tenantID, knownModelUIDs)

	total, err := d.db.Collection(InstanceCollection).CountDocuments(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	opts := options.Find().
		SetProjection(bson.M{"id": 1, "asset_id": 1, "asset_name": 1, "model_uid": 1}).
		SetSort(bson.M{"id": 1}).
		SetSkip(offset).
		SetLimit(limit)

	cursor, err := d.db.Collection(InstanceCollection).Find(ctx, query, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var rows []OrphanInstance
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
