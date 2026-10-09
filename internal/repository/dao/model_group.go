package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const CollectionModelGroup = "ecam_model_group"

// ModelGroup 模型分组数据模型
type ModelGroup struct {
	ID          int64     `bson:"id"`
	UID         string    `bson:"uid"`
	Name        string    `bson:"name"`
	Icon        string    `bson:"icon"`
	SortOrder   int       `bson:"sort_order"`
	IsBuiltin   bool      `bson:"is_builtin"`
	Description string    `bson:"description"`
	CreateTime  time.Time `bson:"create_time"`
	UpdateTime  time.Time `bson:"update_time"`
}

// ModelGroupDAO 模型分组数据访问对象
type ModelGroupDAO struct {
	db *mongox.Mongo
}

// NewModelGroupDAO 创建模型分组DAO
func NewModelGroupDAO(db *mongox.Mongo) *ModelGroupDAO {
	return &ModelGroupDAO{db: db}
}

// Create 创建模型分组
func (d *ModelGroupDAO) Create(ctx context.Context, group ModelGroup) (int64, error) {
	now := time.Now()
	group.CreateTime = now
	group.UpdateTime = now

	// 生成ID
	if group.ID == 0 {
		group.ID = d.db.GetIdGenerator(CollectionModelGroup)
	}

	_, err := d.db.Collection(CollectionModelGroup).InsertOne(ctx, group)
	if err != nil {
		return 0, err
	}
	return group.ID, nil
}

// Update 更新模型分组
func (d *ModelGroupDAO) Update(ctx context.Context, group ModelGroup) error {
	group.UpdateTime = time.Now()
	filter := bson.M{"uid": group.UID}
	update := bson.M{
		"$set": bson.M{
			"name":        group.Name,
			"icon":        group.Icon,
			"sort_order":  group.SortOrder,
			"description": group.Description,
			"update_time": group.UpdateTime,
		},
	}
	_, err := d.db.Collection(CollectionModelGroup).UpdateOne(ctx, filter, update)
	return err
}

// Delete 删除模型分组
func (d *ModelGroupDAO) Delete(ctx context.Context, uid string) error {
	filter := bson.M{"uid": uid}
	_, err := d.db.Collection(CollectionModelGroup).DeleteOne(ctx, filter)
	return err
}

// GetByUID 根据UID获取模型分组
func (d *ModelGroupDAO) GetByUID(ctx context.Context, uid string) (ModelGroup, error) {
	var group ModelGroup
	filter := bson.M{"uid": uid}
	err := d.db.Collection(CollectionModelGroup).FindOne(ctx, filter).Decode(&group)
	if err == mongo.ErrNoDocuments {
		return ModelGroup{}, nil
	}
	return group, err
}

// GetByID 根据ID获取模型分组
func (d *ModelGroupDAO) GetByID(ctx context.Context, id int64) (ModelGroup, error) {
	var group ModelGroup
	filter := bson.M{"id": id}
	err := d.db.Collection(CollectionModelGroup).FindOne(ctx, filter).Decode(&group)
	if err == mongo.ErrNoDocuments {
		return ModelGroup{}, nil
	}
	return group, err
}

// List 获取模型分组列表
func (d *ModelGroupDAO) List(ctx context.Context, filter bson.M, offset, limit int) ([]ModelGroup, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "sort_order", Value: 1}, {Key: "id", Value: 1}}).
		SetSkip(int64(offset))
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := d.db.Collection(CollectionModelGroup).Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var groups []ModelGroup
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// Count 统计模型分组数量
func (d *ModelGroupDAO) Count(ctx context.Context, filter bson.M) (int64, error) {
	return d.db.Collection(CollectionModelGroup).CountDocuments(ctx, filter)
}

// Upsert 更新或插入模型分组
func (d *ModelGroupDAO) Upsert(ctx context.Context, group ModelGroup) error {
	now := time.Now()
	filter := bson.M{"uid": group.UID}

	update := bson.M{
		"$set": bson.M{
			"name":        group.Name,
			"icon":        group.Icon,
			"sort_order":  group.SortOrder,
			"is_builtin":  group.IsBuiltin,
			"description": group.Description,
			"update_time": now,
		},
		"$setOnInsert": bson.M{
			"create_time": now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := d.db.Collection(CollectionModelGroup).UpdateOne(ctx, filter, update, opts)
	return err
}

// InitBuiltinGroups 初始化内置分组
func (d *ModelGroupDAO) InitBuiltinGroups(ctx context.Context, groups []ModelGroup) error {
	for _, group := range groups {
		if err := d.Upsert(ctx, group); err != nil {
			return err
		}
	}
	return nil
}
