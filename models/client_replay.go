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
)

const client_replay_DDL = `
-- 自动回复
CREATE TABLE IF NOT EXISTS client_replay (
  client_id          VARCHAR(36) NOT NULL PRIMARY KEY,
  join_msg           TEXT DEFAULT '', -- 入群前的内容
  welcome            TEXT DEFAULT '', -- 入群时发送的内容
  updated_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
`

type ClientReplay struct {
	ClientID  string    `json:"client_id,omitempty"`
	JoinMsg   string    `json:"join_msg,omitempty"`
	Welcome   string    `json:"welcome,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func updateClientWelcome(ctx context.Context, clientID, welcome string) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE client_replay SET welcome=$2 WHERE client_id=$1`, clientID, welcome)
	return err
}

func UpdateClientReplay(ctx context.Context, c *ClientReplay) error {
	query := durable.InsertQueryOrUpdate("client_replay", "client_id", "join_msg,welcome,updated_at")
	_, err := session.Database(ctx).Exec(ctx, query, c.ClientID, c.JoinMsg, c.Welcome, time.Now())
	return err
}

func GetClientReplay(clientID string) (ClientReplay, error) {
	var c ClientReplay
	if err := session.Database(_ctx).QueryRow(_ctx, `
		SELECT client_id,join_msg,welcome,updated_at
		FROM client_replay WHERE client_id=$1
		`, clientID).Scan(&c.ClientID, &c.JoinMsg, &c.Welcome, &c.UpdatedAt); err != nil {
		return ClientReplay{}, err
	}
	return c, nil
}

func SendJoinMsg(clientID, userID string) {
	client, r, err := GetReplayAndMixinClientByClientID(clientID)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	if err := SendTextMsg(_ctx, clientID, userID, r.JoinMsg); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	if err := SendBtnMsg(_ctx, clientID, userID, mixin.AppButtonGroupMessage{
		{Label: config.Text.Join, Action: fmt.Sprintf("%s/auth", client.Host), Color: "#5979F0"},
	}); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendStickerLimitMsg(clientID, userID string) {
	if err := SendTextMsg(_ctx, clientID, userID, config.Text.StickerWarning); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendCategoryMsg(clientID, userID, category string, status int) {
	msg := strings.ReplaceAll(config.Text.CategoryReject, "{category}", config.Text.Category[category])
	isFreshMember := status < ClientUserStatusLarge
	if isFreshMember {
		msg += config.Text.MemberTips
	}
	if err := SendTextMsg(_ctx, clientID, userID, msg); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	if isFreshMember {
		sendMemberCentreBtn(clientID, userID)
	}
}

func SendWelcomeAndLatestMsg(clientID, userID string) {
	client, r, err := GetReplayAndMixinClientByClientID(clientID)
	if err != nil {
		return
	}
	if err := SendTextMsg(_ctx, clientID, userID, r.Welcome); err != nil {
		session.Logger(_ctx).Println(err)
	}
	btns := mixin.AppButtonGroupMessage{
		{Label: config.Text.Home, Action: client.Host, Color: "#5979F0"},
	}
	if client.AssetID != "" {
		btns = append(btns, mixin.AppButtonMessage{Label: config.Text.Transfer, Action: fmt.Sprintf("%s/trade/%s", client.Host, client.AssetID), Color: "#8A64D0"})
	}
	if err := SendBtnMsg(_ctx, clientID, userID, btns); err != nil {
		session.Logger(_ctx).Println(err)
	}
	conversationStatus := getClientConversationStatus(_ctx, clientID)
	if conversationStatus == "" ||
		conversationStatus == ClientConversationStatusNormal ||
		conversationStatus == ClientConversationStatusMute {
		go sendLatestMsg(client, userID, 20)
	} else if conversationStatus == ClientConversationStatusAudioLive {
		go sendLatestLiveMsg(client, userID)
	}
}

func sendLatestMsg(client *MixinClient, userID string, msgCount int) {
	ctx := _ctx
	c, err := GetClientUserByClientIDAndUserID(ctx, client.ClientID, userID)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	_ = UpdateClientUserPriority(ctx, client.ClientID, userID, ClientUserPriorityPending)
	sendPendingMsgByCount(ctx, client.ClientID, userID, msgCount)
	_ = UpdateClientUserPriority(ctx, client.ClientID, userID, c.Priority)
	SendAssetsNotPassMsg(client.ClientID, userID, "", true)
}

func sendLatestLiveMsg(client *MixinClient, userID string) {
	ctx := _ctx
	c, err := GetClientUserByClientIDAndUserID(ctx, client.ClientID, userID)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	_ = UpdateClientUserPriority(ctx, client.ClientID, userID, ClientUserPriorityPending)
	// 1. 获取直播的开始时间
	var startAt time.Time
	err = session.Database(ctx).QueryRow(ctx, `
SELECT ld.start_at FROM live_data ld
LEFT JOIN lives l ON ld.live_id=l.live_id
WHERE l.status=1
`).Scan(&startAt)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	sendPendingLiveMsg(ctx, client.ClientID, userID, startAt)
	_ = UpdateClientUserPriority(ctx, client.ClientID, userID, c.Priority)
}

func SendAssetsNotPassMsg(clientID, userID, quoteMsgID string, isJoin bool) {
	if isJoin {
		if err := SendTextMsg(_ctx, clientID, userID, config.Text.JoinMsgInfo); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
	} else {
		if err := SendTextMsgWithQuote(_ctx, clientID, userID, config.Text.BalanceReject, quoteMsgID); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
	}
	sendMemberCentreBtn(clientID, userID)
}

func SendLimitMsg(clientID, userID string, limit int) {
	msg := strings.ReplaceAll(config.Text.LimitReject, "{limit}", strconv.Itoa(limit))
	if limit < statusLimitMap[ClientUserStatusGuest] {
		msg += config.Text.MemberTips
	}
	if err := SendTextMsg(_ctx, clientID, userID, msg); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendStopMsg(clientID, userID string) {
	client := GetMixinClientByID(_ctx, clientID)
	if err := SendTextMsg(_ctx, clientID, userID, config.Text.StopMessage); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	if err := SendBtnMsg(_ctx, clientID, userID, mixin.AppButtonGroupMessage{
		{Label: config.Text.StopClose, Action: "input:/received_message", Color: "#5979F0"},
		{Label: config.Text.StopBroadcast, Action: fmt.Sprintf("%s/news", client.Host), Color: "#5979F0"},
	}); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendURLMsg(clientID, userID string) {
	if err := SendTextMsg(_ctx, clientID, userID, config.Text.URLReject); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendMutedMsg(clientID, userID string, mutedTime string, hour, minuted int) {
	msg := strings.ReplaceAll(config.Text.MutedReject, "{muted_time}", mutedTime)
	msg = strings.ReplaceAll(msg, "{hours}", strconv.Itoa(hour))
	msg = strings.ReplaceAll(msg, "{minutes}", strconv.Itoa(minuted))
	if err := SendTextMsg(_ctx, clientID, userID, msg); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendClientMuteMsg(clientID, userID string) {
	if err := SendTextMsg(_ctx, clientID, userID, config.Text.Muting); err != nil {
		session.Logger(_ctx).Println(err)
	}
}

func SendAuthSuccessMsg(clientID, userID string) {
	if err := SendTextMsg(_ctx, clientID, userID, config.Text.AuthSuccess); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendForbidMsg(clientID, userID, category string) {
	msg := strings.ReplaceAll(
		config.Text.Forbid,
		"{category}",
		config.Text.Category[category],
	)
	SendTextMsg(_ctx, clientID, userID, msg)
}

func sendMemberCentreBtn(clientID, userID string) {
	client := GetMixinClientByID(_ctx, clientID)
	if err := SendBtnMsg(_ctx, clientID, userID, mixin.AppButtonGroupMessage{
		{Label: config.Text.MemberCentre, Action: fmt.Sprintf("%s/member", client.Host), Color: "#5979F0"},
	}); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

// 处理 用户的 留言消息
func handleLeaveMsg(clientID, userID, originMsgID string, msg *mixin.MessageView) {
	forwardList, err := getClientManager(_ctx, clientID)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	msgList := make([]*mixin.MessageRequest, 0)
	// 组织管理员的消息
	quoteMsgIDMap, uid, err := getDistributeMessageIDMapByOriginMsgID(_ctx, clientID, originMsgID)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	if uid != "" {
		forwardList = append(forwardList, uid)
	}
	for _, id := range forwardList {
		if id == userID || id == "" {
			continue
		}
		msg := &mixin.MessageRequest{
			ConversationID:   mixin.UniqueConversationID(clientID, id),
			RecipientID:      id,
			MessageID:        tools.GetUUID(),
			Category:         msg.Category,
			Data:             msg.Data,
			RepresentativeID: userID,
			QuoteMessageID:   quoteMsgIDMap[id],
		}
		if id == uid {
			msg.RepresentativeID = ""
		}
		msgList = append(msgList, msg)
	}
	client := GetMixinClientByID(_ctx, clientID)
	if client.ClientID == "" {
		return
	}
	_ = SendMessages(_ctx, client.Client, msgList)
}

// 处理 用户的 链接 或 二维码的消息
func rejectMsgAndDeliverManagerWithOperationBtns(clientID string, msg *mixin.MessageView, sendToReceiver, sendToManager string) {
	// 1. 给用户发送 禁止的消息
	if sendToReceiver != "" {
		go SendTextMsg(_ctx, clientID, msg.UserID, sendToReceiver)
	}
	if err := createMessage(_ctx, clientID, msg, MessageStatusLeaveMessage); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	// 2. 给管理员发送检测到的消息
	managers, err := getClientManager(_ctx, clientID)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	oriMsg := make([]*mixin.MessageRequest, 0)
	quoteNoticeMsg := make([]*mixin.MessageRequest, 0)
	btnMsg := make([]*mixin.MessageRequest, 0)
	//   2.1. 发送原消息
	for _, uid := range managers {
		originMsgID := mixin.UniqueConversationID(msg.MessageID, uid)
		conversationID := mixin.UniqueConversationID(clientID, uid)
		oriMsg = append(oriMsg, &mixin.MessageRequest{
			ConversationID:   conversationID,
			RecipientID:      uid,
			MessageID:        originMsgID,
			Category:         msg.Category,
			Data:             msg.Data,
			RepresentativeID: msg.UserID,
		})
		if sendToManager != "" {
			quoteNoticeMsg = append(quoteNoticeMsg, &mixin.MessageRequest{
				ConversationID: conversationID,
				RecipientID:    uid,
				MessageID:      tools.GetUUID(),
				Category:       mixin.MessageCategoryPlainText,
				Data:           tools.Base64Encode([]byte(sendToManager)),
				QuoteMessageID: originMsgID,
			})
		}
		btnMsg = append(btnMsg, &mixin.MessageRequest{
			ConversationID: conversationID,
			RecipientID:    uid,
			MessageID:      tools.GetUUID(),
			Category:       mixin.MessageCategoryAppButtonGroup,
			Data: getBtnMsg([]mixin.AppButtonMessage{
				{Label: config.Text.Forward, Action: fmt.Sprintf("input:---operation,%s,%s", "forward", msg.MessageID), Color: "#5979F0"},
				{Label: config.Text.Mute, Action: fmt.Sprintf("input:---operation,%s,%s", "mute", msg.MessageID), Color: "#5979F0"},
				{Label: config.Text.Block, Action: fmt.Sprintf("input:---operation,%s,%s", "block", msg.MessageID), Color: "#5979F0"},
			}),
		})
	}
	client := GetMixinClientByID(_ctx, clientID).Client
	err = SendMessages(_ctx, client, oriMsg)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	//   2.2. 发送 quote 原消息的 提醒消息
	err = SendMessages(_ctx, client, quoteNoticeMsg)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
	// 	 2.3. 发送 三个 btn
	err = SendMessages(_ctx, client, btnMsg)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendClientTextMsg(clientID, msg, userID string, isJoinMsg bool) {
	if isJoinMsg && checkIsBlockUser(_ctx, clientID, userID) {
		return
	}
	client := GetMixinClientByID(_ctx, clientID).Client
	msgList := make([]*mixin.MessageRequest, 0)
	users, err := GetClientUserByPriority(_ctx, clientID, []int{ClientUserPriorityHigh, ClientUserPriorityLow}, isJoinMsg, false)
	if err != nil {
		session.Logger(_ctx).Println(err)
	}
	if len(users) <= 0 {
		return
	}
	msgBase64 := tools.Base64Encode([]byte(msg))
	var dataInsert [][]interface{}
	originMsgID := tools.GetUUID()
	if isJoinMsg {
		if err := createMessage(_ctx, clientID, &mixin.MessageView{
			ConversationID: mixin.UniqueConversationID(clientID, userID),
			UserID:         userID,
			MessageID:      originMsgID,
			Category:       mixin.MessageCategoryPlainText,
			Data:           msgBase64,
		}, MessageStatusJoinMsg); err != nil {
			session.Logger(_ctx).Println(err)
		}
	}

	for _, uid := range users {
		msgID := tools.GetUUID()
		if isJoinMsg {
			dataInsert = append(dataInsert,
				_createDistributeMessage(_ctx, clientID, uid, originMsgID, msgID, "", mixin.MessageCategoryPlainText, msgBase64, "", DistributeMessageLevelHigher, MessageStatusBroadcast, time.Now()))
		}
		msgList = append(msgList, &mixin.MessageRequest{
			ConversationID: mixin.UniqueConversationID(clientID, uid),
			RecipientID:    uid,
			MessageID:      msgID,
			Category:       mixin.MessageCategoryPlainText,
			Data:           msgBase64,
		})
	}
	if isJoinMsg {
		if err := createDistributeMsgList(_ctx, dataInsert); err != nil {
			session.Logger(_ctx).Println(err)
		}
	}

	if err := SendBatchMessages(_ctx, client, msgList); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func SendClientMsg(clientID, category, data string) {
	client := GetMixinClientByID(_ctx, clientID).Client
	msgList := make([]*mixin.MessageRequest, 0)
	users, err := GetClientUserByPriority(_ctx, clientID, []int{ClientUserPriorityHigh, ClientUserPriorityLow}, false, false)
	if err != nil {
		session.Logger(_ctx).Println(err)
	}
	if len(users) <= 0 {
		return
	}
	originMsgID := tools.GetUUID()
	if err := createMessage(_ctx, clientID, &mixin.MessageView{
		ConversationID: "",
		UserID:         "",
		MessageID:      originMsgID,
		Category:       category,
		Data:           data,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, MessageStatusClientMsg); err != nil {
		session.Logger(_ctx).Println(err)
	}

	for _, uid := range users {
		msgList = append(msgList, &mixin.MessageRequest{
			ConversationID: mixin.UniqueConversationID(clientID, uid),
			RecipientID:    uid,
			MessageID:      tools.GetUUID(),
			Category:       category,
			Data:           data,
		})
	}

	if err := SendBatchMessages(_ctx, client, msgList); err != nil {
		session.Logger(_ctx).Println(err)
		return
	}
}

func GetReplayAndMixinClientByClientID(clientID string) (*MixinClient, *ClientReplay, error) {
	r, err := GetClientReplay(clientID)
	if err != nil {
		return nil, nil, err
	}
	client := GetMixinClientByID(_ctx, clientID)
	return &client, &r, nil
}

func SendTextMsg(ctx context.Context, clientID, userID, data string) error {
	if data == "" {
		return nil
	}
	client := GetMixinClientByID(ctx, clientID)
	conversationID := mixin.UniqueConversationID(client.ClientID, userID)
	if err := SendMessage(ctx, client.Client, &mixin.MessageRequest{
		ConversationID: conversationID,
		RecipientID:    userID,
		MessageID:      tools.GetUUID(),
		Category:       mixin.MessageCategoryPlainText,
		Data:           tools.Base64Encode([]byte(data)),
	}, false); err != nil {
		return err
	}
	return nil
}

func SendTextMsgWithQuote(ctx context.Context, clientID, userID, data, quoteMsgID string) error {
	if data == "" {
		return nil
	}
	client := GetMixinClientByID(ctx, clientID)
	conversationID := mixin.UniqueConversationID(client.ClientID, userID)
	if err := SendMessage(ctx, client.Client, &mixin.MessageRequest{
		ConversationID: conversationID,
		RecipientID:    userID,
		MessageID:      tools.GetUUID(),
		Category:       mixin.MessageCategoryPlainText,
		Data:           tools.Base64Encode([]byte(data)),
		QuoteMessageID: quoteMsgID,
	}, false); err != nil {
		return err
	}
	return nil
}

func SendBtnMsg(ctx context.Context, clientID, userID string, data mixin.AppButtonGroupMessage) error {
	client := GetMixinClientByID(ctx, clientID)
	conversationID := mixin.UniqueConversationID(client.ClientID, userID)
	if err := SendMessage(ctx, client.Client, &mixin.MessageRequest{
		ConversationID: conversationID,
		RecipientID:    userID,
		MessageID:      tools.GetUUID(),
		Category:       mixin.MessageCategoryAppButtonGroup,
		Data:           getBtnMsg(data),
	}, false); err != nil {
		return err
	}
	return nil
}

func getBtnMsg(data mixin.AppButtonGroupMessage) string {
	btnData, err := json.Marshal(data)
	if err != nil {
		session.Logger(_ctx).Println(err)
		return ""
	}
	return tools.Base64Encode(btnData)
}

func SendRecallMsg(clientID string, msg *mixin.MessageView) {
	client := GetMixinClientByID(_ctx, clientID)
	data, _ := json.Marshal(map[string]string{"message_id": msg.QuoteMessageID})

	if err := SendMessage(_ctx, client.Client, &mixin.MessageRequest{
		ConversationID: msg.ConversationID,
		RecipientID:    msg.UserID,
		MessageID:      tools.GetUUID(),
		Category:       mixin.MessageCategoryMessageRecall,
		Data:           tools.Base64Encode(data),
	}, false); err != nil {
		session.Logger(_ctx).Println(err)
	}
}

func sendPendingMsgByCount(ctx context.Context, clientID, userID string, count int) {
	msgs := make([]*Message, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT user_id,message_id,category,data
FROM messages
WHERE client_id=$1
AND status IN (4,6)
AND category!='MESSAGE_RECALL'
AND created_at > CURRENT_DATE-1
ORDER BY created_at DESC
LIMIT $2`, clientID, count)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}

	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.UserID, &msg.MessageID, &msg.Category, &msg.Data); err != nil {
			return
		}
		msgs = append(msgs, &msg)
	}
	lastCreatedAt, err := distributeMsg(ctx, msgs, clientID, userID)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	for {
		if lastCreatedAt.IsZero() {
			break
		}
		lastCreatedAt, err = sendLeftMsg(ctx, clientID, userID, lastCreatedAt)
		if err != nil {
			session.Logger(ctx).Println(err)
			return
		}
	}
}

func sendPendingLiveMsg(ctx context.Context, clientID, userID string, startTime time.Time) {
	lastCreatedAt := startTime
	var err error
	for {
		if lastCreatedAt.IsZero() {
			break
		}
		lastCreatedAt, err = sendLeftMsg(ctx, clientID, userID, lastCreatedAt)
		if err != nil {
			session.Logger(ctx).Println(err)
			return
		}
	}
}

func sendLeftMsg(ctx context.Context, clientID, userID string, leftTime time.Time) (time.Time, error) {
	msgs := make([]*Message, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT user_id,message_id,category,data
FROM messages
WHERE client_id=$1
AND created_at>$2
AND status IN (4,6)
AND category!='MESSAGE_RECALL'
ORDER BY created_at DESC`, clientID, leftTime)
	if err != nil {
		return time.Time{}, err
	}

	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.UserID, &msg.MessageID, &msg.Category, &msg.Data); err != nil {
			return time.Time{}, err
		}
		msgs = append(msgs, &msg)
	}
	return distributeMsg(ctx, msgs, clientID, userID)
}

func distributeMsg(ctx context.Context, msgList []*Message, clientID, userID string) (time.Time, error) {
	msgs := make([]*mixin.MessageRequest, 0)
	dms := make([]*DistributeMessage, 0)
	conversationID := mixin.UniqueConversationID(clientID, userID)
	for _, message := range msgList {
		if userID == message.UserID {
			continue
		}
		msgID := tools.GetUUID()
		msgs = append([]*mixin.MessageRequest{{
			ConversationID:   conversationID,
			RecipientID:      userID,
			MessageID:        msgID,
			Category:         message.Category,
			Data:             message.Data,
			RepresentativeID: message.UserID,
		}}, msgs...)
		dms = append([]*DistributeMessage{{
			ClientID:         clientID,
			UserID:           userID,
			ConversationID:   conversationID,
			ShardID:          "0",
			OriginMessageID:  message.MessageID,
			MessageID:        msgID,
			Data:             message.Data,
			Category:         message.Category,
			RepresentativeID: message.UserID,
			CreatedAt:        time.Now(),
		}}, dms...)
	}
	if len(dms) == 0 {
		return time.Time{}, nil
	}
	client := GetMixinClientByID(ctx, clientID)
	// 存入成功之后再发送
	for i, dm := range dms {
		if err := createFinishedDistributeMsg(ctx, dm); err != nil {
			session.Logger(ctx).Println(err)
			continue
		}
		_ = SendMessage(ctx, client.Client, msgs[i], true)
	}
	return dms[0].CreatedAt, nil
}

func getLeftDistributeMsgAndDistribute(ctx context.Context, clientID, userID string) (time.Time, error) {
	msgs := make([]*mixin.MessageRequest, 0)
	var originMsgID string
	rows, err := session.Database(ctx).Query(ctx, `
SELECT conversation_id,user_id,message_id,category,data,representative_id,quote_message_id,origin_message_id
FROM distribute_messages
WHERE client_id=$1 AND user_id=$2 AND status=$3
ORDER BY created_at
`, clientID, userID, DistributeMessageStatusAloneList)
	if err != nil {
		return time.Time{}, err
	}

	for rows.Next() {
		var dm mixin.MessageRequest
		if err := rows.Scan(&dm.ConversationID, &dm.RecipientID, &dm.MessageID, &dm.Category, &dm.Data, &dm.RepresentativeID, &dm.QuoteMessageID, &originMsgID); err != nil {
			return time.Time{}, err
		}
		msgs = append(msgs, &dm)
	}
	if len(msgs) == 0 {
		return time.Time{}, nil
	}
	client := GetMixinClientByID(ctx, clientID)
	for _, msg := range msgs {
		if err := SendMessage(ctx, client.Client, msg, true); err == nil {
			if err := UpdateDistributeMessagesStatusToFinished(ctx, []*mixin.MessageRequest{msg}); err != nil {
				session.Logger(ctx).Println(err)
			}
		}
	}

	msg, err := getMsgByClientIDAndMessageID(ctx, clientID, originMsgID)
	if err != nil {
		session.Logger(ctx).Println(err)
		return time.Time{}, err
	}
	return msg.CreatedAt, nil
}
