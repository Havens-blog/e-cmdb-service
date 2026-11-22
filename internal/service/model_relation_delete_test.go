package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// relTypeDeleteRepo 是 ModelRelationTypeRepository 的内存实现，够本轮删除保护使用。
type relTypeDeleteRepo struct {
	exists map[string]bool
	counts map[string]int64
	// countErr 非空时 Count/Exists 之外的统计调用返回该错误（模拟 Mongo 故障）
	deleted []string
}

func (r *relTypeDeleteRepo) Create(context.Context, domain.ModelRelationType) (int64, error) {
	return 0, nil
}

func (r *relTypeDeleteRepo) GetByUID(_ context.Context, uid string) (domain.ModelRelationType, error) {
	if r.exists[uid] {
		return domain.ModelRelationType{ID: 1, UID: uid}, nil
	}
	return domain.ModelRelationType{}, nil
}

func (r *relTypeDeleteRepo) GetByID(context.Context, int64) (domain.ModelRelationType, error) {
	return domain.ModelRelationType{}, nil
}

func (r *relTypeDeleteRepo) List(context.Context, domain.ModelRelationTypeFilter) ([]domain.ModelRelationType, error) {
	return nil, nil
}

func (r *relTypeDeleteRepo) Count(context.Context, domain.ModelRelationTypeFilter) (int64, error) {
	return 0, nil
}

func (r *relTypeDeleteRepo) Update(context.Context, domain.ModelRelationType) error { return nil }

func (r *relTypeDeleteRepo) Delete(_ context.Context, uid string) error {
	r.deleted = append(r.deleted, uid)
	delete(r.exists, uid)
	return nil
}

func (r *relTypeDeleteRepo) Exists(_ context.Context, uid string) (bool, error) {
	return r.exists[uid], nil
}

func (r *relTypeDeleteRepo) FindByModels(context.Context, string, string) ([]domain.ModelRelationType, error) {
	return nil, nil
}

// instRelCounter 只实现 Count；其余方法由 embedded nil 接口在调用时 panic，
// 这样能证明 Delete 只通过 Count 探查引用（不会误删关系）。
type instRelCounter struct {
	repository.InstanceRelationRepository
	count int64
	err   error
	// gotFilter 记录最后一次统计条件，用于断言必须跨租户
	gotFilter domain.InstanceRelationFilter
}

func (c *instRelCounter) Count(_ context.Context, filter domain.InstanceRelationFilter) (int64, error) {
	c.gotFilter = filter
	return c.count, c.err
}

func newServiceForDeleteGuard(types map[string]bool, relCount int64, relErr error) (ModelRelationTypeService, *relTypeDeleteRepo, *instRelCounter) {
	repo := &relTypeDeleteRepo{exists: types}
	counter := &instRelCounter{count: relCount, err: relErr}
	return NewModelRelationTypeService(repo, counter), repo, counter
}

// TestModelRelationTypeDelete_RejectsWhenInUse 锁定本轮缺陷：
// 删除仍被资产关系引用的类型必须拒绝，而不是 code=0 留下一堆孤儿关系。
func TestModelRelationTypeDelete_RejectsWhenInUse(t *testing.T) {
	const uid = "zz_rel_in_use"
	svc, repo, counter := newServiceForDeleteGuard(map[string]bool{uid: true}, 3, nil)

	err := svc.Delete(context.Background(), uid)
	if err == nil {
		t.Fatal("在用类型应拒绝删除")
	}

	var inUse *RelationTypeInUseError
	if !errors.As(err, &inUse) {
		t.Fatalf("err = %#v, 期望 *RelationTypeInUseError", err)
	}
	if inUse.Count != 3 {
		t.Fatalf("Count = %d, 期望 3", inUse.Count)
	}
	if inUse.RelationTypeUID != uid {
		t.Fatalf("RelationTypeUID = %q, 期望 %q", inUse.RelationTypeUID, uid)
	}
	// 文案要给用户可执行的下一步，并带上条数
	want := "该关系类型正在被 3 条资产关系使用，请先删除这些资产关系后再删除类型"
	if inUse.Error() != want {
		t.Fatalf("Error() = %q, 期望 %q", inUse.Error(), want)
	}
	// errors.Is 必须可通过哨兵判定（上层统一分流的依据）
	if !errors.Is(err, errs.ErrRelationTypeInUse) {
		t.Fatal("errors.Is(err, errs.ErrRelationTypeInUse) 应为 true")
	}

	// 统计必须跨租户：TenantID 留 0 表示不加租户谓词
	if counter.gotFilter.TenantID != 0 {
		t.Fatalf("TenantID = %d, 期望 0（模型关系类型是无租户元数据，统计需跨租户）", counter.gotFilter.TenantID)
	}
	if counter.gotFilter.RelationTypeUID != uid {
		t.Fatalf("RelationTypeUID = %q, 期望 %q", counter.gotFilter.RelationTypeUID, uid)
	}

	// 最关键的断言：类型没有被删掉
	if len(repo.deleted) != 0 {
		t.Fatalf("被拒绝时不应调用 repo.Delete，实际删除 %v", repo.deleted)
	}
}

// TestModelRelationTypeDelete_AllowsWhenUnused 未使用时应正常删除。
func TestModelRelationTypeDelete_AllowsWhenUnused(t *testing.T) {
	const uid = "zz_rel_unused"
	svc, repo, _ := newServiceForDeleteGuard(map[string]bool{uid: true}, 0, nil)

	if err := svc.Delete(context.Background(), uid); err != nil {
		t.Fatalf("未使用的类型应允许删除，实际 %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != uid {
		t.Fatalf("deleted = %v, 期望 [%s]", repo.deleted, uid)
	}
}

// TestModelRelationTypeDelete_NotFound 保持原语义：不存在返回 404 语义的错误。
func TestModelRelationTypeDelete_NotFound(t *testing.T) {
	svc, _, _ := newServiceForDeleteGuard(map[string]bool{}, 0, nil)

	err := svc.Delete(context.Background(), "zz_missing")
	if !errors.Is(err, errs.ErrRelationNotFound) {
		t.Fatalf("err = %v, 期望 errs.ErrRelationNotFound", err)
	}
}

// TestModelRelationTypeDelete_CountFailureDoesNotDelete 统计失败时必须保守拒绝，
// 不能因为「查不出来」就当成「没有引用」而删掉元数据。
func TestModelRelationTypeDelete_CountFailureDoesNotDelete(t *testing.T) {
	const uid = "zz_rel_count_fail"
	svc, repo, _ := newServiceForDeleteGuard(map[string]bool{uid: true}, 0, errors.New("mongo timeout"))

	err := svc.Delete(context.Background(), uid)
	if err == nil {
		t.Fatal("统计失败时应拒绝删除")
	}
	if errors.Is(err, errs.ErrRelationTypeInUse) {
		t.Fatalf("统计失败不应伪装成「在用」错误：%v", err)
	}
	if len(repo.deleted) != 0 {
		t.Fatalf("统计失败时不应删除，实际删除 %v", repo.deleted)
	}
}

// TestModelRelationTypeDelete_NilRelationRepo 兼容未注入关系仓储的构造方式（如单测/旧接线），
// 此时跳过引用统计，保持删除可用。
func TestModelRelationTypeDelete_NilRelationRepo(t *testing.T) {
	repo := &relTypeDeleteRepo{exists: map[string]bool{"zz_rel_nil": true}}
	svc := NewModelRelationTypeService(repo, nil)

	if err := svc.Delete(context.Background(), "zz_rel_nil"); err != nil {
		t.Fatalf("未注入关系仓储时应允许删除，实际 %v", err)
	}
	if len(repo.deleted) != 1 {
		t.Fatalf("deleted = %v, 期望删除 1 次", repo.deleted)
	}
}
