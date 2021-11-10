package models

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/MixinNetwork/supergroup/tools"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/jackc/pgx/v4"
)

const distribute_messages_DDL = `
-- 分发的消息
CREATE TABLE IF NOT EXISTS distribute_messages (
	client_id           VARCHAR(36) NOT NULL,
	user_id             VARCHAR(36) NOT NULL,
  conversation_id     VARCHAR(36) NOT NULL,
  shard_id            VARCHAR(36) NOT NULL,
	origin_message_id   VARCHAR(36) NOT NULL,
	message_id          VARCHAR(36) NOT NULL,
	quote_message_id    VARCHAR(36) NOT NULL DEFAULT '',
  data                TEXT NOT NULL DEFAULT '',
  category            VARCHAR NOT NULL DEFAULT '',
  representative_id   VARCHAR(36) NOT NULL DEFAULT '',
	level               SMALLINT NOT NULL, -- 1 高优先级 2 低优先级 3 单独队列
	status              SMALLINT NOT NULL DEFAULT 1, -- 1 待分发 2 已分发
	created_at          TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY(client_id, user_id, origin_message_id)
);
CREATE INDEX IF NOT EXISTS distribute_messages_list_idx ON distribute_messages (client_id, origin_message_id, level);
CREATE INDEX IF NOT EXISTS distribute_messages_all_list_idx ON distribute_messages (client_id, shard_id, status, level, created_at);
CREATE INDEX IF NOT EXISTS distribute_messages_id_idx ON distribute_messages (message_id);
`

type DistributeMessage struct {
	ClientID         string    `json:"client_id,omitempty"`
	UserID           string    `json:"user_id,omitempty"`
	ConversationID   string    `json:"conversation_id,omitempty"`
	ShardID          string    `json:"shard_id,omitempty"`
	OriginMessageID  string    `json:"origin_message_id,omitempty"`
	MessageID        string    `json:"message_id,omitempty"`
	QuoteMessageID   string    `json:"quote_message_id,omitempty"`
	Data             string    `json:"data,omitempty"`
	Category         string    `json:"category,omitempty"`
	RepresentativeID string    `json:"representative_id,omitempty"`
	Level            int       `json:"level,omitempty"`
	Status           int       `json:"status,omitempty"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
}

const (
	DistributeMessageLevelHigher = 1
	DistributeMessageLevelLower  = 2
	DistributeMessageLevelAlone  = 3

	DistributeMessageStatusPending      = 1 // 要发送的消息
	DistributeMessageStatusFinished     = 2 // 成功发送的消息
	DistributeMessageStatusLeaveMessage = 3 // 留言消息
	DistributeMessageStatusBroadcast    = 6 // 公告消息
	DistributeMessageStatusAloneList    = 9 // 单独处理的队列
)

//func SendClientUserPendingMessages(ctx context.Context, clientID, userID string) {
//	// 1. 将剩余的消息发送完
//	if err := sendPendingDistributeMessage(ctx, clientID, userID); err != nil {
//		session.Logger(ctx).Println(err)
//	}
//	if err := checkIsAsyncAndSendPendingMessage(ctx, clientID, userID); err != nil {
//		session.Logger(ctx).Println(err)
//	}
//}

// 删除超时的消息
func RemoveOvertimeDistributeMessages(ctx context.Context) error {
	_, err := session.Database(ctx).Exec(ctx, `DELETE FROM distribute_messages WHERE now()-created_at>interval '3 days' AND status=$1`, DistributeMessageStatusFinished)
	return err
}

// 获取指定的消息
func PendingActiveDistributedMessages(ctx context.Context, clientID, shardID string) ([]*mixin.MessageRequest, error) {
	dms := make([]*mixin.MessageRequest, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT representative_id, user_id, conversation_id, message_id, category, data, quote_message_id FROM distribute_messages
WHERE client_id=$1 AND shard_id=$2 AND status=$3
ORDER BY level, created_at
LIMIT 100
`, clientID, shardID, DistributeMessageStatusPending)
	if err != nil {
		return nil, err
	}

	repeatUser := make(map[string]bool)
	for rows.Next() {
		var dm mixin.MessageRequest
		if err := rows.Scan(&dm.RepresentativeID, &dm.RecipientID, &dm.ConversationID, &dm.MessageID, &dm.Category, &dm.Data, &dm.QuoteMessageID); err != nil {
			return nil, err
		}
		if repeatUser[dm.RecipientID] {
			continue
		}
		repeatUser[dm.RecipientID] = true
		dms = append(dms, &dm)
	}
	return dms, err
}

func SendBatchMessages(ctx context.Context, client *mixin.Client, msgList []*mixin.MessageRequest) error {
	sendTimes := len(msgList)/80 + 1
	var waitSync sync.WaitGroup
	for i := 0; i < sendTimes; i++ {
		start := i * 80
		var end int
		if i == sendTimes-1 {
			end = len(msgList)
		} else {
			end = (i + 1) * 80
		}
		waitSync.Add(1)
		go sendMessages(ctx, client, msgList[start:end], &waitSync, end)
	}
	waitSync.Wait()
	return nil
}

func sendMessages(ctx context.Context, client *mixin.Client, msgList []*mixin.MessageRequest, waitSync *sync.WaitGroup, end int) {
	if len(msgList) == 0 {
		waitSync.Done()
		return
	}
	err := client.SendMessages(ctx, msgList)
	if err != nil {
		time.Sleep(time.Millisecond)
		log.Println("msg Error...", err)
		tools.WriteDataToFile("msg.json", msgList)
		sendMessages(ctx, client, msgList, waitSync, end)
	} else {
		// 发送成功了
		if err := UpdateDistributeMessagesStatusToFinished(ctx, msgList); err != nil {
			session.Logger(ctx).Println(err)
			return
		}
		waitSync.Done()
	}
}

func SendMessage(ctx context.Context, client *mixin.Client, msg *mixin.MessageRequest, withCreate bool) error {
	err := client.SendMessage(ctx, msg)
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			if withCreate {
				d, _ := json.Marshal(msg)
				session.Logger(ctx).Println(err, string(d), client.ClientID)
				return nil
			}
			if _, err := client.CreateConversation(ctx, &mixin.CreateConversationInput{
				Category:       mixin.ConversationCategoryContact,
				ConversationID: mixin.UniqueConversationID(client.ClientID, msg.RecipientID),
				Participants:   []*mixin.Participant{{UserID: msg.RecipientID}},
			}); err != nil {
				return err
			}
			return SendMessage(ctx, client, msg, true)
		}
		time.Sleep(time.Millisecond)
		return SendMessage(ctx, client, msg, false)
	}
	return nil
}

func SendMessages(ctx context.Context, client *mixin.Client, msgs []*mixin.MessageRequest) error {
	err := client.SendMessages(ctx, msgs)
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			return nil
		}
		data, _ := json.Marshal(msgs[0])
		log.Println(err, string(data))
		time.Sleep(time.Millisecond)
		return SendMessages(ctx, client, msgs)
	}
	return nil
}

func UpdateDistributeMessagesStatusToFinished(ctx context.Context, msgs []*mixin.MessageRequest) error {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.MessageID
	}
	_, err := session.Database(ctx).Exec(ctx, `UPDATE distribute_messages SET status=$2 WHERE message_id=ANY($1)`, ids, DistributeMessageStatusFinished)
	return err
}

func getDistributeMessageIDMapByOriginMsgID(ctx context.Context, clientID, originMsgID string) (map[string]string, string, error) {
	// 2. 用 origin_message_id 和 user_id 找出 对应会话 里的 message_id ，这个 message_id 就是要 quote 的 id
	mapList, err := getQuoteMsgIDUserIDMaps(ctx, clientID, originMsgID)
	if err != nil {
		session.Logger(ctx).Println(err)
		return nil, "", err
	}
	msg, err := getMsgByClientIDAndMessageID(ctx, clientID, originMsgID)
	if err == nil {
		mapList[msg.UserID] = originMsgID
		return mapList, msg.UserID, nil
	}
	return mapList, "", nil
}

func DeleteDistributeMsgByClientID(ctx context.Context, clientID string) {
	_, err := session.Database(ctx).Exec(ctx, `
DELETE FROM messages WHERE client_id=$1 AND status=$2`, clientID, MessageStatusPending)
	if err != nil {
		session.Logger(ctx).Println(err)
		DeleteDistributeMsgByClientID(ctx, clientID)
		return
	}
	_, err = session.Database(ctx).Exec(ctx, `
DELETE FROM distribute_messages WHERE client_id=$1 AND status=1`, clientID)
	if err != nil {
		session.Logger(ctx).Println(err)
		DeleteDistributeMsgByClientID(ctx, clientID)
	}
}

func GetRemotePendingMsg(ctx context.Context, clientID string) time.Time {
	var t time.Time
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT created_at FROM distribute_messages WHERE client_id=$1 AND status=1 ORDER BY created_at ASC LIMIT 1
`, clientID).Scan(&t); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			session.Logger(ctx).Println(err)
		}
	}
	return t
}

func GetMsgStatistics(ctx context.Context) ([][]map[string]int, error) {
	sss := make([][]map[string]int, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT c.name, count(1)
FROM distribute_messages  d
LEFT JOIN  client c ON d.client_id = c.client_id
WHERE d.status=1
GROUP BY (c.name)`)
	if err != nil {
		return nil, err
	}
	ssi := make([]map[string]int, 0)
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		ssi = append(ssi, map[string]int{name: count})
	}
	sss = append(sss, ssi)
	rows, err = session.Database(ctx).Query(ctx, `
SELECT c.name, count(1) 
FROM distribute_messages as d 
LEFT JOIN client c ON d.client_id = c.client_id 
GROUP BY (c.name)
`)
	if err != nil {
		return nil, err
	}
	ssj := make([]map[string]int, 0)
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		ssj = append(ssj, map[string]int{name: count})
	}
	sss = append(sss, ssj)
	return sss, err
}

func createFinishedDistributeMsg(ctx context.Context, m *DistributeMessage) error {
	query := durable.InsertQuery("distribute_messages", "client_id,user_id,origin_message_id,conversation_id,shard_id,message_id,quote_message_id,status,level,created_at")
	_, err := session.Database(ctx).Exec(ctx, query, m.ClientID, m.UserID, m.OriginMessageID, m.ConversationID, m.ShardID, m.MessageID, m.QuoteMessageID, DistributeMessageStatusFinished, DistributeMessageLevelHigher, m.CreatedAt)
	return err
}
