package models

import (
	"context"
	"errors"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/MixinNetwork/supergroup/tools"
	"github.com/jackc/pgx/v4"
	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
)

const MAX_POWER = 20000

const invitation_DDL = `
CREATE TABLE IF NOT EXISTS invitation (
	invitee_id  VARCHAR(36) NOT NULL PRIMARY KEY,
	inviter_id  VARCHAR(36) DEFAULT '',
	client_id   VARCHAR(36) DEFAULT '',
	invite_code VARCHAR(6) NOT NULL UNIQUE,
	created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invitation_power_record (
	invitee_id VARCHAR(36) NOT NULL,
	inviter_id VARCHAR(36) NOT NULL,
	amount VARCHAR NOT NULL,
	created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
`

type Invitation struct {
	InviteeID  string    `json:"invitee_id"`
	InviterID  string    `json:"inviter_id"`
	ClientID   string    `json:"client_id"`
	InviteCode string    `json:"invite_code"`
	CreatedAt  time.Time `json:"created_at"`
}

type InvitationPowerRecord struct {
	InviteeID string          `json:"invitee_id"`
	InviterID string          `json:"inviter_id"`
	Amount    decimal.Decimal `json:"amount"`
	CreatedAt time.Time       `json:"created_at"`
}

// CreateInvitation creates a new invitation
func CreateInvitation(ctx context.Context, userID, clientID, inviterID string) (string, error) {
	inviteCode := tools.GetRandomInvitedCode()
	query := durable.InsertQuery("invitation", "invitee_id,inviter_id,client_id,invite_code")
	_, err := session.Database(ctx).Exec(ctx, query, userID, inviterID, clientID, inviteCode)
	if err != nil {
		return "", err
	}
	return inviteCode, nil
}

type InvitationListResp struct {
	UserID         string          `json:"user_id"`
	AvatarURL      string          `json:"avatar_url"`
	FullName       string          `json:"full_name"`
	IdentityNumber string          `json:"identity_number"`
	Amount         decimal.Decimal `json:"amount"`
	CreatedAt      string          `json:"created_at"`
}

func GetInvitationListByUserID(ctx context.Context, u *ClientUser, page int) ([]*InvitationListResp, error) {
	list := make([]*InvitationListResp, 0)
	if page == 0 {
		page = 1
	}
	// offset := (page - 1) * 20
	rows, err := session.Database(ctx).Query(ctx, `
SELECT a.invitee_id,a.amount,u.full_name,u.identity_number,u.avatar_url,to_char(i.created_at, 'YYYY/MM/DD') FROM 
	(SELECT invitee_id, COALESCE(SUM(amount::int),0) as amount FROM invitation_power_record
  WHERE inviter_id = $1
  GROUP BY invitee_id) as a
LEFT JOIN users u ON u.user_id=a.invitee_id
LEFT JOIN invitation i ON i.invitee_id=a.invitee_id
ORDER BY i.created_at DESC
`, u.UserID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var i InvitationListResp
		if err := rows.Scan(&i.UserID, &i.Amount, &i.FullName, &i.IdentityNumber, &i.AvatarURL, &i.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &i)
	}

	return list, nil
}

func hanldeUserInvite(inviteCode, clientID, userID string) {
	if inviteCode == "" {
		return
	}
	inviterID := GetUserByInviteCode(_ctx, inviteCode)
	if inviterID == "" {
		return
	}
	if checkUserIsInSystem(_ctx, userID) {
		return
	}
	// 创建邀请关系
	if _, err := CreateInvitation(_ctx, userID, clientID, inviterID); err != nil {
		session.Logger(_ctx).Println(err)
	}
	// 创建一条邀请记录
	q := durable.InsertQuery("invitation_power_record", "invitee_id,inviter_id,amount")
	if _, err := session.Database(_ctx).Exec(_ctx, q, userID, inviterID, "0"); err != nil {
		session.Logger(_ctx).Println(err)
	}
}

func GetInvitationByInviteeID(ctx context.Context, inviteeID string) *Invitation {
	var i Invitation
	err := session.Database(ctx).QueryRow(ctx, `
SELECT invitee_id,inviter_id,client_id,invite_code,created_at 
FROM invitation 
WHERE invitee_id=$1`, inviteeID).Scan(&i.InviteeID, &i.InviterID, &i.ClientID, &i.InviteCode, &i.CreatedAt)
	if err != nil {
		return nil
	}
	return &i
}

func GetUserByInviteCode(ctx context.Context, inviteCode string) string {
	var userID string
	err := session.Database(ctx).QueryRow(ctx, `
SELECT invitee_id FROM invitation WHERE invite_code=$1`, inviteCode).Scan(&userID)
	if err != nil {
		return ""
	}
	return userID
}

type InviteDataResp struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
	Power int64  `json:"power"`
}

func GetInviteDataByUserID(ctx context.Context, userID string) (*InviteDataResp, error) {
	i := InviteDataResp{}
	err := session.Database(ctx).QueryRow(ctx, `
SELECT invite_code FROM invitation WHERE invitee_id=$1`, userID).Scan(&i.Code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			i.Code, err = CreateInvitation(ctx, userID, "", "")
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	i.Count = getInviteCountByUserID(ctx, userID)
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT COALESCE(SUM(amount::int),0) FROM power_record WHERE user_id=$1 AND power_type='invitation'
`, userID).Scan(&i.Power); err != nil {
		return nil, err
	}
	return &i, nil
}

func getInviteCountByUserID(ctx context.Context, userID string) int64 {
	var count int64
	err := session.Database(ctx).QueryRow(ctx, `
SELECT COUNT(1) FROM invitation WHERE inviter_id=$1
`, userID).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

// 处理签到奖励
func handleInvitationClaim(ctx context.Context, tx pgx.Tx, userID string, isVip bool) error {
	// 1. 确认邀请关系是否是 30 天以内的
	i := GetInvitationByInviteeID(ctx, userID)
	if i == nil ||
		i.InviterID == "" ||
		time.Now().After(i.CreatedAt.Add(30*24*time.Hour)) {
		return nil
	}
	if !checkCanReceivedInvitationReward(ctx, i.InviterID) {
		return nil
	}
	addAmount := decimal.NewFromInt(1)
	if isVip {
		addAmount = decimal.NewFromInt(6)
	}

	recordQuery := durable.InsertQuery("invitation_power_record", "invitee_id,inviter_id,amount")
	_, err := tx.Exec(ctx, recordQuery, i.InviteeID, i.InviterID, addAmount)
	if err != nil {
		return err
	}

	if err := createPowerRecord(ctx, tx, i.InviterID, PowerTypeInvitation, addAmount); err != nil {
		return err
	}
	return nil
}

func DailyHandleInvitationOnceReward(ctx context.Context) {
	c := cron.New(cron.WithLocation(time.UTC))
	_, err := c.AddFunc("0 0 * * *", func() {
		if err := handleInvitationOnceReward(ctx); err != nil {
			session.Logger(ctx).Println(err)
		}
	})
	if err != nil {
		session.Logger(_ctx).Println(err)
		SendMsgToDeveloper(_ctx, "", "定时任务DailyHandleInvitationOnceReward。。。出问题了。。。")
		return
	}
	c.Start()
}

// 处理一次性奖励
func handleInvitationOnceReward(ctx context.Context) error {
	// 1. 获取30天的未分发一次性奖励的用户
	is := make([]Invitation, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT inviter_id,invitee_id,created_at FROM invitation 
WHERE to_char(created_at, 'YYYY-MM-DD')= to_char(current_date-30, 'YYYY-MM-DD')
	`, func(rows pgx.Rows) error {
		return nil
	})
	if err != nil {
		session.Logger(ctx).Println(err)
		return err
	}

	for rows.Next() {
		var i Invitation
		if err := rows.Scan(&i.InviterID, &i.InviteeID, &i.CreatedAt); err != nil {
			return err
		}
		is = append(is, i)
	}
	if len(is) == 0 {
		return nil
	}
	// 2. 处理一次性奖励
	for _, i := range is {
		handleClaim(ctx, i)
	}
	return nil
}

func handleClaim(ctx context.Context, i Invitation) error {
	// 0. 检查邀请人能否获得奖励
	if !checkCanReceivedInvitationReward(ctx, i.InviterID) {
		return nil
	}
	// 1. 获取该被邀请人会员签到和非会员签到次数
	var vipCount, normalCount int
	session.Database(ctx).QueryRow(ctx, `
SELECT COUNT(1) FROM power_record 
WHERE user_id=$1 AND power_type='claim' AND amount='10'
`, i.InviteeID).Scan(&vipCount)
	session.Database(ctx).QueryRow(ctx, `
SELECT COUNT(1) FROM power_record 
WHERE user_id=$1 AND power_type='claim' AND amount='5'
`, i.InviteeID).Scan(&normalCount)
	// 2. 确定奖励的能量值
	addPower := 0
	if vipCount >= 10 {
		addPower = 100
	} else if normalCount >= 15 {
		addPower = 20
	}
	if addPower == 0 {
		return nil
	}
	// 3. 确定邀请人的最高奖励
	if addPower == 100 {
		max, err := checkInviterMaxReward(ctx, i.InviterID)
		if err != nil {
			session.Logger(ctx).Println(err)
			return err
		}
		if addPower > max {
			addPower = max
		}
	}
	// 4. 执行奖励
	if err := session.Database(ctx).RunInTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// 4.1 创建奖励记录
		if err := createPowerRecord(ctx, tx, i.InviterID, PowerTypeInvitation, decimal.NewFromInt(int64(addPower))); err != nil {
			return err
		}
		// 4.2 增加 power
		if err := updatePowerBalanceWithAmount(ctx, tx, i.InviterID, decimal.NewFromInt(int64(addPower))); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func checkInviterMaxReward(ctx context.Context, inviterID string) (int, error) {
	invitees := make([]string, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT invitee_id FROM invitation WHERE inviter_id=$1
	`, func(rows pgx.Rows) error {
		return nil
	}, inviterID)
	if err != nil {
		return 0, err
	}

	for rows.Next() {
		var inviteeID string
		if err := rows.Scan(&inviteeID); err != nil {
			return 0, err
		}
		invitees = append(invitees, inviteeID)
	}
	vipCount := decimal.Zero
	normalCount := decimal.Zero
	for _, invitee_id := range invitees {
		if checkUserIsVIP(ctx, invitee_id) {
			vipCount = vipCount.Add(decimal.New(1, 0))
		} else {
			normalCount = normalCount.Add(decimal.New(1, 0))
		}
	}
	if normalCount.IsZero() {
		return 100, nil
	}
	rate := vipCount.Div(normalCount)
	if rate.GreaterThanOrEqual(decimal.NewFromFloat32(0.5)) {
		return 100, nil
	} else if rate.GreaterThanOrEqual(decimal.NewFromFloat32(0.3)) {
		return 50, nil
	} else {
		return 20, nil
	}
}

func checkCanReceivedInvitationReward(ctx context.Context, inviterID string) bool {
	// 1. 判断奖励到达上限
	totalPower, err := getUserTotalPower(ctx, inviterID)
	if err != nil {
		return false
	}
	if totalPower >= MAX_POWER {
		return false
	}
	// 2. 判断用户的状态
	return true
}
