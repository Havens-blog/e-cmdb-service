package service

import (
	"context"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// --- impact 遍历用 fake repos（内存实现，镜像 DAO 的租户谓词语义） ---

type impactInstanceRepo struct {
	instances map[int64]domain.Instance
}

func newImpactInstanceRepo(instances ...domain.Instance) *impactInstanceRepo {
	r := &impactInstanceRepo{instances: map[int64]domain.Instance{}}
	for _, inst := range instances {
		r.instances[inst.ID] = inst
	}
	return r
}

func (f *impactInstanceRepo) GetByID(ctx context.Context, tenantID int64, id int64) (domain.Instance, error) {
	inst, ok := f.instances[id]
	if !ok || inst.TenantID != tenantID {
		return domain.Instance{}, nil
	}
	return inst, nil
}

func (f *impactInstanceRepo) Create(ctx context.Context, instance domain.Instance) (int64, error) {
	return 0, nil
}
func (f *impactInstanceRepo) CreateBatch(ctx context.Context, instances []domain.Instance) (int64, error) {
	return 0, nil
}
func (f *impactInstanceRepo) Update(ctx context.Context, tenantID int64, instance domain.Instance) error {
	return nil
}
func (f *impactInstanceRepo) GetByAssetID(ctx context.Context, tenantID int64, modelUID, assetID string) (domain.Instance, error) {
	return domain.Instance{}, nil
}
func (f *impactInstanceRepo) List(ctx context.Context, filter domain.InstanceFilter) ([]domain.Instance, error) {
	return nil, nil
}
func (f *impactInstanceRepo) ListByIDs(ctx context.Context, ids []int64, tenantID int64) ([]domain.Instance, error) {
	return nil, nil
}
func (f *impactInstanceRepo) Count(ctx context.Context, filter domain.InstanceFilter) (int64, error) {
	return 0, nil
}

// CountByModelUID 桩实现：数据质量服务端的按模型计数（测试中不使用）
func (f *impactInstanceRepo) CountByModelUID(ctx context.Context, tenantID int64) (map[string]int64, error) {
	return map[string]int64{}, nil
}

// CountMissingAttribute 桩实现：数据质量服务端的缺失字段计数（测试中不使用）
func (f *impactInstanceRepo) CountMissingAttribute(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	return 0, nil
}

func (f *impactInstanceRepo) Delete(ctx context.Context, tenantID int64, id int64) error {
	return nil
}
func (f *impactInstanceRepo) DeleteByAccountID(ctx context.Context, accountID int64) error {
	return nil
}
func (f *impactInstanceRepo) Upsert(ctx context.Context, instance domain.Instance) error {
	return nil
}
func (f *impactInstanceRepo) ListUnbound(ctx context.Context, tenantID int64, offset, limit int64) ([]domain.Instance, error) {
	return nil, nil
}
func (f *impactInstanceRepo) CountUnbound(ctx context.Context, tenantID int64) (int64, error) {
	return 0, nil
}
func (f *impactInstanceRepo) AggregateStatsByIDs(ctx context.Context, ids []int64, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}
func (f *impactInstanceRepo) AggregateAllStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}
func (f *impactInstanceRepo) AggregateUnboundStats(ctx context.Context, tenantID int64) (*dao.AssetStatsResult, error) {
	return nil, nil
}

type impactRelationRepo struct {
	relations []domain.InstanceRelation
}

func newImpactRelationRepo(relations ...domain.InstanceRelation) *impactRelationRepo {
	return &impactRelationRepo{relations: relations}
}

// List 镜像 dao/relation.go buildQuery 的过滤语义：
// source/target > 0 才加谓词，tenant != 0 才加租户谓词。
func (f *impactRelationRepo) List(ctx context.Context, filter domain.InstanceRelationFilter) ([]domain.InstanceRelation, error) {
	out := make([]domain.InstanceRelation, 0, len(f.relations))
	for _, rel := range f.relations {
		if filter.SourceInstanceID > 0 && rel.SourceInstanceID != filter.SourceInstanceID {
			continue
		}
		if filter.TargetInstanceID > 0 && rel.TargetInstanceID != filter.TargetInstanceID {
			continue
		}
		if filter.RelationTypeUID != "" && rel.RelationTypeUID != filter.RelationTypeUID {
			continue
		}
		if filter.TenantID != 0 && rel.TenantID != filter.TenantID {
			continue
		}
		out = append(out, rel)
	}
	return out, nil
}

func (f *impactRelationRepo) Create(ctx context.Context, relation domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *impactRelationRepo) CreateBatch(ctx context.Context, relations []domain.InstanceRelation) (int64, error) {
	return 0, nil
}
func (f *impactRelationRepo) GetByID(ctx context.Context, tenantID int64, id int64) (domain.InstanceRelation, error) {
	return domain.InstanceRelation{}, nil
}
func (f *impactRelationRepo) Count(ctx context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	return 0, nil
}
func (f *impactRelationRepo) Delete(ctx context.Context, tenantID int64, id int64) error {
	return nil
}
func (f *impactRelationRepo) DeleteByInstanceID(ctx context.Context, tenantID int64, instanceID int64) error {
	return nil
}
func (f *impactRelationRepo) Exists(ctx context.Context, tenantID int64, sourceID, targetID int64, relationTypeUID string) (bool, error) {
	return false, nil
}

// findImpactNode 按实例 ID + 方向查找节点。
func findImpactNode(nodes []domain.ImpactNode, id int64, direction string) (domain.ImpactNode, bool) {
	for _, n := range nodes {
		if n.InstanceID == id && n.Direction == direction {
			return n, true
		}
	}
	return domain.ImpactNode{}, false
}

func TestImpactGetImpact_NoRelations_RootOnly(t *testing.T) {
	svc := NewImpactService(
		newImpactInstanceRepo(domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a1", AssetName: "n1"}),
		newImpactRelationRepo(),
	)

	g, err := svc.GetImpact(context.Background(), 100, 1, 3)
	if err != nil {
		t.Fatalf("GetImpact error = %v", err)
	}

	if g.Root.InstanceID != 1 || g.Root.Depth != 0 || g.Root.Direction != "" {
		t.Fatalf("root = %#v, want instance 1 depth 0 direction \"\"", g.Root)
	}
	if g.Root.AssetID != "a1" || g.Root.AssetName != "n1" || g.Root.ModelUID != "m" {
		t.Fatalf("root fields wrong: %#v", g.Root)
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("nodes/edges = %d/%d, want 0/0", len(g.Nodes), len(g.Edges))
	}
	if g.UpstreamCount != 0 || g.DownstreamCount != 0 {
		t.Fatalf("counts = %d/%d, want 0/0", g.UpstreamCount, g.DownstreamCount)
	}
}

func TestImpactGetImpact_UpstreamDownstream(t *testing.T) {
	svc := NewImpactService(
		newImpactInstanceRepo(
			domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a1", AssetName: "root"},
			domain.Instance{ID: 2, TenantID: 100, ModelUID: "m", AssetID: "a2", AssetName: "down2"},
			domain.Instance{ID: 3, TenantID: 100, ModelUID: "m", AssetID: "a3", AssetName: "down3"},
			domain.Instance{ID: 4, TenantID: 100, ModelUID: "m", AssetID: "a4", AssetName: "up4"},
		),
		newImpactRelationRepo(
			domain.InstanceRelation{ID: 10, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "runs_on", TenantID: 100},
			domain.InstanceRelation{ID: 11, SourceInstanceID: 1, TargetInstanceID: 3, RelationTypeUID: "runs_on", TenantID: 100},
			domain.InstanceRelation{ID: 12, SourceInstanceID: 4, TargetInstanceID: 1, RelationTypeUID: "depends_on", TenantID: 100},
		),
	)

	g, err := svc.GetImpact(context.Background(), 100, 1, 5)
	if err != nil {
		t.Fatalf("GetImpact error = %v", err)
	}

	if g.DownstreamCount != 2 || g.UpstreamCount != 1 {
		t.Fatalf("counts = downstream %d / upstream %d, want 2/1", g.DownstreamCount, g.UpstreamCount)
	}
	if len(g.Nodes) != 3 || len(g.Edges) != 3 {
		t.Fatalf("nodes/edges = %d/%d, want 3/3", len(g.Nodes), len(g.Edges))
	}

	down2, ok := findImpactNode(g.Nodes, 2, domain.ImpactDirectionDownstream)
	if !ok || down2.Depth != 1 || down2.AssetName != "down2" {
		t.Fatalf("node 2 = %#v, want downstream depth 1", down2)
	}
	down3, ok := findImpactNode(g.Nodes, 3, domain.ImpactDirectionDownstream)
	if !ok || down3.Depth != 1 {
		t.Fatalf("node 3 = %#v, want downstream depth 1", down3)
	}
	up4, ok := findImpactNode(g.Nodes, 4, domain.ImpactDirectionUpstream)
	if !ok || up4.Depth != 1 || up4.AssetName != "up4" {
		t.Fatalf("node 4 = %#v, want upstream depth 1", up4)
	}

	// 边的 source/target 保持原始关系方向，direction 标注遍历方向。
	downEdges := map[int64]domain.ImpactEdge{} // key = target_id
	upEdges := map[int64]domain.ImpactEdge{}   // key = source_id
	for _, e := range g.Edges {
		switch e.Direction {
		case domain.ImpactDirectionDownstream:
			downEdges[e.TargetID] = e
		case domain.ImpactDirectionUpstream:
			upEdges[e.SourceID] = e
		}
	}
	if e, ok := downEdges[2]; !ok || e.SourceID != 1 || e.TargetID != 2 || e.RelationTypeUID != "runs_on" || e.Direction != "downstream" {
		t.Fatalf("downstream edge to 2 = %#v", downEdges[2])
	}
	if e, ok := downEdges[3]; !ok || e.SourceID != 1 || e.TargetID != 3 || e.Direction != "downstream" {
		t.Fatalf("downstream edge to 3 = %#v", downEdges[3])
	}
	if e, ok := upEdges[4]; !ok || e.SourceID != 4 || e.TargetID != 1 || e.RelationTypeUID != "depends_on" || e.Direction != "upstream" {
		t.Fatalf("upstream edge from 4 = %#v", upEdges[4])
	}
}

func TestImpactGetImpact_DepthLimit(t *testing.T) {
	svc := NewImpactService(
		newImpactInstanceRepo(
			domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a1"},
			domain.Instance{ID: 2, TenantID: 100, ModelUID: "m", AssetID: "a2"},
			domain.Instance{ID: 3, TenantID: 100, ModelUID: "m", AssetID: "a3"},
			domain.Instance{ID: 4, TenantID: 100, ModelUID: "m", AssetID: "a4"},
		),
		newImpactRelationRepo(
			domain.InstanceRelation{ID: 10, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "r", TenantID: 100},
			domain.InstanceRelation{ID: 11, SourceInstanceID: 2, TargetInstanceID: 3, RelationTypeUID: "r", TenantID: 100},
			domain.InstanceRelation{ID: 12, SourceInstanceID: 3, TargetInstanceID: 4, RelationTypeUID: "r", TenantID: 100},
		),
	)

	g, err := svc.GetImpact(context.Background(), 100, 1, 2)
	if err != nil {
		t.Fatalf("GetImpact error = %v", err)
	}

	if g.DownstreamCount != 2 {
		t.Fatalf("downstream_count = %d, want 2", g.DownstreamCount)
	}
	if _, ok := findImpactNode(g.Nodes, 2, domain.ImpactDirectionDownstream); !ok {
		t.Fatalf("node 2 missing")
	}
	n3, ok := findImpactNode(g.Nodes, 3, domain.ImpactDirectionDownstream)
	if !ok || n3.Depth != 2 {
		t.Fatalf("node 3 = %#v, want depth 2", n3)
	}
	if _, ok := findImpactNode(g.Nodes, 4, domain.ImpactDirectionDownstream); ok {
		t.Fatalf("node 4 should be beyond depth limit")
	}
	if len(g.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(g.Edges))
	}
}

func TestImpactGetImpact_SkipsCrossTenantAndMissing(t *testing.T) {
	svc := NewImpactService(
		newImpactInstanceRepo(
			domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a1"},
			domain.Instance{ID: 2, TenantID: 100, ModelUID: "m", AssetID: "a2"},
			domain.Instance{ID: 5, TenantID: 200, ModelUID: "m", AssetID: "a5"}, // 跨租户实例
		),
		newImpactRelationRepo(
			domain.InstanceRelation{ID: 10, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "r", TenantID: 100},
			// 关系本身是租户 100，但目标实例 5 属于租户 200（数据异常）：不计入。
			domain.InstanceRelation{ID: 11, SourceInstanceID: 1, TargetInstanceID: 5, RelationTypeUID: "r", TenantID: 100},
			// 目标实例 6 不存在：不计入。
			domain.InstanceRelation{ID: 12, SourceInstanceID: 1, TargetInstanceID: 6, RelationTypeUID: "r", TenantID: 100},
			// 关系本身属于租户 200：List 按租户过滤，天然排除。
			domain.InstanceRelation{ID: 13, SourceInstanceID: 1, TargetInstanceID: 7, RelationTypeUID: "r", TenantID: 200},
		),
	)

	g, err := svc.GetImpact(context.Background(), 100, 1, 5)
	if err != nil {
		t.Fatalf("GetImpact error = %v", err)
	}

	if g.DownstreamCount != 1 {
		t.Fatalf("downstream_count = %d, want 1", g.DownstreamCount)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].InstanceID != 2 {
		t.Fatalf("nodes = %#v, want only instance 2", g.Nodes)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(g.Edges))
	}
}

func TestImpactGetImpact_RootNotFound(t *testing.T) {
	svc := NewImpactService(
		newImpactInstanceRepo(domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a1"}),
		newImpactRelationRepo(),
	)

	g, err := svc.GetImpact(context.Background(), 100, 99, 3)
	if err != nil {
		t.Fatalf("GetImpact error = %v", err)
	}

	if g.Root.InstanceID != 0 {
		t.Fatalf("root = %#v, want zero value", g.Root)
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("nodes/edges = %d/%d, want empty", len(g.Nodes), len(g.Edges))
	}
	if g.UpstreamCount != 0 || g.DownstreamCount != 0 {
		t.Fatalf("counts = %d/%d, want 0/0", g.UpstreamCount, g.DownstreamCount)
	}
}

func TestImpactGetImpact_ShortestPathAndCycle(t *testing.T) {
	svc := NewImpactService(
		newImpactInstanceRepo(
			domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a1"},
			domain.Instance{ID: 2, TenantID: 100, ModelUID: "m", AssetID: "a2"},
			domain.Instance{ID: 3, TenantID: 100, ModelUID: "m", AssetID: "a3"},
			domain.Instance{ID: 4, TenantID: 100, ModelUID: "m", AssetID: "a4"},
		),
		newImpactRelationRepo(
			domain.InstanceRelation{ID: 10, SourceInstanceID: 1, TargetInstanceID: 2, RelationTypeUID: "r", TenantID: 100},
			domain.InstanceRelation{ID: 11, SourceInstanceID: 1, TargetInstanceID: 3, RelationTypeUID: "r", TenantID: 100},
			domain.InstanceRelation{ID: 12, SourceInstanceID: 2, TargetInstanceID: 4, RelationTypeUID: "r", TenantID: 100},
			domain.InstanceRelation{ID: 13, SourceInstanceID: 3, TargetInstanceID: 4, RelationTypeUID: "r", TenantID: 100},
			// 回边 2→1：root 已被 visited，不应重复计入。
			domain.InstanceRelation{ID: 14, SourceInstanceID: 2, TargetInstanceID: 1, RelationTypeUID: "r", TenantID: 100},
		),
	)

	g, err := svc.GetImpact(context.Background(), 100, 1, 5)
	if err != nil {
		t.Fatalf("GetImpact error = %v", err)
	}

	if g.DownstreamCount != 3 {
		t.Fatalf("downstream_count = %d, want 3", g.DownstreamCount)
	}
	// 4 经 2 与 3 两条路径可达，最短距离应为 2，且只出现一次。
	n4, ok := findImpactNode(g.Nodes, 4, domain.ImpactDirectionDownstream)
	if !ok || n4.Depth != 2 {
		t.Fatalf("node 4 = %#v, want depth 2", n4)
	}
	if _, ok := findImpactNode(g.Nodes, 1, domain.ImpactDirectionDownstream); ok {
		t.Fatalf("root 1 should not be re-added by cycle")
	}
}

func TestImpactGetImpact_UpstreamDepthLimit(t *testing.T) {
	svc := NewImpactService(
		newImpactInstanceRepo(
			domain.Instance{ID: 1, TenantID: 100, ModelUID: "m", AssetID: "a1"},
			domain.Instance{ID: 2, TenantID: 100, ModelUID: "m", AssetID: "a2"},
			domain.Instance{ID: 3, TenantID: 100, ModelUID: "m", AssetID: "a3"},
			domain.Instance{ID: 4, TenantID: 100, ModelUID: "m", AssetID: "a4"},
		),
		newImpactRelationRepo(
			domain.InstanceRelation{ID: 10, SourceInstanceID: 2, TargetInstanceID: 1, RelationTypeUID: "r", TenantID: 100},
			domain.InstanceRelation{ID: 11, SourceInstanceID: 3, TargetInstanceID: 2, RelationTypeUID: "r", TenantID: 100},
			domain.InstanceRelation{ID: 12, SourceInstanceID: 4, TargetInstanceID: 3, RelationTypeUID: "r", TenantID: 100},
		),
	)

	g, err := svc.GetImpact(context.Background(), 100, 1, 2)
	if err != nil {
		t.Fatalf("GetImpact error = %v", err)
	}

	if g.UpstreamCount != 2 {
		t.Fatalf("upstream_count = %d, want 2", g.UpstreamCount)
	}
	if _, ok := findImpactNode(g.Nodes, 2, domain.ImpactDirectionUpstream); !ok {
		t.Fatalf("node 2 missing")
	}
	n3, ok := findImpactNode(g.Nodes, 3, domain.ImpactDirectionUpstream)
	if !ok || n3.Depth != 2 {
		t.Fatalf("node 3 = %#v, want depth 2", n3)
	}
	if _, ok := findImpactNode(g.Nodes, 4, domain.ImpactDirectionUpstream); ok {
		t.Fatalf("node 4 should be beyond depth limit")
	}
	if g.DownstreamCount != 0 {
		t.Fatalf("downstream_count = %d, want 0", g.DownstreamCount)
	}
}

// CountOrphanInstances 桩实现：数据质量孤儿统计（测试中不使用）。
func (f *impactInstanceRepo) CountOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string) (int64, error) {
	return 0, nil
}
// GroupOrphanInstancesByModel 桩实现：数据质量孤儿来源聚合（测试中不使用）。
func (f *impactInstanceRepo) GroupOrphanInstancesByModel(ctx context.Context, tenantID int64, knownModelUIDs []string) ([]repository.OrphanModelGroup, error) {
	return nil, nil
}

// BatchCountMissingFieldGroups 桩实现：数据质量缺失字段批量统计（测试中不使用）。
func (f *impactInstanceRepo) BatchCountMissingFieldGroups(ctx context.Context, tenantID int64, fieldsByModel map[string][]string) (map[string]map[string]int64, error) {
	return nil, nil
}

// ListOrphanInstances 桩实现：数据质量孤儿明细（测试中不使用）。
func (f *impactInstanceRepo) ListOrphanInstances(ctx context.Context, tenantID int64, knownModelUIDs []string, offset, limit int64) ([]repository.OrphanInstanceView, int64, error) {
	return nil, 0, nil
}

// CountMissingFieldGroup 桩实现：数据质量缺失字段分组计数（测试中不使用）。
func (f *impactInstanceRepo) CountMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	return 0, nil
}

// ListMissingFieldGroup 桩实现：数据质量缺失字段分组明细（测试中不使用）。
func (f *impactInstanceRepo) ListMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string, offset, limit int64) ([]repository.MissingFieldInstanceView, error) {
	return nil, nil
}
