package models

import (
	"context"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/shopspring/decimal"
)

const client_asset_lp_check_DDL = `
-- 机器人 lp token 换算表
CREATE TABLE IF NOT EXISTS client_asset_lp_check (
  client_id          VARCHAR(36),
  asset_id           VARCHAR(36),
  updated_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  PRIMARY KEY(client_id, asset_id)
);

`

type ClientAssetLpCheck struct {
	ClientID  string
	AssetID   string
	UpdatedAt time.Time
	CreatedAt time.Time

	PriceUsd decimal.Decimal
}

func UpdateClientAssetLPCheck(ctx context.Context, clientID, assetID string) error {
	query := durable.InsertQueryOrUpdate("client_asset_lp_check", "client_id,asset_id", "updated_at")
	_, err := session.Database(ctx).Exec(ctx, query, clientID, assetID, time.Now())
	return err
}

func GetClientAssetLPCheckMapByID(ctx context.Context, clientID string) (map[string]decimal.Decimal, error) {
	rows, err := session.Database(ctx).Query(ctx, ` SELECT calc.client_id,calc.asset_id,a.price_usd FROM client_asset_lp_check AS calc LEFT JOIN assets AS a ON calc.asset_id=a.asset_id WHERE calc.client_id=$1 `, clientID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]decimal.Decimal)
	for rows.Next() {
		var ca ClientAssetLpCheck
		if err := rows.Scan(&ca.ClientID, &ca.AssetID, &ca.PriceUsd); err != nil {
			return nil, err
		}
		result[ca.AssetID] = ca.PriceUsd
	}
	return result, nil
}
