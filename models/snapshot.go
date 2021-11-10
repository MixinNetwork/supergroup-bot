package models

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/MixinNetwork/supergroup/tools"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/shopspring/decimal"
)

const snapshots_DDL = `
CREATE TABLE IF NOT EXISTS snapshots (
    snapshot_id  				VARCHAR(36) NOT NULL PRIMARY KEY,
    client_id    				VARCHAR(36) NOT NULL,
    trace_id     				VARCHAR(36) NOT NULL,
    user_id      				VARCHAR(36) NOT NULL,
    asset_id     				VARCHAR(36) NOT NULL,
    amount       				VARCHAR NOT NULL,
    memo         				VARCHAR DEFAULT '',
    created_at   				timestamp with time zone NOT NULL
);
`
const transfer_pendding_DDL = `
CREATE TABLE IF NOT EXISTS transfer_pendding (
	trace_id 				VARCHAR(36) NOT NULL PRIMARY KEY,
	client_id 			VARCHAR(36) NOT NULL,
	asset_id 				VARCHAR(36) NOT NULL,
	opponent_id 		VARCHAR(36) NOT NULL,
	amount 					VARCHAR NOT NULL,
	memo 						VARCHAR DEFAULT '',
	status 					SMALLINT NOT NULL DEFAULT 1, -- 1 pending, 2 success
	created_at 			timestamp with time zone NOT NULL
);
`

type Snapshot struct {
	ClientID   string          `json:"client_id"`
	SnapshotID string          `json:"snapshot_id"`
	TraceID    string          `json:"trace_id"`
	UserID     string          `json:"user_id"`
	AssetID    string          `json:"asset_id"`
	Amount     decimal.Decimal `json:"amount"`
	Memo       string          `json:"memo"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Transfer struct {
	*mixin.TransferInput
	ClientID string `json:"client_id"`
}

const (
	TransferStatusPending = 1
	TransferStatusSucceed = 2
)

type snapshot struct {
	Type   string `json:"type,omitempty"`
	Reward string `json:"reward,omitempty"`
	ID     string `json:"id,omitempty"`
}

const (
	SnapshotTypeReward  = "reward"
	SnapshotTypeJoin    = "join"
	SnapshotTypeVip     = "vip"
	SnapshotTypeAirdrop = "airdrop"
)

func ReceivedSnapshot(ctx context.Context, clientID string, msg *mixin.MessageView) error {
	var s mixin.Snapshot
	if err := json.Unmarshal(tools.Base64Decode(msg.Data), &s); err != nil {
		session.Logger(ctx).Println(err)
		tools.PrintJson(msg)
		return nil
	}
	var r snapshot
	if err := json.Unmarshal([]byte(s.Memo), &r); err != nil {
		session.Logger(ctx).Println(err)
		tools.PrintJson(msg)
		return nil
	}
	switch r.Type {
	case "":
		fallthrough
	case SnapshotTypeReward:
		if err := handelRewardSnapshot(ctx, clientID, &s, r.Reward); err != nil {
			session.Logger(ctx).Println(err)
		}
	case SnapshotTypeJoin:
		if err := handelJoinSnapshot(ctx, clientID, &s); err != nil {
			session.Logger(ctx).Println(err)
		}
	case SnapshotTypeVip:
		if err := handelVipSnapshot(ctx, clientID, &s); err != nil {
			session.Logger(ctx).Println(err)
		}
	case SnapshotTypeAirdrop:
		if err := handelAirdropSnapshot(ctx, clientID, &s, r.ID); err != nil {
			session.Logger(ctx).Println(err)
		}
	}
	return nil
}

func handelRewardSnapshot(ctx context.Context, clientID string, s *mixin.Snapshot, reward string) error {
	if reward == "" {
		session.Logger(ctx).Println("reward is empty")
		tools.PrintJson(s)
		return nil
	}
	msg := config.Text.Reward
	from, err := getUserByID(ctx, s.OpponentID)
	if err != nil {
		return err
	}
	to, err := getUserByID(ctx, reward)
	if err != nil {
		return err
	}
	asset, err := GetAssetByID(ctx, nil, s.AssetID)
	if err != nil {
		return err
	}
	client := GetMixinClientByID(ctx, clientID)

	msg = strings.ReplaceAll(msg, "{send_name}", from.FullName)
	msg = strings.ReplaceAll(msg, "{reward_name}", to.FullName)
	msg = strings.ReplaceAll(msg, "{amount}", s.Amount.String())
	msg = strings.ReplaceAll(msg, "{symbol}", asset.Symbol)

	msg = tools.SplitString(msg, 36)
	byteMsg, err := json.Marshal([]mixin.AppButtonMessage{
		{Label: msg, Action: fmt.Sprintf("%s/reward?uid=%s", client.Host, to.IdentityNumber), Color: tools.RandomColor()},
	})
	if err != nil {
		return err
	}

	go SendClientMsg(clientID, mixin.MessageCategoryAppButtonGroup, tools.Base64Encode(byteMsg))
	go handleReward(clientID, s, from, to)
	return nil
}

func handelJoinSnapshot(ctx context.Context, clientID string, s *mixin.Snapshot) error {
	client, err := GetClientByID(ctx, clientID)
	if err != nil {
		return err
	}
	if client.PayStatus == ClientPayStatusOpen {
		a, err := decimal.NewFromString(client.PayAmount)
		if err != nil {
			return err
		}
		if s.AssetID == client.AssetID && s.Amount.Equal(a) {
			// 这是 一次 付费入群... 成功!
			u, err := SearchUser(ctx, s.OpponentID)
			if err != nil {
				return err
			}
			_, err = UpdateClientUser(ctx, &ClientUser{
				ClientID:     clientID,
				UserID:       s.OpponentID,
				AccessToken:  "",
				Priority:     ClientUserPriorityHigh,
				Status:       ClientUserStatusLarge,
				PayExpiredAt: s.CreatedAt.Add(time.Hour * 24 * 365 * 99),
			}, u.FullName)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		session.Logger(ctx).Println("error join snapshots...")
		tools.PrintJson(s)
	}
	return nil
}

const (
	USDTAssetID = "4d8c508b-91c5-375b-92b0-ee702ed2dac5" // erc20
)

func handelVipSnapshot(ctx context.Context, clientID string, s *mixin.Snapshot) error {
	c, err := GetClientByID(ctx, clientID)
	if err != nil {
		return err
	}

	var freshAmount, largeAmount decimal.Decimal
	if c.AssetID == "" {
		c.AssetID = USDTAssetID
		freshAmount = decimal.NewFromInt(1)
		largeAmount = decimal.NewFromInt(10)
	} else {
		cl, err := GetClientAssetLevel(ctx, clientID)
		if err != nil {
			session.Logger(ctx).Println(err)
			return nil
		}
		freshAmount = cl.FreshAmount
		largeAmount = cl.LargeAmount
	}
	if c.AssetID != s.AssetID {
		log.Println("发现异常的 VIP 转账....")
		tools.PrintJson(s)
		return nil
	}
	var status int
	var msg string
	if s.Amount.Equal(freshAmount) {
		status = ClientUserStatusFresh
		msg = config.Text.PayForFresh
	} else if s.Amount.Equal(largeAmount) {
		status = ClientUserStatusLarge
		msg = config.Text.PayForLarge
	} else {
		session.Logger(ctx).Println("member to vip amount error...")
		tools.PrintJson(s)
		return nil
	}
	expTime := s.CreatedAt.Add(time.Hour * 24 * 365)
	if err := UpdateClientUserPayStatus(ctx, clientID, s.OpponentID, status, expTime); err != nil {
		return err
	}
	go SendTextMsg(_ctx, clientID, s.OpponentID, msg)
	return nil
}

func handelAirdropSnapshot(ctx context.Context, clientID string, s *mixin.Snapshot, airdropID string) error {
	return UpdateAirdropToSuccess(ctx, s.TraceID)
}

// 处理 reward 的转账添加
func handleReward(clientID string, s *mixin.Snapshot, from, to *mixin.User) error {
	// 1. 保存转账记录
	if err := addSnapshot(_ctx, clientID, s); err != nil {
		session.Logger(_ctx).Println("add snapshot error", err)
		return err
	}
	// 2. 添加transfer_pendding
	traceID := mixin.UniqueConversationID(s.SnapshotID, s.TraceID)
	msg := strings.ReplaceAll(config.Text.From, "{identity_number}", from.IdentityNumber)
	if err := createTransferPending(_ctx, clientID, traceID, s.AssetID, to.UserID, msg, s.Amount); err != nil {
		session.Logger(_ctx).Println("create transfer_pendding error", err)
		return err
	}
	return nil
}

func HandleTransfer() {
	for {
		handleTransfer(_ctx)
		time.Sleep(5 * time.Second)
	}
}

func handleTransfer(ctx context.Context) {
	ts := make([]*Transfer, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT client_id,trace_id,asset_id,opponent_id,amount,memo 
FROM transfer_pendding 
WHERE status=1`)
	if err != nil {
		session.Logger(ctx).Println("select transfer_pendding error", err)
		return
	}

	for rows.Next() {
		t := Transfer{new(mixin.TransferInput), ""}
		if err := rows.Scan(&t.ClientID, &t.TraceID, &t.AssetID, &t.OpponentID, &t.Amount, &t.Memo); err != nil {
			return
		}
		ts = append(ts, &t)
	}
	for _, t := range ts {
		client := GetMixinClientByID(_ctx, t.ClientID)
		pin, err := getMixinPinByID(_ctx, t.ClientID)
		if err != nil {
			session.Logger(ctx).Println("get pin error", err)
			continue
		}
		s, err := client.Transfer(_ctx, t.TransferInput, pin)
		if err != nil {
			session.Logger(ctx).Println("transfer error", err)
			continue
		}
		if err := addSnapshot(ctx, t.ClientID, s); err != nil {
			session.Logger(ctx).Println("add snapshot error", err)
			continue
		}
		if err := updateTransferToSuccess(_ctx, t.TraceID); err != nil {
			session.Logger(ctx).Println("update transfer_pendding error", err)
			continue
		}
	}
}

func addSnapshot(ctx context.Context, clientID string, s *mixin.Snapshot) error {
	query := durable.InsertQueryOrUpdate("snapshots", "snapshot_id", "client_id,trace_id,user_id,asset_id,amount,memo,created_at")
	_, err := session.Database(ctx).Exec(ctx, query, s.SnapshotID, clientID, s.TraceID, s.OpponentID, s.AssetID, s.Amount.String(), s.Memo, s.CreatedAt)
	return err
}

func createTransferPending(ctx context.Context, client_id, traceID, assetID, opponentID, memo string, amount decimal.Decimal) error {
	query := durable.InsertQuery("transfer_pendding", "client_id,trace_id,asset_id,opponent_id,amount,memo,status,created_at")
	_, err := session.Database(ctx).Exec(ctx, query, client_id, traceID, assetID, opponentID, amount.String(), memo, TransferStatusPending, time.Now())
	return err
}

func updateTransferToSuccess(ctx context.Context, traceID string) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE transfer_pendding SET status = 2 WHERE trace_id = $1`, traceID)
	return err
}
