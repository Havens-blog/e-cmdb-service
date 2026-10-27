package web

import (
	"strconv"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-common-go/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

// OpenAPIHandler 开放 API 管理 HTTP 处理器。
type OpenAPIHandler struct {
	svc service.OpenAPIService
}

// NewOpenAPIHandler 创建开放 API 管理处理器。
func NewOpenAPIHandler(svc service.OpenAPIService) *OpenAPIHandler {
	return &OpenAPIHandler{svc: svc}
}

// RegisterRoutes 注册 /open-api 下的路由。Token 与 Webhook 均属租户作用域，
// 因此整个子组挂 RequireTenant。
func (h *OpenAPIHandler) RegisterRoutes(r *gin.RouterGroup) {
	g := r.Group("/open-api")
	g.Use(middleware.RequireTenant(elog.DefaultLogger))
	{
		g.POST("/tokens", ginx.WrapBody[CreateTokenReq](h.CreateToken))
		g.GET("/tokens", ginx.Wrap(h.ListTokens))
		g.DELETE("/tokens/:id", h.RevokeToken)

		g.POST("/webhooks", ginx.WrapBody[CreateWebhookReq](h.CreateWebhook))
		g.GET("/webhooks", ginx.Wrap(h.ListWebhooks))
		g.PUT("/webhooks/:id", ginx.WrapBody[UpdateWebhookReq](h.UpdateWebhook))
		g.DELETE("/webhooks/:id", h.DeleteWebhook)
	}
}

// CreateTokenReq 创建 API Token 请求。
type CreateTokenReq struct {
	Name    string   `json:"name" binding:"required"`
	Scopes  []string `json:"scopes"`
	TTLDays int      `json:"ttl_days"`
}

// CreateWebhookReq 创建 Webhook 请求。
type CreateWebhookReq struct {
	Name       string   `json:"name" binding:"required"`
	EventTypes []string `json:"event_types" binding:"required"`
	TargetURL  string   `json:"target_url" binding:"required"`
	Secret     string   `json:"secret"`
}

// UpdateWebhookReq Webhook 部分更新请求。
type UpdateWebhookReq struct {
	Name       *string  `json:"name"`
	EventTypes []string `json:"event_types"`
	TargetURL  *string  `json:"target_url"`
	Secret     *string  `json:"secret"`
	Status     *string  `json:"status"`
}

// CreatedTokenVO 创建成功返回的 Token；明文仅在此返回一次。
type CreatedTokenVO struct {
	Token     string   `json:"token"`
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpireAt  int64    `json:"expire_at"`
	CreatedAt int64    `json:"created_at"`
}

// APITokenVO API Token 列表项；不含明文或哈希。
type APITokenVO struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	Status     string   `json:"status"`
	ExpireAt   int64    `json:"expire_at"`
	CreateTime int64    `json:"create_time"`
}

// APITokenListResp API Token 列表响应。
type APITokenListResp struct {
	Items []APITokenVO `json:"items"`
	Total int64        `json:"total"`
}

// WebhookVO Webhook 列表项；不含 secret。
type WebhookVO struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	EventTypes []string `json:"event_types"`
	TargetURL  string   `json:"target_url"`
	Status     string   `json:"status"`
	CreateTime int64    `json:"create_time"`
	UpdateTime int64    `json:"update_time"`
}

// WebhookListResp Webhook 列表响应。
type WebhookListResp struct {
	Items []WebhookVO `json:"items"`
	Total int64       `json:"total"`
}

// CreateToken 创建 API Token。
func (h *OpenAPIHandler) CreateToken(ctx *gin.Context, req CreateTokenReq) (ginx.Result, error) {
	token, plain, err := h.svc.CreateToken(ctx.Request.Context(), middleware.GetTenantID(ctx), req.Name, req.Scopes, req.TTLDays)
	if err != nil {
		return ErrorResultWithMsg(errs.ParamsError, err.Error()), nil
	}

	return Result(CreatedTokenVO{
		Token:     plain,
		ID:        token.ID,
		Name:      token.Name,
		Scopes:    token.Scopes,
		ExpireAt:  token.ExpireAt,
		CreatedAt: token.CreateTime.UnixMilli(),
	}), nil
}

// ListTokens 分页查询 API Token。
func (h *OpenAPIHandler) ListTokens(ctx *gin.Context) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	tokens, total, err := h.svc.ListTokens(ctx.Request.Context(), tenantID, offset, limit)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}

	items := make([]APITokenVO, len(tokens))
	for i, token := range tokens {
		items[i] = APITokenVO{
			ID:         token.ID,
			Name:       token.Name,
			Scopes:     token.Scopes,
			Status:     token.Status,
			ExpireAt:   token.ExpireAt,
			CreateTime: token.CreateTime.UnixMilli(),
		}
	}
	return Result(APITokenListResp{Items: items, Total: total}), nil
}

// RevokeToken 吊销 API Token。
func (h *OpenAPIHandler) RevokeToken(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	err = h.svc.RevokeToken(ctx.Request.Context(), middleware.GetTenantID(ctx), id)
	if err != nil {
		if err == errs.ErrAPITokenNotFound {
			ctx.JSON(404, ErrorResult(errs.OpenAPITokenNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}
	ctx.JSON(200, Result(nil))
}

// CreateWebhook 创建 Webhook 订阅。
func (h *OpenAPIHandler) CreateWebhook(ctx *gin.Context, req CreateWebhookReq) (ginx.Result, error) {
	id, err := h.svc.CreateWebhook(ctx.Request.Context(), middleware.GetTenantID(ctx), req.Name, req.EventTypes, req.TargetURL, req.Secret)
	if err != nil {
		return ErrorResultWithMsg(errs.ParamsError, err.Error()), nil
	}
	return Result(map[string]interface{}{"id": id}), nil
}

// ListWebhooks 分页查询 Webhook 订阅。
func (h *OpenAPIHandler) ListWebhooks(ctx *gin.Context) (ginx.Result, error) {
	tenantID := middleware.GetTenantID(ctx)
	offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	webhooks, total, err := h.svc.ListWebhooks(ctx.Request.Context(), tenantID, offset, limit)
	if err != nil {
		return ErrorResultWithMsg(errs.SystemError, err.Error()), err
	}

	items := make([]WebhookVO, len(webhooks))
	for i, webhook := range webhooks {
		items[i] = WebhookVO{
			ID:         webhook.ID,
			Name:       webhook.Name,
			EventTypes: webhook.EventTypes,
			TargetURL:  webhook.TargetURL,
			Status:     webhook.Status,
			CreateTime: webhook.CreateTime.UnixMilli(),
			UpdateTime: webhook.UpdateTime.UnixMilli(),
		}
	}
	return Result(WebhookListResp{Items: items, Total: total}), nil
}

// UpdateWebhook 部分更新 Webhook 订阅。
func (h *OpenAPIHandler) UpdateWebhook(ctx *gin.Context, req UpdateWebhookReq) (ginx.Result, error) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		return ErrorResult(errs.ParamsError), nil
	}

	update := domain.WebhookUpdate{
		Name:       req.Name,
		EventTypes: req.EventTypes,
		TargetURL:  req.TargetURL,
		Secret:     req.Secret,
		Status:     req.Status,
	}

	err = h.svc.UpdateWebhook(ctx.Request.Context(), middleware.GetTenantID(ctx), id, update)
	if err != nil {
		if err == errs.ErrWebhookNotFound {
			return ErrorResult(errs.OpenAPIWebhookNotFound), nil
		}
		return ErrorResultWithMsg(errs.ParamsError, err.Error()), nil
	}
	return Result(nil), nil
}

// DeleteWebhook 删除 Webhook 订阅。
func (h *OpenAPIHandler) DeleteWebhook(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(400, ErrorResult(errs.ParamsError))
		return
	}

	err = h.svc.DeleteWebhook(ctx.Request.Context(), middleware.GetTenantID(ctx), id)
	if err != nil {
		if err == errs.ErrWebhookNotFound {
			ctx.JSON(404, ErrorResult(errs.OpenAPIWebhookNotFound))
			return
		}
		ctx.JSON(500, ErrorResultWithMsg(errs.SystemError, err.Error()))
		return
	}
	ctx.JSON(200, Result(nil))
}
