package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
)

// upsertFailRepo 在第 failAt 次 Upsert 调用时返回错误，用于验证批量写入的
// 「第几条失败 / 已写入几条」是否准确带出。
type upsertFailRepo struct {
	*fakeInstanceRepo
	failAt   int // 1 起；0 表示不失败
	upserted []domain.Instance
}

func (r *upsertFailRepo) Upsert(ctx context.Context, instance domain.Instance) error {
	if r.failAt > 0 && len(r.upserted)+1 == r.failAt {
		return errors.New("mongo: connection reset by peer")
	}
	r.upserted = append(r.upserted, instance)
	return nil
}

// TestUpsertBatchError_ReportsRowAndWrittenCount 锁定批量写入失败的定位信息。
//
// 背景（2026-11-19）：UpsertBatch 首错即 return 底层错误，调用方只能看到
// "asset id cannot be empty" 这类英文原文，既不知道是第几行，也不知道
// 前面几行是否已经写进库（首错中止不回滚）。导入 50 行时无法判断该整批重试
// 还是从断点续传。
func TestUpsertBatchError_ReportsRowAndWrittenCount(t *testing.T) {
	newSvc := func(repo *upsertFailRepo) InstanceService {
		return NewInstanceService(repo, &fakeModelRepo{getByUID: domain.Model{ID: 1}}, nil, nil, nil, nil)
	}

	t.Run("第二条校验失败：报告第 2 条、已写入 1 条", func(t *testing.T) {
		repo := &upsertFailRepo{fakeInstanceRepo: newFakeInstanceRepo()}
		svc := newSvc(repo)

		err := svc.UpsertBatch(context.Background(), []domain.Instance{
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-ok-1", Attributes: map[string]interface{}{}},
			{TenantID: 3, ModelUID: "ecs", AssetID: "", Attributes: map[string]interface{}{}},
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-never", Attributes: map[string]interface{}{}},
		})
		if err == nil {
			t.Fatal("应返回错误")
		}
		var batchErr *UpsertBatchError
		if !errors.As(err, &batchErr) {
			t.Fatalf("错误类型应为 *UpsertBatchError，实际 %T", err)
		}
		if batchErr.Index != 1 {
			t.Errorf("失败下标应为 1（第 2 条），实际 %d", batchErr.Index)
		}
		if batchErr.Written != 1 {
			t.Errorf("已写入应为 1 条，实际 %d", batchErr.Written)
		}
		if batchErr.Total != 3 {
			t.Errorf("总数应为 3，实际 %d", batchErr.Total)
		}
		if !errors.Is(err, errs.ErrInvalidAssetID) {
			t.Errorf("应能通过 errors.Is 判断根因 ErrInvalidAssetID，实际 %v", err)
		}
		msg := err.Error()
		for _, want := range []string{"第 2 条", "资产ID不能为空", "已写入 1 条", "共 3 条", "其余未处理"} {
			if !strings.Contains(msg, want) {
				t.Errorf("报文缺少 %q：%s", want, msg)
			}
		}
		// 不允许出现英文哨兵原文
		if strings.Contains(msg, "asset id cannot be empty") {
			t.Errorf("报文不应含英文哨兵原文：%s", msg)
		}
		// 第三条不应被写入
		if len(repo.upserted) != 1 {
			t.Errorf("只应写入第 1 条，实际写入 %d 条", len(repo.upserted))
		}
	})

	t.Run("仓储失败：保留原始原因并带定位信息", func(t *testing.T) {
		repo := &upsertFailRepo{fakeInstanceRepo: newFakeInstanceRepo(), failAt: 2}
		svc := newSvc(repo)

		err := svc.UpsertBatch(context.Background(), []domain.Instance{
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-ok", Attributes: map[string]interface{}{}},
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-boom", Attributes: map[string]interface{}{}},
		})
		var batchErr *UpsertBatchError
		if !errors.As(err, &batchErr) {
			t.Fatalf("错误类型应为 *UpsertBatchError，实际 %T", err)
		}
		if batchErr.Index != 1 || batchErr.Written != 1 {
			t.Errorf("下标应为 1、已写入 1，实际 %d / %d", batchErr.Index, batchErr.Written)
		}
		// 仓储错误属运维视角，保留原文
		if !strings.Contains(err.Error(), "connection reset") {
			t.Errorf("仓储错误原文应保留：%s", err.Error())
		}
		if IsInstanceWriteValidationError(err) {
			t.Error("仓储错误不应被判定为调用方数据问题")
		}
	})

	t.Run("全部成功不产生错误", func(t *testing.T) {
		repo := &upsertFailRepo{fakeInstanceRepo: newFakeInstanceRepo()}
		svc := newSvc(repo)
		if err := svc.UpsertBatch(context.Background(), []domain.Instance{
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-a", Attributes: map[string]interface{}{}},
			{TenantID: 3, ModelUID: "ecs", AssetID: "i-b", Attributes: map[string]interface{}{}},
		}); err != nil {
			t.Fatalf("应全部成功: %v", err)
		}
		if len(repo.upserted) != 2 {
			t.Errorf("应写入 2 条，实际 %d", len(repo.upserted))
		}
	})
}

func TestDescribeInstanceWriteError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"asset id", errs.ErrInvalidAssetID, "资产ID不能为空"},
		{"model uid", errs.ErrInvalidModelUID, "模型不能为空"},
		{"tenant", errs.ErrInvalidTenantID, "租户信息缺失，请重新登录后重试"},
		{"attributes", errs.ErrInvalidAttributes, "字段校验未通过"},
		// 语义是「最内层原因」：批量报文由 UpsertBatchError.Error() 负责拼接行号与条数
		{"wrapped", &UpsertBatchError{Err: errs.ErrInvalidAssetID}, "资产ID不能为空"},
		{"unknown", errors.New("mongo: timeout"), "mongo: timeout"},
		{"nil", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DescribeInstanceWriteError(c.err)
			if !strings.Contains(got, c.want) {
				t.Errorf("期望包含 %q，实际 %q", c.want, got)
			}
		})
	}
}

func TestIsInstanceWriteValidationError(t *testing.T) {
	validation := []error{
		errs.ErrInvalidAssetID,
		errs.ErrInvalidModelUID,
		errs.ErrInvalidTenantID,
		errs.ErrInvalidAttributes,
		&UpsertBatchError{Index: 2, Written: 2, Total: 5, Err: errs.ErrInvalidAssetID},
	}
	for _, err := range validation {
		if !IsInstanceWriteValidationError(err) {
			t.Errorf("%v 应判定为调用方数据问题", err)
		}
	}
	notValidation := []error{
		nil,
		errors.New("mongo: server selection timeout"),
		&UpsertBatchError{Index: 0, Err: errors.New("mongo: timeout")},
	}
	for _, err := range notValidation {
		if IsInstanceWriteValidationError(err) {
			t.Errorf("%v 不应判定为调用方数据问题", err)
		}
	}
}
