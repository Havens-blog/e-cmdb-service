package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ModelCollection = "ecam_model"

// Model DAO层模型
type Model struct {
	ID           int64  `bson:"id"`
	UID          string `bson:"uid"`
	Name         string `bson:"name"`
	ModelGroupID int64  `bson:"model_group_id"`
	ParentUID    string `bson:"parent_uid"`
	Category     string `bson:"category"`
	Level        int    `bson:"level"`
	Icon         string `bson:"icon"`
	Description  string `bson:"description"`
	Provider     string `bson:"provider"`
	Extensible   bool   `bson:"extensible"`
	Ctime        int64  `bson:"ctime"`
	Utime        int64  `bson:"utime"`
}

// ModelFilter 模型过滤条件
type ModelFilter struct {
	Provider     string
	Category     string
	ParentUID    string
	Level        int
	ModelGroupID int64
	Extensible   *bool
	Offset       int
	Limit        int
}

// ModelDAO 模型数据访问接口
type ModelDAO interface {
	Create(ctx context.Context, model Model) (int64, error)
	GetByUID(ctx context.Context, uid string) (Model, error)
	GetByID(ctx context.Context, id int64) (Model, error)
	List(ctx context.Context, filter ModelFilter) ([]Model, error)
	Count(ctx context.Context, filter ModelFilter) (int64, error)
	Update(ctx context.Context, model Model) error
	Delete(ctx context.Context, uid string) error
	Exists(ctx context.Context, uid string) (bool, error)
}

type modelDAO struct {
	db *mongox.Mongo
}

// NewModelDAO 创建模型DAO
func NewModelDAO(db *mongox.Mongo) ModelDAO {
	return &modelDAO{db: db}
}

// Create 创建模型
func (d *modelDAO) Create(ctx context.Context, model Model) (int64, error) {
	now := time.Now().UnixMilli()
	model.Ctime = now
	model.Utime = now

	if model.ID == 0 {
		model.ID = d.db.GetIdGenerator(ModelCollection)
	}

	col := d.db.Collection(ModelCollection)
	_, err := col.InsertOne(ctx, model)
	if err != nil {
		return 0, err
	}

	return model.ID, nil
}

// GetByUID 根据UID获取模型
func (d *modelDAO) GetByUID(ctx context.Context, uid string) (Model, error) {
	var model Model
	col := d.db.Collection(ModelCollection)

	filter := bson.M{"uid": uid}
	err := col.FindOne(ctx, filter).Decode(&model)
	if err != nil {
		return Model{}, err
	}

	return model, nil
}

// GetByID 根据ID获取模型
func (d *modelDAO) GetByID(ctx context.Context, id int64) (Model, error) {
	var model Model
	col := d.db.Collection(ModelCollection)

	filter := bson.M{"id": id}
	err := col.FindOne(ctx, filter).Decode(&model)
	if err != nil {
		return Model{}, err
	}

	return model, nil
}

// List 获取模型列表
func (d *modelDAO) List(ctx context.Context, filter ModelFilter) ([]Model, error) {
	col := d.db.Collection(ModelCollection)
	query := d.buildQuery(filter)

	opts := options.Find()
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	opts.SetSort(bson.D{{Key: "id", Value: 1}})

	cursor, err := col.Find(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var models []Model
	if err = cursor.All(ctx, &models); err != nil {
		return nil, err
	}

	return models, nil
}

// Count 统计模型数量
func (d *modelDAO) Count(ctx context.Context, filter ModelFilter) (int64, error) {
	col := d.db.Collection(ModelCollection)
	query := d.buildQuery(filter)
	return col.CountDocuments(ctx, query)
}

// Update 更新模型
func (d *modelDAO) Update(ctx context.Context, model Model) error {
	col := d.db.Collection(ModelCollection)
	model.Utime = time.Now().UnixMilli()

	filter := bson.M{"uid": model.UID}
	update := bson.M{
		"$set": bson.M{
			"name":           model.Name,
			"model_group_id": model.ModelGroupID,
			"icon":           model.Icon,
			"description":    model.Description,
			"extensible":     model.Extensible,
			"utime":          model.Utime,
		},
	}

	result, err := col.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// Delete 删除模型
func (d *modelDAO) Delete(ctx context.Context, uid string) error {
	col := d.db.Collection(ModelCollection)

	filter := bson.M{"uid": uid}
	result, err := col.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// Exists 检查模型是否存在
func (d *modelDAO) Exists(ctx context.Context, uid string) (bool, error) {
	col := d.db.Collection(ModelCollection)

	filter := bson.M{"uid": uid}
	count, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// buildQuery 构建查询条件
func (d *modelDAO) buildQuery(filter ModelFilter) bson.M {
	query := bson.M{}
	if filter.Provider != "" {
		query["provider"] = filter.Provider
	}
	if filter.Category != "" {
		query["category"] = filter.Category
	}
	if filter.ParentUID != "" {
		query["parent_uid"] = filter.ParentUID
	}
	if filter.Level > 0 {
		query["level"] = filter.Level
	}
	if filter.ModelGroupID > 0 {
		query["model_group_id"] = filter.ModelGroupID
	}
	if filter.Extensible != nil {
		query["extensible"] = *filter.Extensible
	}
	return query
}
