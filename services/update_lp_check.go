package services

import (
	"context"
	"log"

	"github.com/MixinNetwork/supergroup/models"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/jackc/pgx/v4"
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
		if err := session.Database(ctx).ConnQuery(ctx, `
SELECT asset_id FROM assets WHERE asset_id IN
  (SELECT lp_asset FROM swap WHERE asset0=$1 OR asset1=$1)
`, func(rows pgx.Rows) error {
			for rows.Next() {
				var assetID string
				if err := rows.Scan(&assetID); err != nil {
					return err
				}
				if err := models.UpdateClientAssetLPCheck(ctx, client.ClientID, assetID); err != nil {
					return err
				}
			}
			return nil
		}, client.AssetID); err != nil {
			log.Println(err)
			return err
		}
	}
	return nil
}
