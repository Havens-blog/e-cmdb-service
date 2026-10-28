package service

import (
	"context"
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// ImpactService 影响分析（blast radius）服务接口。
type ImpactService interface {
	// GetImpact 计算实例的影响分析图。
	// tenantID 必须非 0：遍历按租户隔离（relation/instance 查询都带 TenantID），
	// 0 不作「全部租户」通配。depth 为限深（1..N），depth<1 时按 1 处理。
	GetImpact(ctx context.Context, tenantID int64, instanceID int64, depth int) (*domain.ImpactGraph, error)
}

type impactService struct {
	instanceRepo repository.InstanceRepository
	relationRepo repository.InstanceRelationRepository
	logger       *elog.Component
}

// NewImpactService 创建影响分析服务。
func NewImpactService(
	instanceRepo repository.InstanceRepository,
	relationRepo repository.InstanceRelationRepository,
) ImpactService {
	return &impactService{
		instanceRepo: instanceRepo,
		relationRepo: relationRepo,
		logger:       elog.DefaultLogger,
	}
}

// GetImpact 计算实例的影响分析图。
//
// 语义假设（待产品确认）：
//   - root 节点 depth=0、direction 为空。
//   - downstream：沿出向关系（root 是 source）BFS；upstream：沿入向关系（root 是 target）BFS。
//   - 节点的 depth 为与 root 的最短距离（BFS 天然保证），范围 1..depth。
//   - upstream_count/downstream_count 分别为上游/下游节点数（不含 root）。
//   - 两条方向的遍历各自维护 visited，同一实例若同时沿上下游可达会出现两条节点记录
//     （direction 不同），不做跨方向去重。
//   - root 不存在或跨租户时返回空图（root 零值 + 空 nodes/edges + 0 计数），不报错。
func (s *impactService) GetImpact(ctx context.Context, tenantID int64, instanceID int64, depth int) (*domain.ImpactGraph, error) {
	graph := &domain.ImpactGraph{
		Nodes: make([]domain.ImpactNode, 0),
		Edges: make([]domain.ImpactEdge, 0),
	}

	if depth < 1 {
		depth = 1
	}

	root, err := s.instanceRepo.GetByID(ctx, tenantID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get root instance: %w", err)
	}
	if root.ID == 0 {
		return graph, nil
	}
	graph.Root = instanceToImpactNode(root, 0, "")

	downstreamNodes, downstreamEdges, err := s.expandImpact(ctx, tenantID, root.ID, depth, domain.ImpactDirectionDownstream)
	if err != nil {
		return nil, err
	}
	upstreamNodes, upstreamEdges, err := s.expandImpact(ctx, tenantID, root.ID, depth, domain.ImpactDirectionUpstream)
	if err != nil {
		return nil, err
	}

	graph.Nodes = append(graph.Nodes, downstreamNodes...)
	graph.Nodes = append(graph.Nodes, upstreamNodes...)
	graph.Edges = append(graph.Edges, downstreamEdges...)
	graph.Edges = append(graph.Edges, upstreamEdges...)
	graph.DownstreamCount = len(downstreamNodes)
	graph.UpstreamCount = len(upstreamNodes)

	return graph, nil
}

// expandImpact 沿单一方向（downstream 或 upstream）做限深 BFS，返回该方向的节点与边。
func (s *impactService) expandImpact(ctx context.Context, tenantID int64, rootID int64, maxDepth int, direction string) ([]domain.ImpactNode, []domain.ImpactEdge, error) {
	nodes := make([]domain.ImpactNode, 0)
	edges := make([]domain.ImpactEdge, 0)

	type queueItem struct {
		id    int64
		depth int
	}

	visited := map[int64]bool{rootID: true}
	queue := []queueItem{{id: rootID, depth: 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		// 已达限深，不再继续向外扩展。
		if cur.depth >= maxDepth {
			continue
		}
		nextDepth := cur.depth + 1

		var (
			rels []domain.InstanceRelation
			err  error
		)
		if direction == domain.ImpactDirectionDownstream {
			rels, err = s.relationRepo.List(ctx, domain.InstanceRelationFilter{
				SourceInstanceID: cur.id,
				TenantID:         tenantID,
			})
		} else {
			rels, err = s.relationRepo.List(ctx, domain.InstanceRelationFilter{
				TargetInstanceID: cur.id,
				TenantID:         tenantID,
			})
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list %s relations: %w", direction, err)
		}

		for _, rel := range rels {
			var neighborID int64
			if direction == domain.ImpactDirectionDownstream {
				neighborID = rel.TargetInstanceID
			} else {
				neighborID = rel.SourceInstanceID
			}
			if visited[neighborID] {
				continue
			}

			inst, err := s.instanceRepo.GetByID(ctx, tenantID, neighborID)
			if err != nil || inst.ID == 0 {
				// 实例不存在或跨租户：不计入影响图。
				continue
			}

			visited[neighborID] = true
			nodes = append(nodes, instanceToImpactNode(inst, nextDepth, direction))
			// source_id/target_id 保持原始关系的 source/target，direction 标注遍历方向。
			edges = append(edges, domain.ImpactEdge{
				SourceID:        rel.SourceInstanceID,
				TargetID:        rel.TargetInstanceID,
				RelationTypeUID: rel.RelationTypeUID,
				Direction:       direction,
			})
			queue = append(queue, queueItem{id: neighborID, depth: nextDepth})
		}
	}

	return nodes, edges, nil
}

func instanceToImpactNode(inst domain.Instance, depth int, direction string) domain.ImpactNode {
	return domain.ImpactNode{
		InstanceID: inst.ID,
		AssetID:    inst.AssetID,
		AssetName:  inst.AssetName,
		ModelUID:   inst.ModelUID,
		Depth:      depth,
		Direction:  direction,
	}
}
