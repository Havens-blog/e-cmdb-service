package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// defaultTokenScopes API Token 默认权限范围；当前仅存储，不做鉴权 enforcement。
var defaultTokenScopes = []string{"read", "write"}

// 名称与目标地址的长度上限。上限存在的意义是可用性而不是安全：列表页按名称
// 渲染，无上限的名称会把表格挤成一行字，用户看不出该吊销哪一个。
const (
	maxTokenNameLength   = 64
	maxWebhookNameLength = 64
	maxTargetURLLength   = 2048
	// maxTokenTTLDays 令牌有效期的天数上限（10 年），与前端 el-input-number 的
	// :max="3650" 保持一致。上限不是业务洁癖：expireAtMillis 若用
	// time.Duration(ttlDays)*24*time.Hour 计算，ttlDays 超过约 106751 天时
	// 乘法会静默溢出 int64 纳秒，`now.Add(...)` 反而得到过去的时间戳——
	// 「有效期很长」的请求会变成「创建即过期」的令牌。
	maxTokenTTLDays = 3650
)

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
//
// 名称按 trim 后判空与计长：纯空白名称会写进列表却显示为空白，用户无法分辨
// 该吊销哪一个。ttlDays 只接受「0 或 0..maxTokenTTLDays」——0 是产品已确认的
// 「永不过期」；负数一律拒绝（历史实现把负数当成 0 静默放行，等于把无效输入
// 降级成权限最强的那种令牌）；超过上限也拒绝，避免溢出成「创建即过期」。
func (s *openAPIService) CreateToken(ctx context.Context, tenantID int64, name string, scopes []string, ttlDays int) (domain.ApiToken, string, error) {
	if tenantID == 0 {
		return domain.ApiToken{}, "", errs.ErrInvalidTenantID
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.ApiToken{}, "", errs.ErrInvalidTokenName
	}
	if utf8.RuneCountInString(name) > maxTokenNameLength {
		return domain.ApiToken{}, "", errs.ErrTokenNameTooLong
	}
	if ttlDays < 0 {
		return domain.ApiToken{}, "", errs.ErrInvalidTokenTTL
	}
	if ttlDays > maxTokenTTLDays {
		return domain.ApiToken{}, "", errs.ErrTokenTTLTooLarge
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
//
// name 与 targetURL 同样按 trim 后判空与计长，理由与 CreateToken 一致；
// eventTypes 必须是非空子集，由 validateEventTypes 逐项校验。
func (s *openAPIService) CreateWebhook(ctx context.Context, tenantID int64, name string, eventTypes []string, targetURL, secret string) (int64, error) {
	if tenantID == 0 {
		return 0, errs.ErrInvalidTenantID
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errs.ErrInvalidWebhookName
	}
	if utf8.RuneCountInString(name) > maxWebhookNameLength {
		return 0, errs.ErrWebhookNameTooLong
	}
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return 0, errs.ErrInvalidTargetURL
	}
	if utf8.RuneCountInString(targetURL) > maxTargetURLLength {
		return 0, errs.ErrTargetURLTooLong
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

// normalizeWebhookUpdate 把部分更新里的名称与目标地址按 trim + 长度上限归一，
// 并把归一后的值写回 update，供后续持久化使用。
//
// 更新路径必须与创建路径用同一套规则：只校验创建会让「先建合法、再改空白」
// 绕过限制，列表里照样出现看不见名字的订阅。
func normalizeWebhookUpdate(update domain.WebhookUpdate) (domain.WebhookUpdate, error) {
	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		if name == "" {
			return update, errs.ErrInvalidWebhookName
		}
		if utf8.RuneCountInString(name) > maxWebhookNameLength {
			return update, errs.ErrWebhookNameTooLong
		}
		update.Name = &name
	}
	if update.TargetURL != nil {
		targetURL := strings.TrimSpace(*update.TargetURL)
		if targetURL == "" {
			return update, errs.ErrInvalidTargetURL
		}
		if utf8.RuneCountInString(targetURL) > maxTargetURLLength {
			return update, errs.ErrTargetURLTooLong
		}
		update.TargetURL = &targetURL
	}
	return update, nil
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

	update, err = normalizeWebhookUpdate(update)
	if err != nil {
		return err
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
//
// 用 AddDate 而非 Add(time.Duration(ttlDays)*24*time.Hour)：后者在 ttlDays
// 极大时会把纳秒乘积溢出成负数，得到落在过去的过期时间。service 层已有
// maxTokenTTLDays 上限，这里再用 AddDate 做纵深防御，避免上限被调整后复发。
func expireAtMillis(ttlDays int, now time.Time) int64 {
	if ttlDays <= 0 {
		return 0
	}
	return now.AddDate(0, 0, ttlDays).UnixMilli()
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
	// 空列表以前是被 handler 的 binding:"required" 挡住的；为了让错误信息可读，
	// 那层校验已下移到 service，这里必须自己拒绝空列表，否则订阅永不触发。
	if len(types) == 0 {
		return errs.ErrInvalidEventType
	}
	for _, typ := range types {
		if !validEventType(typ) {
			return errs.ErrInvalidEventType
		}
	}
	return nil
}
