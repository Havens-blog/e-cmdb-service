// Package internal 装配 CMDB 独立服务的各层组件。
package internal

import (
	"github.com/Havens-blog/e-cmdb-service/internal/middleware"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
	"github.com/Havens-blog/e-cmdb-service/internal/service"
	"github.com/Havens-blog/e-cmdb-service/internal/web"
	"github.com/Havens-blog/e-common-go/mongox"
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

	// Repository
	instanceRepo := repository.NewInstanceRepository(instanceDAO)
	modelRepo := repository.NewModelRepository(modelDAO)
	relationRepo := repository.NewInstanceRelationRepository(relationDAO)
	modelRelRepo := repository.NewModelRelationTypeRepository(modelRelDAO)
	modelGroupRepo := repository.NewModelGroupRepository(modelGroupDAO)
	attributeRepo := repository.NewAttributeRepository(attributeDAO)
	attributeGroupRepo := repository.NewAttributeGroupRepository(attributeGroupDAO)

	// Service
	instanceSvc := service.NewInstanceService(instanceRepo)
	modelSvc := service.NewModelService(modelRepo)
	relationSvc := service.NewRelationService(relationRepo)
	modelRelSvc := service.NewModelRelationTypeService(modelRelRepo)
	topologySvc := service.NewTopologyService(instanceRepo, relationRepo, modelRepo, modelRelRepo)
	modelGroupSvc := service.NewModelGroupService(modelGroupRepo, modelRepo)
	attributeSvc := service.NewAttributeService(attributeRepo, attributeGroupRepo, modelRepo)

	// Handler
	instanceHandler := web.NewInstanceHandler(instanceSvc)
	modelHandler := web.NewModelHandler(modelSvc)
	relationHandler := web.NewRelationHandler(modelRelSvc, relationSvc, topologySvc)
	modelGroupHandler := web.NewModelGroupHandler(modelGroupSvc)
	attributeHandler := web.NewAttributeHandler(attributeSvc)
	internalAPI := web.NewInternalAPIHandler(instanceRepo)

	return &Module{
		InstanceHandler:   instanceHandler,
		ModelHandler:      modelHandler,
		RelationHandler:   relationHandler,
		ModelGroupHandler: modelGroupHandler,
		AttributeHandler:  attributeHandler,
		InstanceSvc:       instanceSvc,
		InstanceRepo:      instanceRepo,
		InternalAPI:       internalAPI,
	}
}

// RegisterRoutes 在 /api/v1/cmdb 前缀下注册 CMDB 路由。
//
// 两类路由分组：
//   - 面向终端用户：经 GatewayAuth 透传租户，租户边界由各 handler 内 RequireTenant 承接。
//   - 面向服务间（/internal）：供 e-cam-service 的 RemoteCMDBAdapter 调用，租户由调用方
//     显式传参，不走网关租户中间件（调用方已在其网关后完成会话校验）。
func (m *Module) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/api/v1/cmdb", middleware.GatewayAuth(elog.DefaultLogger))
	m.InstanceHandler.RegisterRoutes(g)
	m.ModelHandler.RegisterRoutes(g)
	m.RelationHandler.RegisterRoutes(g)
	m.ModelGroupHandler.RegisterRoutes(g)
	m.AttributeHandler.RegisterRoutes(g)

	// 内部服务间 API（无网关租户中间件）
	internal := server.Group("/api/v1/cmdb")
	m.InternalAPI.RegisterRoutes(internal)
}
