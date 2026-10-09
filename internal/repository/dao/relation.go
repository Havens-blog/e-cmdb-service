package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const InstanceRelationCollection = "ecam_instance_relation"

// InstanceRelation DAO层实例关系模型
type InstanceRelation struct {
	ID               int64  `bson:"id"`
	SourceInstanceID int64  `bson:"source_instance_id"`
	TargetInstanceID int64  `bson:"target_instance_id"`
	RelationTypeUID  string `bson:"relation_type_uid"`
	TenantID         int64  `bson:"tenant_id"`
	Ctime            int64  `bson:"ctime"`
}

// InstanceRelationFilter DAO层关系过滤条件
type InstanceRelationFilter struct {
	SourceInstanceID int64
	TargetInstanceID int64
	RelationTypeUID  string
	TenantID         int64
	Offset           int64
	Limit            int64
}

// InstanceRelationDAO 实例关系数据访问接口
type InstanceRelationDAO interface {
	Create(ctx context.Context, relation InstanceRelation) (int64, error)
	CreateBatch(ctx context.Context, relations []InstanceRelation) (int64, error)
	GetByID(ctx context.Context, id int64) (InstanceRelation, error)
	List(ctx context.Context, filter InstanceRelationFilter) ([]InstanceRelation, error)
	Count(ctx context.Context, filter InstanceRelationFilter) (int64, error)
	Delete(ctx context.Context, id int64) error
	DeleteByInstanceID(ctx context.Context, instanceID int64) error
	Exists(ctx context.Context, sourceID, targetID int64, relationTypeUID string) (bool, error)
}

type instanceRelationDAO struct {
	db *mongox.Mongo
}

// NewInstanceRelationDAO 创建实例关系DAO
func NewInstanceRelationDAO(db *mongox.Mongo) InstanceRelationDAO {
	return &instanceRelationDAO{db: db}
}

// Create 创建关系
func (d *instanceRelationDAO) Create(ctx context.Context, relation InstanceRelation) (int64, error) {
	now := time.Now().UnixMilli()
	relation.Ctime = now

	if relation.ID == 0 {
		relation.ID = d.db.GetIdGenerator(InstanceRelationCollection)
	}

	_, err := d.db.Collection(InstanceRelationCollection).InsertOne(ctx, relation)
	if err != nil {
		return 0, err
	}

	return relation.ID, nil
}

// CreateBatch 批量创建关系
func (d *instanceRelationDAO) CreateBatch(ctx context.Context, relations []InstanceRelation) (int64, error) {
	if len(relations) == 0 {
		return 0, nil
	}

	now := time.Now().UnixMilli()
	docs := make([]interface{}, len(relations))

	for i := range relations {
		if relations[i].ID == 0 {
			relations[i].ID = d.db.GetIdGenerator(InstanceRelationCollection)
		}
		relations[i].Ctime = now
		docs[i] = relations[i]
	}

	result, err := d.db.Collection(InstanceRelationCollection).InsertMany(ctx, docs)
	if err != nil {
		return 0, err
	}

	return int64(len(result.InsertedIDs)), nil
}

// GetByID 根据ID获取关系
func (d *instanceRelationDAO) GetByID(ctx context.Context, id int64) (InstanceRelation, error) {
	var relation InstanceRelation
	filter := bson.M{"id": id}

	err := d.db.Collection(InstanceRelationCollection).FindOne(ctx, filter).Decode(&relation)
	return relation, err
}

// List 获取关系列表
func (d *instanceRelationDAO) List(ctx context.Context, filter InstanceRelationFilter) ([]InstanceRelation, error) {
	query := d.buildQuery(filter)

	opts := options.Find()
	if filter.Offset > 0 {
		opts.SetSkip(filter.Offset)
	}
	if filter.Limit > 0 {
		opts.SetLimit(filter.Limit)
	}
	opts.SetSort(bson.M{"ctime": -1})

	cursor, err := d.db.Collection(InstanceRelationCollection).Find(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var relations []InstanceRelation
	err = cursor.All(ctx, &relations)
	return relations, err
}

// Count 统计关系数量
func (d *instanceRelationDAO) Count(ctx context.Context, filter InstanceRelationFilter) (int64, error) {
	query := d.buildQuery(filter)
	return d.db.Collection(InstanceRelationCollection).CountDocuments(ctx, query)
}

// Delete 删除关系
func (d *instanceRelationDAO) Delete(ctx context.Context, id int64) error {
	filter := bson.M{"id": id}
	result, err := d.db.Collection(InstanceRelationCollection).DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// DeleteByInstanceID 删除与指定实例相关的所有关系
func (d *instanceRelationDAO) DeleteByInstanceID(ctx context.Context, instanceID int64) error {
	filter := bson.M{
		"$or": []bson.M{
			{"source_instance_id": instanceID},
			{"target_instance_id": instanceID},
		},
	}
	_, err := d.db.Collection(InstanceRelationCollection).DeleteMany(ctx, filter)
	return err
}

// Exists 检查关系是否存在
func (d *instanceRelationDAO) Exists(ctx context.Context, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	filter := bson.M{
		"source_instance_id": sourceID,
		"target_instance_id": targetID,
		"relation_type_uid":  relationTypeUID,
	}

	count, err := d.db.Collection(InstanceRelationCollection).CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// buildQuery 构建查询条件
func (d *instanceRelationDAO) buildQuery(filter InstanceRelationFilter) bson.M {
	query := bson.M{}

	if filter.SourceInstanceID > 0 {
		query["source_instance_id"] = filter.SourceInstanceID
	}
	if filter.TargetInstanceID > 0 {
		query["target_instance_id"] = filter.TargetInstanceID
	}
	if filter.RelationTypeUID != "" {
		query["relation_type_uid"] = filter.RelationTypeUID
	}
	if filter.TenantID != 0 {
		query["tenant_id"] = filter.TenantID
	}

	return query
}
