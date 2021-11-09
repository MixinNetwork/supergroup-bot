package models

import (
	"context"
	"fmt"
	"time"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/jackc/pgx/v4"
	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
)

func DailyStatisticMsg() {
	// 每天凌晨0点统计资产
	c := cron.New(cron.WithLocation(time.UTC))
	_, err := c.AddFunc("0 0 * * *", func() {
		LotteryStatistic(_ctx)
	})
	if err != nil {
		session.Logger(_ctx).Println(err)
	}
	c.Start()
}

func LotteryStatistic(ctx context.Context) {
	c := getLotteryClient()
	if c == nil {
		return
	}
	yesterdayFinishedAssetMap := getYesterdaySendReward(ctx)
	times := getYesterdayLotteryTimes(ctx)
	var sendStr string
	sendStr += fmt.Sprintf("昨日抽奖次数:%d\n", times)
	totalClaim, vipClaim, err := getYesterdayClaim(ctx)
	if err == nil {
		sendStr += fmt.Sprintf("昨日会员签到人数:%d\n", vipClaim)
		sendStr += fmt.Sprintf("昨日总签到人数:%d\n", totalClaim)
	}
	sendStr += "\n昨日发放奖励\n"
	for assetID, amount := range yesterdayFinishedAssetMap {
		a, _ := GetAssetByID(ctx, nil, assetID)
		sendStr += fmt.Sprintf("%s:%s\n", a.Symbol, amount.String())
	}
	as, err := getLotteryClient().ReadAssets(ctx)
	if err == nil {
		sendStr += "\n机器人余额\n"
		asMap := make(map[string]*mixin.Asset)
		for _, a := range as {
			asMap[a.AssetID] = a
		}
		lotteryList := getLotteryAssetMaxRewardMap()
		for assetID, maxReward := range lotteryList {
			a := asMap[assetID]
			if a != nil {
				sendStr += fmt.Sprintf("%s:%s\n", a.Symbol, a.Balance)
				if a.Balance.LessThan(maxReward.Mul(decimal.NewFromInt(config.NoticeLotteryTimes))) {
					sendStr += fmt.Sprintf("不足最大份数的%d倍，请及时充值...\n", config.NoticeLotteryTimes)
				}
			} else {
				a, _ := GetAssetByID(ctx, nil, assetID)
				sendStr += fmt.Sprintf("%s:0\n", a.Symbol)
				sendStr += fmt.Sprintf("不足最大份数的%d倍，请及时充值...\n", config.NoticeLotteryTimes)
			}
		}
	} else {
		session.Logger(ctx).Println(err)
	}
	SendMonitorGroupMsg(ctx, nil, sendStr)
}

func getYesterdayLotteryTimes(ctx context.Context) int {
	var times int
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT COUNT(1) 
FROM lottery_record 
WHERE created_at between CURRENT_DATE-1 and CURRENT_DATE`,
	).Scan(&times); err != nil {
		session.Logger(ctx).Println(err)
	}
	return times
}

func getYesterdaySendReward(ctx context.Context) map[string]decimal.Decimal {
	finishedAssetMap := make(map[string]decimal.Decimal)
	if err := session.Database(ctx).ConnQuery(ctx, `
SELECT asset_id, COALESCE(SUM(amount::decimal),0)
FROM lottery_record 
WHERE is_received = true AND
created_at between CURRENT_DATE-1 and CURRENT_DATE
GROUP BY asset_id`,
		func(rows pgx.Rows) error {
			for rows.Next() {
				var assetID string
				var amount decimal.Decimal
				if err := rows.Scan(&assetID, &amount); err != nil {
					return err
				}
				finishedAssetMap[assetID] = amount
			}
			return nil
		},
	); err != nil {
		session.Logger(ctx).Println(err)
	}
	return finishedAssetMap
}

// 获取奖品列表及最大的奖品数量
func getLotteryAssetMaxRewardMap() map[string]decimal.Decimal {
	res := make(map[string]decimal.Decimal)
	for _, l := range config.Config.Lottery.List {
		if res[l.AssetID] == decimal.Zero {
			res[l.AssetID] = l.Amount
		} else {
			if res[l.AssetID].LessThan(l.Amount) {
				res[l.AssetID] = l.Amount
			}
		}
	}
	return res
}
