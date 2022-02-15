package models

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
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

type transcript map[string]interface{}

type MessagePinBody struct {
	Action     string   `json:"action"`
	MessageIDs []string `json:"message_ids"`
}

// 创建消息 和 分发消息列表
func createAndDistributeMessage(ctx context.Context, clientID string, msg *mixin.MessageView) error {
	// 1. 创建消息
	err := createMessage(ctx, clientID, msg, MessageStatusNormal)
	if err != nil && !durable.CheckIsPKRepeatError(err) {
		session.Logger(ctx).Println(err)
		return err
	}
	// 2. 创建分发消息列表
	return CreateDistributeMsgAndMarkStatus(ctx, clientID, msg, []int{ClientUserPriorityHigh, ClientUserPriorityLow})
}

// 创建分发消息 标记 并把消息标记
func CreateDistributeMsgAndMarkStatus(ctx context.Context, clientID string, msg *mixin.MessageView, priorityList []int) error {
	userList, err := GetClientUserByPriority(ctx, clientID, priorityList, false, false)
	if err != nil {
		return err
	}
	level := priorityList[0]
	var status int
	if level == ClientUserPriorityHigh {
		status = MessageStatusPrivilege
	} else if level == ClientUserPriorityLow {
		status = MessageStatusFinished
	}
	// 处理 撤回 消息
	recallMsgIDMap := make(map[string]string)
	if msg.Category == mixin.MessageCategoryMessageRecall {
		recallMsgIDMap, err = getOriginMsgIDMapAndUpdateMsg(ctx, clientID, msg)
		if err != nil {
			return err
		}
		if recallMsgIDMap == nil {
			if err := updateMessageStatus(ctx, clientID, msg.MessageID, status); err != nil {
				session.Logger(ctx).Println(err)
				return err
			}
			return nil
		}
	}
	// 处理 PIN 消息
	var action string
	var pinMsgIDs map[string][]string
	if msg.Category == "MESSAGE_PIN" {
		pinMsgIDs, action, err = getPINMsgIDMapAndUpdateMsg(ctx, msg, clientID)
		if err != nil {
			return err
		}
		if pinMsgIDs == nil {
			// 没有 pin 消息（可能被删除了）
			if status == MessageStatusFinished {
				go SendTextMsg(_ctx, clientID, msg.UserID, config.Text.PINMessageErorr)
			}
			if err := updateMessageStatus(ctx, clientID, msg.MessageID, status); err != nil {
				session.Logger(ctx).Println(err)
				return err
			}
			return nil
		}
		defer func() {
			msgIDs := make([]string, len(pinMsgIDs))
			for _, pinMsg := range pinMsgIDs {
				msgIDs = append(msgIDs, pinMsg...)
			}
			if action == "UNPIN" {
				go UpdateDistributeMessagesStatusToFinished(_ctx, msgIDs)
			} else if action == "PIN" {
				go UpdateDistributeMessagesStatusToPIN(_ctx, msgIDs)
			}
		}()
	}
	// 创建消息
	var dataToInsert [][]interface{}
	quoteMessageIDMap := make(map[string]string)
	if msg.QuoteMessageID != "" {
		originMsg, _ := getDistributeMessageByClientIDAndMessageID(ctx, clientID, msg.QuoteMessageID)
		if originMsg.OriginMessageID != "" {
			quoteMessageIDMap, _, err = getDistributeMessageIDMapByOriginMsgID(ctx, clientID, originMsg.OriginMessageID)
			if err != nil {
				session.Logger(ctx).Println(err)
			}
		}
	}
	sendUserID := msg.UserID
	if sendUserID != config.Config.LuckCoinAppID &&
		sendUserID != "b523c28b-1946-4b98-a131-e1520780e8af" {
		if GetClientProxy(ctx, clientID) == ClientProxyStatusOn {
			u, err := GetClientUserByClientIDAndUserID(ctx, clientID, sendUserID)
			if err != nil {
				session.Logger(ctx).Println(err)
				return nil
			}
			if u.Status != ClientUserStatusAdmin {
				proxy, err := getClientUserProxyByProxyID(ctx, clientID, sendUserID)
				if err != nil {
					session.Logger(ctx).Println(err)
					return nil
				} else {
					sendUserID = proxy.UserID
				}
			}
		}
	}

	_data := ""

	for _, s := range userList {
		if s == msg.UserID || s == msg.RepresentativeID || checkIsBlockUser(ctx, clientID, s) {
			continue
		}

		// 处理 撤回 消息
		if msg.Category == mixin.MessageCategoryMessageRecall {
			if recallMsgIDMap[s] == "" {
				continue
			}
			data, err := json.Marshal(map[string]string{"message_id": recallMsgIDMap[s]})
			if err != nil {
				return err
			}
			msg.QuoteMessageID = ""
			_data = tools.Base64Encode(data)
		}

		// 处理 PIN 消息
		if msg.Category == "MESSAGE_PIN" {
			if pinMsgIDs[s] == nil || len(pinMsgIDs[s]) == 0 {
				continue
			}
			data, _ := json.Marshal(map[string]interface{}{"message_ids": pinMsgIDs[s], "action": action})
			_data = tools.Base64Encode(data)
		}
		if msg.QuoteMessageID != "" && quoteMessageIDMap[s] == "" {
			quoteMessageIDMap[s] = msg.QuoteMessageID
		}

		// 处理 聊天记录 消息
		msgID := tools.GetUUID()
		if msg.Category == "PLAIN_TRANSCRIPT" ||
			msg.Category == "ENCRYPTED_TRANSCRIPT" {
			t := make([]transcript, 0)
			err := json.Unmarshal(tools.Base64Decode(msg.Data), &t)
			if err != nil {
				session.Logger(ctx).Println(err)
				return err
			}
			for i := range t {
				t[i]["transcript_id"] = msgID
			}
			byteData, err := json.Marshal(t)
			if err != nil {
				session.Logger(ctx).Println(err)
				return err
			}
			_data = tools.Base64Encode(byteData)
		}
		row := _createDistributeMessage(ctx, clientID, s, msg.MessageID, msgID, quoteMessageIDMap[s], msg.Category, _data, sendUserID, level, DistributeMessageStatusPending, time.Now())
		dataToInsert = append(dataToInsert, row)
	}
	now := time.Now().UnixNano()
	if err := createDistributeMsgList(ctx, dataToInsert); err != nil {
		session.Logger(ctx).Println(err)
		return err
	}
	tools.PrintTimeDuration(fmt.Sprintf("%d条消息入库%s", len(dataToInsert), clientID), now)
	if err := updateMessageStatus(ctx, clientID, msg.MessageID, status); err != nil {
		session.Logger(ctx).Println(err)
		return err
	}
	return nil
}

func CreatedManagerRecallMsg(ctx context.Context, clientID string, msgID, uid string) error {
	dataByte, _ := json.Marshal(map[string]string{"message_id": msgID})

	if err := createAndDistributeMessage(ctx, clientID, &mixin.MessageView{
		UserID:    uid,
		MessageID: tools.GetUUID(),
		Category:  mixin.MessageCategoryMessageRecall,
		Data:      tools.Base64Encode(dataByte),
	}); err != nil {
		session.Logger(ctx).Println(err)
	}

	return nil
}

var distributeCols = []string{"client_id", "user_id", "shard_id", "conversation_id", "origin_message_id", "message_id", "quote_message_id", "category", "data", "representative_id", "level", "status", "created_at"}

func createDistributeMsgList(ctx context.Context, insert [][]interface{}) error {
	var ident = pgx.Identifier{"distribute_messages"}
	if len(insert) == 0 {
		return nil
	}
	var isPending = false
	var clientID string
	if insert[0][11].(int) == DistributeMessageStatusPending {
		isPending = true
		clientID = insert[0][0].(string)
	}
	for {
		if len(insert) == 0 {
			break
		}
		batch := [][]interface{}{}
		if len(insert) > 200 {
			batch = insert[:200]
			insert = insert[200:]
		} else {
			batch = insert
			insert = [][]interface{}{}
		}
		_, err := session.Database(ctx).CopyFrom(ctx, ident, distributeCols, pgx.CopyFromRows(batch))
		time.Sleep(time.Millisecond * 10)
		if err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				session.Logger(ctx).Println(err)
			}
		}
	}
	if isPending {
		session.Redis(ctx).QPublish(ctx, "distribute", clientID)
	}
	return nil
}

func getOriginMsgIDMapAndUpdateMsg(ctx context.Context, clientID string, msg *mixin.MessageView) (map[string]string, error) {
	originMsgID := getRecallOriginMsgID(ctx, msg.Data)
	_ = updateMessageStatus(ctx, clientID, originMsgID, MessageStatusRecallMsg)
	recallMsgIDMap, err := getQuoteMsgIDUserIDMaps(ctx, clientID, originMsgID)
	if err != nil {
		return nil, err
	}
	if len(recallMsgIDMap) == 0 {
		return nil, nil
	}
	return recallMsgIDMap, nil
}

func getPINMsgIDMapAndUpdateMsg(ctx context.Context, msg *mixin.MessageView, clientID string) (map[string][]string, string, error) {
	action, orginMsgIDs := getPinOriginMsgIDs(ctx, msg.Data)
	pinMsgIDMaps, err := getQuoteMsgIDUserIDsMaps(ctx, clientID, orginMsgIDs)
	if err != nil {
		return nil, "", err
	}
	if len(pinMsgIDMaps) == 0 {
		return nil, "", nil
	}
	status := MessageStatusPINMsg
	if action == "UNPIN" {
		status = MessageStatusFinished
	}
	for _, msgID := range orginMsgIDs {
		updateMessageStatus(ctx, clientID, msgID, status)
	}
	return pinMsgIDMaps, action, nil
}

func getQuoteMsgIDUserIDMaps(ctx context.Context, clientID, originMsgID string) (map[string]string, error) {
	recallMsgIDMap := make(map[string]string)
	if err := session.Database(ctx).ConnQuery(ctx, `
SELECT message_id, user_id
FROM distribute_messages
WHERE client_id=$1 AND origin_message_id=$2`, func(rows pgx.Rows) error {
		for rows.Next() {
			var msgID, userID string
			if err := rows.Scan(&msgID, &userID); err != nil {
				return err
			}
			recallMsgIDMap[userID] = msgID
		}
		return nil
	}, clientID, originMsgID); err != nil {
		return nil, err
	}
	if len(recallMsgIDMap) == 0 {
		// 消息已经被删除...
		return nil, nil
	}
	return recallMsgIDMap, nil
}

func getQuoteMsgIDUserIDsMaps(ctx context.Context, clientID string, originMsgID []string) (map[string][]string, error) {
	recallMsgIDMap := make(map[string][]string)
	if err := session.Database(ctx).ConnQuery(ctx, `
SELECT message_id, user_id
FROM distribute_messages
WHERE client_id=$1 AND origin_message_id=ANY($2)`, func(rows pgx.Rows) error {
		for rows.Next() {
			var msgID, userID string
			if err := rows.Scan(&msgID, &userID); err != nil {
				return err
			}
			if recallMsgIDMap[userID] == nil {
				recallMsgIDMap[userID] = make([]string, 0)
			}
			recallMsgIDMap[userID] = append(recallMsgIDMap[userID], msgID)
		}
		return nil
	}, clientID, originMsgID); err != nil {
		return nil, err
	}
	if len(recallMsgIDMap) == 0 {
		// 消息已经被删除...
		return nil, nil
	}
	return recallMsgIDMap, nil
}

func _createDistributeMessage(ctx context.Context, clientID, userID, originMsgID, messageID, quoteMsgID, category, data, representativeID string, level, status int, createdAt time.Time) []interface{} {
	conversationID := mixin.UniqueConversationID(clientID, userID)
	shardID := ClientShardIDMap[clientID][userID]
	if shardID == "" {
		shardID = getRandomMessageShard()
	}
	if category == mixin.MessageCategoryMessageRecall {
		representativeID = ""
	}
	var row []interface{}
	row = append(row, clientID)
	row = append(row, userID)
	row = append(row, shardID)
	row = append(row, conversationID)
	row = append(row, originMsgID)
	row = append(row, messageID)
	row = append(row, quoteMsgID)
	row = append(row, category)
	row = append(row, data)
	row = append(row, representativeID)
	row = append(row, level)
	row = append(row, status)
	row = append(row, createdAt)
	return row
}

func getRecallOriginMsgID(ctx context.Context, msgData string) string {
	data := tools.Base64Decode(msgData)
	var msg struct {
		MessageID string `json:"message_id"`
	}
	err := json.Unmarshal(data, &msg)
	if err != nil {
		session.Logger(ctx).Println(err)
		return ""
	}
	return msg.MessageID
}

func getPinOriginMsgIDs(ctx context.Context, msgData string) (string, []string) {
	data := tools.Base64Decode(msgData)
	var msg MessagePinBody
	_ = json.Unmarshal(data, &msg)
	msgIDs := make([]string, len(msg.MessageIDs))
	if err := session.Database(ctx).ConnQuery(ctx, `
SELECT origin_message_id
FROM distribute_messages
WHERE message_id=ANY($1)`, func(rows pgx.Rows) error {
		for rows.Next() {
			var originMsgID string
			if err := rows.Scan(&originMsgID); err != nil {
				return err
			}
			msgIDs = append(msgIDs, originMsgID)
		}
		return nil
	}, msg.MessageIDs); err != nil {
		session.Logger(ctx).Println(err)
	}
	return msg.Action, msgIDs
}

var ClientShardIDMap = make(map[string]map[string]string)

func InitShardID(ctx context.Context, clientID string) error {
	ClientShardIDMap[clientID] = make(map[string]string)
	// 1. 获取有多少个协程，就分配多少个编号
	count := decimal.NewFromInt(config.MessageShardSize)
	// 2. 获取优先级高/低的所有用户，及高低比例
	high, low, err := GetClientUserReceived(ctx, clientID)
	if err != nil {
		return err
	}
	// 每个分组的平均人数
	highCount := int(decimal.NewFromInt(int64(len(high))).Div(count).Ceil().IntPart())
	lowCount := int(decimal.NewFromInt(int64(len(low))).Div(count).Ceil().IntPart())
	// 3. 给这个大群里 每个用户进行 编号
	for shardID := 0; shardID < int(config.MessageShardSize); shardID++ {
		strShardID := strconv.Itoa(shardID)
		cutCount := 0
		hC := len(high)
		for i := 0; i < highCount; i++ {
			if i == hC {
				break
			}
			cutCount++
			ClientShardIDMap[clientID][high[i]] = strShardID
		}
		if cutCount > 0 {
			high = high[cutCount:]
		}

		cutCount = 0
		lC := len(low)
		for i := 0; i < lowCount; i++ {
			if i == lC {
				break
			}
			cutCount++
			ClientShardIDMap[clientID][low[i]] = strShardID
		}
		if cutCount > 0 {
			low = low[cutCount:]
		}
	}
	return nil
}

func getRandomMessageShard() string {
	return strconv.Itoa(rand.Intn(int(config.MessageShardSize)))
}
