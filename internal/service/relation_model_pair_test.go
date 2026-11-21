package service

import (
	"context"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
)

// fakeModelRelRepo 只实现 GetByUID：关系创建校验只需要按 uid 取关系类型声明。
// 其余方法在测试里不会被调用，返回零值以便满足仓储接口。
type fakeModelRelRepo struct {
	types map[string]domain.ModelRelationType
}

func newFakeModelRelRepo(types ...domain.ModelRelationType) *fakeModelRelRepo {
	repo := &fakeModelRelRepo{types: make(map[string]domain.ModelRelationType, len(types))}
	for _, t := range types {
		repo.types[t.UID] = t
	}
	return repo
}

func (f *fakeModelRelRepo) Create(context.Context, domain.ModelRelationType) (int64, error) {
	return 0, nil
}

func (f *fakeModelRelRepo) GetByUID(_ context.Context, uid string) (domain.ModelRelationType, error) {
	return f.types[uid], nil
}

func (f *fakeModelRelRepo) GetByID(context.Context, int64) (domain.ModelRelationType, error) {
	return domain.ModelRelationType{}, nil
}

func (f *fakeModelRelRepo) List(context.Context, domain.ModelRelationTypeFilter) ([]domain.ModelRelationType, error) {
	return nil, nil
}

func (f *fakeModelRelRepo) Count(context.Context, domain.ModelRelationTypeFilter) (int64, error) {
	return 0, nil
}

func (f *fakeModelRelRepo) Update(context.Context, domain.ModelRelationType) error {
	return nil
}

func (f *fakeModelRelRepo) Delete(context.Context, string) error {
	return nil
}

func (f *fakeModelRelRepo) Exists(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeModelRelRepo) FindByModels(context.Context, string, string) ([]domain.ModelRelationType, error) {
	return nil, nil
}
