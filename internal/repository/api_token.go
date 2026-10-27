package repository

import (
	"context"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"go.mongodb.org/mongo-driver/mongo"
)

// ApiTokenRepository API Token 仓储接口。
type ApiTokenRepository interface {
	Create(ctx context.Context, token domain.ApiToken) (int64, error)
	GetByID(ctx context.Context, tenantID, id int64) (domain.ApiToken, error)
	List(ctx context.Context, tenantID int64, offset, limit int) ([]domain.ApiToken, error)
	Count(ctx context.Context, tenantID int64) (int64, error)
	UpdateStatus(ctx context.Context, tenantID, id int64, status string) error
}

type apiTokenRepository struct {
	dao dao.ApiTokenDAO
}

// NewApiTokenRepository 创建 API Token 仓储。
func NewApiTokenRepository(d dao.ApiTokenDAO) ApiTokenRepository {
	return &apiTokenRepository{dao: d}
}

func (r *apiTokenRepository) Create(ctx context.Context, token domain.ApiToken) (int64, error) {
	return r.dao.Create(ctx, r.toDAO(token))
}

func (r *apiTokenRepository) GetByID(ctx context.Context, tenantID, id int64) (domain.ApiToken, error) {
	token, err := r.dao.GetByID(ctx, tenantID, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return domain.ApiToken{}, nil
		}
		return domain.ApiToken{}, err
	}
	return r.toDomain(token), nil
}

func (r *apiTokenRepository) List(ctx context.Context, tenantID int64, offset, limit int) ([]domain.ApiToken, error) {
	tokens, err := r.dao.List(ctx, tenantID, offset, limit)
	if err != nil {
		return nil, err
	}

	result := make([]domain.ApiToken, len(tokens))
	for i, token := range tokens {
		result[i] = r.toDomain(token)
	}
	return result, nil
}

func (r *apiTokenRepository) Count(ctx context.Context, tenantID int64) (int64, error) {
	return r.dao.Count(ctx, tenantID)
}

func (r *apiTokenRepository) UpdateStatus(ctx context.Context, tenantID, id int64, status string) error {
	return r.dao.UpdateStatus(ctx, tenantID, id, status)
}

func (r *apiTokenRepository) toDAO(token domain.ApiToken) dao.ApiToken {
	return dao.ApiToken{
		ID:        token.ID,
		TenantID:  token.TenantID,
		Name:      token.Name,
		Scopes:    token.Scopes,
		Status:    token.Status,
		TokenHash: token.TokenHash,
		ExpireAt:  token.ExpireAt,
	}
}

func (r *apiTokenRepository) toDomain(token dao.ApiToken) domain.ApiToken {
	return domain.ApiToken{
		ID:         token.ID,
		TenantID:   token.TenantID,
		Name:       token.Name,
		Scopes:     token.Scopes,
		Status:     token.Status,
		TokenHash:  token.TokenHash,
		ExpireAt:   token.ExpireAt,
		CreateTime: time.UnixMilli(token.Ctime),
		UpdateTime: time.UnixMilli(token.Utime),
	}
}
