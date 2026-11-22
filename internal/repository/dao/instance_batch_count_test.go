package dao

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

// TestInsertedInFailedInsert 锁定「有序批量插入失败时，本块已写入多少条」的推断口径。
//
// 背景（2026-11-20，评审 R3 #1/#2/#3）：早期实现直接用
// BulkWriteException.WriteErrors[0].Index 当「整个批次已写入条数」，但驱动的
// retryable writes 会把超大批量切成多个子批次，下标只相对失败子批次；同时
// 下标 0 基、等于块长度属异常值；仅写关注错误时文档其实已全部写入。
func TestInsertedInFailedInsert(t *testing.T) {
	t.Run("写错误按下标返回已写入条数", func(t *testing.T) {
		err := mongo.BulkWriteException{WriteErrors: []mongo.BulkWriteError{{WriteError: mongo.WriteError{Index: 2}}}}
		n, ok := insertedInFailedInsert(err, 5)
		if !ok || n != 2 {
			t.Fatalf("期望 (2, true)，实际 (%d, %v)", n, ok)
		}
	})

	t.Run("首条即失败返回 0 条且可确定", func(t *testing.T) {
		err := mongo.BulkWriteException{WriteErrors: []mongo.BulkWriteError{{WriteError: mongo.WriteError{Index: 0}}}}
		n, ok := insertedInFailedInsert(err, 5)
		if !ok || n != 0 {
			t.Fatalf("期望 (0, true)，实际 (%d, %v)", n, ok)
		}
	})

	t.Run("下标等于块长度属异常值，按无法确定处理", func(t *testing.T) {
		// 0 基下标最大只能是 chunkLen-1；出现 chunkLen 说明不能据此推断，
		// 否则会把失败的那条也当成已写入（评审 R3 #2）。
		err := mongo.BulkWriteException{WriteErrors: []mongo.BulkWriteError{{WriteError: mongo.WriteError{Index: 5}}}}
		if n, ok := insertedInFailedInsert(err, 5); ok {
			t.Fatalf("不应判定为可确定，实际 (%d, true)", n)
		}
	})

	t.Run("负下标按无法确定处理", func(t *testing.T) {
		err := mongo.BulkWriteException{WriteErrors: []mongo.BulkWriteError{{WriteError: mongo.WriteError{Index: -1}}}}
		if n, ok := insertedInFailedInsert(err, 5); ok {
			t.Fatalf("不应判定为可确定，实际 (%d, true)", n)
		}
	})

	t.Run("仅写关注错误按整块已写入处理", func(t *testing.T) {
		err := mongo.BulkWriteException{WriteConcernError: &mongo.WriteConcernError{Code: 64, Message: "wtimeout"}}
		n, ok := insertedInFailedInsert(err, 5)
		if !ok || n != 5 {
			t.Fatalf("期望 (5, true)，实际 (%d, %v)", n, ok)
		}
	})

	t.Run("普通错误不推断", func(t *testing.T) {
		if n, ok := insertedInFailedInsert(errors.New("network"), 5); ok {
			t.Fatalf("不应推断，实际 (%d, true)", n)
		}
	})
}
