package config

import (
	"io/ioutil"
	"log"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v2"
)

const (
	BuildVersion = "BUILD_VERSION"
)

const (
	MessageShardSize           = int64(20)
	CacheTime                  = 15 * time.Minute
	AssetsCheckTime            = 12 * time.Hour
	NotActiveCheckTime         = 14 * 24.0
	NotOpenAssetsCheckMsgLimit = 10
	NoticeLotteryTimes         = 5
)

type Lottery struct {
	LotteryID string          `json:"lottery_id"`
	AssetID   string          `json:"asset_id"`
	Amount    decimal.Decimal `json:"amount"`
	IconURL   string          `json:"icon_url"`
	ClientID  string          `json:"client_id"`
}
type config struct {
	Lang      string `yaml:"lang"`
	Port      int    `yaml:"port"`
	Dev       string `yaml:"dev"`
	Encrypted bool   `yaml:"encrypted"`
	Database  struct {
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Name     string `yaml:"name"`
	} `yaml:"database"`

	Monitor struct {
		ClientID       string `yaml:"client_id"`
		SessionID      string `yaml:"session_id"`
		PrivateKey     string `yaml:"private_key"`
		ConversationID string `yaml:"conversation_id"`
	} `yaml:"monitor"`

	Lottery struct {
		ClientID   string                     `yaml:"client_id"`
		SessionID  string                     `yaml:"session_id"`
		PrivateKey string                     `yaml:"private_key"`
		PinToken   string                     `yaml:"pin_token"`
		PIN        string                     `yaml:"pin"`
		List       []Lottery                  `yaml:"list"`
		Rate       map[string]decimal.Decimal `yaml:"rate"`
	} `yaml:"lottery"`

	Qiniu struct {
		AccessKey string `yaml:"access_key"`
		SecretKey string `yaml:"secret_key"`
		Bucket    string `yaml:"bucket"`
		Region    string `yaml:"region"`
	} `yaml:"qiniu,omitempty"`

	RedisAddr string `yaml:"redis_addr"`

	ClientList     []string `yaml:"client_list"`
	ShowClientList []string `yaml:"show_client_list"`
	LuckCoinAppID  string   `yaml:"luck_coin_app_id"`

	FoxToken     string `yaml:"fox_token"`
	ExinToken    string `yaml:"exin_token"`
	ExinLocalKey string `yaml:"exin_local_key"`
}

type text struct {
	Desc            string
	Join            string
	Home            string
	News            string
	Transfer        string
	Activity        string
	Auth            string
	Forward         string
	Mute            string
	Block           string
	JoinMsg         string
	AuthSuccess     string
	PrefixLeaveMsg  string
	LeaveGroup      string
	OpenChatStatus  string
	CloseChatStatus string
	MuteOpen        string
	MuteClose       string
	Muting          string
	VideoLiving     string
	VideoLiveEnd    string
	Living          string
	LiveEnd         string
	WelcomeUpdate   string
	StopMessage     string
	StopClose       string
	StopBroadcast   string
	StickerWarning  string
	StatusSet       string
	StatusCancel    string
	StatusAdmin     string
	StatusGuest     string
	Reward          string
	From            string
	MemberCentre    string
	PayForFresh     string
	PayForLarge     string
	AuthForFresh    string
	AuthForLarge    string
	LimitReject     string
	MutedReject     string
	URLReject       string
	QrcodeReject    string
	URLAdmin        string
	LanguageReject  string
	LanguageAdmin   string
	BalanceReject   string
	CategoryReject  string
	Forbid          string
	BotCard         string
	MemberTips      string
	JoinMsgInfo     string
	PINMessageErorr string
	Category        map[string]string
}

var Config config

var Text text

func init() {
	data, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Println("config.json open fail...", err)
		return
	}

	err = yaml.Unmarshal(data, &Config)
	if err != nil {
		log.Println("config.json parse err...", err)
	}
	if Config.Lang == "zh" {
		Text = zh_CN_Text
	} else if Config.Lang == "en" {
		Text = en_Text
	}

	log.Println("config.json load success...")
}
