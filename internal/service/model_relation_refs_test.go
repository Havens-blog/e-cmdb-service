package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// refModelRepo 只实现关系类型创建时用到的那一个方法所需的语义：
// 按 uid 返回模型，未命中返回零值（ID==0），与 dao 层「查不到就是空结构」一致。
type refModelRepo struct {
	byUID map[string]domain.Model
	err   error
}

func (f *refModelRepo) Create(context.Context, domain.Model) (int64, error) { return 0, nil }

func (f *refModelRepo) GetByUID(_ context.Context, uid string) (domain.Model, error) {
	if f.err != nil {
		return domain.Model{}, f.err
	}
	return f.byUID[uid], nil
}

func (f *refModelRepo) GetByID(context.Context, int64) (domain.Model, error) {
	return domain.Model{}, nil
}

func (f *refModelRepo) List(context.Context, domain.ModelFilter) ([]domain.Model, error) {
	return nil, nil
}

func (f *refModelRepo) Count(context.Context, domain.ModelFilter) (int64, error) { return 0, nil }

func (f *refModelRepo) Update(context.Context, domain.Model) error { return nil }

func (f *refModelRepo) Delete(context.Context, string) error { return nil }

// refRelationRepo 记录 Create 是否被调用，用来证明「校验失败时不落库」。
type refRelationRepo struct {
	existing  map[string]bool
	created   []domain.ModelRelationType
	updated   []domain.ModelRelationType
	createErr error
	existsErr error
}

func (f *refRelationRepo) Create(_ context.Context, rel domain.ModelRelationType) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.created = append(f.created, rel)
	return int64(len(f.created)), nil
}

func (f *refRelationRepo) GetByUID(_ context.Context, uid string) (domain.ModelRelationType, error) {
	return domain.ModelRelationType{UID: uid, ID: 1}, nil
}

func (f *refRelationRepo) List(context.Context, domain.ModelRelationTypeFilter) ([]domain.ModelRelationType, error) {
	return nil, nil
}

func (f *refRelationRepo) Count(context.Context, domain.ModelRelationTypeFilter) (int64, error) {
	return 0, nil
}

func (f *refRelationRepo) Update(_ context.Context, rel domain.ModelRelationType) error {
	f.updated = append(f.updated, rel)
	return nil
}

func (f *refRelationRepo) Delete(context.Context, string) error { return nil }

func (f *refRelationRepo) Exists(_ context.Context, uid string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.existing[uid], nil
}

func (f *refRelationRepo) FindByModels(_ context.Context, _, _ string) ([]domain.ModelRelationType, error) {
	return nil, nil
}

func newRefService(modelRepo *refModelRepo, relRepo *refRelationRepo) ModelRelationTypeService {
	return NewModelRelationTypeService(relRepo, modelRepo)
}

func validRelation() domain.ModelRelationType {
	return domain.ModelRelationType{
		UID:            "zz_test_ecs_disk",
		Name:           "测试：云服务器挂载云盘",
		SourceModelUID: "ecs",
		TargetModelUID: "disk",
		RelationType:   domain.RelationTypeContains,
		Direction:      domain.RelationDirectionOneToMany,
	}
}

// TestCreateRelationType_RejectsMissingModels 复现 iter272 的缺陷：
// 创建时若源/目标模型不存在，必须在服务层拒绝且不落库。
func TestCreateRelationType_RejectsMissingModels(t *testing.T) {
	models := &refModelRepo{byUID: map[string]domain.Model{
		"ecs":  {ID: 1, UID: "ecs"},
		"disk": {ID: 2, UID: "disk"},
	}}

	t.Run("源模型不存在", func(t *testing.T) {
		rels := &refRelationRepo{}
		svc := newRefService(models, rels)
		rel := validRelation()
		rel.SourceModelUID = "cloud_ecs"

		_, err := svc.Create(context.Background(), rel)
		if !errors.Is(err, errs.ErrRelationSourceModelNotFound) {
			t.Fatalf("期望 ErrRelationSourceModelNotFound，实际 %v", err)
		}
		if len(rels.created) != 0 {
			t.Fatalf("校验失败不应落库，实际写入 %d 条", len(rels.created))
		}
	})

	t.Run("目标模型不存在", func(t *testing.T) {
		rels := &refRelationRepo{}
		svc := newRefService(models, rels)
		rel := validRelation()
		rel.TargetModelUID = "cloud_vpc"

		_, err := svc.Create(context.Background(), rel)
		if !errors.Is(err, errs.ErrRelationTargetModelNotFound) {
			t.Fatalf("期望 ErrRelationTargetModelNotFound，实际 %v", err)
		}
		if len(rels.created) != 0 {
			t.Fatalf("校验失败不应落库，实际写入 %d 条", len(rels.created))
		}
	})

	t.Run("两端都不存在时先报源模型", func(t *testing.T) {
		svc := newRefService(models, &refRelationRepo{})
		rel := validRelation()
		rel.SourceModelUID = "zz_no_src"
		rel.TargetModelUID = "zz_no_tgt"

		_, err := svc.Create(context.Background(), rel)
		if !errors.Is(err, errs.ErrRelationSourceModelNotFound) {
			t.Fatalf("期望先报源模型，实际 %v", err)
		}
		if !strings.Contains(err.Error(), "model_uid=zz_no_src") {
			t.Fatalf("错误信息应带上具体 uid，实际 %v", err)
		}
	})
}

// TestCreateRelationType_AcceptsExistingModels 正常路径不被新校验挡住。
func TestCreateRelationType_AcceptsExistingModels(t *testing.T) {
	models := &refModelRepo{byUID: map[string]domain.Model{
		"ecs":  {ID: 1, UID: "ecs"},
		"disk": {ID: 2, UID: "disk"},
	}}
	rels := &refRelationRepo{}
	svc := newRefService(models, rels)

	id, err := svc.Create(context.Background(), validRelation())
	if err != nil {
		t.Fatalf("期望创建成功，实际 %v", err)
	}
	if id == 0 || len(rels.created) != 1 {
		t.Fatalf("期望落库 1 条，实际 id=%d created=%d", id, len(rels.created))
	}
	if rels.created[0].SourceModelUID != "ecs" || rels.created[0].TargetModelUID != "disk" {
		t.Fatalf("落库的模型对不符：%+v", rels.created[0])
	}
}

// TestCreateRelationType_TrimsWhitespace 单项空格曾能绕过 binding:"required" 落库成 " "。
func TestCreateRelationType_TrimsWhitespace(t *testing.T) {
	models := &refModelRepo{byUID: map[string]domain.Model{
		"ecs":  {ID: 1, UID: "ecs"},
		"disk": {ID: 2, UID: "disk"},
	}}
	rels := &refRelationRepo{}
	svc := newRefService(models, rels)

	rel := validRelation()
	rel.UID = "  zz_test_trim  "
	rel.SourceModelUID = "  ecs  "
	rel.TargetModelUID = " disk "
	rel.Name = "  裁剪测试  "

	if _, err := svc.Create(context.Background(), rel); err != nil {
		t.Fatalf("期望创建成功，实际 %v", err)
	}
	got := rels.created[0]
	if got.UID != "zz_test_trim" || got.SourceModelUID != "ecs" || got.TargetModelUID != "disk" || got.Name != "裁剪测试" {
		t.Fatalf("落库值未裁剪：%+v", got)
	}
}

// TestCreateRelationType_RejectsBlankAfterTrim 裁剪后变空串必须被 Validate 拦下。
func TestCreateRelationType_RejectsBlankAfterTrim(t *testing.T) {
	svc := newRefService(&refModelRepo{byUID: map[string]domain.Model{}}, &refRelationRepo{})

	rel := validRelation()
	rel.UID = "   "
	_, err := svc.Create(context.Background(), rel)
	if !errors.Is(err, domain.ErrInvalidRelationTypeUID) {
		t.Fatalf("期望 ErrInvalidRelationTypeUID，实际 %v", err)
	}

	rel = validRelation()
	rel.SourceModelUID = "   "
	_, err = svc.Create(context.Background(), rel)
	if !errors.Is(err, domain.ErrInvalidRelationModel) {
		t.Fatalf("期望 ErrInvalidRelationModel，实际 %v", err)
	}

	// Name 同样要在裁剪后判空：binding:"required" 放行纯空格，
	// 否则会落库成「名字为空白」的关系类型（与本次要修的脏数据同类）。
	rel = validRelation()
	rel.Name = "   "
	_, err = svc.Create(context.Background(), rel)
	if !errors.Is(err, domain.ErrInvalidRelationName) {
		t.Fatalf("期望 ErrInvalidRelationName，实际 %v", err)
	}
}

// TestCreateRelationType_DuplicateUIDStillRejected 新校验不得掩盖原有重复检查。
func TestCreateRelationType_DuplicateUIDStillRejected(t *testing.T) {
	models := &refModelRepo{byUID: map[string]domain.Model{
		"ecs":  {ID: 1, UID: "ecs"},
		"disk": {ID: 2, UID: "disk"},
	}}
	rels := &refRelationRepo{existing: map[string]bool{"zz_test_ecs_disk": true}}
	svc := newRefService(models, rels)

	_, err := svc.Create(context.Background(), validRelation())
	if !errors.Is(err, errs.ErrRelationExists) {
		t.Fatalf("期望 ErrRelationExists，实际 %v", err)
	}
	if len(rels.created) != 0 {
		t.Fatalf("重复时不应落库")
	}
}

// TestCreateRelationType_NilModelRepoSkipsCheck 保留旧装配路径（未注入模型仓储时不校验）。
func TestCreateRelationType_NilModelRepoSkipsCheck(t *testing.T) {
	rels := &refRelationRepo{}
	svc := NewModelRelationTypeService(rels, nil)

	rel := validRelation()
	rel.SourceModelUID = "zz_no_such"
	rel.TargetModelUID = "zz_no_such_either"
	if _, err := svc.Create(context.Background(), rel); err != nil {
		t.Fatalf("nil modelRepo 时应跳过校验，实际 %v", err)
	}
}

// TestCreateRelationType_PropagatesRepoError 模型仓储出错要向上传播，不能当成「模型不存在」。
func TestCreateRelationType_PropagatesRepoError(t *testing.T) {
	boom := errors.New("db down")
	models := &refModelRepo{byUID: map[string]domain.Model{}, err: boom}
	rels := &refRelationRepo{}
	svc := newRefService(models, rels)

	_, err := svc.Create(context.Background(), validRelation())
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("期望带上底层错误，实际 %v", err)
	}
	if errors.Is(err, errs.ErrRelationSourceModelNotFound) {
		t.Fatalf("仓储故障不应被误判为「模型不存在」")
	}
}

// 补齐接口方法（仓储接口比本测试用到的更多）。
func (f *refModelRepo) Exists(_ context.Context, uid string) (bool, error) {
	_, ok := f.byUID[uid]
	return ok, nil
}

func (f *refRelationRepo) GetByID(_ context.Context, id int64) (domain.ModelRelationType, error) {
	return domain.ModelRelationType{ID: id}, nil
}

// Update 必须与 Create 同一套裁剪 + 校验：
// 否则 PUT /model-relations/:uid 传 "   " 就能把名称改成空白。
func TestUpdateRelationType_RejectsBlankName(t *testing.T) {
	rels := &refRelationRepo{existing: map[string]bool{"ecs_disk": true}}
	svc := newRefService(&refModelRepo{}, rels)

	rel := validRelation()
	rel.Name = "   "
	if err := svc.Update(context.Background(), rel); !errors.Is(err, domain.ErrInvalidRelationName) {
		t.Fatalf("期望 ErrInvalidRelationName，实际 %v", err)
	}
	if len(rels.updated) != 0 {
		t.Fatalf("空白名称不应落库，实际写入 %+v", rels.updated)
	}
}

func TestUpdateRelationType_TrimsBeforeWrite(t *testing.T) {
	rels := &refRelationRepo{existing: map[string]bool{"zz_rel_trim": true}}
	svc := newRefService(&refModelRepo{}, rels)

	rel := validRelation()
	rel.UID = "  zz_rel_trim  "
	rel.Name = "  裁剪后的名称  "
	rel.SourceModelUID = " ecs "
	rel.TargetModelUID = " disk "
	if err := svc.Update(context.Background(), rel); err != nil {
		t.Fatalf("期望更新成功，实际 %v", err)
	}
	if len(rels.updated) != 1 {
		t.Fatalf("期望写入 1 次，实际 %d", len(rels.updated))
	}
	got := rels.updated[0]
	if got.UID != "zz_rel_trim" || got.Name != "裁剪后的名称" || got.SourceModelUID != "ecs" || got.TargetModelUID != "disk" {
		t.Fatalf("落库值未裁剪：%+v", got)
	}
}

func TestUpdateRelationType_UnknownUIDIsNotFound(t *testing.T) {
	rels := &refRelationRepo{existing: map[string]bool{}}
	svc := newRefService(&refModelRepo{}, rels)

	if err := svc.Update(context.Background(), validRelation()); !errors.Is(err, errs.ErrRelationNotFound) {
		t.Fatalf("期望 ErrRelationNotFound，实际 %v", err)
	}
	if len(rels.updated) != 0 {
		t.Fatalf("不存在的关系类型不应落库")
	}
}
