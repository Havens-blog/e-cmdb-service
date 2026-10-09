package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const AttributeCollection = "ecam_attribute"
const AttributeGroupCollection = "ecam_attribute_group"

// Attribute DAO层属性模型
type Attribute struct {
	ID          int64       `bson:"id"`
	FieldUID    string      `bson:"field_uid"`
	FieldName   string      `bson:"field_name"`
	FieldType   string      `bson:"field_type"`
	ModelUID    string      `bson:"model_uid"`
	GroupID     int64       `bson:"group_id"`
	DisplayName string      `bson:"display_name"`
	Display     bool        `bson:"display"`
	Index       int         `bson:"index"`
	Required    bool        `bson:"required"`
	Editable    bool        `bson:"editable"`
	Searchable  bool        `bson:"searchable"`
	Unique      bool        `bson:"unique"`
	Secure      bool        `bson:"secure"`
	Link        bool        `bson:"link"`
	LinkModel   string      `bson:"link_model"`
	Option      interface{} `bson:"option"`
	Default     string      `bson:"default"`
	Placeholder string      `bson:"placeholder"`
	Description string      `bson:"description"`
	Ctime       int64       `bson:"ctime"`
	Utime       int64       `bson:"utime"`
}

// AttributeGroup DAO层属性分组模型
type AttributeGroup struct {
	ID          int64  `bson:"id"`
	UID         string `bson:"uid"`
	Name        string `bson:"name"`
	ModelUID    string `bson:"model_uid"`
	Index       int    `bson:"index"`
	IsBuiltin   bool   `bson:"is_builtin"`
	Description string `bson:"description"`
	Ctime       int64  `bson:"ctime"`
	Utime       int64  `bson:"utime"`
}

// AttributeFilter DAO层属性过滤条件
type AttributeFilter struct {
	ModelUID   string
	GroupID    int64
	FieldType  string
	Display    *bool
	Required   *bool
	Searchable *bool
	Offset     int
	Limit      int
}

// AttributeDAO 属性数据访问接口
type AttributeDAO interface {
	Create(ctx context.Context, attr Attribute) (int64, error)
	CreateBatch(ctx context.Context, attrs []Attribute) (int64, error)
	GetByID(ctx context.Context, id int64) (Attribute, error)
	GetByFieldUID(ctx context.Context, modelUID, fieldUID string) (Attribute, error)
	List(ctx context.Context, filter AttributeFilter) ([]Attribute, error)
	Count(ctx context.Context, filter AttributeFilter) (int64, error)
	Update(ctx context.Context, attr Attribute) error
	Delete(ctx context.Context, id int64) error
	DeleteByModelUID(ctx context.Context, modelUID string) error
	Exists(ctx context.Context, modelUID, fieldUID string) (bool, error)
}

type attributeDAO struct {
	db *mongox.Mongo
}

// NewAttributeDAO 创建属性DAO
func NewAttributeDAO(db *mongox.Mongo) AttributeDAO {
	return &attributeDAO{db: db}
}

// Create 创建属性
func (d *attributeDAO) Create(ctx context.Context, attr Attribute) (int64, error) {
	now := time.Now().UnixMilli()
	attr.Ctime = now
	attr.Utime = now

	if attr.ID == 0 {
		attr.ID = d.db.GetIdGenerator(AttributeCollection)
	}

	_, err := d.db.Collection(AttributeCollection).InsertOne(ctx, attr)
	if err != nil {
		return 0, err
	}
	return attr.ID, nil
}

// CreateBatch 批量创建属性
func (d *attributeDAO) CreateBatch(ctx context.Context, attrs []Attribute) (int64, error) {
	if len(attrs) == 0 {
		return 0, nil
	}

	now := time.Now().UnixMilli()
	docs := make([]interface{}, len(attrs))

	for i := range attrs {
		if attrs[i].ID == 0 {
			attrs[i].ID = d.db.GetIdGenerator(AttributeCollection)
		}
		attrs[i].Ctime = now
		attrs[i].Utime = now
		docs[i] = attrs[i]
	}

	result, err := d.db.Collection(AttributeCollection).InsertMany(ctx, docs)
	if err != nil {
		return 0, err
	}
	return int64(len(result.InsertedIDs)), nil
}

// GetByID 根据ID获取属性
func (d *attributeDAO) GetByID(ctx context.Context, id int64) (Attribute, error) {
	var attr Attribute
	filter := bson.M{"id": id}
	err := d.db.Collection(AttributeCollection).FindOne(ctx, filter).Decode(&attr)
	return attr, err
}

// GetByFieldUID 根据模型UID和字段UID获取属性
func (d *attributeDAO) GetByFieldUID(ctx context.Context, modelUID, fieldUID string) (Attribute, error) {
	var attr Attribute
	filter := bson.M{"model_uid": modelUID, "field_uid": fieldUID}
	err := d.db.Collection(AttributeCollection).FindOne(ctx, filter).Decode(&attr)
	return attr, err
}

// List 获取属性列表
func (d *attributeDAO) List(ctx context.Context, filter AttributeFilter) ([]Attribute, error) {
	query := d.buildQuery(filter)

	opts := options.Find()
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	opts.SetSort(bson.D{{Key: "group_id", Value: 1}, {Key: "index", Value: 1}})

	cursor, err := d.db.Collection(AttributeCollection).Find(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	attrs := make([]Attribute, 0)
	err = cursor.All(ctx, &attrs)
	return attrs, err
}

// Count 统计属性数量
func (d *attributeDAO) Count(ctx context.Context, filter AttributeFilter) (int64, error) {
	query := d.buildQuery(filter)
	return d.db.Collection(AttributeCollection).CountDocuments(ctx, query)
}

// Update 更新属性
func (d *attributeDAO) Update(ctx context.Context, attr Attribute) error {
	attr.Utime = time.Now().UnixMilli()

	filter := bson.M{"id": attr.ID}
	update := bson.M{
		"$set": bson.M{
			"field_name":  attr.FieldName,
			"field_type":  attr.FieldType,
			"group_id":    attr.GroupID,
			"display":     attr.Display,
			"index":       attr.Index,
			"required":    attr.Required,
			"editable":    attr.Editable,
			"searchable":  attr.Searchable,
			"unique":      attr.Unique,
			"secure":      attr.Secure,
			"link":        attr.Link,
			"link_model":  attr.LinkModel,
			"option":      attr.Option,
			"default":     attr.Default,
			"placeholder": attr.Placeholder,
			"description": attr.Description,
			"utime":       attr.Utime,
		},
	}

	result, err := d.db.Collection(AttributeCollection).UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// Delete 删除属性
func (d *attributeDAO) Delete(ctx context.Context, id int64) error {
	filter := bson.M{"id": id}
	_, err := d.db.Collection(AttributeCollection).DeleteOne(ctx, filter)
	return err
}

// DeleteByModelUID 删除指定模型的所有属性
func (d *attributeDAO) DeleteByModelUID(ctx context.Context, modelUID string) error {
	filter := bson.M{"model_uid": modelUID}
	_, err := d.db.Collection(AttributeCollection).DeleteMany(ctx, filter)
	return err
}

// Exists 检查属性是否存在
func (d *attributeDAO) Exists(ctx context.Context, modelUID, fieldUID string) (bool, error) {
	filter := bson.M{"model_uid": modelUID, "field_uid": fieldUID}
	count, err := d.db.Collection(AttributeCollection).CountDocuments(ctx, filter)
	return count > 0, err
}

// buildQuery 构建查询条件
func (d *attributeDAO) buildQuery(filter AttributeFilter) bson.M {
	query := bson.M{}
	if filter.ModelUID != "" {
		query["model_uid"] = filter.ModelUID
	}
	if filter.GroupID > 0 {
		query["group_id"] = filter.GroupID
	}
	if filter.FieldType != "" {
		query["field_type"] = filter.FieldType
	}
	if filter.Display != nil {
		query["display"] = *filter.Display
	}
	if filter.Required != nil {
		query["required"] = *filter.Required
	}
	if filter.Searchable != nil {
		query["searchable"] = *filter.Searchable
	}
	return query
}

// AttributeGroupDAO 属性分组数据访问接口
type AttributeGroupDAO interface {
	Create(ctx context.Context, group AttributeGroup) (int64, error)
	GetByID(ctx context.Context, id int64) (AttributeGroup, error)
	GetByUID(ctx context.Context, modelUID, uid string) (AttributeGroup, error)
	List(ctx context.Context, modelUID string) ([]AttributeGroup, error)
	Update(ctx context.Context, group AttributeGroup) error
	Delete(ctx context.Context, id int64) error
	DeleteByModelUID(ctx context.Context, modelUID string) error
	Upsert(ctx context.Context, group AttributeGroup) error
}

type attributeGroupDAO struct {
	db *mongox.Mongo
}

// NewAttributeGroupDAO 创建属性分组DAO
func NewAttributeGroupDAO(db *mongox.Mongo) AttributeGroupDAO {
	return &attributeGroupDAO{db: db}
}

// Create 创建属性分组
func (d *attributeGroupDAO) Create(ctx context.Context, group AttributeGroup) (int64, error) {
	now := time.Now().UnixMilli()
	group.Ctime = now
	group.Utime = now

	if group.ID == 0 {
		group.ID = d.db.GetIdGenerator(AttributeGroupCollection)
	}

	_, err := d.db.Collection(AttributeGroupCollection).InsertOne(ctx, group)
	if err != nil {
		return 0, err
	}
	return group.ID, nil
}

// GetByID 根据ID获取属性分组
func (d *attributeGroupDAO) GetByID(ctx context.Context, id int64) (AttributeGroup, error) {
	var group AttributeGroup
	filter := bson.M{"id": id}
	err := d.db.Collection(AttributeGroupCollection).FindOne(ctx, filter).Decode(&group)
	return group, err
}

// GetByUID 根据模型UID和分组UID获取属性分组
func (d *attributeGroupDAO) GetByUID(ctx context.Context, modelUID, uid string) (AttributeGroup, error) {
	var group AttributeGroup
	filter := bson.M{"model_uid": modelUID, "uid": uid}
	err := d.db.Collection(AttributeGroupCollection).FindOne(ctx, filter).Decode(&group)
	return group, err
}

// List 获取属性分组列表
func (d *attributeGroupDAO) List(ctx context.Context, modelUID string) ([]AttributeGroup, error) {
	filter := bson.M{"model_uid": modelUID}
	opts := options.Find().SetSort(bson.D{{Key: "index", Value: 1}, {Key: "id", Value: 1}})

	cursor, err := d.db.Collection(AttributeGroupCollection).Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var groups []AttributeGroup
	err = cursor.All(ctx, &groups)
	return groups, err
}

// Update 更新属性分组
func (d *attributeGroupDAO) Update(ctx context.Context, group AttributeGroup) error {
	group.Utime = time.Now().UnixMilli()

	filter := bson.M{"id": group.ID}
	update := bson.M{
		"$set": bson.M{
			"name":        group.Name,
			"index":       group.Index,
			"description": group.Description,
			"utime":       group.Utime,
		},
	}

	result, err := d.db.Collection(AttributeGroupCollection).UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// Delete 删除属性分组
func (d *attributeGroupDAO) Delete(ctx context.Context, id int64) error {
	filter := bson.M{"id": id}
	_, err := d.db.Collection(AttributeGroupCollection).DeleteOne(ctx, filter)
	return err
}

// DeleteByModelUID 删除指定模型的所有属性分组
func (d *attributeGroupDAO) DeleteByModelUID(ctx context.Context, modelUID string) error {
	filter := bson.M{"model_uid": modelUID}
	_, err := d.db.Collection(AttributeGroupCollection).DeleteMany(ctx, filter)
	return err
}

// Upsert 更新或插入属性分组
func (d *attributeGroupDAO) Upsert(ctx context.Context, group AttributeGroup) error {
	now := time.Now().UnixMilli()
	filter := bson.M{"model_uid": group.ModelUID, "uid": group.UID}

	update := bson.M{
		"$set": bson.M{
			"name":        group.Name,
			"index":       group.Index,
			"is_builtin":  group.IsBuiltin,
			"description": group.Description,
			"utime":       now,
		},
		"$setOnInsert": bson.M{
			"id":    d.db.GetIdGenerator(AttributeGroupCollection),
			"ctime": now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := d.db.Collection(AttributeGroupCollection).UpdateOne(ctx, filter, update, opts)
	return err
}
