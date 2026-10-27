package dao

import (
	"context"
	"time"

	"github.com/Havens-blog/e-common-go/mongox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const WebhookCollection = "ecam_webhook"

// Webhook Webhook DAO 模型。
type Webhook struct {
	ID         int64    `bson:"id"`
	TenantID   int64    `bson:"tenant_id"`
	Name       string   `bson:"name"`
	EventTypes []string `bson:"event_types"`
	TargetURL  string   `bson:"target_url"`
	Secret     string   `bson:"secret"`
	Status     string   `bson:"status"`
	Ctime      int64    `bson:"ctime"`
	Utime      int64    `bson:"utime"`
}

// WebhookUpdate Webhook 部分更新字段；nil 表示不修改。
type WebhookUpdate struct {
	Name       *string
	EventTypes []string
	TargetURL  *string
	Secret     *string
	Status     *string
}

// WebhookDAO Webhook 数据访问接口。
type WebhookDAO interface {
	Create(ctx context.Context, webhook Webhook) (int64, error)
	GetByID(ctx context.Context, tenantID, id int64) (Webhook, error)
	List(ctx context.Context, tenantID int64, offset, limit int) ([]Webhook, error)
	Count(ctx context.Context, tenantID int64) (int64, error)
	Update(ctx context.Context, tenantID, id int64, update WebhookUpdate) error
	Delete(ctx context.Context, tenantID, id int64) error
}

type webhookDAO struct {
	db *mongox.Mongo
}

// NewWebhookDAO 创建 Webhook DAO。
func NewWebhookDAO(db *mongox.Mongo) WebhookDAO {
	return &webhookDAO{db: db}
}

// Create 创建 Webhook。
func (d *webhookDAO) Create(ctx context.Context, webhook Webhook) (int64, error) {
	now := time.Now().UnixMilli()
	webhook.Ctime = now
	webhook.Utime = now

	if webhook.ID == 0 {
		webhook.ID = d.db.GetIdGenerator(WebhookCollection)
	}

	_, err := d.db.Collection(WebhookCollection).InsertOne(ctx, webhook)
	if err != nil {
		return 0, err
	}
	return webhook.ID, nil
}

// GetByID 按租户与 ID 获取 Webhook。
func (d *webhookDAO) GetByID(ctx context.Context, tenantID, id int64) (Webhook, error) {
	var webhook Webhook
	err := d.db.Collection(WebhookCollection).
		FindOne(ctx, bson.M{"id": id, "tenant_id": tenantID}).
		Decode(&webhook)
	return webhook, err
}

// List 按租户分页查询 Webhook。
func (d *webhookDAO) List(ctx context.Context, tenantID int64, offset, limit int) ([]Webhook, error) {
	opts := options.Find()
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	opts.SetSort(bson.D{{Key: "id", Value: -1}})

	cursor, err := d.db.Collection(WebhookCollection).Find(ctx, bson.M{"tenant_id": tenantID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var webhooks []Webhook
	if err := cursor.All(ctx, &webhooks); err != nil {
		return nil, err
	}
	return webhooks, nil
}

// Count 按租户统计 Webhook 数量。
func (d *webhookDAO) Count(ctx context.Context, tenantID int64) (int64, error) {
	return d.db.Collection(WebhookCollection).CountDocuments(ctx, bson.M{"tenant_id": tenantID})
}

// Update 按租户与 ID 部分更新 Webhook。
func (d *webhookDAO) Update(ctx context.Context, tenantID, id int64, update WebhookUpdate) error {
	set := bson.M{"utime": time.Now().UnixMilli()}
	if update.Name != nil {
		set["name"] = *update.Name
	}
	if update.EventTypes != nil {
		set["event_types"] = update.EventTypes
	}
	if update.TargetURL != nil {
		set["target_url"] = *update.TargetURL
	}
	if update.Secret != nil {
		set["secret"] = *update.Secret
	}
	if update.Status != nil {
		set["status"] = *update.Status
	}

	result, err := d.db.Collection(WebhookCollection).
		UpdateOne(ctx, bson.M{"id": id, "tenant_id": tenantID}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// Delete 按租户与 ID 物理删除 Webhook。
func (d *webhookDAO) Delete(ctx context.Context, tenantID, id int64) error {
	result, err := d.db.Collection(WebhookCollection).
		DeleteOne(ctx, bson.M{"id": id, "tenant_id": tenantID})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
