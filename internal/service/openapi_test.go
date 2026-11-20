package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

func TestGenerateAPIToken(t *testing.T) {
	a, ha, err := generateAPIToken()
	if err != nil {
		t.Fatalf("generateAPIToken() error = %v", err)
	}
	b, hb, err := generateAPIToken()
	if err != nil {
		t.Fatalf("generateAPIToken() error = %v", err)
	}

	if len(a) != 64 || len(b) != 64 {
		t.Fatalf("token length = %d/%d, want 64", len(a), len(b))
	}
	if !isHex(a) || !isHex(b) {
		t.Fatalf("token should be hex: %q / %q", a, b)
	}
	if a == b {
		t.Fatalf("tokens should not repeat")
	}
	if ha == a || hb == b {
		t.Fatalf("hash must differ from plaintext")
	}
	if len(ha) != 64 || len(hb) != 64 {
		t.Fatalf("hash length = %d/%d, want 64", len(ha), len(hb))
	}
}

func TestHashToken(t *testing.T) {
	const in = "deadbeef"
	got := hashToken(in)

	if got == in {
		t.Fatalf("hashToken(%q) = %q, must differ from plaintext", in, got)
	}
	if len(got) != 64 {
		t.Fatalf("hash length = %d, want 64", len(got))
	}
	if hashToken(in) != got {
		t.Fatalf("hashToken must be deterministic")
	}
}

func TestExpireAtMillis(t *testing.T) {
	now := time.UnixMilli(1_000_000_000_000)

	if got := expireAtMillis(0, now); got != 0 {
		t.Fatalf("expireAtMillis(0) = %d, want 0", got)
	}
	if got := expireAtMillis(-3, now); got != 0 {
		t.Fatalf("expireAtMillis(-3) = %d, want 0", got)
	}
	if got := expireAtMillis(1, now); got != now.UnixMilli()+86400000 {
		t.Fatalf("expireAtMillis(1) = %d, want %d", got, now.UnixMilli()+86400000)
	}
	if got := expireAtMillis(2, now); got != now.UnixMilli()+2*86400000 {
		t.Fatalf("expireAtMillis(2) = %d, want %d", got, now.UnixMilli()+2*86400000)
	}
}

func TestNormalizeTokenScopes(t *testing.T) {
	got := normalizeTokenScopes(nil)
	if len(got) != 2 || got[0] != "read" || got[1] != "write" {
		t.Fatalf("normalizeTokenScopes(nil) = %#v, want default", got)
	}

	got = normalizeTokenScopes([]string{})
	if len(got) != 2 || got[0] != "read" || got[1] != "write" {
		t.Fatalf("normalizeTokenScopes(empty) = %#v, want default", got)
	}

	custom := []string{"read"}
	got = normalizeTokenScopes(custom)
	if len(got) != 1 || got[0] != "read" {
		t.Fatalf("normalizeTokenScopes(custom) = %#v, want preserved", got)
	}
}

func TestValidEventType(t *testing.T) {
	for _, et := range []string{
		"instance.created", "instance.updated", "instance.deleted",
		"relation.created", "relation.deleted",
	} {
		if !validEventType(et) {
			t.Fatalf("validEventType(%q) = false, want true", et)
		}
	}
	if validEventType("instance.paused") {
		t.Fatalf("validEventType(invalid) = true, want false")
	}
	if validEventType("") {
		t.Fatalf("validEventType(empty) = true, want false")
	}
}

func TestValidWebhookStatus(t *testing.T) {
	if !validWebhookStatus("active") || !validWebhookStatus("disabled") {
		t.Fatalf("active/disabled should be valid")
	}
	if validWebhookStatus("revoked") || validWebhookStatus("") {
		t.Fatalf("revoked/empty should be invalid")
	}
}

// --- 内存 fake repo，测 service 组装逻辑 ---

type fakeTokenRepo struct {
	nextID  int64
	created domain.ApiToken
	stored  []domain.ApiToken
}

func (f *fakeTokenRepo) Create(ctx context.Context, token domain.ApiToken) (int64, error) {
	f.nextID++
	token.ID = f.nextID
	f.created = token
	f.stored = append(f.stored, token)
	return token.ID, nil
}

func (f *fakeTokenRepo) GetByID(ctx context.Context, tenantID, id int64) (domain.ApiToken, error) {
	for _, token := range f.stored {
		if token.ID == id && token.TenantID == tenantID {
			return token, nil
		}
	}
	return domain.ApiToken{}, nil
}

func (f *fakeTokenRepo) List(ctx context.Context, tenantID int64, offset, limit int) ([]domain.ApiToken, error) {
	return f.stored, nil
}

func (f *fakeTokenRepo) Count(ctx context.Context, tenantID int64) (int64, error) {
	return int64(len(f.stored)), nil
}

func (f *fakeTokenRepo) UpdateStatus(ctx context.Context, tenantID, id int64, status string) error {
	return nil
}

type fakeWebhookRepo struct {
	nextID     int64
	created    domain.Webhook
	stored     []domain.Webhook
	lastUpdate domain.WebhookUpdate
}

func (f *fakeWebhookRepo) Create(ctx context.Context, webhook domain.Webhook) (int64, error) {
	f.nextID++
	webhook.ID = f.nextID
	f.created = webhook
	f.stored = append(f.stored, webhook)
	return webhook.ID, nil
}

func (f *fakeWebhookRepo) GetByID(ctx context.Context, tenantID, id int64) (domain.Webhook, error) {
	for _, webhook := range f.stored {
		if webhook.ID == id && webhook.TenantID == tenantID {
			return webhook, nil
		}
	}
	return domain.Webhook{}, nil
}

func (f *fakeWebhookRepo) List(ctx context.Context, tenantID int64, offset, limit int) ([]domain.Webhook, error) {
	return f.stored, nil
}

func (f *fakeWebhookRepo) Count(ctx context.Context, tenantID int64) (int64, error) {
	return int64(len(f.stored)), nil
}

func (f *fakeWebhookRepo) Update(ctx context.Context, tenantID, id int64, update domain.WebhookUpdate) error {
	f.lastUpdate = update
	return nil
}

func (f *fakeWebhookRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return nil
}

func TestCreateToken_Service(t *testing.T) {
	repo := &fakeTokenRepo{}
	svc := NewOpenAPIService(repo, &fakeWebhookRepo{})

	token, plain, err := svc.CreateToken(context.Background(), 10, "ci", nil, 1)
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	if len(plain) != 64 {
		t.Fatalf("plain token length = %d, want 64", len(plain))
	}
	if repo.created.TokenHash == "" || repo.created.TokenHash == plain {
		t.Fatalf("token hash should be stored, not plaintext")
	}
	if len(repo.created.Scopes) != 2 {
		t.Fatalf("scopes should default: %#v", repo.created.Scopes)
	}
	if token.ID == 0 || token.Status != domain.APITokenStatusActive {
		t.Fatalf("unexpected created token: %#v", token)
	}
	if token.ExpireAt <= time.Now().UnixMilli() {
		t.Fatalf("expire_at should be in the future: %d", token.ExpireAt)
	}
}

func TestCreateWebhook_Validation(t *testing.T) {
	repo := &fakeWebhookRepo{}
	svc := NewOpenAPIService(&fakeTokenRepo{}, repo)

	if _, err := svc.CreateWebhook(context.Background(), 10, "w", []string{"instance.created"}, "https://example.com/hook", "s"); err != nil {
		t.Fatalf("valid webhook should pass: %v", err)
	}
	if repo.created.Status != domain.WebhookStatusActive {
		t.Fatalf("status = %q, want active", repo.created.Status)
	}

	if _, err := svc.CreateWebhook(context.Background(), 10, "w", []string{"bad.event"}, "https://example.com/hook", ""); !errors.Is(err, errs.ErrInvalidEventType) {
		t.Fatalf("invalid event type error = %v, want ErrInvalidEventType", err)
	}
}

func TestUpdateWebhook_Validation(t *testing.T) {
	repo := &fakeWebhookRepo{}
	if _, err := NewOpenAPIService(&fakeTokenRepo{}, repo).CreateWebhook(context.Background(), 10, "w", []string{"instance.created"}, "https://example.com/hook", ""); err != nil {
		t.Fatal(err)
	}

	svc := NewOpenAPIService(&fakeTokenRepo{}, repo)
	badStatus := "revoked"
	if err := svc.UpdateWebhook(context.Background(), 10, 1, domain.WebhookUpdate{Status: &badStatus}); !errors.Is(err, errs.ErrInvalidStatus) {
		t.Fatalf("invalid status error = %v, want ErrInvalidStatus", err)
	}
	if err := svc.UpdateWebhook(context.Background(), 10, 999, domain.WebhookUpdate{}); !errors.Is(err, errs.ErrWebhookNotFound) {
		t.Fatalf("not found error = %v, want ErrWebhookNotFound", err)
	}
}

// TestCreateToken_Validation 覆盖名称归一与有效期边界。
//
// 背景：负数 ttl_days 以前与 0 同义（都返回 0 = 永不过期），等于把「填错了」
// 静默降级成权限最强的那种令牌；纯空白名称也会入库，列表里看不见名字。
func TestCreateToken_Validation(t *testing.T) {
	longName := strings.Repeat("a", maxTokenNameLength+1)
	cases := []struct {
		name    string
		rawName string
		ttlDays int
		wantErr error
	}{
		{name: "empty", rawName: "", ttlDays: 1, wantErr: errs.ErrInvalidTokenName},
		{name: "blank", rawName: "   ", ttlDays: 1, wantErr: errs.ErrInvalidTokenName},
		{name: "too long", rawName: longName, ttlDays: 1, wantErr: errs.ErrTokenNameTooLong},
		{name: "negative ttl", rawName: "ci", ttlDays: -5, wantErr: errs.ErrInvalidTokenTTL},
		{name: "ttl beyond maximum", rawName: "ci", ttlDays: maxTokenTTLDays + 1, wantErr: errs.ErrTokenTTLTooLarge},
		// 极端值必须走拒绝而不是交给 expireAtMillis：老实现把 ttlDays 乘成纳秒
		// 后溢出 int64，会得到一个落在过去的过期时间（创建即过期）。
		{name: "ttl far beyond maximum", rawName: "ci", ttlDays: 1 << 40, wantErr: errs.ErrTokenTTLTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewOpenAPIService(&fakeTokenRepo{}, &fakeWebhookRepo{})
			if _, _, err := svc.CreateToken(context.Background(), 10, tc.rawName, nil, tc.ttlDays); !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("trims name and keeps zero ttl as never-expire", func(t *testing.T) {
		repo := &fakeTokenRepo{}
		svc := NewOpenAPIService(repo, &fakeWebhookRepo{})
		token, _, err := svc.CreateToken(context.Background(), 10, "  ci-deploy  ", nil, 0)
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		if repo.created.Name != "ci-deploy" {
			t.Fatalf("stored name = %q, want trimmed %q", repo.created.Name, "ci-deploy")
		}
		if token.ExpireAt != 0 {
			t.Fatalf("expire_at = %d, want 0 (never expire)", token.ExpireAt)
		}
	})

	t.Run("accepts name at the length limit", func(t *testing.T) {
		svc := NewOpenAPIService(&fakeTokenRepo{}, &fakeWebhookRepo{})
		if _, _, err := svc.CreateToken(context.Background(), 10, strings.Repeat("a", maxTokenNameLength), nil, 1); err != nil {
			t.Fatalf("name at limit should pass: %v", err)
		}
	})

	t.Run("accepts ttl at the maximum", func(t *testing.T) {
		svc := NewOpenAPIService(&fakeTokenRepo{}, &fakeWebhookRepo{})
		if _, _, err := svc.CreateToken(context.Background(), 10, "ci", nil, maxTokenTTLDays); err != nil {
			t.Fatalf("ttl at limit should pass: %v", err)
		}
	})
}

// TestCreateWebhook_Normalization 覆盖 Webhook 创建的名称/地址归一与长度上限。
func TestCreateWebhook_Normalization(t *testing.T) {
	const (
		validEvents = "instance.created"
		validURL    = "https://example.com/hook"
	)
	phone := []string{validEvents}

	cases := []struct {
		name      string
		rawName   string
		events    []string
		targetURL string
		wantErr   error
	}{
		{name: "blank name", rawName: "  ", events: phone, targetURL: validURL, wantErr: errs.ErrInvalidWebhookName},
		{name: "name too long", rawName: strings.Repeat("w", maxWebhookNameLength+1), events: phone, targetURL: validURL, wantErr: errs.ErrWebhookNameTooLong},
		{name: "blank url", rawName: "w", events: phone, targetURL: "   ", wantErr: errs.ErrInvalidTargetURL},
		{name: "url too long", rawName: "w", events: phone, targetURL: validURL + strings.Repeat("a", maxTargetURLLength), wantErr: errs.ErrTargetURLTooLong},
		{name: "empty events", rawName: "w", events: nil, targetURL: validURL, wantErr: errs.ErrInvalidEventType},
		{name: "empty events slice", rawName: "w", events: []string{}, targetURL: validURL, wantErr: errs.ErrInvalidEventType},
		// 长度按 code point 计：2048 个中文字符的地址字节数远超 2048，但字符数
		// 正好在上限，必须放行，否则提示「不能超过 2048 个字符」与实现不符。
		{name: "url at the rune limit", rawName: "w", events: phone, targetURL: "https://example.com/" + strings.Repeat("字", maxTargetURLLength-20)},
		{name: "url beyond the rune limit", rawName: "w", events: phone, targetURL: "https://example.com/" + strings.Repeat("字", maxTargetURLLength), wantErr: errs.ErrTargetURLTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewOpenAPIService(&fakeTokenRepo{}, &fakeWebhookRepo{})
			_, err := svc.CreateWebhook(context.Background(), 10, tc.rawName, tc.events, tc.targetURL, "")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("trims name and url", func(t *testing.T) {
		repo := &fakeWebhookRepo{}
		svc := NewOpenAPIService(&fakeTokenRepo{}, repo)
		if _, err := svc.CreateWebhook(context.Background(), 10, "  hook  ", phone, "  "+validURL+"  ", ""); err != nil {
			t.Fatalf("CreateWebhook() error = %v", err)
		}
		if repo.created.Name != "hook" {
			t.Fatalf("stored name = %q, want %q", repo.created.Name, "hook")
		}
		if repo.created.TargetURL != validURL {
			t.Fatalf("stored target_url = %q, want %q", repo.created.TargetURL, validURL)
		}
	})
}

// TestUpdateWebhook_Normalization 保证更新路径与创建路径用同一套规则。
func TestUpdateWebhook_Normalization(t *testing.T) {
	repo := &fakeWebhookRepo{}
	svc := NewOpenAPIService(&fakeTokenRepo{}, repo)
	if _, err := svc.CreateWebhook(context.Background(), 10, "hook", []string{"instance.created"}, "https://example.com/hook", ""); err != nil {
		t.Fatal(err)
	}

	blank := "   "
	if err := svc.UpdateWebhook(context.Background(), 10, 1, domain.WebhookUpdate{Name: &blank}); !errors.Is(err, errs.ErrInvalidWebhookName) {
		t.Fatalf("blank name error = %v, want ErrInvalidWebhookName", err)
	}
	tooLong := strings.Repeat("w", maxWebhookNameLength+1)
	if err := svc.UpdateWebhook(context.Background(), 10, 1, domain.WebhookUpdate{Name: &tooLong}); !errors.Is(err, errs.ErrWebhookNameTooLong) {
		t.Fatalf("long name error = %v, want ErrWebhookNameTooLong", err)
	}
	longURL := "https://example.com/" + strings.Repeat("a", maxTargetURLLength)
	if err := svc.UpdateWebhook(context.Background(), 10, 1, domain.WebhookUpdate{TargetURL: &longURL}); !errors.Is(err, errs.ErrTargetURLTooLong) {
		t.Fatalf("long url error = %v, want ErrTargetURLTooLong", err)
	}

	padded := "  new-hook  "
	if err := svc.UpdateWebhook(context.Background(), 10, 1, domain.WebhookUpdate{Name: &padded}); err != nil {
		t.Fatalf("valid update error = %v", err)
	}
	if repo.lastUpdate.Name == nil || *repo.lastUpdate.Name != "new-hook" {
		t.Fatalf("persisted name = %v, want trimmed %q", repo.lastUpdate.Name, "new-hook")
	}
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
