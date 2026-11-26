package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"go.mongodb.org/mongo-driver/mongo"
)

// updateErrGroupRepo 在 Update 上返回预设错误，用于锁定「目标行消失」的语义翻转。
type updateErrGroupRepo struct {
	fakeGroupRepo
	updateErr error
}

func (f *updateErrGroupRepo) Update(context.Context, domain.ModelGroup) error { return f.updateErr }

// TestModelGroupUpdate_MissingRowIsNotFound 锁定：DAO 用 MatchedCount==0 报告并发删除时，
// service 必须翻成 404 语义，而不是把 no-op 当成 200 成功返回给客户端。
func TestModelGroupUpdate_MissingRowIsNotFound(t *testing.T) {
	t.Parallel()

	repo := &updateErrGroupRepo{
		fakeGroupRepo: fakeGroupRepo{existing: domain.ModelGroup{ID: 3, UID: "basic", Name: "基础"}},
		updateErr:     mongo.ErrNoDocuments,
	}
	svc := NewModelGroupService(repo, nil)

	err := svc.Update(context.Background(), domain.ModelGroup{UID: "basic", Name: "基础"})
	if !errors.Is(err, errs.ErrModelGroupNotFound) {
		t.Fatalf("应翻成 ErrModelGroupNotFound，实际 %v", err)
	}
}

// TestModelGroupUpdate_PropagatesOtherErrors 防止把真正的仓储故障也当成 404 吞掉。
func TestModelGroupUpdate_PropagatesOtherErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("mongo: connection refused")
	repo := &updateErrGroupRepo{
		fakeGroupRepo: fakeGroupRepo{existing: domain.ModelGroup{ID: 3, UID: "basic", Name: "基础"}},
		updateErr:     sentinel,
	}
	svc := NewModelGroupService(repo, nil)

	err := svc.Update(context.Background(), domain.ModelGroup{UID: "basic", Name: "基础"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("仓储故障应原样透传，实际 %v", err)
	}
	if errors.Is(err, errs.ErrModelGroupNotFound) {
		t.Fatal("仓储故障不能被翻成 404")
	}
}
