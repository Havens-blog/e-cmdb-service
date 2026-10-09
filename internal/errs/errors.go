package errs

import "errors"

// ErrorCode 错误码结构
type ErrorCode struct {
	Code int
	Msg  string
}

// 通用错误码
var (
	Success     = ErrorCode{Code: 0, Msg: "success"}
	ParamsError = ErrorCode{Code: 400001, Msg: "params error"}
	SystemError = ErrorCode{Code: 500001, Msg: "system error"}
)

// 模型相关错误码
var (
	ModelNotFound = ErrorCode{Code: 404001, Msg: "model not found"}
	ModelExists   = ErrorCode{Code: 409001, Msg: "model already exists"}
	ModelInvalid  = ErrorCode{Code: 400002, Msg: "model invalid"}
)

// 实例相关错误码
var (
	InstanceNotFound = ErrorCode{Code: 404002, Msg: "instance not found"}
	InstanceExists   = ErrorCode{Code: 409002, Msg: "instance already exists"}
	InstanceInvalid  = ErrorCode{Code: 400003, Msg: "instance invalid"}
)

// 关系相关错误码
var (
	RelationNotFound = ErrorCode{Code: 404003, Msg: "relation not found"}
	RelationExists   = ErrorCode{Code: 409003, Msg: "relation already exists"}
	RelationInvalid  = ErrorCode{Code: 400004, Msg: "relation invalid"}
)

// 属性相关错误码
var (
	AttributeNotFound = ErrorCode{Code: 404004, Msg: "attribute not found"}
	AttributeExists   = ErrorCode{Code: 409004, Msg: "attribute already exists"}
	AttributeInvalid  = ErrorCode{Code: 400005, Msg: "attribute invalid"}
)

// 模型分组相关错误码
var (
	ModelGroupNotFound  = ErrorCode{Code: 404005, Msg: "model group not found"}
	ModelGroupExists    = ErrorCode{Code: 409005, Msg: "model group already exists"}
	ModelGroupInvalid   = ErrorCode{Code: 400006, Msg: "model group invalid"}
	GroupHasModels      = ErrorCode{Code: 400007, Msg: "group has models, cannot delete"}
	CannotDeleteBuiltin = ErrorCode{Code: 400008, Msg: "cannot delete builtin group"}
)

// 标准错误
var (
	ErrInvalidModelUID      = errors.New("model uid cannot be empty")
	ErrInvalidModelName     = errors.New("model name cannot be empty")
	ErrInvalidModelCategory = errors.New("model category cannot be empty")
	ErrModelNotFound        = errors.New("model not found")
	ErrModelExists          = errors.New("model already exists")

	ErrInvalidAssetID    = errors.New("asset id cannot be empty")
	ErrInvalidTenantID   = errors.New("tenant id cannot be empty")
	ErrInstanceNotFound  = errors.New("instance not found")
	ErrInstanceExists    = errors.New("instance already exists")
	ErrInvalidAttributes = errors.New("invalid attributes")

	ErrRelationNotFound = errors.New("relation not found")
	ErrRelationExists   = errors.New("relation already exists")
	ErrInvalidRelation  = errors.New("invalid relation")

	ErrAttributeNotFound = errors.New("attribute not found")
	ErrAttributeExists   = errors.New("attribute already exists")

	ErrModelGroupNotFound  = errors.New("model group not found")
	ErrModelGroupExists    = errors.New("model group already exists")
	ErrGroupHasModels      = errors.New("group has models, cannot delete")
	ErrCannotDeleteBuiltin = errors.New("cannot delete builtin group")

	// 属性相关错误
	ErrInvalidAttributeUID   = errors.New("attribute uid cannot be empty")
	ErrInvalidAttributeName  = errors.New("attribute name cannot be empty")
	ErrInvalidAttributeType  = errors.New("invalid attribute type")
	ErrInvalidAttributeGroup = errors.New("invalid attribute group")

	// 属性分组相关错误
	ErrAttributeGroupNotFound   = errors.New("attribute group not found")
	ErrBuiltinGroupCannotDelete = errors.New("builtin group cannot be deleted")
)
