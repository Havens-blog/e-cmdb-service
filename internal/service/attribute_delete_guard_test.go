package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-cmdb-service/internal/repository"
)

// fakeAttrRepoForDelete 是删除字段保护用例专用的属性仓储桩：
// 只实现 GetByID / Delete 的最小行为，并记录 Delete 是否被调用、被删的 id。
type fakeAttrRepoForDelete struct {
	attr      domain.Attribute
	getErr    error
	deleteErr error
	deleted   []int64
}

var _ repository.AttributeRepository = (*fakeAttrRepoForDelete)(nil)

func (f *fakeAttrRepoForDelete) Create(ctx context.Context, attr domain.Attribute) (int64, error) {
	return 0, nil
}

func (f *fakeAttrRepoForDelete) GetByID(ctx context.Context, id int64) (domain.Attribute, error) {
	if f.getErr != nil {
		return domain.Attribute{}, f.getErr
	}
	if f.attr.ID == 0 || f.attr.ID != id {
		return domain.Attribute{}, errors.New("not found")
	}
	return f.attr, nil
}

func (f *fakeAttrRepoForDelete) GetByFieldUID(ctx context.Context, modelUID, fieldUID string) (domain.Attribute, error) {
	return domain.Attribute{}, nil
}

func (f *fakeAttrRepoForDelete) List(ctx context.Context, filter domain.AttributeFilter) ([]domain.Attribute, error) {
	return nil, nil
}

func (f *fakeAttrRepoForDelete) Count(ctx context.Context, filter domain.AttributeFilter) (int64, error) {
	return 0, nil
}

func (f *fakeAttrRepoForDelete) Update(ctx context.Context, attr domain.Attribute) error { return nil }

func (f *fakeAttrRepoForDelete) Delete(ctx context.Context, id int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeAttrRepoForDelete) DeleteByModelUID(ctx context.Context, modelUID string) error {
	return nil
}

func (f *fakeAttrRepoForDelete) Exists(ctx context.Context, modelUID, fieldUID string) (bool, error) {
	return false, nil
}

// usedField 是用例共用的字段定义：id=66，属于 host 模型的 status 字段。
var usedField = domain.Attribute{ID: 66, ModelUID: "host", FieldUID: "status", FieldType: "enum"}

func int64Ptr(v int64) *int64 { return &v }

// TestDeleteAttribute_RejectsWhenInstancesUseIt 字段仍有实例数据时拒绝删除，且不触碰仓储。
func TestDeleteAttribute_RejectsWhenInstancesUseIt(t *testing.T) {
	attrRepo := &fakeAttrRepoForDelete{attr: usedField}
	instanceRepo := newFakeInstanceRepo()
	instanceRepo.attributeUsage = int64Ptr(3)

	svc := NewAttributeService(attrRepo, nil, nil, instanceRepo)
	err := svc.DeleteAttribute(context.Background(), 66)

	if err == nil {
		t.Fatal("expected error when instances still use the field, got nil")
	}
	if !errors.Is(err, errs.ErrAttributeInUse) {
		t.Fatalf("expected errors.Is(err, ErrAttributeInUse), got %v", err)
	}
	var inUse *AttributeInUseError
	if !errors.As(err, &inUse) {
		t.Fatalf("expected *AttributeInUseError, got %T", err)
	}
	if inUse.Count != 3 {
		t.Fatalf("expected count 3, got %d", inUse.Count)
	}
	if inUse.ModelUID != "host" || inUse.FieldUID != "status" {
		t.Fatalf("unexpected model/field in error: %+v", inUse)
	}
	if want := "该字段仍被 3 个实例填写了数据"; !strings.Contains(inUse.Error(), want) {
		t.Fatalf("expected message to contain %q, got %q", want, inUse.Error())
	}
	if len(attrRepo.deleted) != 0 {
		t.Fatalf("Delete must not be called when field is in use, called with %v", attrRepo.deleted)
	}
}

// TestDeleteAttribute_AllowsWhenNoInstanceUsesIt 没有实例使用时正常删除。
func TestDeleteAttribute_AllowsWhenNoInstanceUsesIt(t *testing.T) {
	attrRepo := &fakeAttrRepoForDelete{attr: usedField}
	instanceRepo := newFakeInstanceRepo()
	instanceRepo.attributeUsage = int64Ptr(0)

	svc := NewAttributeService(attrRepo, nil, nil, instanceRepo)
	if err := svc.DeleteAttribute(context.Background(), 66); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(attrRepo.deleted) != 1 || attrRepo.deleted[0] != 66 {
		t.Fatalf("expected Delete(66) to be called once, got %v", attrRepo.deleted)
	}
}

// TestDeleteAttribute_UsageCheckErrorBlocksDeletion 统计失败时不得继续删除（fail-closed）。
func TestDeleteAttribute_UsageCheckErrorBlocksDeletion(t *testing.T) {
	attrRepo := &fakeAttrRepoForDelete{attr: usedField}
	instanceRepo := newFakeInstanceRepo()
	instanceRepo.attributeUsage = int64Ptr(0)
	instanceRepo.attributeUsageErr = errors.New("mongo: connection refused")

	svc := NewAttributeService(attrRepo, nil, nil, instanceRepo)
	err := svc.DeleteAttribute(context.Background(), 66)

	if err == nil {
		t.Fatal("expected error when usage check fails, got nil")
	}
	if errors.Is(err, errs.ErrAttributeInUse) {
		t.Fatalf("usage-check failure must not be reported as in-use, got %v", err)
	}
	if len(attrRepo.deleted) != 0 {
		t.Fatalf("Delete must not be called when usage check fails, called with %v", attrRepo.deleted)
	}
}

// TestDeleteAttribute_NilInstanceRepoSkipsCheck 未注入实例仓储（旧装配）时跳过使用校验。
func TestDeleteAttribute_NilInstanceRepoSkipsCheck(t *testing.T) {
	attrRepo := &fakeAttrRepoForDelete{attr: usedField}

	svc := NewAttributeService(attrRepo, nil, nil, nil)
	if err := svc.DeleteAttribute(context.Background(), 66); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(attrRepo.deleted) != 1 {
		t.Fatalf("expected Delete to be called once, got %v", attrRepo.deleted)
	}
}

// TestDeleteAttribute_NotFoundStillWins 字段不存在时返回 ErrAttributeNotFound，且不做使用统计。
func TestDeleteAttribute_NotFoundStillWins(t *testing.T) {
	attrRepo := &fakeAttrRepoForDelete{} // attr.ID == 0 → GetByID 返回 not found
	instanceRepo := newFakeInstanceRepo()
	// 即使配置了「有实例在用」，也不应被使用校验拦截（存在性检查在前）
	instanceRepo.attributeUsage = int64Ptr(5)

	svc := NewAttributeService(attrRepo, nil, nil, instanceRepo)
	err := svc.DeleteAttribute(context.Background(), 404)

	if !errors.Is(err, errs.ErrAttributeNotFound) {
		t.Fatalf("expected ErrAttributeNotFound, got %v", err)
	}
	if len(attrRepo.deleted) != 0 {
		t.Fatalf("Delete must not be called, got %v", attrRepo.deleted)
	}
}

// TestDeleteAttribute_GetByIDErrorIsNotFound 仓储读取错误按「不存在」处理（与既有语义一致）。
func TestDeleteAttribute_GetByIDErrorIsNotFound(t *testing.T) {
	attrRepo := &fakeAttrRepoForDelete{attr: usedField, getErr: errors.New("mongo: timeout")}

	svc := NewAttributeService(attrRepo, nil, nil, newFakeInstanceRepo())
	err := svc.DeleteAttribute(context.Background(), 66)
	if !errors.Is(err, errs.ErrAttributeNotFound) {
		t.Fatalf("expected ErrAttributeNotFound on repo error, got %v", err)
	}
}
