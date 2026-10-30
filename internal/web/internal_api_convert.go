package web

import (
	"github.com/Havens-blog/e-cmdb-service/internal/domain"
	"github.com/Havens-blog/e-cmdb-service/internal/repository/dao"
)

// toInstanceDTOs 将 domain.Instance 列表转换为跨服务 DTO。
func toInstanceDTOs(instances []domain.Instance) []InstanceDTO {
	out := make([]InstanceDTO, len(instances))
	for i, inst := range instances {
		out[i] = InstanceDTO{
			ID:         inst.ID,
			ModelUID:   inst.ModelUID,
			AssetID:    inst.AssetID,
			AssetName:  inst.AssetName,
			TenantID:   inst.TenantID,
			AccountID:  inst.AccountID,
			Attributes: inst.Attributes,
			CreateTime: inst.CreateTime.UnixMilli(),
			UpdateTime: inst.UpdateTime.UnixMilli(),
		}
	}
	return out
}

// toStatsDTO 将 dao.AssetStatsResult 转换为跨服务 DTO。
func toStatsDTO(result *dao.AssetStatsResult) StatsDTO {
	if result == nil {
		return StatsDTO{
			ByAssetType: []TypeCountDTO{},
			ByProvider:  []TypeCountDTO{},
			ByBusiness:  []TypeCountDTO{},
		}
	}
	byType := make([]TypeCountDTO, len(result.ByAssetType))
	for i, item := range result.ByAssetType {
		byType[i] = TypeCountDTO{Key: item.AssetType, Count: item.Count}
	}
	byProvider := make([]TypeCountDTO, len(result.ByProvider))
	for i, item := range result.ByProvider {
		byProvider[i] = TypeCountDTO{Key: item.Provider, Count: item.Count}
	}
	byBusiness := make([]TypeCountDTO, len(result.ByBusiness))
	for i, item := range result.ByBusiness {
		byBusiness[i] = TypeCountDTO{Key: item.Business, Count: item.Count}
	}
	return StatsDTO{
		Total:       result.Total,
		ByAssetType: byType,
		ByProvider:  byProvider,
		ByBusiness:  byBusiness,
	}
}
