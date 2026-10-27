package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ApiTokenCollection = "ecam_api_token"

// ApiToken API Token DAO 模型。
type ApiToken struct {
	ID        int64    `bson:"id"`
	TenantID  int64    `bson:"tenant_id"`
	Name      string   `bson:"name"`
	Scopes    []string `bson:"scopes"`
	Status    string   `bson:"status"`
	TokenHash string   `bson:"token_hash"`
	ExpireAt  int64    `bson:"expire_at"`
	Ctime     int64    `bson:"ctime"`
	Utime     int64    `bson:"utime"`
}

// ApiTokenDAO API Token 数据访问接口。
type ApiTokenDAO interface {
	Create(ctx context.Context, token ApiToken) (int64, error)
	GetByID(ctx context.Context, tenantID, id int64) (ApiToken, error)
	List(ctx context.Context, tenantID int64, offset, limit int) ([]ApiToken, error)
	Count(ctx context.Context, tenantID int64) (int64, error)
	UpdateStatus(ctx context.Context, tenantID, id int64, status string) error
}

type apiTokenDAO struct {
	db *mongox.Mongo
}

// NewApiTokenDAO 创建 API Token DAO。
func NewApiTokenDAO(db *mongox.Mongo) ApiTokenDAO {
	return &apiTokenDAO{db: db}
}

// Create 创建 API Token。
func (d *apiTokenDAO) Create(ctx context.Context, token ApiToken) (int64, error) {
	now := time.Now().UnixMilli()
	token.Ctime = now
	token.Utime = now

	if token.ID == 0 {
		token.ID = d.db.GetIdGenerator(ApiTokenCollection)
	}

	_, err := d.db.Collection(ApiTokenCollection).InsertOne(ctx, token)
	if err != nil {
		return 0, err
	}
	return token.ID, nil
}

// GetByID 按租户与 ID 获取 API Token。
func (d *apiTokenDAO) GetByID(ctx context.Context, tenantID, id int64) (ApiToken, error) {
	var token ApiToken
	err := d.db.Collection(ApiTokenCollection).
		FindOne(ctx, bson.M{"id": id, "tenant_id": tenantID}).
		Decode(&token)
	return token, err
}

// List 按租户分页查询 API Token。
func (d *apiTokenDAO) List(ctx context.Context, tenantID int64, offset, limit int) ([]ApiToken, error) {
	opts := options.Find()
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	opts.SetSort(bson.D{{Key: "id", Value: -1}})

	cursor, err := d.db.Collection(ApiTokenCollection).Find(ctx, bson.M{"tenant_id": tenantID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tokens []ApiToken
	if err := cursor.All(ctx, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// Count 按租户统计 API Token 数量。
func (d *apiTokenDAO) Count(ctx context.Context, tenantID int64) (int64, error) {
	return d.db.Collection(ApiTokenCollection).CountDocuments(ctx, bson.M{"tenant_id": tenantID})
}

// UpdateStatus 吊销/更新 API Token 状态（不物理删除）。
func (d *apiTokenDAO) UpdateStatus(ctx context.Context, tenantID, id int64, status string) error {
	update := bson.M{"$set": bson.M{
		"status": status,
		"utime":  time.Now().UnixMilli(),
	}}

	result, err := d.db.Collection(ApiTokenCollection).
		UpdateOne(ctx, bson.M{"id": id, "tenant_id": tenantID}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
