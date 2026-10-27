package service

import (
	"context"
	"errors"
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
	nextID  int64
	created domain.Webhook
	stored  []domain.Webhook
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

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
