package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// fakeModelSvc 记录 Update 收到的最新模型，便于断言「显式清空」是否真的落到 model 上。
type fakeModelSvc struct {
	existing  domain.Model
	getErr    error
	updateErr error
	updated   *domain.Model
	created   *domain.Model
	createErr error
}

func (f *fakeModelSvc) Create(_ context.Context, m domain.Model) (int64, error) {
	f.created = &m
	if f.createErr != nil {
		return 0, f.createErr
	}
	return 1, nil
}

func (f *fakeModelSvc) GetByUID(_ context.Context, _ string) (domain.Model, error) {
	if f.getErr != nil {
		return domain.Model{}, f.getErr
	}
	return f.existing, nil
}

func (f *fakeModelSvc) GetByID(_ context.Context, _ int64) (domain.Model, error) {
	return f.existing, nil
}

func (f *fakeModelSvc) List(_ context.Context, _ domain.ModelFilter) ([]domain.Model, int64, error) {
	return nil, 0, nil
}

func (f *fakeModelSvc) Update(_ context.Context, m domain.Model) error {
	f.updated = &m
	return f.updateErr
}

func (f *fakeModelSvc) Delete(_ context.Context, _ string) error { return nil }

type fakeModelGroupSvc struct {
	existing  domain.ModelGroup
	getErr    error
	updateErr error
	updated   *domain.ModelGroup
}

func (f *fakeModelGroupSvc) Create(_ context.Context, g domain.ModelGroup) (int64, error) {
	return 1, nil
}
func (f *fakeModelGroupSvc) Update(_ context.Context, g domain.ModelGroup) error {
	f.updated = &g
	return f.updateErr
}
func (f *fakeModelGroupSvc) Delete(_ context.Context, _ string) error { return nil }
func (f *fakeModelGroupSvc) GetByUID(_ context.Context, _ string) (domain.ModelGroup, error) {
	if f.getErr != nil {
		return domain.ModelGroup{}, f.getErr
	}
	return f.existing, nil
}
func (f *fakeModelGroupSvc) GetByID(_ context.Context, _ int64) (domain.ModelGroup, error) {
	return f.existing, nil
}
func (f *fakeModelGroupSvc) List(_ context.Context, _ domain.ModelGroupFilter) ([]domain.ModelGroup, int64, error) {
	return nil, 0, nil
}
func (f *fakeModelGroupSvc) ListWithModels(_ context.Context) ([]domain.ModelGroupWithModels, error) {
	return nil, nil
}
func (f *fakeModelGroupSvc) InitBuiltinGroups(_ context.Context) error { return nil }

// doJSON 发一个 JSON 请求并返回状态码与响应体。
func doJSON(t *testing.T, handler gin.HandlerFunc, method, path, body string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, path, handler)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// 状态码映射唯一出口：未找到 404、冲突/重复 409、其余业务拒绝 400。
func TestModelConfigErrorStatus(t *testing.T) {
	cases := []struct {
		name string
		code int
		want int
	}{
		{"model not found", errs.ModelNotFound.Code, http.StatusNotFound},
		{"group not found", errs.ModelGroupNotFound.Code, http.StatusNotFound},
		{"model exists", errs.ModelExists.Code, http.StatusConflict},
		{"group exists", errs.ModelGroupExists.Code, http.StatusConflict},
		{"group has models", errs.GroupHasModels.Code, http.StatusConflict},
		{"model has instances", errs.ModelHasInstances.Code, http.StatusConflict},
		{"builtin cannot delete", errs.CannotDeleteBuiltin.Code, http.StatusBadRequest},
		{"model invalid", errs.ModelInvalid.Code, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := modelConfigErrorStatus(c.code); got != c.want {
				t.Fatalf("code %d → %d，期望 %d", c.code, got, c.want)
			}
		})
	}
}

// 更新不存在的模型必须是 HTTP 404（此前由 ginx.WrapBody 固定 200）。
func TestUpdateModel_NotFoundReturns404(t *testing.T) {
	h := NewModelHandler(&fakeModelSvc{getErr: errs.ErrModelNotFound})
	code, body := doJSON(t, h.Update, http.MethodPut, "/models/:uid", `{"name":"x"}`)
	if code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d，body=%s", code, body)
	}
	if !strings.Contains(body, "模型不存在") {
		t.Fatalf("缺少中文原因：%s", body)
	}
}

// 显式传空名称必须被拒绝（400 + 中文），而不是被「零值=未传」静默忽略。
func TestUpdateModel_ExplicitEmptyNameIsRejected(t *testing.T) {
	svc := &fakeModelSvc{
		existing:  domain.Model{ID: 3, UID: "host", Name: "主机"},
		updateErr: errs.ErrInvalidModelName,
	}
	h := NewModelHandler(svc)
	code, body := doJSON(t, h.Update, http.MethodPut, "/models/:uid", `{"name":""}`)
	if code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d，body=%s", code, body)
	}
	if !strings.Contains(body, "模型名称不能为空") {
		t.Fatalf("缺少中文原因：%s", body)
	}
	if svc.updated == nil || svc.updated.Name != "" {
		t.Fatalf("空名称应被传给服务层校验，实际 %+v", svc.updated)
	}
}

// model_group_id=0 表示「移回未分组」，必须真正生效（此前 `> 0` 判断把它吞掉）。
func TestUpdateModel_CanClearModelGroup(t *testing.T) {
	svc := &fakeModelSvc{existing: domain.Model{ID: 3, UID: "host", Name: "主机", ModelGroupID: 11}}
	h := NewModelHandler(svc)
	code, body := doJSON(t, h.Update, http.MethodPut, "/models/:uid", `{"model_group_id":0}`)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d，body=%s", code, body)
	}
	if svc.updated == nil || svc.updated.ModelGroupID != 0 {
		t.Fatalf("分组应被清零，实际 %+v", svc.updated)
	}
}

// 未传的字段不能改动既有值（部分更新语义）。
func TestUpdateModel_KeepsUnspecifiedFields(t *testing.T) {
	svc := &fakeModelSvc{existing: domain.Model{ID: 3, UID: "host", Name: "主机", Icon: "ic", Description: "d", ModelGroupID: 11}}
	h := NewModelHandler(svc)
	if code, body := doJSON(t, h.Update, http.MethodPut, "/models/:uid", `{}`); code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d，body=%s", code, body)
	}
	if svc.updated == nil {
		t.Fatal("未调用 Update")
	}
	got := *svc.updated
	if got.Name != "主机" || got.Icon != "ic" || got.Description != "d" || got.ModelGroupID != 11 {
		t.Fatalf("未传字段被改写：%+v", got)
	}
}

// 请求体格式错误必须返回 400 + 信封（ginx.WrapBody 时代是空 200）。
func TestCreateModel_MalformedBodyReturns400(t *testing.T) {
	h := NewModelHandler(&fakeModelSvc{})
	code, body := doJSON(t, h.Create, http.MethodPost, "/models", `{"uid":123}`)
	if code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d，body=%s", code, body)
	}
	if !strings.Contains(body, "请求体格式不正确") {
		t.Fatalf("缺少中文原因：%s", body)
	}
}

// 侧向一致性：分组 GetByUID 也必须走统一翻译（此前返回英文 model group not found）。
func TestModelGroupGetByUID_NotFoundIs404Chinese(t *testing.T) {
	h := NewModelGroupHandler(&fakeModelGroupSvc{getErr: errs.ErrModelGroupNotFound})
	code, body := doJSON(t, h.GetByUID, http.MethodGet, "/model-groups/:uid", "")
	if code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d，body=%s", code, body)
	}
	if !strings.Contains(body, "模型分组不存在") {
		t.Fatalf("缺少中文原因：%s", body)
	}
	if strings.Contains(body, "model group not found") {
		t.Fatalf("不应泄漏英文原文：%s", body)
	}
}

// 包装过的哨兵错误也要能命中 404（原先用 `err == 哨兵` 比对）。
func TestModelGroupGetByUID_WrappedNotFoundIs404(t *testing.T) {
	wrapped := &wrappedErr{msg: "failed to get group", err: errs.ErrModelGroupNotFound}
	h := NewModelGroupHandler(&fakeModelGroupSvc{getErr: wrapped})
	code, body := doJSON(t, h.GetByUID, http.MethodGet, "/model-groups/:uid", "")
	if code != http.StatusNotFound {
		t.Fatalf("包装后的未找到应为 404，实际 %d，body=%s", code, body)
	}
}

type wrappedErr struct {
	msg string
	err error
}

func (w *wrappedErr) Error() string { return w.msg + ": " + w.err.Error() }
func (w *wrappedErr) Unwrap() error { return w.err }

// 分组更新：sort_order=0（置顶）与 description=""（清空）必须真正生效。
func TestUpdateModelGroup_ZeroValuesAreApplied(t *testing.T) {
	svc := &fakeModelGroupSvc{existing: domain.ModelGroup{ID: 11, UID: "g", Name: "分组", SortOrder: 5, Description: "old"}}
	h := NewModelGroupHandler(svc)
	code, body := doJSON(t, h.Update, http.MethodPut, "/model-groups/:uid", `{"sort_order":0,"description":""}`)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d，body=%s", code, body)
	}
	if svc.updated == nil {
		t.Fatal("未调用 Update")
	}
	if svc.updated.SortOrder != 0 || svc.updated.Description != "" {
		t.Fatalf("零值字段未生效：%+v", *svc.updated)
	}
	if svc.updated.Name != "分组" {
		t.Fatalf("未传的名称被改写：%q", svc.updated.Name)
	}
}

// 分组更新：显式传纯空白名称要原样交给服务层，由 normalizeModelGroup 裁剪后拒绝。
// handler 不再二次裁剪 —— 否则 icon/description 只在服务层裁，两层口径会分叉。
func TestUpdateModelGroup_BlankNameIsRejectedByService(t *testing.T) {
	svc := &fakeModelGroupSvc{
		existing:  domain.ModelGroup{ID: 11, UID: "g", Name: "分组"},
		updateErr: errs.ErrInvalidModelGroupName,
	}
	h := NewModelGroupHandler(svc)
	code, body := doJSON(t, h.Update, http.MethodPut, "/model-groups/:uid", `{"name":"   ","icon":"   "}`)
	if code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d，body=%s", code, body)
	}
	if svc.updated == nil {
		t.Fatal("未调用 Update")
	}
	// handler 只负责「传了就覆盖」，裁剪与判空是 service.normalizeModelGroup 的职责。
	if svc.updated.Name != "   " || svc.updated.Icon != "   " {
		t.Fatalf("handler 不应二次裁剪：%+v", *svc.updated)
	}
	if !strings.Contains(body, "分组名称不能为空") {
		t.Fatalf("缺少中文原因：%s", body)
	}
}
