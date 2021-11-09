package models

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/shopspring/decimal"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
)

const claim_DDL = `
CREATE TABLE IF NOT EXISTS claim (
	user_id   VARCHAR(36) NOT NULL,
	date 		  DATE DEFAULT NOW(),
	ua 				VARCHAR DEFAULT '',
	addr			VARCHAR DEFAULT '',
	client_id VARCHAR DEFAULT '',
	PRIMARY KEY (user_id, date)
);
alter table claim add if not exists ua varchar DEFAULT '';
alter table claim add if not exists addr varchar DEFAULT '';
alter table claim add if not exists client_id varchar DEFAULT '';
`

const power_DDL = `
CREATE TABLE IF NOT EXISTS power (
	user_id   VARCHAR(36) NOT NULL PRIMARY KEY,
	balance   VARCHAR NOT NULL DEFAULT '0',
	lottery_times 	 INTEGER NOT NULL DEFAULT 0
);
`

const power_record_DDL = `
CREATE TABLE IF NOT EXISTS power_record (
	power_type VARCHAR(128) NOT NULL,
	user_id    VARCHAR(36) NOT NULL,
	amount 	   VARCHAR NOT NULL DEFAULT '0',
	created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
`

type Claim struct {
	UserID string    `json:"user_id"`
	Date   time.Time `json:"date"`
	UA     string    `json:"ua"`
}

type Power struct {
	UserID       string          `json:"user_id,omitempty"`
	Balance      decimal.Decimal `json:"balance"`
	LotteryTimes int             `json:"lottery_times"`
}

type PowerRecord struct {
	UserID    string    `json:"user_id"`
	PowerType string    `json:"power_type"`
	Amount    string    `json:"amount"`
	CreatedAt time.Time `json:"created_at"`

	Date string `json:"date"`
}

const (
	PowerTypeClaim      = "claim"
	PowerTypeClaimExtra = "claim_extra"
	PowerTypeLottery    = "lottery"
	PowerTypeInvitation = "invitation"
)

type LotteryList struct {
	config.Lottery
	Description string          `json:"description"`
	Symbol      string          `json:"symbol"`
	PriceUSD    decimal.Decimal `json:"price_usd"`
}

type CliamPageResp struct {
	LastLottery []LotteryRecord `json:"last_lottery"`
	LotteryList []LotteryList   `json:"lottery_list"`
	Power       Power           `json:"power"`               // 当前能量 times
	IsClaim     bool            `json:"is_claim"`            // 是否已经签到
	Count       int             `json:"count"`               // 本周签到天数
	InviteCount int64           `json:"invite_count"`        // 邀请人数
	Receiving   *LotteryRecord  `json:"receiving,omitempty"` // receviing 抽奖了没有领
}

func GetClaimAndLotteryInitData(ctx context.Context, u *ClientUser) (*CliamPageResp, error) {
	resp := &CliamPageResp{
		LastLottery: getLastLottery(ctx),
		LotteryList: getLotteryList(ctx),
		Power:       getPower(ctx, u.UserID),
		IsClaim:     checkIsClaim(ctx, u.UserID),
		Count:       getWeekClaimDay(ctx, u.UserID),
		Receiving:   getReceivingLottery(ctx, u.UserID),
		InviteCount: getInviteCountByUserID(ctx, u.UserID),
	}
	return resp, nil
}

func PostClaim(ctx context.Context, u *ClientUser) error {
	if checkIsBlockUser(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	if checkIsClaim(ctx, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	isVip := checkUserIsVIP(ctx, u.UserID)
	if err := session.Database(ctx).RunInTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// 1. 创建一个 claim
		if err := createClaim(ctx, tx, u); err != nil {
			return err
		}
		var addPower decimal.Decimal
		if isVip {
			addPower = decimal.NewFromInt(10)
		} else {
			addPower = decimal.NewFromInt(5)
		}

		// 2. 创建一条 power_record
		if err := createPowerRecord(ctx, tx, u.UserID, PowerTypeClaim, addPower); err != nil {
			return err
		}
		// 2.1 如果签到 4 天，则这一次加上额外的。
		if needAddExtraPower(ctx, u.UserID) {
			var extraPower decimal.Decimal
			if isVip {
				extraPower = decimal.NewFromInt(50)
			} else {
				extraPower = decimal.NewFromInt(25)
			}
			if err := createPowerRecord(ctx, tx, u.UserID, PowerTypeClaimExtra, extraPower); err != nil {
				return err
			}
			addPower = addPower.Add(extraPower)
		}
		// 3. 更新 power balance
		if err := updatePowerBalanceWithAmount(ctx, tx, u.UserID, addPower); err != nil {
			return err
		}
		// 5. 处理邀请奖励
		if err := handleInvitationClaim(ctx, tx, u.UserID, isVip); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func updatePowerBalanceWithAmount(ctx context.Context, tx pgx.Tx, userID string, amount decimal.Decimal) error {
	// 1. 拿到 power_balance
	pow := getPower(ctx, userID)
	balance := pow.Balance.Add(amount)
	// 2. 更新 power_balance
	return updatePower(ctx, tx, userID, balance.String(), pow.LotteryTimes)
}

func PostExchangeLottery(ctx context.Context, u *ClientUser) error {
	pow := getPower(ctx, u.UserID)
	if pow.Balance.LessThan(decimal.NewFromInt(100)) {
		return session.ForbiddenError(ctx)
	}
	b := pow.Balance.Sub(decimal.NewFromInt(100))
	return session.Database(ctx).RunInTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := createPowerRecord(ctx, tx, u.UserID, PowerTypeLottery, decimal.NewFromInt(-100)); err != nil {
			return err
		}
		if err := updatePower(ctx, tx, u.UserID, b.String(), pow.LotteryTimes+1); err != nil {
			return err
		}
		return nil
	})
}

func GetPowerRecordList(ctx context.Context, u *ClientUser, page int) ([]PowerRecord, error) {
	if page < 1 {
		page = 1
	}
	rows, err := session.Database(ctx).Query(ctx,
		"SELECT power_type, amount, to_char(created_at, 'YYYY-MM-DD') AS created_at FROM power_record WHERE user_id = $1 ORDER BY created_at DESC OFFSET $2 LIMIT 20",
		u.UserID, (page-1)*20)
	if err != nil {
		return nil, err
	}
	var list []PowerRecord
	for rows.Next() {
		var r PowerRecord
		if err := rows.Scan(&r.PowerType, &r.Amount, &r.Date); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func createClaim(ctx context.Context, tx pgx.Tx, u *ClientUser) error {
	query := durable.InsertQuery("claim", "client_id,user_id,ua,addr")
	req := session.Request(ctx)
	_, err := tx.Exec(ctx, query, u.ClientID, u.UserID, req.Header.Get("User-Agent"), req.RemoteAddr)
	return err
}

func checkIsClaim(ctx context.Context, userID string) bool {
	var count int
	if err := session.Database(ctx).QueryRow(ctx, "SELECT count(1) FROM claim WHERE user_id=$1 AND date=current_date", userID).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func createPowerRecord(ctx context.Context, tx pgx.Tx, userID, powerType string, amount decimal.Decimal) error {
	query := durable.InsertQuery("power_record", "user_id,power_type,amount")
	_, err := tx.Exec(ctx, query, userID, powerType, amount)
	return err
}

func getPower(ctx context.Context, userID string) Power {
	var p Power
	if err := session.Database(ctx).QueryRow(ctx, "SELECT balance, lottery_times FROM power WHERE user_id=$1", userID).Scan(&p.Balance, &p.LotteryTimes); err != nil {
		if err == pgx.ErrNoRows {
			if checkUserIsVIP(ctx, userID) {
				p.LotteryTimes = 1
			}
			_, err := session.Database(ctx).Exec(ctx, durable.InsertQuery("power", "user_id,balance,lottery_times"), userID, "0", p.LotteryTimes)
			if err != nil {
				session.Logger(ctx).Println(err)
			}
			return p
		}
	}
	return p
}

func updatePower(ctx context.Context, tx pgx.Tx, userID, balance string, times int) error {
	_, err := tx.Exec(ctx, "UPDATE power SET balance=$1, lottery_times=$2 WHERE user_id=$3", balance, times, userID)
	return err
}

func getWeekClaimDay(ctx context.Context, userID string) int {
	var count int
	if err := session.Database(ctx).QueryRow(
		ctx,
		fmt.Sprintf("SELECT count(1) FROM claim WHERE user_id=$1 AND date >= CURRENT_DATE-%d", getFirstDateOffsetOfWeek()),
		userID,
	).Scan(&count); err != nil {
		session.Logger(ctx).Println(err)
		return 0
	}
	return count
}

func getFirstDateOffsetOfWeek() int {
	todayWeekday := time.Now().Weekday()
	if config.Config.Lang == "zh" {
		if todayWeekday == time.Sunday {
			todayWeekday = 7
		}
		return int(todayWeekday) - 1
	} else {
		return int(todayWeekday)
	}
}

func needAddExtraPower(ctx context.Context, userID string) bool {
	passDays := int(time.Now().Weekday())
	if config.Config.Lang == "zh" {
		if passDays == 0 {
			passDays = 7
		}
		if passDays < 5 {
			return false
		}

		var count int
		if err := session.Database(ctx).QueryRow(ctx,
			fmt.Sprintf("SELECT count(1) FROM claim WHERE user_id=$1 AND date>CURRENT_DATE-%d", passDays),
			userID,
		).Scan(&count); err != nil {
			session.Logger(ctx).Println(err)
			return false
		}
		return count == 4
	} else {
		if passDays < 4 {
			return false
		}
		var count int
		if err := session.Database(ctx).QueryRow(ctx,
			fmt.Sprintf("SELECT count(1) FROM claim WHERE user_id=$1 AND date>CURRENT_DATE-%d", passDays+1),
			userID,
		).Scan(&count); err != nil {
			session.Logger(ctx).Println(err)
			return false
		}
		return count == 4
	}
}

func getYesterdayClaim(ctx context.Context) (int, int, error) {
	var vipAmount int
	var normalAmount int
	if err := session.Database(ctx).QueryRow(ctx, "SELECT count(1) FROM power_record WHERE to_char(created_at, 'YYYY-MM-DD')= to_char(current_date-1, 'YYYY-MM-DD')  AND amount='10'").Scan(&vipAmount); err != nil {
		return 0, 0, err
	}
	if err := session.Database(ctx).QueryRow(ctx, "SELECT count(1) FROM power_record WHERE to_char(created_at, 'YYYY-MM-DD')= to_char(current_date-1, 'YYYY-MM-DD')  AND amount='5'").Scan(&normalAmount); err != nil {
		return 0, 0, err
	}
	return normalAmount + vipAmount, vipAmount, nil
}

func getUserTotalPower(ctx context.Context, userID string) (int, error) {
	var amount int
	err := session.Database(ctx).QueryRow(ctx, `
SELECT coalesce(SUM(amount::integer),0) FROM power_record 
WHERE user_id=$1 AND power_type='invitation'
`, userID).Scan(&amount)
	return amount, err
}
