// Package internal 装配 CMDB 独立服务的各层组件。
package internal

import (
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-cmdb-service/internal/web"
	"github.com/Havens-blog/e-common-go/mongox"
	"github.com/ecodeclub/ginx/session"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

// Module CMDB 模块（独立服务版）
type Module struct {
	InstanceHandler   *web.InstanceHandler
	ModelHandler      *web.ModelHandler
	RelationHandler   *web.RelationHandler
	ModelGroupHandler *web.ModelGroupHandler
	AttributeHandler  *web.AttributeHandler
	QualityHandler    *web.QualityHandler
	OpenAPIHandler    *web.OpenAPIHandler
	ImpactHandler     *web.ImpactHandler

	// 对外暴露 service 与 repo，供内部 HTTP API（供 e-cam-service 远程调用）复用。
	InstanceSvc  service.InstanceService
	InstanceRepo repository.InstanceRepository
	InternalAPI  *web.InternalAPIHandler
}

// InitModule 初始化 CMDB 模块
func InitModule(db *mongox.Mongo) *Module {
	// DAO
	instanceDAO := dao.NewInstanceDAO(db)
	modelDAO := dao.NewModelDAO(db)
	relationDAO := dao.NewInstanceRelationDAO(db)
	modelRelDAO := dao.NewModelRelationTypeDAO(db)
	modelGroupDAO := dao.NewModelGroupDAO(db)
	attributeDAO := dao.NewAttributeDAO(db)
	attributeGroupDAO := dao.NewAttributeGroupDAO(db)
	apiTokenDAO := dao.NewApiTokenDAO(db)
	webhookDAO := dao.NewWebhookDAO(db)

	// Repository
	instanceRepo := repository.NewInstanceRepository(instanceDAO)
	modelRepo := repository.NewModelRepository(modelDAO)
	relationRepo := repository.NewInstanceRelationRepository(relationDAO)
	modelRelRepo := repository.NewModelRelationTypeRepository(modelRelDAO)
	modelGroupRepo := repository.NewModelGroupRepository(modelGroupDAO)
	attributeRepo := repository.NewAttributeRepository(attributeDAO)
	attributeGroupRepo := repository.NewAttributeGroupRepository(attributeGroupDAO)
	apiTokenRepo := repository.NewApiTokenRepository(apiTokenDAO)
	webhookRepo := repository.NewWebhookRepository(webhookDAO)

	// Service
	// instanceSvc 注入 attributeRepo：Create 时按模型字段定义做必填硬拦校验
	instanceSvc := service.NewInstanceService(instanceRepo, modelRepo, attributeRepo)
	modelSvc := service.NewModelService(modelRepo, instanceRepo)
	relationSvc := service.NewRelationService(relationRepo)
	modelRelSvc := service.NewModelRelationTypeService(modelRelRepo)
	topologySvc := service.NewTopologyService(instanceRepo, relationRepo, modelRepo, modelRelRepo)
	modelGroupSvc := service.NewModelGroupService(modelGroupRepo, modelRepo)
	attributeSvc := service.NewAttributeService(attributeRepo, attributeGroupRepo, modelRepo)
	qualitySvc := service.NewQualityService(instanceRepo, modelRepo, relationRepo, attributeRepo)
	openAPISvc := service.NewOpenAPIService(apiTokenRepo, webhookRepo)
	impactSvc := service.NewImpactService(instanceRepo, relationRepo)

	// Handler
	instanceHandler := web.NewInstanceHandler(instanceSvc)
	modelHandler := web.NewModelHandler(modelSvc)
	relationHandler := web.NewRelationHandler(modelRelSvc, relationSvc, topologySvc)
	modelGroupHandler := web.NewModelGroupHandler(modelGroupSvc)
	attributeHandler := web.NewAttributeHandler(attributeSvc)
	qualityHandler := web.NewQualityHandler(qualitySvc)
	openAPIHandler := web.NewOpenAPIHandler(openAPISvc)
	impactHandler := web.NewImpactHandler(impactSvc)
	internalAPI := web.NewInternalAPIHandler(instanceRepo, modelRepo)

	return &Module{
		InstanceHandler:   instanceHandler,
		ModelHandler:      modelHandler,
		RelationHandler:   relationHandler,
		ModelGroupHandler: modelGroupHandler,
		AttributeHandler:  attributeHandler,
		QualityHandler:    qualityHandler,
		OpenAPIHandler:    openAPIHandler,
		ImpactHandler:     impactHandler,
		InstanceSvc:       instanceSvc,
		InstanceRepo:      instanceRepo,
		InternalAPI:       internalAPI,
	}
}

// RegisterRoutes 在 /api/v1/cmdb 前缀下注册 CMDB 路由。
//
// 两类路由分组：
//   - 面向终端用户：经 SessionAuth 本地校验平台共享会话（eiam 签发），租户取自
//     会话 claims（客户端自报的 X-Tenant-Id 一律不采纳），租户边界由各 handler
//     内 RequireTenant 承接。
//   - 面向服务间（/internal）：供 e-cam-service 的 RemoteCMDBAdapter 调用，经 S2SAuth
//     验 eiam 签发的服务令牌（eiam 唯一签发方）；租户由调用方显式传参。
func (m *Module) RegisterRoutes(server *gin.Engine, sp session.Provider, s2sPublicKeyPEM string) {
	g := server.Group("/api/v1/cmdb", middleware.SessionAuth(sp, elog.DefaultLogger))
	m.InstanceHandler.RegisterRoutes(g)
	m.ModelHandler.RegisterRoutes(g)
	m.RelationHandler.RegisterRoutes(g)
	m.ModelGroupHandler.RegisterRoutes(g)
	m.AttributeHandler.RegisterRoutes(g)
	m.QualityHandler.RegisterRoutes(g)
	m.OpenAPIHandler.RegisterRoutes(g)
	m.ImpactHandler.RegisterRoutes(g)

	// 内部服务间 API：S2S 令牌验签（eiam 唯一签发方），租户由请求显式传参
	internal := server.Group("/api/v1/cmdb", middleware.S2SAuth(s2sPublicKeyPEM))
	m.InternalAPI.RegisterRoutes(internal)
}
