package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
)

// TestDeleteModelRelation_RelationTypeInUse 锁定删除在用关系类型时的响应契约：
// 必须是 400 + code 400010 + 中文原因（含引用条数），不能落进 500。
func TestDeleteModelRelation_RelationTypeInUse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	relSvc := &stubModelRelationServiceInUse{err: &service.RelationTypeInUseError{
		RelationTypeUID: "ecs_disk",
		Count:           1304,
	}}
	h := NewRelationHandler(relSvc, nil, nil)

	r := gin.New()
	r.DELETE("/model-relations/:uid", h.DeleteModelRelation)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/model-relations/ecs_disk", http.NoBody)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, 期望 400（业务拒绝不应是 5xx）", w.Code)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是 JSON：%v body=%s", err, w.Body.String())
	}
	if resp.Code != errs.RelationTypeInUse.Code {
		t.Fatalf("code = %d, 期望 %d", resp.Code, errs.RelationTypeInUse.Code)
	}
	if !strings.Contains(resp.Msg, "1304") {
		t.Fatalf("msg = %q, 应包含引用条数 1304", resp.Msg)
	}
	if !strings.Contains(resp.Msg, "请先删除这些资产关系") {
		t.Fatalf("msg = %q, 应给出可执行的下一步", resp.Msg)
	}
	// 不能把英文哨兵透给用户
	if strings.Contains(resp.Msg, "relation type is in use") {
		t.Fatalf("msg = %q, 不应透出英文哨兵文案", resp.Msg)
	}
}

// TestDeleteModelRelation_NotFoundKeeps404 确认既有 404 语义未被本轮改动破坏。
func TestDeleteModelRelation_NotFoundKeeps404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewRelationHandler(&stubModelRelationServiceInUse{err: errs.ErrRelationNotFound}, nil, nil)
	r := gin.New()
	r.DELETE("/model-relations/:uid", h.DeleteModelRelation)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/model-relations/zz_missing", http.NoBody))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, 期望 404", w.Code)
	}
}

// stubModelRelationServiceInUse 只需覆盖 Delete。
type stubModelRelationServiceInUse struct {
	service.ModelRelationTypeService
	err error
}

func (s *stubModelRelationServiceInUse) Delete(_ context.Context, _ string) error {
	return s.err
}
