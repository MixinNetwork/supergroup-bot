package models

import (
	"context"
	"errors"
	"time"

	"github.com/MixinNetwork/supergroup/tools"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/jackc/pgx/v4"
	"github.com/shopspring/decimal"
)

const client_DDL = `
-- 机器人信息
CREATE TABLE IF NOT EXISTS client (
  client_id          VARCHAR(36) NOT NULL PRIMARY KEY,
  client_secret      VARCHAR NOT NULL,
  session_id         VARCHAR(36) NOT NULL,
  pin_token          VARCHAR NOT NULL,
  private_key        VARCHAR NOT NULL,
  pin                VARCHAR(6) DEFAULT '',
  name               VARCHAR NOT NULL,
  icon_url           VARCHAR NOT NULL DEFAULT '',
  description        VARCHAR NOT NULL,
  host               VARCHAR NOT NULL, -- 前端部署的 host
  asset_id           VARCHAR(36) NOT NULL,
	owner_id					 VARCHAR(36) NOT NULL,
  speak_status       SMALLINT NOT NULL DEFAULT 1, -- 1 正常发言 2 持仓发言
	pay_status				 SMALLINT NOT NULL DEFAULT 0, -- 0 关闭 1 开启
	pay_amount			   VARCHAR NOT NULL DEFAULT '', -- 付费入群开启的金额
  created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
)
`

type Client struct {
	ClientID     string    `json:"client_id,omitempty"`
	ClientSecret string    `json:"client_secret,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	PinToken     string    `json:"pin_token,omitempty"`
	PrivateKey   string    `json:"private_key,omitempty"`
	Pin          string    `json:"pin,omitempty"`
	Name         string    `json:"name,omitempty"`
	Description  string    `json:"description,omitempty"`
	Host         string    `json:"host,omitempty"`
	AssetID      string    `json:"asset_id,omitempty"`
	OwnerID      string    `json:"owner_id,omitempty"`
	SpeakStatus  int       `json:"speak_status,omitempty"`
	PayStatus    int       `json:"pay_status,omitempty"`
	PayAmount    string    `json:"pay_amount,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`

	IconURL string `json:"icon_url,omitempty"`
	Symbol  string `json:"symbol,omitempty"`
	Welcome string `json:"welcome,omitempty"`
	Amount  string `json:"amount,omitempty"`
}

const (
	ClientSpeckStatusOpen  = 1 // 持仓发言打开，
	ClientSpeckStatusClose = 2 // 持仓发言关闭

	ClientPayStatusOpen = 1 // 入群开启，

)

func UpdateClientSetting(ctx context.Context, u *ClientUser, desc, welcome string) error {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	if desc != "" {
		if _, err := session.Database(ctx).Exec(ctx, `
UPDATE client SET description=$2 WHERE client_id=$1
`, u.ClientID, desc); err != nil {
			return err
		}
	}
	if welcome != "" {
		if err := updateClientWelcome(ctx, u.ClientID, welcome); err != nil {
			return err
		}
		go func() {
			// 给管理员发两条消息
			SendToClientManager(u.ClientID, &mixin.MessageView{
				ConversationID: mixin.UniqueConversationID(u.ClientID, u.UserID),
				UserID:         u.UserID,
				MessageID:      tools.GetUUID(),
				Category:       mixin.MessageCategoryPlainText,
				Data:           tools.Base64Encode([]byte(config.Text.WelcomeUpdate)),
				CreatedAt:      time.Now(),
			}, false, false)
			SendToClientManager(u.ClientID, &mixin.MessageView{
				ConversationID: mixin.UniqueConversationID(u.ClientID, u.UserID),
				UserID:         u.UserID,
				MessageID:      tools.GetUUID(),
				Category:       mixin.MessageCategoryPlainText,
				Data:           tools.Base64Encode([]byte(welcome)),
				CreatedAt:      time.Now(),
			}, false, false)
		}()
	}
	return nil
}

func UpdateClient(ctx context.Context, c *Client) error {
	query := durable.InsertQueryOrUpdate("client", "client_id", "client_secret,session_id,pin_token,private_key,pin,name,icon_url,description,asset_id,host,speak_status,owner_id")
	_, err := session.Database(ctx).Exec(ctx, query, c.ClientID, c.ClientSecret, c.SessionID, c.PinToken, c.PrivateKey, c.Pin, c.Name, c.IconURL, c.Description, c.AssetID, c.Host, c.SpeakStatus, c.OwnerID)
	return err
}

func GetClientByID(ctx context.Context, clientID string) (Client, error) {
	var c Client
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT client_id,session_id,pin_token,private_key,pin,name,description,speak_status,host,asset_id,icon_url,owner_id,pay_status,pay_amount
FROM client 
WHERE client.client_id=$1`,
		clientID).Scan(&c.ClientID, &c.SessionID, &c.PinToken, &c.PrivateKey, &c.Pin, &c.Name, &c.Description, &c.SpeakStatus, &c.Host, &c.AssetID, &c.IconURL, &c.OwnerID, &c.PayStatus, &c.PayAmount); err != nil {
		return c, err
	}
	return c, nil
}

func GetClientList(ctx context.Context) ([]*Client, error) {
	rows, err := session.Database(ctx).Query(ctx, `
SELECT client_id, session_id,pin_token,private_key,pin,speak_status,asset_id,created_at
FROM client
WHERE client_id=ANY($1)`, config.Config.ClientList)
	if err != nil {
		return nil, err
	}
	var clientList []*Client
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ClientID, &c.SessionID, &c.PinToken, &c.PrivateKey, &c.Pin, &c.SpeakStatus, &c.AssetID, &c.CreatedAt); err != nil {
			return nil, err
		}
		clientList = append(clientList, &c)
	}
	return clientList, err
}

func GetFirstClient(ctx context.Context) *mixin.Client {
	c, err := getAllClient(ctx)
	if err != nil {
		return nil
	}
	return GetMixinClientByID(ctx, c[0]).Client
}

type MixinClient struct {
	*mixin.Client
	Secret      string
	AssetID     string
	SpeakStatus int
	Host        string
}

func GetMixinClientByHost(ctx context.Context, host string) *MixinClient {
	if host == "" {
		host = "http://192.168.2.237:8000"
	}
	var keystore mixin.Keystore
	var secret, assetID string
	var speakStatus int
	err := session.Database(ctx).QueryRow(ctx, `
SELECT client_id,client_secret,session_id,pin_token,private_key,speak_status,asset_id
FROM client WHERE host=$1
`, host).Scan(&keystore.ClientID, &secret, &keystore.SessionID, &keystore.PinToken, &keystore.PrivateKey, &speakStatus, &assetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			session.Logger(ctx).Println(host, "...Host NOT FOUND")
		}
		return nil
	}
	client, err := mixin.NewFromKeystore(&keystore)
	if err != nil {
		session.Logger(ctx).Println(err)
		return nil
	}
	return &MixinClient{Client: client, Secret: secret, SpeakStatus: speakStatus, AssetID: assetID}
}

var cacheIdClientMap = make(map[string]MixinClient)
var nilIDClientMap = MixinClient{}

func GetMixinClientByID(ctx context.Context, clientID string) MixinClient {
	if cacheIdClientMap[clientID] == nilIDClientMap {
		var keystore mixin.Keystore
		var secret, assetID, host string
		var speakStatus int
		err := session.Database(ctx).QueryRow(ctx, `
SELECT client_id,client_secret,session_id,pin_token,private_key,speak_status,asset_id,host
FROM client WHERE client_id=$1
`, clientID).Scan(&keystore.ClientID, &secret, &keystore.SessionID, &keystore.PinToken, &keystore.PrivateKey, &speakStatus, &assetID, &host)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				session.Logger(ctx).Println(err)
			}
			return MixinClient{}
		}
		client, err := mixin.NewFromKeystore(&keystore)
		if err != nil {
			session.Logger(ctx).Println(err)
			return MixinClient{}
		}
		cacheIdClientMap[clientID] = MixinClient{Client: client, Secret: secret, SpeakStatus: speakStatus, AssetID: assetID, Host: host}
	}
	return cacheIdClientMap[clientID]
}

var cachePinMap = make(map[string]string)

func getMixinPinByID(ctx context.Context, clientID string) (string, error) {
	if cachePinMap[clientID] == "" {
		var pin string
		if err := session.Database(ctx).QueryRow(ctx, `
SELECT pin FROM client WHERE client_id=$1
`, clientID).Scan(&pin); err != nil {
			return "", err
		}
		cachePinMap[clientID] = pin
	}
	return cachePinMap[clientID], nil
}

func GetClientStatusByID(ctx context.Context, u *ClientUser) string {
	return getClientConversationStatus(ctx, u.ClientID)
}

type ClientInfo struct {
	*Client
	PriceUsd      decimal.Decimal `json:"price_usd,omitempty"`
	ChangeUsd     string          `json:"change_usd,omitempty"`
	TotalPeople   decimal.Decimal `json:"total_people"`
	WeekPeople    decimal.Decimal `json:"week_people"`
	Activity      []*Activity     `json:"activity,omitempty"`
	HasReward     bool            `json:"has_reward"`
	NeedPayAmount decimal.Decimal `json:"need_pay_amount,omitempty"`
	LargeAmount   string          `json:"large_amount,omitempty"`
}

const (
	ExinOneClientID = "47cdbc9e-e2b9-4d1f-b13e-42fec1d8853d"
	XinAssetID      = "c94ac88f-4671-3976-b60a-09064f1811e8"
	BtcAssetID      = "c6d0c728-2624-429b-8e0d-d9d19b6592fa"
)

func GetClientInfoByHostOrID(ctx context.Context, host, id string) (*ClientInfo, error) {
	var mixinClient MixinClient
	if id != "" {
		mixinClient = GetMixinClientByID(ctx, id)
	} else {
		c := GetMixinClientByHost(ctx, host)
		if c == nil {
			return nil, session.BadDataError(ctx)
		}
		mixinClient = *c
	}
	client, err := GetClientByID(ctx, mixinClient.ClientID)
	if err != nil {
		return nil, err
	}
	var c ClientInfo
	if client.Pin != "" {
		c.HasReward = true
	}
	client.SessionID = ""
	client.PinToken = ""
	client.PrivateKey = ""
	client.Pin = ""
	c.Client = &client
	assetID := client.AssetID
	if c.ClientID == ExinOneClientID {
		assetID = XinAssetID
	} else if assetID == "" {
		assetID = BtcAssetID
	}
	asset, err := GetAssetByID(ctx, mixinClient.Client, assetID)
	if err == nil {
		c.PriceUsd = asset.PriceUsd
		c.ChangeUsd = asset.ChangeUsd
		c.Symbol = asset.Symbol
		if client.AssetID != "" {
			c.IconURL = asset.IconUrl
		}
	}
	r, err := GetClientReplay(client.ClientID)
	if err == nil {
		c.Welcome = r.Welcome
	}
	amount, err := GetClientAssetLevel(ctx, client.ClientID)
	if err == nil {
		c.Amount = amount.Fresh.String()
		c.LargeAmount = amount.Large.String()
	}

	c.TotalPeople, c.WeekPeople, err = getClientPeopleCount(ctx, client.ClientID)
	if err != nil {
		return nil, err
	}
	c.Activity, err = GetActivityByClientID(ctx, client.ClientID)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func GetAllClientInfo(ctx context.Context) ([]ClientInfo, error) {
	rows, err := session.Database(ctx).Query(ctx, ` SELECT client_id FROM client WHERE client_id=ANY($1) `, config.Config.ShowClientList)

	if err != nil {
		return nil, err
	}

	cis := make([]ClientInfo, 0)
	for rows.Next() {
		var clientID string
		if err := rows.Scan(&clientID); err != nil {
			return nil, err
		}
		if ci, err := GetClientInfoByHostOrID(ctx, "", clientID); err != nil {
			return nil, err
		} else {
			cis = append(cis, *ci)
		}
	}
	return cis, nil
}

func getAllClient(ctx context.Context) ([]string, error) {
	rows, err := session.Database(ctx).Query(ctx, ` SELECT client_id FROM client limit 1000 `)
	if err != nil {
		return nil, err
	}

	var cs []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cs = append(cs, c)
	}
	return cs, err
}
