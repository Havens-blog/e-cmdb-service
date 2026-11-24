package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
)

// stubDeriveRelationService 只实现 DeriveRelations（其余方法仅满足接口，
// 本用例不会触达；用 panic 让误用立刻暴露）。
type stubDeriveRelationService struct {
	result domain.DeriveResult
	err    error
	calls  int
}

func (s *stubDeriveRelationService) Create(context.Context, domain.InstanceRelation) (int64, error) {
	panic("Create 不应被调用")
}

func (s *stubDeriveRelationService) CreateBatch(context.Context, []domain.InstanceRelation) (int64, error) {
	panic("CreateBatch 不应被调用")
}

func (s *stubDeriveRelationService) GetByID(context.Context, int64, int64) (domain.InstanceRelation, error) {
	panic("GetByID 不应被调用")
}

func (s *stubDeriveRelationService) List(context.Context, domain.InstanceRelationFilter) ([]domain.InstanceRelation, int64, error) {
	panic("List 不应被调用")
}

func (s *stubDeriveRelationService) ListDetailed(
	context.Context, domain.InstanceRelationFilter,
) ([]domain.InstanceRelationDetail, int64, error) {
	panic("ListDetailed 不应被调用")
}

func (s *stubDeriveRelationService) Delete(context.Context, int64, int64) error {
	panic("Delete 不应被调用")
}

func (s *stubDeriveRelationService) DeleteByInstanceID(context.Context, int64, int64) error {
	panic("DeleteByInstanceID 不应被调用")
}

func (s *stubDeriveRelationService) DeriveRelations(
	_ context.Context, _ int64, _ string, _ bool,
) (domain.DeriveResult, error) {
	s.calls++
	return s.result, s.err
}

func deriveContext(t *testing.T, tenantID int64) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/instance-relations/derive", http.NoBody)
	if tenantID != 0 {
		c.Set(middleware.TenantIDKey, tenantID)
	}
	return c
}

// TestDeriveInstanceRelations_RejectsUnselectedTenant 锁定推导入口的租户守卫文案。
//
// 缺陷背景（评审指出，2026-11-24）：该分支此前回英文 "tenant not resolved from session"，
// 而同一文件其余关系入口都是中文可行动文案；推导由定时任务/脚本直接调用 service，
// 未选租户是调用方配置问题，必须给出可操作提示。
func TestDeriveInstanceRelations_RejectsUnselectedTenant(t *testing.T) {
	svc := &stubDeriveRelationService{}
	h := NewRelationHandler(nil, svc, nil)

	res, err := h.DeriveInstanceRelations(deriveContext(t, 0), DeriveInstanceRelationsReq{})
	if err != nil {
		t.Fatalf("业务拒绝不应返回 error: %v", err)
	}
	if res.Code != errs.ParamsError.Code {
		t.Fatalf("code = %d, 期望 %d", res.Code, errs.ParamsError.Code)
	}
	if !strings.Contains(res.Msg, "未选择租户") || !strings.Contains(res.Msg, "具体租户") {
		t.Fatalf("msg 应给出可操作中文提示，实际 %q", res.Msg)
	}
	if svc.calls != 0 {
		t.Fatalf("租户未选定即应早退，实际调用 service %d 次", svc.calls)
	}
}

// TestDeriveInstanceRelations_MapsServiceErrors 锁定推导入口接入了统一错误映射。
//
// 缺陷背景（评审指出，2026-11-24）：该入口此前一律 "SystemError + err.Error()"，
// 关系类型被删除（ErrRelationNotFound）这类可修复问题会被回成 5xx 英文原文，
// 前端只能提示「服务器内部错误」。
func TestDeriveInstanceRelations_MapsServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "关系类型不存在走 404 段",
			err:      errors.Join(errs.ErrRelationNotFound, errors.New("relation_type_uid=zz_none")),
			wantCode: errs.RelationNotFound.Code,
			wantMsg:  "关系类型不存在",
		},
		{
			name:     "未选租户（service 层守卫）走参数错误",
			err:      errors.Join(errs.ErrInvalidTenantID, errors.New("tenant_id=0")),
			wantCode: errs.ParamsError.Code,
			wantMsg:  "未选择租户",
		},
		{
			name:     "其它错误回落系统错误",
			err:      errors.New("mongo: connection refused"),
			wantCode: errs.SystemError.Code,
			wantMsg:  "mongo: connection refused",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubDeriveRelationService{err: tc.err}
			h := NewRelationHandler(nil, svc, nil)

			res, err := h.DeriveInstanceRelations(deriveContext(t, 3), DeriveInstanceRelationsReq{})
			if err != nil {
				t.Fatalf("handler 不应把业务错误当 error 返回: %v", err)
			}
			if res.Code != tc.wantCode {
				t.Fatalf("code = %d, 期望 %d", res.Code, tc.wantCode)
			}
			if !strings.Contains(res.Msg, tc.wantMsg) {
				t.Fatalf("msg = %q, 期望包含 %q", res.Msg, tc.wantMsg)
			}
		})
	}
}

// TestDeriveInstanceRelations_PassesDryRun 锁定 dry_run 缺省为 true（安全优先）。
func TestDeriveInstanceRelations_PassesDryRun(t *testing.T) {
	var gotDryRun *bool
	svc := &capturingDeriveService{onDerive: func(dryRun bool) { gotDryRun = &dryRun }}
	h := NewRelationHandler(nil, svc, nil)

	if _, err := h.DeriveInstanceRelations(deriveContext(t, 3), DeriveInstanceRelationsReq{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotDryRun == nil || !*gotDryRun {
		t.Fatalf("未显式传 dry_run 时应默认 true，实际 %v", gotDryRun)
	}

	no := false
	if _, err := h.DeriveInstanceRelations(deriveContext(t, 3), DeriveInstanceRelationsReq{DryRun: &no}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotDryRun == nil || *gotDryRun {
		t.Fatalf("显式 dry_run=false 应透传，实际 %v", gotDryRun)
	}
}

type capturingDeriveService struct {
	stubDeriveRelationService
	onDerive func(dryRun bool)
}

func (s *capturingDeriveService) DeriveRelations(
	_ context.Context, _ int64, _ string, dryRun bool,
) (domain.DeriveResult, error) {
	s.onDerive(dryRun)
	return domain.DeriveResult{}, nil
}
