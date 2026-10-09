package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ModelRelationTypeCollection = "ecam_model_relation_type"

// ModelRelationType DAO层模型关系类型
type ModelRelationType struct {
	ID             int64  `bson:"id"`
	UID            string `bson:"uid"`
	Name           string `bson:"name"`
	SourceModelUID string `bson:"source_model_uid"`
	TargetModelUID string `bson:"target_model_uid"`
	RelationType   string `bson:"relation_type"`
	Direction      string `bson:"direction"`
	SourceToTarget string `bson:"source_to_target"`
	TargetToSource string `bson:"target_to_source"`
	Description    string `bson:"description"`
	Ctime          int64  `bson:"ctime"`
	Utime          int64  `bson:"utime"`
}

// ModelRelationTypeFilter DAO层过滤条件
type ModelRelationTypeFilter struct {
	SourceModelUID string
	TargetModelUID string
	RelationType   string
	Offset         int
	Limit          int
}

// ModelRelationTypeDAO 模型关系类型数据访问接口
type ModelRelationTypeDAO interface {
	Create(ctx context.Context, rel ModelRelationType) (int64, error)
	GetByUID(ctx context.Context, uid string) (ModelRelationType, error)
	GetByID(ctx context.Context, id int64) (ModelRelationType, error)
	List(ctx context.Context, filter ModelRelationTypeFilter) ([]ModelRelationType, error)
	Count(ctx context.Context, filter ModelRelationTypeFilter) (int64, error)
	Update(ctx context.Context, rel ModelRelationType) error
	Delete(ctx context.Context, uid string) error
	Exists(ctx context.Context, uid string) (bool, error)
	FindByModels(ctx context.Context, sourceUID, targetUID string) ([]ModelRelationType, error)
}

type modelRelationTypeDAO struct {
	db *mongox.Mongo
}

// NewModelRelationTypeDAO 创建模型关系类型DAO
func NewModelRelationTypeDAO(db *mongox.Mongo) ModelRelationTypeDAO {
	return &modelRelationTypeDAO{db: db}
}

func (d *modelRelationTypeDAO) Create(ctx context.Context, rel ModelRelationType) (int64, error) {
	now := time.Now().UnixMilli()
	rel.Ctime = now
	rel.Utime = now

	if rel.ID == 0 {
		rel.ID = d.db.GetIdGenerator(ModelRelationTypeCollection)
	}

	_, err := d.db.Collection(ModelRelationTypeCollection).InsertOne(ctx, rel)
	if err != nil {
		return 0, err
	}
	return rel.ID, nil
}

func (d *modelRelationTypeDAO) GetByUID(ctx context.Context, uid string) (ModelRelationType, error) {
	var rel ModelRelationType
	filter := bson.M{"uid": uid}
	err := d.db.Collection(ModelRelationTypeCollection).FindOne(ctx, filter).Decode(&rel)
	return rel, err
}

func (d *modelRelationTypeDAO) GetByID(ctx context.Context, id int64) (ModelRelationType, error) {
	var rel ModelRelationType
	filter := bson.M{"id": id}
	err := d.db.Collection(ModelRelationTypeCollection).FindOne(ctx, filter).Decode(&rel)
	return rel, err
}

func (d *modelRelationTypeDAO) List(ctx context.Context, filter ModelRelationTypeFilter) ([]ModelRelationType, error) {
	query := d.buildQuery(filter)

	opts := options.Find()
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	opts.SetSort(bson.M{"ctime": -1})

	cursor, err := d.db.Collection(ModelRelationTypeCollection).Find(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rels []ModelRelationType
	err = cursor.All(ctx, &rels)
	return rels, err
}

func (d *modelRelationTypeDAO) Count(ctx context.Context, filter ModelRelationTypeFilter) (int64, error) {
	query := d.buildQuery(filter)
	return d.db.Collection(ModelRelationTypeCollection).CountDocuments(ctx, query)
}

func (d *modelRelationTypeDAO) Update(ctx context.Context, rel ModelRelationType) error {
	rel.Utime = time.Now().UnixMilli()

	filter := bson.M{"uid": rel.UID}
	update := bson.M{
		"$set": bson.M{
			"name":             rel.Name,
			"source_to_target": rel.SourceToTarget,
			"target_to_source": rel.TargetToSource,
			"description":      rel.Description,
			"utime":            rel.Utime,
		},
	}

	result, err := d.db.Collection(ModelRelationTypeCollection).UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (d *modelRelationTypeDAO) Delete(ctx context.Context, uid string) error {
	filter := bson.M{"uid": uid}
	result, err := d.db.Collection(ModelRelationTypeCollection).DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (d *modelRelationTypeDAO) Exists(ctx context.Context, uid string) (bool, error) {
	filter := bson.M{"uid": uid}
	count, err := d.db.Collection(ModelRelationTypeCollection).CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *modelRelationTypeDAO) FindByModels(ctx context.Context, sourceUID, targetUID string) ([]ModelRelationType, error) {
	// 查找两个模型之间的所有关系（双向）
	query := bson.M{
		"$or": []bson.M{
			{"source_model_uid": sourceUID, "target_model_uid": targetUID},
			{"source_model_uid": targetUID, "target_model_uid": sourceUID},
		},
	}

	cursor, err := d.db.Collection(ModelRelationTypeCollection).Find(ctx, query)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rels []ModelRelationType
	err = cursor.All(ctx, &rels)
	return rels, err
}

func (d *modelRelationTypeDAO) buildQuery(filter ModelRelationTypeFilter) bson.M {
	query := bson.M{}
	if filter.SourceModelUID != "" {
		query["source_model_uid"] = filter.SourceModelUID
	}
	if filter.TargetModelUID != "" {
		query["target_model_uid"] = filter.TargetModelUID
	}
	if filter.RelationType != "" {
		query["relation_type"] = filter.RelationType
	}
	return query
}
