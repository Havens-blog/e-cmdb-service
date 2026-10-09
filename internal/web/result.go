package web

import (
	"github.com/Havens-blog/e-cmdb-service/internal/errs"
	"github.com/Havens-blog/e-common-go/ginx"
)

// Result 统一响应结果
func Result(data any) ginx.Result {
	return ginx.Result{
		Code: errs.Success.Code,
		Msg:  errs.Success.Msg,
		Data: data,
	}
}

// ErrorResult 错误响应结果
func ErrorResult(err errs.ErrorCode) ginx.Result {
	return ginx.Result{
		Code: err.Code,
		Msg:  err.Msg,
		Data: nil,
	}
}

// ErrorResultWithMsg 带自定义消息的错误响应结果
func ErrorResultWithMsg(err errs.ErrorCode, msg string) ginx.Result {
	return ginx.Result{
		Code: err.Code,
		Msg:  msg,
		Data: nil,
	}
}
