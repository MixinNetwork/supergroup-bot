package models

import (
	"context"
	"encoding/json"
	"fmt"
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

type (
	transcript struct {
		TranscriptID   string    `json:"transcript_id,omitempty"`
		MessageID      string    `json:"message_id,omitempty"`
		UserID         string    `json:"user_id,omitempty"`
		UserFullName   string    `json:"user_full_name,omitempty"`
		Category       string    `json:"category,omitempty"`
		Content        string    `json:"content,omitempty"`
		MediaURL       string    `json:"media_url,omitempty"`
		MediaName      string    `json:"media_name,omitempty"`
		MediaSize      int64     `json:"media_size,omitempty"`
		MediaWidth     int64     `json:"media_width,omitempty"`
		MediaHeight    int64     `json:"media_height,omitempty"`
		MediaDuration  int64     `json:"media_duration,omitempty"`
		MediaMimeType  string    `json:"media_mime_type,omitempty"`
		MediaStatus    string    `json:"media_status,omitempty"`
		MediaWaveform  string    `json:"media_waveform,omitempty"`
		MediaKey       string    `json:"media_key,omitempty"`
		MediaDigest    string    `json:"media_digest,omitempty"`
		MediaCreatedAt time.Time `json:"media_created_at,omitempty"`
		ThumbImage     string    `json:"thumb_image,omitempty"`
		ThumbURL       string    `json:"thumb_url,omitempty"`
		StickerID      string    `json:"sticker_id,omitempty"`
		SharedUserID   string    `json:"shared_user_id,omitempty"`
		Mentions       string    `json:"mentions,omitempty"`
		QuoteID        string    `json:"quote_id,omitempty"`
		QuoteContent   string    `json:"quote_content,omitempty"`
		Caption        string    `json:"caption,omitempty"`
		CreatedAt      time.Time `json:"created_at,omitempty"`
	}
)

// 创建消息 和 分发消息列表
func createAndDistributeMessage(ctx context.Context, clientID string, msg *mixin.MessageView) error {
	// 1. 创建消息
	err := createMessage(ctx, clientID, msg, MessageStatusNormal)
	if err != nil && !durable.CheckIsPKRepeatError(err) {
		session.Logger(ctx).Println(err)
		return err
	}
	// 2. 创建分发消息列表
	return CreateDistributeMsgAndMarkStatus(ctx, clientID, msg, []int{ClientUserPriorityHigh})
}

// 创建分发消息 标记 并把消息标记
func CreateDistributeMsgAndMarkStatus(ctx context.Context, clientID string, msg *mixin.MessageView, priorityList []int) error {
	userList, err := GetClientUserByPriority(ctx, clientID, priorityList, false, false)
	if err != nil {
		return err
	}
	recallMsgIDMap := make(map[string]string)
	level := priorityList[0]
	var status int
	if level == ClientUserPriorityHigh {
		status = MessageStatusPrivilege
	} else if level == ClientUserPriorityLow {
		status = MessageStatusFinished
	}
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
	for _, s := range userList {
		if s == msg.UserID || s == msg.RepresentativeID || checkIsBlockUser(ctx, clientID, s) {
			continue
		}
		if msg.Category == mixin.MessageCategoryMessageRecall {
			if recallMsgIDMap[s] == "" {
				continue
			}
			data, err := json.Marshal(map[string]string{"message_id": recallMsgIDMap[s]})
			if err != nil {
				return err
			}
			msg.QuoteMessageID = ""
			msg.Data = tools.Base64Encode(data)
		}
		if msg.QuoteMessageID != "" && quoteMessageIDMap[s] == "" {
			quoteMessageIDMap[s] = msg.QuoteMessageID
		}
		msgID := tools.GetUUID()
		if msg.Category == "PLAIN_TRANSCRIPT" {
			t := make([]*transcript, 0)
			err := json.Unmarshal(tools.Base64Decode(msg.Data), &t)
			if err != nil {
				session.Logger(ctx).Println(err)
				return err
			}
			for i := range t {
				t[i].TranscriptID = msgID
			}
			byteData, err := json.Marshal(t)
			if err != nil {
				session.Logger(ctx).Println(err)
				return err
			}
			msg.Data = tools.Base64Encode(byteData)
		}
		row := _createDistributeMessage(ctx, clientID, s, msg.MessageID, msgID, quoteMessageIDMap[s], msg.Category, msg.Data, msg.UserID, level, DistributeMessageStatusPending, time.Now())
		dataToInsert = append(dataToInsert, row)
	}
	now := time.Now().UnixNano()
	if err := createDistributeMsgList(ctx, dataToInsert); err != nil {
		session.Logger(ctx).Println(err)
		return err
	}
	tools.PrintTimeDuration(fmt.Sprintf("%d条消息入库%s", len(dataToInsert), clientID), now)
	// 3. 标记消息为 privilege
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

var cols = []string{"client_id", "user_id", "shard_id", "conversation_id", "origin_message_id", "message_id", "quote_message_id", "category", "data", "representative_id", "level", "status", "created_at"}

func createDistributeMsgList(ctx context.Context, insert [][]interface{}) error {
	var ident = pgx.Identifier{"distribute_messages"}
	_, err := session.Database(ctx).CopyFrom(ctx, ident, cols, pgx.CopyFromRows(insert))
	if err != nil {
		if !strings.Contains(err.Error(), "duplicate key") {
			session.Logger(ctx).Println(err)
		}
	}
	return nil
}

var recallMsgCategorySupportList = []string{
	mixin.MessageCategoryPlainPost,
	mixin.MessageCategoryPlainText,
	mixin.MessageCategoryPlainImage,
	mixin.MessageCategoryPlainSticker,
	"PLAIN_AUDIO",
	mixin.MessageCategoryPlainVideo,
	mixin.MessageCategoryPlainData,
	mixin.MessageCategoryPlainContact,
	"PLAIN_LOCATION",
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

func getQuoteMsgIDUserIDMaps(ctx context.Context, clientID, originMsgID string) (map[string]string, error) {
	recallMsgIDMap := make(map[string]string)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT message_id, user_id
FROM distribute_messages
WHERE client_id=$1 AND origin_message_id=$2`, func(rows pgx.Rows) error {
		return nil
	}, clientID, originMsgID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var msgID, userID string
		if err := rows.Scan(&msgID, &userID); err != nil {
			return nil, err
		}
		recallMsgIDMap[userID] = msgID
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
		shardID = "0"
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
