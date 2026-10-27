package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// defaultTokenScopes API Token 默认权限范围；当前仅存储，不做鉴权 enforcement。
var defaultTokenScopes = []string{"read", "write"}

// validEventTypes Webhook 合法事件类型（当前仅存储，不做投递）。
var validEventTypes = map[string]struct{}{
	"instance.created": {},
	"instance.updated": {},
	"instance.deleted": {},
	"relation.created": {},
	"relation.deleted": {},
}

// OpenAPIService 开放 API 管理服务接口。
type OpenAPIService interface {
	CreateToken(ctx context.Context, tenantID int64, name string, scopes []string, ttlDays int) (domain.ApiToken, string, error)
	ListTokens(ctx context.Context, tenantID int64, offset, limit int) ([]domain.ApiToken, int64, error)
	RevokeToken(ctx context.Context, tenantID int64, id int64) error

	CreateWebhook(ctx context.Context, tenantID int64, name string, eventTypes []string, targetURL, secret string) (int64, error)
	ListWebhooks(ctx context.Context, tenantID int64, offset, limit int) ([]domain.Webhook, int64, error)
	UpdateWebhook(ctx context.Context, tenantID int64, id int64, update domain.WebhookUpdate) error
	DeleteWebhook(ctx context.Context, tenantID int64, id int64) error
}

type openAPIService struct {
	tokenRepo   repository.ApiTokenRepository
	webhookRepo repository.WebhookRepository
}

// NewOpenAPIService 创建开放 API 管理服务。
func NewOpenAPIService(tokenRepo repository.ApiTokenRepository, webhookRepo repository.WebhookRepository) OpenAPIService {
	return &openAPIService{tokenRepo: tokenRepo, webhookRepo: webhookRepo}
}

// CreateToken 创建 API Token，返回领域模型与仅此一次可见的明文 token。
func (s *openAPIService) CreateToken(ctx context.Context, tenantID int64, name string, scopes []string, ttlDays int) (domain.ApiToken, string, error) {
	if tenantID == 0 {
		return domain.ApiToken{}, "", errs.ErrInvalidTenantID
	}
	if name == "" {
		return domain.ApiToken{}, "", errs.ErrInvalidTokenName
	}

	plain, hash, err := generateAPIToken()
	if err != nil {
		return domain.ApiToken{}, "", fmt.Errorf("generate api token: %w", err)
	}

	now := time.Now()
	token := domain.ApiToken{
		TenantID:   tenantID,
		Name:       name,
		Scopes:     normalizeTokenScopes(scopes),
		Status:     domain.APITokenStatusActive,
		TokenHash:  hash,
		ExpireAt:   expireAtMillis(ttlDays, now),
		CreateTime: now,
		UpdateTime: now,
	}

	id, err := s.tokenRepo.Create(ctx, token)
	if err != nil {
		return domain.ApiToken{}, "", fmt.Errorf("create api token: %w", err)
	}
	token.ID = id

	return token, plain, nil
}

// ListTokens 按租户分页查询 API Token。
func (s *openAPIService) ListTokens(ctx context.Context, tenantID int64, offset, limit int) ([]domain.ApiToken, int64, error) {
	tokens, err := s.tokenRepo.List(ctx, tenantID, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list api tokens: %w", err)
	}

	total, err := s.tokenRepo.Count(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("count api tokens: %w", err)
	}
	return tokens, total, nil
}

// RevokeToken 吊销 API Token（status 置为 revoked，不物理删除）。
func (s *openAPIService) RevokeToken(ctx context.Context, tenantID int64, id int64) error {
	existing, err := s.tokenRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("get api token: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrAPITokenNotFound
	}
	if existing.Status == domain.APITokenStatusRevoked {
		return nil
	}
	return s.tokenRepo.UpdateStatus(ctx, tenantID, id, domain.APITokenStatusRevoked)
}

// CreateWebhook 创建 Webhook 订阅。
func (s *openAPIService) CreateWebhook(ctx context.Context, tenantID int64, name string, eventTypes []string, targetURL, secret string) (int64, error) {
	if tenantID == 0 {
		return 0, errs.ErrInvalidTenantID
	}
	if name == "" {
		return 0, errs.ErrInvalidWebhookName
	}
	if targetURL == "" {
		return 0, errs.ErrInvalidTargetURL
	}
	if err := validateEventTypes(eventTypes); err != nil {
		return 0, err
	}

	now := time.Now()
	webhook := domain.Webhook{
		TenantID:   tenantID,
		Name:       name,
		EventTypes: eventTypes,
		TargetURL:  targetURL,
		Secret:     secret,
		Status:     domain.WebhookStatusActive,
		CreateTime: now,
		UpdateTime: now,
	}
	return s.webhookRepo.Create(ctx, webhook)
}

// ListWebhooks 按租户分页查询 Webhook。
func (s *openAPIService) ListWebhooks(ctx context.Context, tenantID int64, offset, limit int) ([]domain.Webhook, int64, error) {
	webhooks, err := s.webhookRepo.List(ctx, tenantID, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list webhooks: %w", err)
	}

	total, err := s.webhookRepo.Count(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("count webhooks: %w", err)
	}
	return webhooks, total, nil
}

// UpdateWebhook 按租户与 ID 部分更新 Webhook。
func (s *openAPIService) UpdateWebhook(ctx context.Context, tenantID int64, id int64, update domain.WebhookUpdate) error {
	existing, err := s.webhookRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("get webhook: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrWebhookNotFound
	}
	if update.Name != nil && *update.Name == "" {
		return errs.ErrInvalidWebhookName
	}
	if update.TargetURL != nil && *update.TargetURL == "" {
		return errs.ErrInvalidTargetURL
	}
	if update.Status != nil && !validWebhookStatus(*update.Status) {
		return errs.ErrInvalidStatus
	}
	if update.EventTypes != nil {
		if err := validateEventTypes(update.EventTypes); err != nil {
			return err
		}
	}
	return s.webhookRepo.Update(ctx, tenantID, id, update)
}

// DeleteWebhook 按租户与 ID 删除 Webhook。
func (s *openAPIService) DeleteWebhook(ctx context.Context, tenantID int64, id int64) error {
	existing, err := s.webhookRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("get webhook: %w", err)
	}
	if existing.ID == 0 {
		return errs.ErrWebhookNotFound
	}
	return s.webhookRepo.Delete(ctx, tenantID, id)
}

// generateAPIToken 生成 32 字节随机值并 hex 编码为明文 token，同时返回其哈希。
func generateAPIToken() (plaintext, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plaintext = hex.EncodeToString(buf)
	return plaintext, hashToken(plaintext), nil
}

// hashToken 返回 token 的 SHA-256(hex) 的 hex 表示。
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// expireAtMillis 计算过期毫秒时间戳；ttlDays <= 0 返回 0（永不过期）。
func expireAtMillis(ttlDays int, now time.Time) int64 {
	if ttlDays <= 0 {
		return 0
	}
	return now.Add(time.Duration(ttlDays) * 24 * time.Hour).UnixMilli()
}

// normalizeTokenScopes 空 scopes 时返回默认权限范围。
func normalizeTokenScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return append([]string(nil), defaultTokenScopes...)
	}
	return scopes
}

// validWebhookStatus 校验 Webhook 状态枚举。
func validWebhookStatus(status string) bool {
	return status == domain.WebhookStatusActive || status == domain.WebhookStatusDisabled
}

// validEventType 校验 Webhook 事件类型枚举。
func validEventType(typ string) bool {
	_, ok := validEventTypes[typ]
	return ok
}

// validateEventTypes 校验事件类型列表。
func validateEventTypes(types []string) error {
	for _, typ := range types {
		if !validEventType(typ) {
			return errs.ErrInvalidEventType
		}
	}
	return nil
}
