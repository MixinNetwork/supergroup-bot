package models

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/MixinNetwork/supergroup/tools"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/jackc/pgx/v4"
	"github.com/shopspring/decimal"
)

const bot_user_DDL = `
-- 机器人用户信息表
-- 机器人用户信息表
CREATE TABLE IF NOT EXISTS client_users (
  client_id          VARCHAR(36),
  user_id            VARCHAR(36),
  access_token       VARCHAR(512),
  priority           SMALLINT NOT NULL DEFAULT 2, -- 1 优先级高 2 优先级低 3 补发中 4 超时没接受消息 暂停发送
  status             SMALLINT NOT NULL DEFAULT 0, -- 0 未入群 1 观众 2 入门 3 资深 5 大户 8 嘉宾 9 管理
  muted_time         VARCHAR DEFAULT '',
  muted_at           TIMESTAMP WITH TIME ZONE default '1970-01-01 00:00:00+00',
	pay_status				 SMALLINT NOT NULL DEFAULT 1,
	pay_expired_at     TIMESTAMP WITH TIME ZONE default '1970-01-01 00:00:00+00',
  is_received        BOOLEAN NOT NULL DEFAULT true,
  is_notice_join     BOOLEAN NOT NULL DEFAULT true,
  deliver_at         TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  read_at            TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  created_at         TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (client_id, user_id)
);
CREATE INDEX IF NOT EXISTS client_user_idx ON client_users(client_id);
`

type ClientUser struct {
	ClientID     string    `json:"client_id,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	Priority     int       `json:"priority,omitempty"`
	Status       int       `json:"status,omitempty"`
	PayStatus    int       `json:"pay_status,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	IsReceived   bool      `json:"is_received,omitempty"`
	IsNoticeJoin bool      `json:"is_notice_join,omitempty"`
	MutedTime    string    `json:"muted_time,omitempty"`
	MutedAt      time.Time `json:"muted_at,omitempty"`
	PayExpiredAt time.Time `json:"pay_expired_at,omitempty"`
	ReadAt       time.Time `json:"read_at,omitempty"`
	DeliverAt    time.Time `json:"deliver_at,omitempty"`

	AssetID     string `json:"asset_id,omitempty"`
	SpeakStatus int    `json:"speak_status,omitempty"`
}

const (
	ClientUserPriorityHigh    = 1 // 高优先级
	ClientUserPriorityLow     = 2 // 低优先级
	ClientUserPriorityPending = 3 // 补发中
	ClientUserPriorityStop    = 4 // 暂停发送

	ClientUserStatusExit     = 0 // 退群
	ClientUserStatusAudience = 1 // 观众
	ClientUserStatusFresh    = 2 // 入门
	ClientUserStatusSenior   = 3 // 资深
	ClientUserStatusLarge    = 5 // 大户
	ClientUserStatusGuest    = 8 // 嘉宾
	ClientUserStatusAdmin    = 9 // 管理员
)

func UpdateClientUser(ctx context.Context, user *ClientUser, fullName string) (bool, error) {
	u, err := GetClientUserByClientIDAndUserID(ctx, user.ClientID, user.UserID)
	isNewUser := false
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 第一次入群
			isNewUser = true
		}
	}
	if user.AccessToken != "" {
		go SendAuthSuccessMsg(user.ClientID, user.UserID)
		var msg string
		if user.Status == ClientUserStatusLarge {
			if u.Status < ClientUserStatusLarge {
				msg = config.Text.AuthForLarge
			}
		} else if user.Status != ClientUserStatusAudience {
			if u.Status < user.Status {
				msg = config.Text.AuthForFresh
			}
		}
		go SendTextMsg(_ctx, user.ClientID, user.UserID, msg)
	}
	if u.Status == ClientUserStatusAdmin || u.Status == ClientUserStatusGuest {
		user.Status = u.Status
		user.Priority = ClientUserPriorityHigh
	} else if u.PayStatus > ClientUserStatusAudience {
		user.Status = u.PayStatus
		user.Priority = ClientUserPriorityHigh
	}
	if user.PayExpiredAt.IsZero() {
		query := durable.InsertQueryOrUpdate("client_users", "client_id,user_id", "access_token,priority,status")
		_, err = session.Database(ctx).Exec(ctx, query, user.ClientID, user.UserID, user.AccessToken, user.Priority, user.Status)
	} else {
		query := durable.InsertQueryOrUpdate("client_users", "client_id,user_id", "access_token,priority,status,pay_status,pay_expired_at")
		_, err = session.Database(ctx).Exec(ctx, query, user.ClientID, user.UserID, user.AccessToken, user.Priority, user.Status, ClientUserStatusLarge, user.PayExpiredAt)
	}
	if isNewUser {
		cs := getClientConversationStatus(ctx, user.ClientID)
		// conversation 状态为普通的时候入群通知是打开的，就通知用户入群。
		if cs == ClientConversationStatusNormal &&
			getClientNewMemberNotice(ctx, user.ClientID) == ClientNewMemberNoticeOn {
			go SendClientTextMsg(user.ClientID, strings.ReplaceAll(config.Text.JoinMsg, "{name}", tools.SplitString(fullName, 12)), user.UserID, true)
		}
		go SendWelcomeAndLatestMsg(user.ClientID, user.UserID)
	}
	return isNewUser, err
}

// 用户导入时
func CreateOrUpdateClientUser(ctx context.Context, u *ClientUser) error {
	query := durable.InsertQueryOrUpdate("client_users", "client_id,user_id", "access_token,priority,status,deliver_at,read_at")
	_, err := session.Database(ctx).Exec(ctx, query, u.ClientID, u.UserID, u.AccessToken, u.Priority, u.Status, u.DeliverAt, u.ReadAt)
	return err
}

func GetClientUserByClientIDAndUserID(ctx context.Context, clientID, userID string) (*ClientUser, error) {
	var b ClientUser
	err := session.Database(ctx).QueryRow(ctx, `
SELECT client_id,user_id,priority,access_token,status,muted_time,muted_at,is_received,is_notice_join,pay_status,pay_expired_at,created_at 
FROM client_users 
WHERE client_id=$1 AND user_id=$2
`, clientID, userID).Scan(&b.ClientID, &b.UserID, &b.Priority, &b.AccessToken, &b.Status, &b.MutedTime, &b.MutedAt, &b.IsReceived, &b.IsNoticeJoin, &b.PayStatus, &b.PayExpiredAt, &b.CreatedAt)
	return &b, err
}

func GetClientUserReceived(ctx context.Context, clientID string) ([]string, []string, error) {
	privilegeUserList, err := GetClientUserByPriority(ctx, clientID, []int{ClientUserPriorityHigh}, false, false)
	if err != nil {
		return nil, nil, err
	}
	normalList := []int{ClientUserPriorityLow}
	normalUserList, err := GetClientUserByPriority(ctx, clientID, normalList, false, false)
	if err != nil {
		return nil, nil, err
	}
	return privilegeUserList, normalUserList, nil
}

func GetClientUserByPriority(ctx context.Context, clientID string, priority []int, isJoinMsg, isBroadcast bool) ([]string, error) {
	userList := make([]string, 0)
	addQuery := ""
	if isJoinMsg {
		addQuery = "AND is_notice_join=true"
	}
	if !isBroadcast {
		addQuery = fmt.Sprintf("%s %s", addQuery, "AND is_received=true")
	}
	query := fmt.Sprintf(`
SELECT user_id FROM client_users 
WHERE client_id=$1 AND priority=ANY($2) %s AND status!=$3
ORDER BY created_at
`, addQuery)

	rows, err := session.Database(ctx).Query(ctx, query, clientID, priority, ClientUserStatusExit)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		userList = append(userList, b)
	}
	return userList, nil
}

func GetAllClientNeedAssetsCheckUser(ctx context.Context, hasPayedUser bool) ([]*ClientUser, error) {
	allUser := make([]*ClientUser, 0)
	query := `
SELECT cu.client_id, cu.user_id, cu.access_token, cu.priority, cu.status, coalesce(c.asset_id, '') as asset_id, c.speak_status, cu.deliver_at
FROM client_users AS cu
LEFT JOIN client AS c ON c.client_id=cu.client_id
WHERE cu.priority IN (1,2)
AND cu.status IN (1,2,3,4,5)
`
	if !hasPayedUser {
		query += `AND cu.pay_expired_at<NOW()`
	}
	rows, err := session.Database(ctx).Query(ctx, query)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cu ClientUser
		if err := rows.Scan(&cu.ClientID, &cu.UserID, &cu.AccessToken, &cu.Priority, &cu.Status, &cu.AssetID, &cu.SpeakStatus, &cu.DeliverAt); err != nil {
			return nil, err
		}
		allUser = append(allUser, &cu)
	}
	return allUser, err
}

func updateClientUserStatus(ctx context.Context, clientID, userID string, status int) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE client_users SET status=$3 WHERE client_id=$1 AND user_id=$2`, clientID, userID, status)
	return err
}

func UpdateClientUserPriority(ctx context.Context, clientID, userID string, priority int) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE client_users SET priority=$3 WHERE client_id=$1 AND user_id=$2`, clientID, userID, priority)
	return err
}

func UpdateClientUserPriorityAndStatus(ctx context.Context, clientID, userID string, priority, status int) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE client_users SET priority=$3,status=$4 WHERE client_id=$1 AND user_id=$2`, clientID, userID, priority, status)
	return err
}

func UpdateClientUserActive(ctx context.Context, clientID, userID string, priority, status int) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE client_users SET priority=$3,status=$4,deliver_at=$5,read_at=$5 WHERE client_id=$1 AND user_id=$2`, clientID, userID, priority, status, time.Now())
	return err
}

func UpdateClientUserPayStatus(ctx context.Context, clientID, userID string, status int, expiredAt time.Time) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE client_users SET status=$3,priority=1,pay_status=$3,pay_expired_at=$4 WHERE client_id=$1 AND user_id=$2`, clientID, userID, status, expiredAt)
	return err
}

func UpdateClientUserDeliverTime(ctx context.Context, clientID, msgID string, deliverTime time.Time, status string) error {
	if status != "DELIVERED" && status != "READ" {
		return nil
	}
	dm, err := getDistributeMessageByClientIDAndMessageID(ctx, clientID, msgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	user, err := GetClientUserByClientIDAndUserID(ctx, clientID, dm.UserID)
	if err != nil {
		return err
	}
	if user.Priority == ClientUserPriorityStop {
		activeUser(user)
	}
	if status == "READ" {
		_, _ = session.Database(ctx).Exec(ctx, `UPDATE client_users SET read_at=$3,deliver_at=$3 WHERE client_id=$1 AND user_id=$2`, clientID, dm.UserID, deliverTime)
	} else {
		_, _ = session.Database(ctx).Exec(ctx, `UPDATE client_users SET deliver_at=$3 WHERE client_id=$1 AND user_id=$2`, clientID, dm.UserID, deliverTime)
	}
	return nil
}

func LeaveGroup(ctx context.Context, u *ClientUser) error {
	//_, err := session.Database(ctx).Exec(ctx, `DELETE FROM client_users WHERE client_id=$1 AND user_id=$2`, u.ClientID, u.UserID)
	if err := updateClientUserStatus(ctx, u.ClientID, u.UserID, ClientUserStatusExit); err != nil {
		return err
	}
	go SendTextMsg(_ctx, u.ClientID, u.UserID, config.Text.LeaveGroup)
	return nil
}

func UpdateClientUserChatStatus(ctx context.Context, u *ClientUser, isReceived, isNoticeJoin bool) (*ClientUser, error) {
	msg := ""
	if isReceived {
		msg = config.Text.OpenChatStatus
	} else {
		msg = config.Text.CloseChatStatus
		isNoticeJoin = false
	}

	_, err := session.Database(ctx).Exec(ctx, `
UPDATE client_users 
SET is_received=$3,is_notice_join=$4 
WHERE client_id=$1 AND user_id=$2
`, u.ClientID, u.UserID, isReceived, isNoticeJoin)
	if err != nil {
		return nil, err
	}
	if u.IsReceived != isReceived {
		go SendTextMsg(_ctx, u.ClientID, u.UserID, msg)
	}
	return GetClientUserByClientIDAndUserID(ctx, u.ClientID, u.UserID)
}

func SendDistributeMsgAloneList(ctx context.Context, clientID, userID string, priority, curStatus int) {
	startAt, err := getLeftDistributeMsgAndDistribute(ctx, clientID, userID)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	for {
		if startAt.IsZero() {
			break
		}
		startAt, err = sendLeftMsg(ctx, clientID, userID, startAt)
		if err != nil {
			session.Logger(ctx).Println(err)
			return
		}
	}
	err = UpdateClientUserPriorityAndStatus(ctx, clientID, userID, priority, curStatus)
	if err != nil {
		session.Logger(ctx).Println(err)
	}
}

func CheckUserIsActive(ctx context.Context, user *ClientUser, lastMsgCreatedAt time.Time) error {
	if lastMsgCreatedAt.IsZero() {
		return nil
	}
	if lastMsgCreatedAt.Sub(user.DeliverAt).Hours() > config.NotActiveCheckTime {
		// 标记用户为不活跃，停止消息推送
		go SendStopMsg(user.ClientID, user.UserID)
		if err := UpdateClientUserPriority(ctx, user.ClientID, user.UserID, ClientUserPriorityStop); err != nil {
			session.Logger(ctx).Println(err)
			return err
		}
	} else if user.Priority == ClientUserPriorityStop {
		activeUser(user)
	}
	return nil
}

func getClientManager(ctx context.Context, clientID string) ([]string, error) {
	users, err := getClientUserByClientIDAndStatus(ctx, clientID, ClientUserStatusAdmin)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func getClientUserByClientIDAndStatus(ctx context.Context, clientID string, status int) ([]string, error) {
	users := make([]string, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT user_id FROM client_users WHERE client_id=$1 AND status=$2
`, clientID, status)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var user string
		if err := rows.Scan(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, err
}

func getClientPeopleCount(ctx context.Context, clientID string) (decimal.Decimal, decimal.Decimal, error) {
	queryAll := `SELECT COUNT(1) FROM client_users WHERE client_id=$1 AND status!=$2`
	queryWeek := queryAll + " AND NOW() - created_at < interval '7 days'"
	var all, week decimal.Decimal
	if err := session.Database(ctx).QueryRow(ctx, queryAll, clientID, ClientUserStatusExit).Scan(&all); err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	if err := session.Database(ctx).QueryRow(ctx, queryWeek, clientID, ClientUserStatusExit).Scan(&week); err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	return all, week, nil
}

func activeUser(u *ClientUser) {
	if u.Priority != ClientUserPriorityStop {
		return
	}
	var err error
	status := ClientUserStatusAudience
	priority := ClientUserPriorityLow
	if u.PayExpiredAt.After(time.Now()) {
		status = u.PayStatus
	} else {
		status, err = GetClientUserStatusByClientUser(_ctx, u)
		if err != nil {
			session.Logger(_ctx).Println(err)
		}
	}
	if status != ClientUserStatusAudience {
		priority = ClientUserPriorityHigh
	}
	if err := UpdateClientUserActive(_ctx, u.ClientID, u.UserID, priority, status); err != nil {
		session.Logger(_ctx).Println(err)
	}
}

func GetPendingClientUser(ctx context.Context) ([]*ClientUser, error) {
	cus := make([]*ClientUser, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT client_id,user_id,access_token
FROM client_users
WHERE priority=$1
`, ClientUserPriorityPending)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var cu ClientUser
		if err := rows.Scan(&cu.ClientID, &cu.UserID, &cu.AccessToken); err != nil {
			return nil, err
		}
		cus = append(cus, &cu)
	}
	return cus, err
}

type clientUserView struct {
	UserID         string    `json:"user_id,omitempty"`
	AvatarURL      string    `json:"avatar_url,omitempty"`
	FullName       string    `json:"full_name,omitempty"`
	IdentityNumber string    `json:"identity_number,omitempty"`
	Status         int       `json:"status,omitempty"`
	ActiveAt       time.Time `json:"active_at,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
}

var clientUserViewPrefix = `SELECT u.user_id,avatar_url,full_name,identity_number,status,deliver_at,cu.created_at
FROM client_users cu
LEFT JOIN users u ON cu.user_id=u.user_id 
WHERE client_id=$1 `

// {state}: all 全部 mute 禁言 block 拉黑 guest 嘉宾 admin 管理员
func GetClientUserList(ctx context.Context, u *ClientUser, page int, status string) ([]*clientUserView, error) {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return nil, session.ForbiddenError(ctx)
	}
	cs, err := getMuteOrBlockClientUserList(ctx, u, status)
	if err != nil {
		return nil, err
	}
	if cs == nil {
		cs, err = getAllOrGuestOrAdminClientUserList(ctx, u, page, status)
	}
	return cs, err
}

func getMuteOrBlockClientUserList(ctx context.Context, u *ClientUser, status string) ([]*clientUserView, error) {
	if status == "mute" {
		// 获取禁言用户列表
		return getClientUserView(ctx, clientUserViewPrefix+`
AND muted_at>NOW()
`, u.ClientID)

	}
	if status == "block" {
		// 获取拉黑用户列表
		return getClientBlockUserView(ctx, u.ClientID)
	}
	return nil, nil
}

func getClientBlockUserView(ctx context.Context, clientID string) ([]*clientUserView, error) {
	cus := make([]*clientUserView, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT cbu.user_id,avatar_url,full_name,identity_number,cu.status,deliver_at,cu.created_at
FROM client_block_user cbu
LEFT JOIN client_users cu ON cbu.user_id=cu.user_id AND cbu.client_id=cu.client_id
LEFT JOIN users u ON cu.user_id=u.user_id
WHERE cbu.client_id=$1
`, clientID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var u clientUserView
		if err := rows.Scan(&u.UserID, &u.AvatarURL, &u.FullName, &u.IdentityNumber, &u.Status, &u.ActiveAt, &u.CreatedAt); err != nil {
			return nil, err
		}
		cus = append(cus, &u)
	}
	return cus, err
}

var clientUserStatusMap = map[string][]int{
	"all": {
		ClientUserStatusAudience,
		ClientUserStatusFresh,
		ClientUserStatusSenior,
		ClientUserStatusLarge,
	},
	"guest": {ClientUserStatusGuest},
	"admin": {ClientUserStatusAdmin},
}

func getAllOrGuestOrAdminClientUserList(ctx context.Context, u *ClientUser, page int, status string) ([]*clientUserView, error) {
	if status == "mute" || status == "block" {
		session.Logger(ctx).Println("status::", status)
		return nil, nil
	}
	statusList := clientUserStatusMap[status]
	if statusList == nil {
		session.Logger(ctx).Println("status::", status)
		return nil, nil
	}
	cs, err := getClientUserView(ctx, clientUserViewPrefix+`
AND status=ANY($3)
ORDER BY created_at ASC OFFSET $2 LIMIT 20`, u.ClientID, (page-1)*20, statusList)
	if err != nil {
		return nil, err
	}
	if page == 1 && status == "all" {
		if users, err := getClientUserView(ctx, clientUserViewPrefix+`
AND status IN (8,9)
ORDER BY status DESC 
`, u.ClientID); err != nil {
			return nil, err
		} else {
			c, _ := GetClientByID(ctx, u.ClientID)
			for i, v := range users {
				if v.UserID == c.OwnerID {
					users[0], users[i] = users[i], users[0]
					break
				}
			}
			cs = append(users, cs...)
		}
	}
	return cs, nil
}

func GetAdminAndGuestUserList(ctx context.Context, u *ClientUser) ([]*clientUserView, error) {
	return getClientUserView(ctx, clientUserViewPrefix+`
AND status IN (8,9)
ORDER BY status DESC 
`, u.ClientID)
}

// 获取 全部用户数量/禁言用户数量/拉黑用户数量/嘉宾数量/管理员数量
func GetClientUserStat(ctx context.Context, u *ClientUser) (map[string]int, error) {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return nil, session.ForbiddenError(ctx)
	}
	var allUserCount, muteUserCount, blockUserCount, guestUserCount, adminUserCount int
	// res := make(map[string]int)
	if err := session.Database(ctx).QueryRow(ctx, `SELECT count(1) FROM client_users WHERE client_id=$1`, u.ClientID).Scan(&allUserCount); err != nil {
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `SELECT count(1) FROM client_block_user WHERE client_id=$1`, u.ClientID).Scan(&blockUserCount); err != nil {
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `SELECT count(1) FROM client_users WHERE client_id=$1 AND muted_at>NOW()`, u.ClientID).Scan(&muteUserCount); err != nil {
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `SELECT count(1) FROM client_users WHERE client_id=$1 AND status=8`, u.ClientID).Scan(&guestUserCount); err != nil {
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `SELECT count(1) FROM client_users WHERE client_id=$1 AND status=9`, u.ClientID).Scan(&adminUserCount); err != nil {
		return nil, err
	}
	return map[string]int{
		"all":   allUserCount,
		"mute":  muteUserCount,
		"block": blockUserCount,
		"guest": guestUserCount,
		"admin": adminUserCount,
	}, nil
}

func GetClientUserByIDOrName(ctx context.Context, u *ClientUser, key string) ([]*clientUserView, error) {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return nil, session.ForbiddenError(ctx)
	}
	return getClientUserView(ctx, clientUserViewPrefix+`
AND (
	(u.identity_number ILIKE '%' || $2 || '%') OR 
	(u.full_name ILIKE '%' || $2 || '%')
)
LIMIT 20
`, u.ClientID, key)
}

func getClientUserView(ctx context.Context, query string, p ...interface{}) ([]*clientUserView, error) {
	cs := make([]*clientUserView, 0)
	rows, err := session.Database(ctx).Query(ctx, query, p...)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var u clientUserView
		if err := rows.Scan(&u.UserID, &u.AvatarURL, &u.FullName, &u.IdentityNumber, &u.Status, &u.ActiveAt, &u.CreatedAt); err != nil {
			return nil, err
		}
		cs = append(cs, &u)
	}
	return cs, err
}

func UpdateClientUserStatus(ctx context.Context, u *ClientUser, userID string, status int, isCancel bool) error {
	if status == ClientUserStatusAdmin {
		if !checkIsOwner(ctx, u.ClientID, u.UserID) {
			return session.ForbiddenError(ctx)
		}
	} else {
		if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
			return session.ForbiddenError(ctx)
		}
	}
	var s string
	var msg string
	if status == ClientUserStatusAdmin {
		s = config.Text.StatusAdmin
	} else if status == ClientUserStatusGuest {
		s = config.Text.StatusGuest
	}
	if isCancel {
		msg = config.Text.StatusCancel
		status = ClientUserStatusLarge
	} else {
		msg = config.Text.StatusSet
	}
	if _, err := session.Database(ctx).Exec(ctx, `
UPDATE client_users SET status=$3 WHERE client_id=$1 AND user_id=$2
`, u.ClientID, userID, status); err != nil {
		return err
	}
	user, err := getUserByID(ctx, userID)
	if err != nil {
		session.Logger(ctx).Println("设置用户状态的时候没找到用户...", err)
		return err
	}
	msg = strings.ReplaceAll(msg, "{full_name}", user.FullName)
	msg = strings.ReplaceAll(msg, "{identity_number}", user.IdentityNumber)
	msg = strings.ReplaceAll(msg, "{status}", s)
	if !isCancel && status == ClientUserStatusGuest {
		go SendTextMsg(_ctx, u.ClientID, userID, msg)
	}
	go SendToClientManager(u.ClientID, &mixin.MessageView{
		ConversationID: mixin.UniqueConversationID(u.ClientID, userID),
		UserID:         userID,
		MessageID:      tools.GetUUID(),
		Category:       mixin.MessageCategoryPlainText,
		Data:           tools.Base64Encode([]byte(msg)),
		CreatedAt:      time.Now(),
	}, false, false)
	return nil
}

func MuteUserByID(ctx context.Context, u *ClientUser, userID, muteTime string) error {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	return muteClientUser(ctx, u.ClientID, userID, muteTime)
}

func BlockUserByID(ctx context.Context, u *ClientUser, userID string, isCancel bool) error {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	return blockClientUser(ctx, u.ClientID, userID, isCancel)
}

func checkUserIsVIP(ctx context.Context, userID string) bool {
	var count int
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM client_users WHERE user_id=$1 AND status>1
`, userID).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func checkUserIsInSystem(ctx context.Context, userID string) bool {
	var count int
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM client_users WHERE user_id=$1
`, userID).Scan(&count); err != nil {
		return false
	}
	return count > 0
}
