package dao

import (
	"context"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MissingFieldInstance 缺字段实例的最小字段视图（数据质量明细用）。
type MissingFieldInstance struct {
	ID        int64  `bson:"id"`
	AssetID   string `bson:"asset_id"`
	AssetName string `bson:"asset_name"`
	ModelUID  string `bson:"model_uid"`
}

// missingFieldQuery 构造「某模型缺少指定属性」的查询。
func missingFieldQuery(tenantID int64, modelUID, fieldUID string) bson.M {
	return bson.M{
		"tenant_id":              tenantID,
		"model_uid":              modelUID,
		"attributes." + fieldUID: bson.M{"$exists": false},
	}
}

// CountMissingAttributeForModel 统计某 (模型, 字段) 组合下缺该属性的实例数。
// 命名与既有 CountMissingAttribute 区分：后者已存在，此处显式按组合计数。
func (d *instanceDAO) CountMissingFieldGroup(ctx context.Context, tenantID int64, modelUID, fieldUID string) (int64, error) {
	if modelUID == "" || fieldUID == "" {
		return 0, nil
	}
	return d.db.Collection(InstanceCollection).CountDocuments(ctx, missingFieldQuery(tenantID, modelUID, fieldUID))
}

// ListMissingFieldGroup 分页拉取某 (模型, 字段) 组合下缺该属性的实例。
//
// 排序固定按 id 升序，保证同一页多次请求结果稳定。
func (d *instanceDAO) ListMissingFieldGroup(
	ctx context.Context,
	tenantID int64,
	modelUID, fieldUID string,
	offset, limit int64,
) ([]MissingFieldInstance, error) {
	if modelUID == "" || fieldUID == "" || limit <= 0 {
		return nil, nil
	}
	if offset < 0 {
		offset = 0
	}

	opts := options.Find().
		SetProjection(bson.M{"id": 1, "asset_id": 1, "asset_name": 1, "model_uid": 1}).
		SetSort(bson.M{"id": 1}).
		SetSkip(offset).
		SetLimit(limit)

	cursor, err := d.db.Collection(InstanceCollection).Find(ctx, missingFieldQuery(tenantID, modelUID, fieldUID), opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []MissingFieldInstance
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// MissingFieldCountsByModel 批量统计结果：某模式下各字段的缺失实例数。
type MissingFieldCountsByModel struct {
	ModelUID string
	// Counts: field_uid -> 缺失该字段的实例数（仅包含 count > 0 的字段）
	Counts map[string]int64
}

// BatchCountMissingFieldGroups 一次聚合算出「多个 (模型,字段) 组合」的缺失实例数。
//
// 解决的问题：原实现 issuesMissingByPushdown 对每个 (模型,字段) 组合串行调用
// CountMissingFieldGroup。实测 tenant 3 有 59 个必填字段组合，即 59 次串行
// Mongo count，实测总耗时约 518ms（平均每次 8.8ms）。
// 而一次全表 $group 扫描实测仅 51ms —— 差距来自 59 次网络往返。
//
// 实现：按 model_uid 分组，对每个待统计字段用
//
//	$sum: { $cond: [ { $eq: [ { $type: "$attributes.<field>" }, "missing" ] }, 1, 0 ] }
//
// 在同一次扫描中累加。语义与 missingFieldQuery 的
// `attributes.<field>: {$exists:false}` 等价（$type 返回 "missing" 表示字段不存在）。
//
// 参数 fieldsByModel: model_uid -> 该模型下要统计的 field_uid 列表。
// 只会扫描这些 model_uid（$match 限定），不会全库扫描。
func (d *instanceDAO) BatchCountMissingFieldGroups(
	ctx context.Context,
	tenantID int64,
	fieldsByModel map[string][]string,
) (map[string]MissingFieldCountsByModel, error) {
	if len(fieldsByModel) == 0 {
		return map[string]MissingFieldCountsByModel{}, nil
	}

	models := make([]string, 0, len(fieldsByModel))
	for m := range fieldsByModel {
		if m != "" {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return map[string]MissingFieldCountsByModel{}, nil
	}

	// 每个字段一个 $sum/$cond 累加器，键名用字段序号避免特殊字符（如中文、点）冲突
	type fieldRef struct {
		fieldUID string
		alias    string
	}
	var refs []fieldRef
	groupStage := bson.D{{Key: "_id", Value: "$model_uid"}}
	seen := make(map[string]bool)
	for _, m := range models {
		for _, f := range fieldsByModel[m] {
			if f == "" {
				continue
			}
			key := m + "\x00" + f
			if seen[key] {
				continue
			}
			seen[key] = true
			alias := "f" + strconv.Itoa(len(refs))
			refs = append(refs, fieldRef{fieldUID: f, alias: alias})
			// $sum: { $cond: [ { $eq: [ { $type: "$attributes.<f>" }, "missing" ] }, 1, 0 ] }
			typeExpr := bson.D{{Key: "$type", Value: "$attributes." + f}}
			eqExpr := bson.D{{Key: "$eq", Value: bson.A{typeExpr, "missing"}}}
			condExpr := bson.D{{Key: "$cond", Value: bson.A{eqExpr, 1, 0}}}
			sumExpr := bson.D{{Key: "$sum", Value: condExpr}}
			groupStage = append(groupStage, bson.E{Key: alias, Value: sumExpr})
		}
	}
	if len(refs) == 0 {
		return map[string]MissingFieldCountsByModel{}, nil
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"tenant_id": tenantID,
			"model_uid": bson.M{"$in": models},
		}}},
		{{Key: "$group", Value: groupStage}},
	}

	cur, err := d.db.Collection(InstanceCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var rows []bson.M
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}

	// alias -> fieldUID（同一 model 内 alias 唯一，但 alias 是按出现顺序全局分配的，
	// 因此需按 model 分别建立 alias -> field 映射）
	aliasByModel := make(map[string]map[string]string, len(models))
	for _, m := range models {
		m2 := make(map[string]string)
		for _, f := range fieldsByModel[m] {
			if f == "" {
				continue
			}
			for _, r := range refs {
				if r.fieldUID == f {
					m2[r.alias] = f
					break
				}
			}
		}
		aliasByModel[m] = m2
	}

	out := make(map[string]MissingFieldCountsByModel, len(rows))
	for _, row := range rows {
		mu, _ := row["_id"].(string)
		if mu == "" {
			continue
		}
		counts := make(map[string]int64)
		for alias, f := range aliasByModel[mu] {
			if v, ok := toInt64(row[alias]); ok && v > 0 {
				counts[f] = v
			}
		}
		out[mu] = MissingFieldCountsByModel{ModelUID: mu, Counts: counts}
	}
	return out, nil
}

// toInt64 宽容地把聚合结果中的数值转成 int64（Mongo 可能返回 int32/int64/float64）。
func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}
