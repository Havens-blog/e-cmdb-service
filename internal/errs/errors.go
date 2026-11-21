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
	ModelNotFound     = ErrorCode{Code: 404001, Msg: "model not found"}
	ModelExists       = ErrorCode{Code: 409001, Msg: "model already exists"}
	ModelInvalid      = ErrorCode{Code: 400002, Msg: "model invalid"}
	ModelHasInstances = ErrorCode{Code: 400009, Msg: "model has instances, cannot delete"}
	// RelationTypeInUse：模型关系类型仍被资产关系引用。
	// 与 ModelHasInstances 同族（400 段），前端按 code 判 4xx 即可提示。
	RelationTypeInUse = ErrorCode{Code: 400010, Msg: "relation type is in use by instance relations, cannot delete"}
	// AttributeInUse：字段仍被实例数据使用，不能删除。
	// 与 ModelHasInstances / RelationTypeInUse 同族（「被引用不可删」语义）。
	// 注：400011–400013 已被在途分支预留（关系类型引用的源/目标模型不存在、实例关系模型对不匹配）。
	AttributeInUse = ErrorCode{Code: 400014, Msg: "attribute in use by instances, cannot delete"}
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
	// RelationModelMismatch: 关系类型声明了固定的「源模型 → 目标模型」，
	// 两端实例的模型必须与之匹配，否则关系语义（方向/基数）无法成立。
	RelationModelMismatch = ErrorCode{Code: 400010, Msg: "relation model mismatch"}
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

// 开放 API 相关错误码
var (
	OpenAPITokenNotFound   = ErrorCode{Code: 404006, Msg: "api token not found"}
	OpenAPIWebhookNotFound = ErrorCode{Code: 404007, Msg: "webhook not found"}
)

// 标准错误
var (
	ErrInvalidModelUID      = errors.New("model uid cannot be empty")
	ErrInvalidModelName     = errors.New("model name cannot be empty")
	ErrInvalidModelCategory = errors.New("model category cannot be empty")
	ErrModelNotFound        = errors.New("model not found")
	ErrModelExists          = errors.New("model already exists")
	ErrModelHasInstances    = errors.New("model has instances, cannot delete")
	// ErrRelationTypeInUse 由 service.RelationTypeInUseError 通过 Is 方法匹配。
	ErrRelationTypeInUse = errors.New("relation type is in use by instance relations")

	ErrInvalidAssetID    = errors.New("asset id cannot be empty")
	ErrInvalidTenantID   = errors.New("tenant id cannot be empty")
	ErrInstanceNotFound  = errors.New("instance not found")
	ErrInstanceExists    = errors.New("instance already exists")
	ErrInvalidAttributes = errors.New("invalid attributes")

	ErrRelationNotFound      = errors.New("relation not found")
	ErrRelationExists        = errors.New("relation already exists")
	ErrInvalidRelation       = errors.New("invalid relation")
	ErrRelationModelMismatch = errors.New("relation model mismatch")

	ErrAttributeNotFound = errors.New("attribute not found")
	ErrAttributeExists   = errors.New("attribute already exists")
	// ErrAttributeInUse 由 service.AttributeInUseError 通过 Is 方法匹配。
	ErrAttributeInUse = errors.New("attribute in use by instances")

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

	// 开放 API 相关错误
	ErrAPITokenNotFound   = errors.New("api token not found")
	ErrWebhookNotFound    = errors.New("webhook not found")
	ErrInvalidTokenName   = errors.New("token name cannot be empty")
	ErrTokenNameTooLong   = errors.New("token name is too long")
	ErrInvalidTokenTTL    = errors.New("token ttl_days cannot be negative")
	ErrTokenTTLTooLarge   = errors.New("token ttl_days exceeds the maximum")
	ErrInvalidWebhookName = errors.New("webhook name cannot be empty")
	ErrWebhookNameTooLong = errors.New("webhook name is too long")
	ErrInvalidTargetURL   = errors.New("webhook target_url cannot be empty")
	ErrTargetURLTooLong   = errors.New("webhook target_url is too long")
	ErrInvalidEventType   = errors.New("invalid webhook event type")
	ErrInvalidStatus      = errors.New("invalid status")
)
