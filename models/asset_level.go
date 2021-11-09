package models

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/jackc/pgx/v4"
	"github.com/shopspring/decimal"
)

const client_asset_level_DDL = `
-- 用户的持仓等级
CREATE TABLE IF NOT EXISTS client_asset_level (
  client_id          VARCHAR(36) PRIMARY KEY,
  fresh              VARCHAR NOT NULL DEFAULT '0',
  senior             VARCHAR NOT NULL DEFAULT '0',
  large              VARCHAR NOT NULL DEFAULT '0',
	fresh_amount       VARCHAR NOT NULL DEFAULT '0',
	large_amount       VARCHAR NOT NULL DEFAULT '0',
  created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
`

type ClientAssetLevel struct {
	ClientID    string          `json:"client_id,omitempty"`
	Fresh       decimal.Decimal `json:"fresh,omitempty"`
	Senior      decimal.Decimal `json:"senior,omitempty"`
	Large       decimal.Decimal `json:"large,omitempty"`
	FreshAmount decimal.Decimal `json:"fresh_amount,omitempty"`
	LargeAmount decimal.Decimal `json:"large_amount,omitempty"`
	CreatedAt   time.Time       `json:"created_at,omitempty"`
}

func UpdateClientAssetLevel(ctx context.Context, level *ClientAssetLevel) error {
	query := durable.InsertQueryOrUpdate("client_asset_level", "client_id", "fresh,senior,large")
	_, err := session.Database(ctx).Exec(ctx, query, level.ClientID, level.Fresh, level.Senior, level.Large)
	return err
}

var cacheClientAssetLevel = make(map[string]ClientAssetLevel)
var nilAssetLevel = ClientAssetLevel{}

func GetClientVipAmount(ctx context.Context, host string) (ClientAssetLevel, error) {
	c, err := GetClientInfoByHostOrID(ctx, host, "")
	if err != nil {
		return ClientAssetLevel{}, err
	}
	return GetClientAssetLevel(ctx, c.ClientID)
}

func GetClientAssetLevel(ctx context.Context, clientID string) (ClientAssetLevel, error) {
	if cacheClientAssetLevel[clientID] == nilAssetLevel {
		var cal ClientAssetLevel
		if err := session.Database(ctx).QueryRow(ctx, `
SELECT client_id, fresh, senior, large, fresh_amount, large_amount
FROM client_asset_level
WHERE client_id=$1
`, clientID).Scan(&cal.ClientID, &cal.Fresh, &cal.Senior, &cal.Large, &cal.FreshAmount, &cal.LargeAmount); err != nil {
			return cal, err
		}
		cacheClientAssetLevel[clientID] = cal
	}
	return cacheClientAssetLevel[clientID], nil
}

func GetClientUserStatusByClientUser(ctx context.Context, u *ClientUser) (int, error) {
	foxUserAssetMap, _ := GetAllUserFoxShares(ctx, []string{u.UserID})
	exinUserAssetMap, _ := GetAllUserExinShares(ctx, []string{u.UserID})
	return GetClientUserStatus(ctx, u, foxUserAssetMap[u.UserID], exinUserAssetMap[u.UserID])
}

func GetClientUserUsdAmountByClientUser(ctx context.Context, u *ClientUser) (decimal.Decimal, error) {
	client := GetMixinClientByID(ctx, u.ClientID)
	assets, err := GetUserAssets(ctx, u.AccessToken)
	if err != nil {
		return decimal.Zero, err
	}
	foxAsset, _ := GetAllUserFoxShares(ctx, []string{u.UserID})
	exinAsset, _ := GetAllUserExinShares(ctx, []string{u.UserID})
	return getNoAssetUserStatus(ctx, &client, assets, foxAsset[u.UserID], exinAsset[u.UserID])
}

// 更新每个社群的币资产数量
func GetClientUserStatus(ctx context.Context, u *ClientUser, foxAsset durable.AssetMap, exinAsset durable.AssetMap) (int, error) {
	client := GetMixinClientByID(ctx, u.ClientID)
	if client.ClientID == "" {
		return ClientUserStatusAudience, session.BadDataError(ctx)
	}
	assets, err := GetUserAssets(ctx, u.AccessToken)
	if err != nil {
		// 获取资产出现问题
		if strings.Contains(err.Error(), "Forbidden") {
			// 授权出问题了，降为 观众
			return ClientUserStatusAudience, nil
		}
		return ClientUserStatusAudience, err
	}
	assetLevel, err := GetClientAssetLevel(ctx, client.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ClientUserStatusAudience, nil
		}
		return ClientUserStatusAudience, err
	}
	totalAmount := decimal.Zero
	if client.AssetID != "" {
		totalAmount, err = getHasAssetUserStatus(ctx, &client, assets, assetLevel, foxAsset, exinAsset)
	} else {
		totalAmount, err = getNoAssetUserStatus(ctx, &client, assets, foxAsset, exinAsset)
	}
	if err != nil {
		session.Logger(ctx).Println(err)
		return ClientUserStatusAudience, nil
	}

	if assetLevel.Large.LessThanOrEqual(totalAmount) {
		return ClientUserStatusLarge, nil
	}
	if assetLevel.Senior.LessThanOrEqual(totalAmount) {
		return ClientUserStatusSenior, nil
	}
	if assetLevel.Fresh.LessThanOrEqual(totalAmount) {
		return ClientUserStatusFresh, nil
	}
	return ClientUserStatusAudience, nil
}

func getHasAssetUserStatus(ctx context.Context, client *MixinClient, assets []*mixin.Asset, assetLevel ClientAssetLevel, foxAsset durable.AssetMap, exinAsset durable.AssetMap) (decimal.Decimal, error) {
	lpPriceMap, err := GetClientAssetLPCheckMapByID(ctx, client.ClientID)
	if err != nil {
		return decimal.Zero, err
	}
	asset, err := GetAssetByID(ctx, client.Client, client.AssetID)
	if err != nil {
		return decimal.Zero, err
	}
	totalAmount := decimal.Zero
	for _, m := range assets {
		if !lpPriceMap[m.AssetID].IsZero() {
			if asset.PriceUsd.IsZero() {
				return assetLevel.Large, err
			}
			amount := lpPriceMap[m.AssetID].Mul(m.Balance).Div(asset.PriceUsd)
			totalAmount = totalAmount.Add(amount)
		}
		if m.AssetID == asset.AssetID {
			totalAmount = totalAmount.Add(m.Balance)
		}
	}
	if !foxAsset[asset.AssetID].IsZero() {
		totalAmount = totalAmount.Add(foxAsset[asset.AssetID])
	}
	if !exinAsset[asset.AssetID].IsZero() {
		totalAmount = totalAmount.Add(exinAsset[asset.AssetID])
	}
	return totalAmount, nil
}

func getNoAssetUserStatus(ctx context.Context, client *MixinClient, assets []*mixin.Asset, foxAsset durable.AssetMap, exinAsset durable.AssetMap) (decimal.Decimal, error) {
	totalAmount := decimal.Zero
	for _, asset := range assets {
		if !asset.PriceUSD.IsZero() {
			totalAmount = totalAmount.Add(asset.PriceUSD.Mul(asset.Balance))
		}
	}
	for assetID, balance := range foxAsset {
		asset, err := GetAssetByID(ctx, client.Client, assetID)
		if err == nil && !asset.PriceUsd.IsZero() && !balance.IsZero() {
			totalAmount = totalAmount.Add(asset.PriceUsd.Mul(balance))
		}
	}
	for assetID, balance := range exinAsset {
		asset, err := GetAssetByID(ctx, client.Client, assetID)
		if err == nil && !asset.PriceUsd.IsZero() && !balance.IsZero() {
			totalAmount = totalAmount.Add(asset.PriceUsd.Mul(balance))
		}
	}
	return totalAmount, nil
}

func GetAllUserFoxShares(ctx context.Context, userIDs []string) (durable.UserSharesMap, error) {
	userSharesMap := make(durable.UserSharesMap)
	if config.Config.FoxToken != "" {
		err := session.Api(context.Background()).FoxSharesCheck(userIDs, &userSharesMap)
		if err != nil {
			return nil, err
		}
	}
	return userSharesMap, nil
}

func GetAllUserExinShares(ctx context.Context, userIDs []string) (durable.UserSharesMap, error) {
	userSharesMap := make(durable.UserSharesMap)
	if config.Config.ExinToken != "" {
		assetIDs, err := getAllCheckAssetID(ctx)
		if err != nil {
			return nil, err
		}
		times := len(userIDs)/30 + 1
		for i := 0; i < times; i++ {
			start := i * 30
			var end int
			if i == times-1 {
				end = len(userIDs)
			} else {
				end = (i + 1) * 30
			}
			if err := session.Api(context.Background()).ExinSharesCheck(userIDs[start:end], assetIDs, &userSharesMap); err != nil {
				session.Logger(ctx).Println(err)
			}
		}
	}
	return userSharesMap, nil
}

func getAllCheckAssetID(ctx context.Context) ([]string, error) {
	rows, err := session.Database(ctx).Query(ctx, `SELECT distinct(c.asset_id) FROM client as c LEFT JOIN assets AS a ON c.asset_id=a.asset_id`)
	if err != nil {
		return nil, err
	}

	var assetIDs []string
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			return nil, err
		}
		assetIDs = append(assetIDs, assetID)
	}
	return assetIDs, err
}
