package dao

import (
	"context"
	"time"

	shareddomain "github.com/Havens-blog/e-cloudx-sdk/domain"
	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const InstanceCollection = "ecam_instance"

// Instance DAO层资产实例模型
type Instance struct {
	ID         int64                  `bson:"id"`
	ModelUID   string                 `bson:"model_uid"`
	AssetID    string                 `bson:"asset_id"`
	AssetName  string                 `bson:"asset_name"`
	TenantID   int64                  `bson:"tenant_id"`
	AccountID  int64                  `bson:"account_id"`
	Attributes map[string]interface{} `bson:"attributes"`
	Ctime      int64                  `bson:"ctime"`
	Utime      int64                  `bson:"utime"`
}

// InstanceFilter DAO层过滤条件
type InstanceFilter struct {
	ModelUID   string
	TenantID   int64
	AccountID  int64
	AssetName  string
	Attributes map[string]interface{}
	Offset     int64
	Limit      int64
}

// AssetStatsResult 资产统计结果
type AssetStatsResult struct {
	Total       int64            `bson:"total"`
	ByAssetType []AssetTypeCount `bson:"by_asset_type"`
	ByProvider  []ProviderCount  `bson:"by_provider"`
}

// AssetTypeCount 按资产类型统计
type AssetTypeCount struct {
	AssetType string `bson:"_id"`
	Count     int64  `bson:"count"`
}

// ProviderCount 按云厂商统计
type ProviderCount struct {
	Provider string `bson:"_id"`
	Count    int64  `bson:"count"`
}

// InstanceDAO 资产实例数据访问接口
type InstanceDAO interface {
	Create(ctx context.Context, instance Instance) (int64, error)
	CreateBatch(ctx context.Context, instances []Instance) (int64, error)
	Update(ctx context.Context, instance Instance) error
	GetByID(ctx context.Context, id int64) (Instance, error)
	GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (Instance, error)
	List(ctx context.Context, filter InstanceFilter) ([]Instance, error)
	ListByIDs(ctx context.Context, ids []int64) ([]Instance, error)
	Count(ctx context.Context, filter InstanceFilter) (int64, error)
	Delete(ctx context.Context, id int64) error
	DeleteByAccountID(ctx context.Context, accountID int64) error
	Upsert(ctx context.Context, instance Instance) error
	// ListUnbound 查询未绑定到任何服务树节点的资产 (通过 $lookup 排除已有 binding 的)
	ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]Instance, error)
	// CountUnbound 统计未绑定资产数量
	CountUnbound(ctx context.Context, tenantID int64) (int64, error)
	// AggregateStatsByIDs 根据资源ID列表聚合统计（高性能）
	AggregateStatsByIDs(ctx context.Context, ids []int64) (*AssetStatsResult, error)
	// AggregateAllStats 聚合统计全部资产
	AggregateAllStats(ctx context.Context, tenantID int64) (*AssetStatsResult, error)
	// AggregateUnboundStats 聚合统计未绑定资产
	AggregateUnboundStats(ctx context.Context, tenantID int64) (*AssetStatsResult, error)
}

type instanceDAO struct {
	db *mongox.Mongo
}

// NewInstanceDAO 创建实例DAO
func NewInstanceDAO(db *mongox.Mongo) InstanceDAO {
	return &instanceDAO{db: db}
}

// Create 创建单个实例
func (d *instanceDAO) Create(ctx context.Context, instance Instance) (int64, error) {
	now := time.Now().UnixMilli()
	instance.Ctime = now
	instance.Utime = now

	if instance.ID == 0 {
		instance.ID = d.db.GetIdGenerator(InstanceCollection)
	}

	_, err := d.db.Collection(InstanceCollection).InsertOne(ctx, instance)
	if err != nil {
		return 0, err
	}

	return instance.ID, nil
}

// CreateBatch 批量创建实例
func (d *instanceDAO) CreateBatch(ctx context.Context, instances []Instance) (int64, error) {
	if len(instances) == 0 {
		return 0, nil
	}

	now := time.Now().UnixMilli()
	docs := make([]interface{}, len(instances))

	for i := range instances {
		if instances[i].ID == 0 {
			instances[i].ID = d.db.GetIdGenerator(InstanceCollection)
		}
		instances[i].Ctime = now
		instances[i].Utime = now
		docs[i] = instances[i]
	}

	result, err := d.db.Collection(InstanceCollection).InsertMany(ctx, docs)
	if err != nil {
		return 0, err
	}

	return int64(len(result.InsertedIDs)), nil
}

// Update 更新实例
func (d *instanceDAO) Update(ctx context.Context, instance Instance) error {
	now := time.Now().UnixMilli()

	filter := bson.M{"id": instance.ID}
	update := bson.M{"$set": bson.M{
		"asset_name": instance.AssetName,
		"model_uid":  instance.ModelUID,
		"asset_id":   instance.AssetID,
		"tenant_id":  instance.TenantID,
		"account_id": instance.AccountID,
		"attributes": instance.Attributes,
		"utime":      now,
	}}

	result, err := d.db.Collection(InstanceCollection).UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// GetByID 根据ID获取实例
func (d *instanceDAO) GetByID(ctx context.Context, id int64) (Instance, error) {
	var instance Instance
	filter := bson.M{"id": id}

	err := d.db.Collection(InstanceCollection).FindOne(ctx, filter).Decode(&instance)
	return instance, err
}

// GetByAssetID 根据云厂商资产ID获取实例
func (d *instanceDAO) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (Instance, error) {
	var instance Instance
	filter := bson.M{
		"tenant_id": tenantID,
		"model_uid": modelUID,
		"asset_id":  assetID,
	}

	err := d.db.Collection(InstanceCollection).FindOne(ctx, filter).Decode(&instance)
	return instance, err
}

// ListByIDs 根据ID列表批量查询实例
func (d *instanceDAO) ListByIDs(ctx context.Context, ids []int64) ([]Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	filter := bson.M{"id": bson.M{"$in": ids}}
	cursor, err := d.db.Collection(InstanceCollection).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var instances []Instance
	return instances, cursor.All(ctx, &instances)
}

// List 获取实例列表
func (d *instanceDAO) List(ctx context.Context, filter InstanceFilter) ([]Instance, error) {
	query := d.buildQuery(filter)

	opts := options.Find()
	if filter.Offset > 0 {
		opts.SetSkip(filter.Offset)
	}
	if filter.Limit > 0 {
		opts.SetLimit(filter.Limit)
	}
	opts.SetSort(bson.M{"ctime": -1})

	cursor, err := d.db.Collection(InstanceCollection).Find(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var instances []Instance
	err = cursor.All(ctx, &instances)
	return instances, err
}

// Count 统计实例数量
func (d *instanceDAO) Count(ctx context.Context, filter InstanceFilter) (int64, error) {
	query := d.buildQuery(filter)
	return d.db.Collection(InstanceCollection).CountDocuments(ctx, query)
}

// Delete 删除实例
func (d *instanceDAO) Delete(ctx context.Context, id int64) error {
	filter := bson.M{"id": id}
	_, err := d.db.Collection(InstanceCollection).DeleteOne(ctx, filter)
	return err
}

// DeleteByAccountID 删除指定云账号的所有实例
func (d *instanceDAO) DeleteByAccountID(ctx context.Context, accountID int64) error {
	filter := bson.M{"account_id": accountID}
	_, err := d.db.Collection(InstanceCollection).DeleteMany(ctx, filter)
	return err
}

// Upsert 更新或插入实例 (根据 tenant_id + model_uid + asset_id 判断)
func (d *instanceDAO) Upsert(ctx context.Context, instance Instance) error {
	now := time.Now().UnixMilli()
	instance.Utime = now

	filter := bson.M{
		"tenant_id": instance.TenantID,
		"model_uid": instance.ModelUID,
		"asset_id":  instance.AssetID,
	}

	update := bson.M{
		"$set": bson.M{
			"asset_name": instance.AssetName,
			"account_id": instance.AccountID,
			"attributes": instance.Attributes,
			"utime":      now,
		},
		"$setOnInsert": bson.M{
			"id":    d.db.GetIdGenerator(InstanceCollection),
			"ctime": now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := d.db.Collection(InstanceCollection).UpdateOne(ctx, filter, update, opts)
	return err
}

// buildQuery 构建查询条件
func (d *instanceDAO) buildQuery(filter InstanceFilter) bson.M {
	query := bson.M{}

	if filter.ModelUID != "" {
		query["model_uid"] = filter.ModelUID
	}
	if filter.TenantID != 0 {
		query["tenant_id"] = filter.TenantID
	}
	if filter.AccountID > 0 {
		query["account_id"] = filter.AccountID
	}
	if filter.AssetName != "" {
		query["asset_name"] = bson.M{"$regex": filter.AssetName, "$options": "i"}
	}

	// 动态属性查询
	for key, value := range filter.Attributes {
		query["attributes."+key] = value
	}

	return query
}

// unboundPipeline 构建查询未绑定资产的聚合管道
// 思路: c_instance LEFT JOIN c_resource_binding，保留 binding 为空的记录
func (d *instanceDAO) unboundPipeline(tenantID int64) mongo.Pipeline {
	return mongo.Pipeline{
		// 1. 按租户过滤
		{{Key: "$match", Value: bson.M{"tenant_id": tenantID}}},
		// 2. LEFT JOIN binding 表: 用 instance.id 关联 binding.resource_id
		{{Key: "$lookup", Value: bson.M{
			"from": "c_resource_binding",
			"let":  bson.M{"inst_id": "$id", "tenant": "$tenant_id"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"$expr": bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$resource_id", "$$inst_id"}},
					bson.M{"$eq": bson.A{"$tenant_id", "$$tenant"}},
					bson.M{"$eq": bson.A{"$resource_type", "instance"}},
				}}}}},
				// 只需要知道有没有，取1条就够
				{{Key: "$limit", Value: 1}},
				{{Key: "$project", Value: bson.M{"_id": 1}}},
			},
			"as": "_bindings",
		}}},
		// 3. 只保留没有 binding 的
		{{Key: "$match", Value: bson.M{"_bindings": bson.M{"$size": 0}}}},
		// 4. 去掉临时字段
		{{Key: "$project", Value: bson.M{"_bindings": 0}}},
	}
}

// ListUnbound 查询未绑定到任何服务树节点的资产
func (d *instanceDAO) ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]Instance, error) {
	pipeline := d.unboundPipeline(tenantID)

	// 排序
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.M{"ctime": -1}}})
	// 分页
	if offset > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$skip", Value: offset}})
	}
	if limit > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: limit}})
	}

	cursor, err := d.db.Collection(InstanceCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var instances []Instance
	err = cursor.All(ctx, &instances)
	return instances, err
}

// CountUnbound 统计未绑定资产数量
func (d *instanceDAO) CountUnbound(ctx context.Context, tenantID int64) (int64, error) {
	pipeline := d.unboundPipeline(tenantID)
	pipeline = append(pipeline, bson.D{{Key: "$count", Value: "total"}})

	cursor, err := d.db.Collection(InstanceCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result []struct {
		Total int64 `bson:"total"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}
	return result[0].Total, nil
}

// AggregateStatsByIDs 根据资源ID列表聚合统计（高性能，单次聚合）
func (d *instanceDAO) AggregateStatsByIDs(ctx context.Context, ids []int64) (*AssetStatsResult, error) {
	if len(ids) == 0 {
		return &AssetStatsResult{ByAssetType: []AssetTypeCount{}, ByProvider: []ProviderCount{}}, nil
	}

	// 使用 $facet 一次聚合获取所有统计
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"id": bson.M{"$in": ids}}}},
		{{Key: "$facet", Value: bson.M{
			"total": bson.A{
				bson.M{"$count": "count"},
			},
			"by_asset_type": bson.A{
				bson.M{"$group": bson.M{"_id": "$model_uid", "count": bson.M{"$sum": 1}}},
			},
			"by_provider": bson.A{
				bson.M{"$match": bson.M{"attributes.provider": bson.M{"$ne": nil}}},
				bson.M{"$group": bson.M{"_id": "$attributes.provider", "count": bson.M{"$sum": 1}}},
			},
		}}},
	}

	cursor, err := d.db.Collection(InstanceCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		Total []struct {
			Count int64 `bson:"count"`
		} `bson:"total"`
		ByAssetType []AssetTypeCount `bson:"by_asset_type"`
		ByProvider  []ProviderCount  `bson:"by_provider"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return &AssetStatsResult{ByAssetType: []AssetTypeCount{}, ByProvider: []ProviderCount{}}, nil
	}

	r := results[0]
	total := int64(0)
	if len(r.Total) > 0 {
		total = r.Total[0].Count
	}

	return &AssetStatsResult{
		Total:       total,
		ByAssetType: r.ByAssetType,
		ByProvider:  r.ByProvider,
	}, nil
}

// AggregateAllStats 聚合统计全部资产
func (d *instanceDAO) AggregateAllStats(ctx context.Context, tenantID int64) (*AssetStatsResult, error) {
	match := bson.M{}
	// 租户过滤恒定生效：0 不作通配。未选定租户时本查询返回空集，
	// 而不是退化为「不加过滤」从而返回全部租户数据。
	match["tenant_id"] = tenantID
	// 附属资源(网卡/子网/安全组/云盘/快照/镜像)不作独立实例统计,
	// 与仪表盘资产总数口径一致(见 domain.AttachmentAssetTypes)
	match["model_uid"] = bson.M{
		"$not": bson.M{"$regex": shareddomain.AttachmentModelUIDPattern()},
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$facet", Value: bson.M{
			"total": bson.A{
				bson.M{"$count": "count"},
			},
			"by_asset_type": bson.A{
				bson.M{"$group": bson.M{"_id": "$model_uid", "count": bson.M{"$sum": 1}}},
			},
			"by_provider": bson.A{
				bson.M{"$match": bson.M{"attributes.provider": bson.M{"$ne": nil}}},
				bson.M{"$group": bson.M{"_id": "$attributes.provider", "count": bson.M{"$sum": 1}}},
			},
		}}},
	}

	cursor, err := d.db.Collection(InstanceCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		Total []struct {
			Count int64 `bson:"count"`
		} `bson:"total"`
		ByAssetType []AssetTypeCount `bson:"by_asset_type"`
		ByProvider  []ProviderCount  `bson:"by_provider"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return &AssetStatsResult{ByAssetType: []AssetTypeCount{}, ByProvider: []ProviderCount{}}, nil
	}

	r := results[0]
	total := int64(0)
	if len(r.Total) > 0 {
		total = r.Total[0].Count
	}

	return &AssetStatsResult{
		Total:       total,
		ByAssetType: r.ByAssetType,
		ByProvider:  r.ByProvider,
	}, nil
}

// AggregateUnboundStats 聚合统计未绑定资产
func (d *instanceDAO) AggregateUnboundStats(ctx context.Context, tenantID int64) (*AssetStatsResult, error) {
	// 基于未绑定 pipeline，追加统计阶段
	pipeline := d.unboundPipeline(tenantID)
	pipeline = append(pipeline, bson.D{{Key: "$facet", Value: bson.M{
		"total": bson.A{
			bson.M{"$count": "count"},
		},
		"by_asset_type": bson.A{
			bson.M{"$group": bson.M{"_id": "$model_uid", "count": bson.M{"$sum": 1}}},
		},
		"by_provider": bson.A{
			bson.M{"$match": bson.M{"attributes.provider": bson.M{"$ne": nil}}},
			bson.M{"$group": bson.M{"_id": "$attributes.provider", "count": bson.M{"$sum": 1}}},
		},
	}}})

	cursor, err := d.db.Collection(InstanceCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		Total []struct {
			Count int64 `bson:"count"`
		} `bson:"total"`
		ByAssetType []AssetTypeCount `bson:"by_asset_type"`
		ByProvider  []ProviderCount  `bson:"by_provider"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return &AssetStatsResult{ByAssetType: []AssetTypeCount{}, ByProvider: []ProviderCount{}}, nil
	}

	r := results[0]
	total := int64(0)
	if len(r.Total) > 0 {
		total = r.Total[0].Count
	}

	return &AssetStatsResult{
		Total:       total,
		ByAssetType: r.ByAssetType,
		ByProvider:  r.ByProvider,
	}, nil
}
