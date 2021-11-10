package services

import (
	"context"

	"github.com/MixinNetwork/supergroup/models"
	"github.com/MixinNetwork/supergroup/session"
)

type UpdateLpCheckService struct{}

func (service *UpdateLpCheckService) Run(ctx context.Context) error {
	// 1. 获取 client_id 相关
	list, err := models.GetClientList(ctx)
	if err != nil {
		return err
	}

	for _, client := range list {
		if client.AssetID == "" {
			continue
		}
		// 根据 asset_id 找到 swap 中 两个交易对有其一的
		rows, err := session.Database(ctx).Query(ctx, `
SELECT asset_id FROM assets WHERE asset_id IN
	(SELECT lp_asset FROM swap WHERE asset0=$1 OR asset1=$1)`, client.AssetID)
		if err != nil {
			return err
		}

		for rows.Next() {
			var assetID string
			if err := rows.Scan(&assetID); err != nil {
				return err
			}
			if err := models.UpdateClientAssetLPCheck(ctx, client.ClientID, assetID); err != nil {
				return err
			}
		}
	}
	return nil
}
