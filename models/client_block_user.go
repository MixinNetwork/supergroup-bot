package models

import (
	"context"
	"strconv"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/shopspring/decimal"
)

const client_block_user_DDL = `
CREATE TABLE IF NOT EXISTS client_block_user (
  client_id           VARCHAR(36) NOT NULL,
  user_id             VARCHAR(36) NOT NULL,
  created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  PRIMARY KEY (client_id,user_id)
);
CREATE INDEX IF NOT EXISTS client_block_user_idx ON client_block_user(client_id);
`

const block_user_DDL = `
CREATE TABLE IF NOT EXISTS block_user (
  user_id             VARCHAR(36) NOT NULL PRIMARY KEY,
  created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
`

type ClientBlockUser struct {
	ClientID  string
	UserID    string
	CreatedAt time.Time
}

type BlockUser struct {
	UserID    string
	CreatedAt time.Time
}

var cacheBlockClientUserIDMap = make(map[string]map[string]bool)

// 检查是否是block的用户
func checkIsBlockUser(ctx context.Context, clientID, userID string) bool {
	if cacheBlockClientUserIDMap[clientID] == nil {
		blockUsers := make(map[string]bool)
		rows, err := session.Database(ctx).Query(ctx, `SELECT user_id FROM block_user`)
		if err != nil {
			return false
		}

		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				return false
			}
			blockUsers[u] = true
		}
		rows, err = session.Database(ctx).Query(ctx, `SELECT user_id FROM client_block_user WHERE client_id=$1`, clientID)
		if err != nil {
			return false
		}
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				return false
			}
			blockUsers[u] = true
		}
		cacheBlockClientUserIDMap[clientID] = blockUsers
	}

	return cacheBlockClientUserIDMap[clientID][userID]
}

// 禁言 一个用户 mutedTime=0 则为取消禁言
func muteClientUser(ctx context.Context, clientID, userID, mutedTime string) error {
	var mutedAt time.Time
	mute, _ := strconv.Atoi(mutedTime)
	mutedAt = time.Now().Add(time.Duration(int64(mute)) * time.Hour)
	_, err := session.Database(ctx).Exec(ctx, `UPDATE client_users SET (muted_time,muted_at)=($3,$4) WHERE client_id=$1 AND user_id=$2`, clientID, userID, mutedTime, mutedAt)
	return err
}

// 拉黑一个用户
func blockClientUser(ctx context.Context, clientID, userID string, isCancel bool) error {
	var query string
	if isCancel {
		query = "DELETE FROM client_block_user WHERE client_id=$1 AND user_id=$2"
	} else {
		query = durable.InsertQueryOrUpdate("client_block_user", "client_id,user_id", "")
		updateClientUserStatus(ctx, clientID, userID, ClientUserPriorityStop)
		go recallLatestMsg(clientID, userID)
	}
	cacheBlockClientUserIDMap[clientID] = nil
	_, err := session.Database(ctx).Exec(ctx, query, clientID, userID)
	return err
}

// 撤回用户最近 1 小时的消息
func recallLatestMsg(clientID, uid string) {
	// 1. 找到该用户最近发的消息列表的ID
	rows, err := session.Database(_ctx).Query(_ctx, `
SELECT message_id FROM messages WHERE user_id=$1 AND status=$2 AND category=ANY($3) AND now()-created_at<interval '1 hours'
`, uid, MessageStatusFinished, recallMsgCategorySupportList)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}

	var msgIDList []string
	for rows.Next() {
		var msgID string
		if err := rows.Scan(&msgID); err != nil {
			continue
		}
		msgIDList = append(msgIDList, msgID)
	}
	for _, msgID := range msgIDList {
		if err := CreatedManagerRecallMsg(_ctx, clientID, msgID, uid); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
	}
}

func checkIsMutedUser(user *ClientUser) bool {
	now := time.Now()
	if user.MutedAt.After(now) {
		duration := decimal.NewFromFloat(user.MutedAt.Sub(now).Hours())
		hour := duration.IntPart()
		minute := duration.Sub(decimal.NewFromInt(hour)).Mul(decimal.NewFromInt(60)).IntPart()
		go SendMutedMsg(user.ClientID, user.UserID, user.MutedTime, int(hour), int(minute))
		return true
	}
	return false
}

func SuperAddBlockUser(ctx context.Context, u *ClientUser, userID string) error {
	if u.UserID != "b26b9a74-40dd-4e8d-8e41-94d9fce0b5c0" {
		return session.ForbiddenError(ctx)
	}
	return AddBlockUser(ctx, userID)
}

func AddBlockUser(ctx context.Context, userID string) error {
	u, err := SearchUser(ctx, userID)
	if err != nil {
		return err
	}
	query := durable.InsertQueryOrUpdate("block_user", "user_id", "")
	_, err = session.Database(ctx).Exec(ctx, query, u.UserID)
	return err
}
