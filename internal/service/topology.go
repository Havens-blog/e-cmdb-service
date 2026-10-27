package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// TopologyService 拓扑服务接口
type TopologyService interface {
	// GetInstanceTopology 获取实例拓扑图
	GetInstanceTopology(ctx context.Context, query domain.TopologyQuery) (*domain.TopologyGraph, error)
	// GetModelTopology 获取模型拓扑图（模型间的关系定义）
	GetModelTopology(ctx context.Context, provider string) (*domain.ModelTopologyGraph, error)
	// GetRelatedInstances 获取关联实例列表。
	// tenantID 必须非 0：查询按租户隔离，0 不作「全部租户」通配。
	GetRelatedInstances(ctx context.Context, tenantID int64, instanceID int64, relationTypeUID string) ([]domain.Instance, error)
}

type topologyService struct {
	instanceRepo repository.InstanceRepository
	relationRepo repository.InstanceRelationRepository
	modelRepo    repository.ModelRepository
	modelRelRepo repository.ModelRelationTypeRepository
	logger       *elog.Component
}

// NewTopologyService 创建拓扑服务
func NewTopologyService(
	instanceRepo repository.InstanceRepository,
	relationRepo repository.InstanceRelationRepository,
	modelRepo repository.ModelRepository,
	modelRelRepo repository.ModelRelationTypeRepository,
) TopologyService {
	return &topologyService{
		instanceRepo: instanceRepo,
		relationRepo: relationRepo,
		modelRepo:    modelRepo,
		modelRelRepo: modelRelRepo,
		logger:       elog.DefaultLogger,
	}
}

// GetInstanceTopology 获取实例拓扑图
func (s *topologyService) GetInstanceTopology(ctx context.Context, query domain.TopologyQuery) (*domain.TopologyGraph, error) {
	graph := &domain.TopologyGraph{
		Nodes: make([]domain.TopologyNode, 0),
		Edges: make([]domain.TopologyEdge, 0),
	}

	// 获取起始实例
	instance, err := s.instanceRepo.GetByID(ctx, query.TenantID, query.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}
	if instance.ID == 0 {
		return graph, nil
	}

	// 获取模型信息
	model, _ := s.modelRepo.GetByUID(ctx, instance.ModelUID)

	// 添加起始节点
	graph.Nodes = append(graph.Nodes, s.instanceToNode(instance, model))

	// 递归获取关联实例
	visited := make(map[int64]bool)
	visited[instance.ID] = true

	if err := s.expandTopology(ctx, graph, instance.ID, query, visited, 1); err != nil {
		return nil, err
	}

	return graph, nil
}

// expandTopology 递归展开拓扑
func (s *topologyService) expandTopology(ctx context.Context, graph *domain.TopologyGraph, instanceID int64, query domain.TopologyQuery, visited map[int64]bool, depth int) error {
	if query.Depth > 0 && depth > query.Depth {
		return nil
	}

	// 获取出向关系
	if query.Direction == "" || query.Direction == "both" || query.Direction == "outgoing" {
		outFilter := domain.InstanceRelationFilter{
			SourceInstanceID: instanceID,
			TenantID:         query.TenantID,
		}
		outRelations, err := s.relationRepo.List(ctx, outFilter)
		if err != nil {
			return fmt.Errorf("failed to get outgoing relations: %w", err)
		}

		for _, rel := range outRelations {
			if visited[rel.TargetInstanceID] {
				continue
			}

			targetInstance, err := s.instanceRepo.GetByID(ctx, query.TenantID, rel.TargetInstanceID)
			if err != nil || targetInstance.ID == 0 {
				continue
			}

			// 按模型过滤
			if query.ModelUID != "" && targetInstance.ModelUID != query.ModelUID {
				continue
			}

			visited[rel.TargetInstanceID] = true
			model, _ := s.modelRepo.GetByUID(ctx, targetInstance.ModelUID)
			graph.Nodes = append(graph.Nodes, s.instanceToNode(targetInstance, model))

			// 获取关系类型信息
			relType, _ := s.modelRelRepo.GetByUID(ctx, rel.RelationTypeUID)
			graph.Edges = append(graph.Edges, domain.TopologyEdge{
				SourceID:        instanceID,
				TargetID:        rel.TargetInstanceID,
				RelationTypeUID: rel.RelationTypeUID,
				RelationName:    relType.Name,
				RelationType:    relType.RelationType,
			})

			// 递归展开
			if err := s.expandTopology(ctx, graph, rel.TargetInstanceID, query, visited, depth+1); err != nil {
				return err
			}
		}
	}

	// 获取入向关系
	if query.Direction == "" || query.Direction == "both" || query.Direction == "incoming" {
		inFilter := domain.InstanceRelationFilter{
			TargetInstanceID: instanceID,
			TenantID:         query.TenantID,
		}
		inRelations, err := s.relationRepo.List(ctx, inFilter)
		if err != nil {
			return fmt.Errorf("failed to get incoming relations: %w", err)
		}

		for _, rel := range inRelations {
			if visited[rel.SourceInstanceID] {
				continue
			}

			sourceInstance, err := s.instanceRepo.GetByID(ctx, query.TenantID, rel.SourceInstanceID)
			if err != nil || sourceInstance.ID == 0 {
				continue
			}

			if query.ModelUID != "" && sourceInstance.ModelUID != query.ModelUID {
				continue
			}

			visited[rel.SourceInstanceID] = true
			model, _ := s.modelRepo.GetByUID(ctx, sourceInstance.ModelUID)
			graph.Nodes = append(graph.Nodes, s.instanceToNode(sourceInstance, model))

			relType, _ := s.modelRelRepo.GetByUID(ctx, rel.RelationTypeUID)
			graph.Edges = append(graph.Edges, domain.TopologyEdge{
				SourceID:        rel.SourceInstanceID,
				TargetID:        instanceID,
				RelationTypeUID: rel.RelationTypeUID,
				RelationName:    relType.Name,
				RelationType:    relType.RelationType,
			})

			if err := s.expandTopology(ctx, graph, rel.SourceInstanceID, query, visited, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetModelTopology 获取模型拓扑图
func (s *topologyService) GetModelTopology(ctx context.Context, provider string) (*domain.ModelTopologyGraph, error) {
	graph := &domain.ModelTopologyGraph{
		Nodes: make([]domain.ModelTopologyNode, 0),
		Edges: make([]domain.ModelTopologyEdge, 0),
	}

	// 获取所有模型
	modelFilter := domain.ModelFilter{Provider: provider, Limit: 1000}
	models, err := s.modelRepo.List(ctx, modelFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	modelMap := make(map[string]domain.Model)
	for _, m := range models {
		modelMap[m.UID] = m
		graph.Nodes = append(graph.Nodes, domain.ModelTopologyNode{
			UID:      m.UID,
			Name:     m.Name,
			Category: m.Category,
			Provider: m.Provider,
			Icon:     m.Icon,
		})
	}

	// 获取所有模型关系
	relFilter := domain.ModelRelationTypeFilter{Limit: 1000}
	relations, err := s.modelRelRepo.List(ctx, relFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list model relations: %w", err)
	}

	for _, rel := range relations {
		// 过滤掉不在当前 provider 范围内的关系
		if provider != "" {
			sourceModel, ok1 := modelMap[rel.SourceModelUID]
			targetModel, ok2 := modelMap[rel.TargetModelUID]
			if !ok1 || !ok2 {
				continue
			}
			if sourceModel.Provider != provider && sourceModel.Provider != "all" {
				continue
			}
			if targetModel.Provider != provider && targetModel.Provider != "all" {
				continue
			}
		}

		graph.Edges = append(graph.Edges, domain.ModelTopologyEdge{
			SourceModelUID: rel.SourceModelUID,
			TargetModelUID: rel.TargetModelUID,
			RelationUID:    rel.UID,
			RelationName:   rel.Name,
			RelationType:   rel.RelationType,
		})
	}

	return graph, nil
}

// GetRelatedInstances 获取关联实例列表
func (s *topologyService) GetRelatedInstances(ctx context.Context, tenantID int64, instanceID int64, relationTypeUID string) ([]domain.Instance, error) {
	// TenantID 必须显式带上：同文件另两处（:89、:136）都带了，唯独此处漏了，
	// 而 dao/relation.go:193 是 `if filter.TenantID != 0 { 加租户谓词 }` ——
	// 漏传会丢弃租户谓词，按 instance_id 读到**其他租户**的关联关系。
	filter := domain.InstanceRelationFilter{
		SourceInstanceID: instanceID,
		RelationTypeUID:  relationTypeUID,
		TenantID:         tenantID,
		Limit:            100,
	}

	relations, err := s.relationRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list relations: %w", err)
	}

	instances := make([]domain.Instance, 0, len(relations))
	for _, rel := range relations {
		inst, err := s.instanceRepo.GetByID(ctx, tenantID, rel.TargetInstanceID)
		if err != nil || inst.ID == 0 {
			continue
		}
		instances = append(instances, inst)
	}

	return instances, nil
}

func (s *topologyService) instanceToNode(inst domain.Instance, model domain.Model) domain.TopologyNode {
	return domain.TopologyNode{
		ID:         inst.ID,
		ModelUID:   inst.ModelUID,
		ModelName:  model.Name,
		AssetID:    inst.AssetID,
		AssetName:  inst.AssetName,
		Attributes: inst.Attributes,
		Icon:       model.Icon,
		Category:   model.Category,
	}
}
