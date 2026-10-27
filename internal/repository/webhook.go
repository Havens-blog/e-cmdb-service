package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/mongo"
)

// WebhookRepository Webhook 仓储接口。
type WebhookRepository interface {
	Create(ctx context.Context, webhook domain.Webhook) (int64, error)
	GetByID(ctx context.Context, tenantID, id int64) (domain.Webhook, error)
	List(ctx context.Context, tenantID int64, offset, limit int) ([]domain.Webhook, error)
	Count(ctx context.Context, tenantID int64) (int64, error)
	Update(ctx context.Context, tenantID, id int64, update domain.WebhookUpdate) error
	Delete(ctx context.Context, tenantID, id int64) error
}

type webhookRepository struct {
	dao dao.WebhookDAO
}

// NewWebhookRepository 创建 Webhook 仓储。
func NewWebhookRepository(d dao.WebhookDAO) WebhookRepository {
	return &webhookRepository{dao: d}
}

func (r *webhookRepository) Create(ctx context.Context, webhook domain.Webhook) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(webhook))
}

func (r *webhookRepository) GetByID(ctx context.Context, tenantID, id int64) (domain.Webhook, error) {
	webhook, err := r.dao.GetByID(ctx, tenantID, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.Webhook{}, nil
		}
		return domain.Webhook{}, err
	}
	return r.toDomain(webhook), nil
}

func (r *webhookRepository) List(ctx context.Context, tenantID int64, offset, limit int) ([]domain.Webhook, error) {
	webhooks, err := r.dao.List(ctx, tenantID, offset, limit)
	if err != nil {
		return nil, err
	}

	result := make([]domain.Webhook, len(webhooks))
	for i, webhook := range webhooks {
		result[i] = r.toDomain(webhook)
	}
	return result, nil
}

func (r *webhookRepository) Count(ctx context.Context, tenantID int64) (int64, error) {
	return r.dao.Count(ctx, tenantID)
}

func (r *webhookRepository) Update(ctx context.Context, tenantID, id int64, update domain.WebhookUpdate) error {
	return r.dao.Update(ctx, tenantID, id, toDAOUpdate(update))
}

func (r *webhookRepository) Delete(ctx context.Context, tenantID, id int64) error {
	return r.dao.Delete(ctx, tenantID, id)
}

func (r *webhookRepository) toDAO(webhook domain.Webhook) dao.Webhook {
	return dao.Webhook{
		ID:         webhook.ID,
		TenantID:   webhook.TenantID,
		Name:       webhook.Name,
		EventTypes: webhook.EventTypes,
		TargetURL:  webhook.TargetURL,
		Secret:     webhook.Secret,
		Status:     webhook.Status,
	}
}

func (r *webhookRepository) toDomain(webhook dao.Webhook) domain.Webhook {
	return domain.Webhook{
		ID:         webhook.ID,
		TenantID:   webhook.TenantID,
		Name:       webhook.Name,
		EventTypes: webhook.EventTypes,
		TargetURL:  webhook.TargetURL,
		Secret:     webhook.Secret,
		Status:     webhook.Status,
		CreateTime: time.UnixMilli(webhook.Ctime),
		UpdateTime: time.UnixMilli(webhook.Utime),
	}
}

func toDAOUpdate(update domain.WebhookUpdate) dao.WebhookUpdate {
	return dao.WebhookUpdate{
		Name:       update.Name,
		EventTypes: update.EventTypes,
		TargetURL:  update.TargetURL,
		Secret:     update.Secret,
		Status:     update.Status,
	}
}
