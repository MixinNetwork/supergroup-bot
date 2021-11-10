package models

import (
	"context"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
)

const swap_DDL = `
CREATE TABLE IF NOT EXISTS swap(
  lp_asset            VARCHAR(36) NOT NULL PRIMARY KEY, -- lpToken asset_id
  asset0              VARCHAR(36) NOT NULL, -- asset0 asset_id
  asset0_price        VARCHAR NOT NULL, -- asset0 价格
  asset0_amount       VARCHAR NOT NULL DEFAULT '', -- asset0 数量
  asset1              VARCHAR(36) NOT NULL, -- asset1 asset_id
  asset1_price        VARCHAR NOT NULL, -- asset1 价格
  asset1_amount       VARCHAR NOT NULL DEFAULT '', -- asset1 数量
  type                VARCHAR(1) NOT NULL, -- 0 exinswap交易 1 4swap交易 2 ExinOne交易 3 ExinLocal交易
  pool                VARCHAR NOT NULL, -- 资金池总量
  earn                VARCHAR NOT NULL, -- 24小时年化收益率
  amount              VARCHAR NOT NULL, -- 24小时总交易量
  updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(), -- 更新时间
  created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW() -- 创建时间
);
CREATE INDEX IF NOT EXISTS swap_asset0_idx ON swap(asset0);
CREATE INDEX IF NOT EXISTS swap_asset1_idx ON swap(asset1);
`

type Swap struct {
	// ExinSwap
	LpAsset     string    `json:"lp_asset,omitempty"`
	Asset0      string    `json:"asset0,omitempty"`
	Asset0Price string    `json:"asset0_price,omitempty"`
	Asset1      string    `json:"asset1,omitempty"`
	Asset1Price string    `json:"asset1_price,omitempty"`
	Type        string    `json:"type,omitempty"`
	Pool        string    `json:"pool,omitempty"`
	Earn        string    `json:"earn,omitempty"`
	Amount      string    `json:"amount,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`

	// 4Swap
	IconURL      string `json:"icon_url,omitempty"`
	Asset0Symbol string `json:"asset0_symbol,omitempty"`
	Asset0Amount string `json:"asset0_amount,omitempty"`

	Asset1Symbol string `json:"asset1_symbol,omitempty"`
	Asset1Amount string `json:"asset1_amount,omitempty"`
	PriceUsd     string `json:"price_usd,omitempty"`
	ChangeUsd    string `json:"change_usd,omitempty"`

	// otc
	ExinOtcAsset
}

const (
	SwapTypeExinSwap    = "0"
	SwapType4SwapMtg    = "1"
	SwapTypeExinOne     = "2"
	SwapTypeExinLocal   = "3"
	SwapType4SwapNormal = "4"
)

func UpdateSwap(ctx context.Context, s *Swap) error {
	query := durable.InsertQueryOrUpdate("swap", "lp_asset", "asset0,asset0_price,asset0_amount,asset1,asset1_price,asset1_amount,type,pool,earn,amount,updated_at")
	_, err := session.Database(ctx).Exec(ctx, query, s.LpAsset, s.Asset0, s.Asset0Price, s.Asset0Amount, s.Asset1, s.Asset1Price, s.Asset1Amount, s.Type, s.Pool, s.Earn, s.Amount, s.UpdatedAt)
	return err
}

type SwapResp struct {
	List  []*Swap   `json:"list"`
	Asset *Asset    `json:"asset"`
	Ad    []*ExinAd `json:"ad"`
}

func GetSwapList(ctx context.Context, id string) (*SwapResp, error) {
	var ss []*Swap
	// CNB 特殊处理...
	if id == "965e5c6e-434c-3fa9-b780-c50f43cd955c" {
		rows, err := session.Database(ctx).Query(ctx, `
SELECT s.lp_asset,
s.asset0,s.asset0_price,s.asset0_amount,
s.asset1,s.asset1_price,s.asset1_amount,
s.type,s.pool,s.earn,s.amount,s.updated_at,
lp.icon_url,a0.symbol as asset0_symbol,a1.symbol as asset1_symbol
FROM swap as s
LEFT JOIN assets as lp ON lp.asset_id=s.lp_asset
LEFT JOIN assets as a0 ON a0.asset_id=s.asset0
LEFT JOIN assets as a1 ON a1.asset_id=s.asset1
WHERE (s.asset0=$1 OR s.asset1=$1)
AND lp.icon_url IS NOT null
ORDER BY s.pool::real DESC`, id)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var s Swap
			err := rows.Scan(&s.LpAsset,
				&s.Asset0, &s.Asset0Price, &s.Asset0Amount,
				&s.Asset1, &s.Asset1Price, &s.Asset1Amount,
				&s.Type, &s.Pool, &s.Earn, &s.Amount, &s.UpdatedAt, &s.IconURL, &s.Asset0Symbol, &s.Asset1Symbol)
			if err != nil {
				session.Logger(ctx).Println(err)
				return nil, err
			}
			ss = append(ss, &s)
		}
	} else {
		rows, err := session.Database(ctx).Query(ctx,
			`SELECT s.lp_asset,
s.asset0,s.asset0_price,s.asset0_amount,
s.asset1,s.asset1_price,s.asset1_amount,
s.type,s.pool,s.earn,s.amount,s.updated_at,
lp.icon_url,a0.symbol as asset0_symbol,a1.symbol as asset1_symbol
FROM swap as s
LEFT JOIN assets as lp ON lp.asset_id=s.lp_asset
LEFT JOIN assets as a0 ON a0.asset_id=s.asset0
LEFT JOIN assets as a1 ON a1.asset_id=s.asset1
WHERE s.pool::real > 10000 AND (s.asset0=$1 OR s.asset1=$1) AND lp.icon_url is not null
ORDER BY s.pool::real DESC`)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var s Swap
			err := rows.Scan(&s.LpAsset,
				&s.Asset0, &s.Asset0Price, &s.Asset0Amount,
				&s.Asset1, &s.Asset1Price, &s.Asset1Amount,
				&s.Type, &s.Pool, &s.Earn, &s.Amount, &s.UpdatedAt, &s.IconURL, &s.Asset0Symbol, &s.Asset1Symbol)
			if err != nil {
				return nil, err
			}
			ss = append(ss, &s)
		}
	}

	exin, err1 := GetExinOtcAssetByID(ctx, id)
	if err1 == nil {
		ss = append(ss, exin)
	}

	asset, _ := GetAssetByID(ctx, nil, id)
	return &SwapResp{
		List:  ss,
		Asset: &asset,
		Ad:    cacheExin,
	}, nil
}
