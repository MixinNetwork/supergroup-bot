package models

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/MixinNetwork/supergroup/tools"
	"github.com/jackc/pgx/v4"
	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
)

const liquidity_mining_DDL = `
CREATE TABLE IF NOT EXISTS liquidity_mining (
	mining_id VARCHAR(36) NOT NULL PRIMARY KEY,

	title VARCHAR NOT NULL,
	description VARCHAR NOT NULL,
	faq VARCHAR NOT NULL,
	join_tips VARCHAR NOT NULL DEFAULT '',
	join_url VARCHAR NOT NULL DEFAULT '',
	
	asset_id VARCHAR(36) NOT NULL,
	client_id VARCHAR(36) NOT NULL,
	first_time timestamp NOT NULL DEFAULT NOW(),
	first_end  timestamp NOT NULL DEFAULT NOW(),
	daily_time timestamp NOT NULL DEFAULT NOW(),
	daily_end  timestamp NOT NULL DEFAULT NOW(),
	
	reward_asset_id VARCHAR(36) NOT NULL,
	first_amount varchar NOT NULL DEFAULT '0',
	daily_amount varchar NOT NULL DEFAULT '0',

	extra_asset_id varchar NOT NULL DEFAULT '',
	extra_first_amount varchar NOT NULL DEFAULT '0',
	extra_daily_amount varchar NOT NULL DEFAULT '0',
	created_at timestamp NOT NULL DEFAULT NOW()
);
`

type LiquidityMining struct {
	MiningID         string          `json:"mining_id,omitempty"`
	Title            string          `json:"title,omitempty"`
	Bg               string          `json:"bg,omitempty"`
	Description      string          `json:"description,omitempty"`
	Faq              string          `json:"faq,omitempty"`
	JoinTips         string          `json:"join_tips,omitempty"`
	JoinURL          string          `json:"join_url,omitempty"`
	AssetID          string          `json:"asset_id,omitempty"`
	ClientID         string          `json:"client_id,omitempty"`
	FirstTime        time.Time       `json:"first_time,omitempty"`
	FirstEnd         time.Time       `json:"first_end,omitempty"`
	FirstDesc        string          `json:"first_desc,omitempty"`
	DailyTime        time.Time       `json:"daily_time,omitempty"`
	DailyEnd         time.Time       `json:"daily_end,omitempty"`
	DailyDesc        string          `json:"daily_desc,omitempty"`
	RewardAssetID    string          `json:"reward_asset_id,omitempty"`
	FirstAmount      decimal.Decimal `json:"first_amount,omitempty"`
	DailyAmount      decimal.Decimal `json:"daily_amount,omitempty"`
	ExtraAssetID     string          `json:"extra_asset_id,omitempty"`
	ExtraFirstAmount decimal.Decimal `json:"extra_first_amount,omitempty"`
	ExtraDailyAmount decimal.Decimal `json:"extra_daily_amount,omitempty"`
	CreatedAt        time.Time       `json:"created_at,omitempty"`

	Symbol string `json:"symbol,omitempty"`
	Status string `json:"status,omitempty"`

	RewardSymbol string `json:"reward_symbol,omitempty"`
	ExtraSymbol  string `json:"extra_symbol,omitempty"`
}

const (
	LiquidityMiningFirst = 1 // ????????????
	LiquidityMiningDaily = 2 // ????????????

	LiquidityMiningStatusAuth    = "auth"    // ???????????????
	LiquidityMiningStatusPending = "pending" // ????????????????????????
	LiquidityMiningStatusDone    = "done"    // ????????????????????????
)

func GetLiquidityMiningRespByID(ctx context.Context, u *ClientUser, id string) (*LiquidityMining, error) {
	m, err := GetLiquidityMiningByID(ctx, id)
	if err != nil {
		return nil, err
	}
	a, err := GetAssetByID(ctx, nil, m.AssetID)
	if err != nil {
		return nil, err
	}
	m.Symbol = a.Symbol
	// ????????????token???????????????
	m.Status = LiquidityMiningStatusAuth

	rewardAsset, err := GetAssetByID(ctx, nil, m.RewardAssetID)
	if err != nil {
		return nil, err
	}
	m.RewardSymbol = rewardAsset.Symbol
	extraAsset, err := GetAssetByID(ctx, nil, m.ExtraAssetID)
	if err != nil {
		return nil, err
	}
	m.ExtraSymbol = extraAsset.Symbol
	// ??????token?????????????????????
	assets, err := GetUserAssets(ctx, u)
	if err == nil && len(assets) > 0 {
		// ??????????????????????????????????????????
		m.Status = LiquidityMiningStatusPending
		lpAssets, err := GetClientAssetLPCheckMapByID(ctx, u.ClientID)
		if err != nil {
			return nil, err
		}
		for _, a := range assets {
			if _, ok := lpAssets[a.AssetID]; ok {
				if a.Balance.GreaterThan(decimal.Zero) {
					m.Status = LiquidityMiningStatusDone
					break
				}
			}
		}
		if m.Status == LiquidityMiningStatusDone {
			// ??????????????????????????????
			if err := CreateLiquidityMiningUser(ctx, &LiquidityMiningUser{
				MiningID: m.MiningID,
				UserID:   u.UserID,
			}); err != nil {
				log.Println(err)
				return nil, err
			}
		}
	}
	return m, nil
}

func CreateLiquidityMining(ctx context.Context, m *LiquidityMining) error {
	query := durable.InsertQuery("liquidity_mining", "mining_id, title, description, asset_id, first_time, first_end, daily_time, daily_end, reward_asset_id, first_amount, daily_amount, extra_asset_id, extra_first_amount, extra_daily_amount")
	_, err := session.Database(ctx).Exec(ctx, query, m.MiningID, m.Title, m.Description, m.AssetID, m.FirstTime, m.FirstEnd, m.DailyTime, m.DailyEnd, m.RewardAssetID, m.FirstAmount, m.DailyAmount, m.ExtraAssetID, m.ExtraFirstAmount, m.ExtraDailyAmount)
	return err
}

func GetLiquidityMiningByID(ctx context.Context, id string) (*LiquidityMining, error) {
	var m LiquidityMining
	err := session.Database(ctx).QueryRow(ctx, `
SELECT mining_id, client_id, title, description, faq, join_tips, join_url, asset_id, first_time, first_end, daily_time, daily_end, reward_asset_id, first_amount, daily_amount, extra_asset_id, extra_first_amount, extra_daily_amount, first_desc, daily_desc,bg
FROM liquidity_mining WHERE mining_id=$1`, id).
		Scan(&m.MiningID, &m.ClientID, &m.Title, &m.Description, &m.Faq, &m.JoinTips, &m.JoinURL, &m.AssetID, &m.FirstTime, &m.FirstEnd, &m.DailyTime, &m.DailyEnd, &m.RewardAssetID, &m.FirstAmount, &m.DailyAmount, &m.ExtraAssetID, &m.ExtraFirstAmount, &m.ExtraDailyAmount, &m.FirstDesc, &m.DailyDesc, &m.Bg)
	return &m, err
}

func GetLiquidityMiningList(ctx context.Context) ([]*LiquidityMining, error) {
	ms := make([]*LiquidityMining, 0)
	err := session.Database(ctx).ConnQuery(ctx, `
SELECT mining_id, client_id, asset_id, first_time, first_end, daily_time, daily_end, reward_asset_id, first_amount, daily_amount, extra_asset_id, extra_first_amount, extra_daily_amount
FROM liquidity_mining`, func(rows pgx.Rows) error {
		for rows.Next() {
			var m LiquidityMining
			if err := rows.Scan(&m.MiningID, &m.ClientID, &m.AssetID, &m.FirstTime, &m.FirstEnd, &m.DailyTime, &m.DailyEnd, &m.RewardAssetID, &m.FirstAmount, &m.DailyAmount, &m.ExtraAssetID, &m.ExtraFirstAmount, &m.ExtraDailyAmount); err != nil {
				return err
			}
			ms = append(ms, &m)
		}
		return nil
	})
	return ms, err
}

func StartMintJob() {
	c := cron.New(cron.WithLocation(time.UTC))
	_, err := c.AddFunc("55 1 * * *", func() {
		log.Println("start mint job")
		HandleMintStatictis(_ctx)
	})
	if err != nil {
		session.Logger(_ctx).Println(err)
		SendMsgToDeveloper(_ctx, "", "????????????StartMintJob??????????????????????????????")
		return
	}
	c.Start()
}

func HandleMintStatictis(ctx context.Context) {
	ms, err := GetLiquidityMiningList(ctx)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	for _, m := range ms {
		// 1. ????????? first_time ??????
		if m.FirstTime.After(time.Now()) {
			continue
		}
		// 2. ??????first_time????????? first_end??????????????????????????????
		if m.FirstEnd.After(time.Now()) {
			// ??????????????????
			if err := handleStatisticsAssets(ctx, m, LiquidityMiningFirst); err != nil {
				session.Logger(ctx).Println(err)
			}
			continue
		}
		// 3. ??????first_end????????? daily_time ??????
		if m.DailyTime.After(time.Now()) {
			continue
		}
		// 4. ??????daily_time????????? daily_end??????????????????????????????
		if m.DailyEnd.After(time.Now()) {
			// ??????????????????
			if err := handleStatisticsAssets(ctx, m, LiquidityMiningDaily); err != nil {
				session.Logger(ctx).Println(err)
			}
			continue
		}
		// 5. ??????daily_end?????????
	}
}

func handleStatisticsAssets(ctx context.Context, m *LiquidityMining, mintStatus int) error {
	// ???????????????????????????
	users, err := GetLiquidityMiningUsersByID(ctx, m.ClientID, m.MiningID)
	if err != nil {
		return err
	}
	// ?????????????????????
	lpAssets, err := GetClientAssetLPCheckMapByID(ctx, m.ClientID)
	if err != nil {
		return err
	}
	assetReward := m.FirstAmount
	extraReward := m.ExtraFirstAmount

	if mintStatus == LiquidityMiningDaily {
		assetReward = m.DailyAmount
		extraReward = m.ExtraDailyAmount
	}
	totalAmount, usersAmount, tmpData := statisticsUsersPartAndTotalAmount(ctx, m.MiningID, users, lpAssets)
	if totalAmount.IsZero() {
		session.Logger(ctx).Println(m.MiningID + "... totalAmount is zero...")
		return nil
	}
	for userID, v := range usersAmount {
		if v.IsZero() {
			continue
		}
		// ??????
		part := v.Div(totalAmount)
		assetRewardAmount := assetReward.Mul(part).Truncate(8)
		extraAssetRewardAmount := extraReward.Mul(part).Truncate(8)
		if assetRewardAmount.IsZero() && extraAssetRewardAmount.IsZero() {
			continue
		}
		if err := session.Database(ctx).RunInTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
			recordID := tools.GetUUID()
			if !assetRewardAmount.IsZero() {
				if err := CreateLiquidityMiningTxWithTx(ctx, tx, &LiquidityMiningTx{
					RecordID: recordID,
					MiningID: m.MiningID,
					AssetID:  m.RewardAssetID,
					UserID:   userID,
					Amount:   assetRewardAmount,
					TraceID:  tools.GetUUID(),
					Status:   LiquidityMiningRecordStatusPending,
				}); err != nil {
					return err
				}
			}
			if m.ExtraAssetID != "" && !extraAssetRewardAmount.IsZero() {
				if err := CreateLiquidityMiningTxWithTx(ctx, tx, &LiquidityMiningTx{
					RecordID: recordID,
					MiningID: m.MiningID,
					AssetID:  m.ExtraAssetID,
					UserID:   userID,
					Amount:   extraAssetRewardAmount,
					TraceID:  tools.GetUUID(),
					Status:   LiquidityMiningRecordStatusPending,
				}); err != nil {
					return err
				}
			}

			for _, v := range tmpData[userID] {
				if err := CreateLiquidityMiningRecord(ctx, tx, &LiquidityMiningRecord{
					RecordID: recordID,
					MiningID: m.MiningID,
					UserID:   userID,
					AssetID:  v.AssetID,
					Amount:   v.Amount,
					Profit:   v.Profit,
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			session.Logger(ctx).Println(err)
			continue
		}
	}
	return nil
}

type tmpRecordData struct {
	AssetID string
	Amount  decimal.Decimal
	Profit  decimal.Decimal
	AddPart decimal.Decimal
}

func statisticsUsersPartAndTotalAmount(ctx context.Context, mintID string, users []*ClientUser, lpAssets map[string]decimal.Decimal) (decimal.Decimal, map[string]decimal.Decimal, map[string][]*tmpRecordData) {
	// ????????????????????????????????????
	totalAmount := decimal.Zero
	usersAmount := make(map[string]decimal.Decimal)
	records := make(map[string][]*tmpRecordData)
	for _, u := range users {
		userAssets, err := GetUserAssets(ctx, u)
		if err != nil {
			if strings.Contains(err.Error(), "Forbidden") {
				// ??????????????????????????????????????????????????????
				if err := CreateLiquidityMiningTx(ctx, &LiquidityMiningTx{
					RecordID: tools.GetUUID(),
					MiningID: mintID,
					UserID:   u.UserID,
					AssetID:  "",
					Amount:   decimal.Zero,
					Status:   LiquidityMiningRecordStatusFailed,
					TraceID:  tools.GetUUID(),
				}); err != nil {
					session.Logger(ctx).Println(err)
				}
				continue
			}
			session.Logger(ctx).Println(err)
			continue
		}
		// ?????????????????????
		for _, a := range userAssets {
			if a.Balance.IsZero() {
				continue
			}
			if price, ok := lpAssets[a.AssetID]; ok {
				if price.IsZero() {
					price, err = getLiquidityAssetPrice(ctx, a.AssetID)
					if err != nil {
						session.Logger(ctx).Println(err)
						continue
					}
				}
				addPart := a.Balance.Mul(price)
				// ??????????????? ??? ????????????
				if _, ok := usersAmount[u.UserID]; !ok {
					usersAmount[u.UserID] = decimal.Zero
					records[u.UserID] = make([]*tmpRecordData, 0)
				}
				usersAmount[u.UserID] = usersAmount[u.UserID].Add(addPart)
				totalAmount = totalAmount.Add(addPart)
				records[u.UserID] = append(records[u.UserID], &tmpRecordData{
					AssetID: a.AssetID,
					Amount:  a.Balance,
					AddPart: addPart,
				})
			}
		}
		if records[u.UserID] != nil {
			for _, v := range records[u.UserID] {
				v.Profit = v.AddPart.Div(usersAmount[u.UserID])
			}
		}
	}
	// 1. ?????? ?????????
	// 2. ?????? ?????????
	return totalAmount, usersAmount, records
}
