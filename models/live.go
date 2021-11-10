package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/MixinNetwork/supergroup/tools"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/storage"
)

const lives_DDL = `
-- 直播
CREATE TABLE IF NOT EXISTS lives (
    live_id             VARCHAR(36) NOT NULL PRIMARY KEY,
    client_id           VARCHAR(36) NOT NULL,
    img_url             VARCHAR(512) DEFAULT '',
    category            SMALLINT DEFAULT 1, -- 视频直播 图片+语音直播
    title               VARCHAR NOT NULL,
    description         VARCHAR NOT NULL,
    status              SMALLINT DEFAULT 1, -- 1 直播 2 回放
    top_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT '1970-1-1',
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
`

const live_data_DDL = `
-- 直播数据
CREATE TABLE IF NOT EXISTS live_data (
    live_id             VARCHAR(36) NOT NULL PRIMARY KEY,
    read_count          INTEGER DEFAULT 0, -- 观看用户
    deliver_count       INTEGER DEFAULT 0, -- 广播用户
    msg_count           INTEGER DEFAULT 0, -- 消息发言数量
    user_count          INTEGER DEFAULT 0, -- 发言人数
    start_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    end_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
`

const live_replay_DDL = `
CREATE TABLE IF NOT EXISTS live_replay (
    message_id          VARCHAR(36) NOT NULL PRIMARY KEY,
    client_id           VARCHAR(36) NOT NULL,
    live_id             VARCHAR(36) NOT NULL DEFAULT '',
    category            VARCHAR NOT NULL,
    data                VARCHAR NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
`

const live_play_DDL = `
CREATE TABLE IF NOT EXISTS live_play (
    live_id             VARCHAR(36) NOT NULL,
    user_id             VARCHAR NOT NULL,
    addr                VARCHAR NOT NULL DEFAULT '',
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
`

type Live struct {
	LiveID      string    `json:"live_id,omitempty"`
	ClientID    string    `json:"client_id,omitempty"`
	ImgURL      string    `json:"img_url,omitempty"`
	Category    int       `json:"category,omitempty"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Status      int       `json:"status"`
	TopAt       time.Time `json:"top_at,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}
type LiveData struct {
	LiveID       string    `json:"live_id,omitempty"`
	ReadCount    int       `json:"read_count"`
	DeliverCount int       `json:"deliver_count"`
	MsgCount     int       `json:"msg_count"`
	UserCount    int       `json:"user_count"`
	StartAt      time.Time `json:"start_at,omitempty"`
	EndAt        time.Time `json:"end_at,omitempty"`
}
type LiveReplay struct {
	MessageID string    `json:"message_id,omitempty"`
	ClientID  string    `json:"client_id,omitempty"`
	LiveID    string    `json:"live_id,omitempty"`
	Category  string    `json:"category,omitempty"`
	Data      string    `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}
type LivePlay struct {
	LiveID    string    `json:"live_id,omitempty"`
	UserID    string    `json:"user_id,omitempty"`
	Addr      string    `json:"addr,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

const (
	LiveStatusBefore   = 0
	LiveStatusLiving   = 1
	LiveStatusFinished = 2

	LiveCategoryVideo         = 1
	LiveCategoryAudioAndImage = 2
)

func UpdateLive(ctx context.Context, u *ClientUser, l *Live) error {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	if l.LiveID == "" {
		l.LiveID = tools.GetUUID()
	}
	query := durable.InsertQueryOrUpdate("lives", "live_id", "client_id,img_url,category,title,description,status")
	_, err := session.Database(ctx).Exec(ctx, query, l.LiveID, u.ClientID, l.ImgURL, l.Category, l.Title, l.Description, LiveStatusBefore)
	return err
}

func GetLiveByID(ctx context.Context, liveID string) (*Live, error) {
	var l Live
	err := session.Database(ctx).QueryRow(ctx, `
SELECT live_id,client_id,img_url,category,title,description,status,top_at
FROM lives
WHERE live_id=$1
`, liveID).Scan(&l.LiveID, &l.ClientID, &l.ImgURL, &l.Category, &l.Title, &l.Description, &l.Status, &l.TopAt)
	return &l, err
}

func GetLivesByClientID(ctx context.Context, u *ClientUser) ([]*Live, error) {
	ls := make([]*Live, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT live_id,client_id,img_url,category,title,description,status,created_at,top_at
FROM lives 
WHERE client_id=$1 ORDER BY created_at DESC
`, u.ClientID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var l Live
		if err := rows.Scan(&l.LiveID, &l.ClientID, &l.ImgURL, &l.Category, &l.Title, &l.Description, &l.Status, &l.CreatedAt, &l.TopAt); err != nil {
			return nil, err
		}
		ls = append(ls, &l)
	}
	return ls, err
}

func StartLive(ctx context.Context, u *ClientUser, liveID, url string) error {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	l, err := GetLiveByID(ctx, liveID)
	if err != nil {
		return err
	}
	if l.Category == LiveCategoryAudioAndImage {
		if err := setClientConversationStatusByIDAndStatus(ctx, l.ClientID, ClientConversationStatusAudioLive); err != nil {
			return err
		}
		DeleteDistributeMsgByClientID(_ctx, u.ClientID)
		go SendClientTextMsg(l.ClientID, config.Text.Living, "", false)

	} else {
		go sendLiveCard(_ctx, u.ClientID, url, liveID)
	}
	return startLive(ctx, l)
}

func StopLive(ctx context.Context, u *ClientUser, liveID string) error {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	l, err := GetLiveByID(ctx, liveID)
	if err != nil {
		return err
	}
	if l.Category == LiveCategoryAudioAndImage {
		go SendClientTextMsg(l.ClientID, config.Text.LiveEnd, "", false)
		if err := setClientConversationStatusByIDAndStatus(ctx, l.ClientID, ClientConversationStatusNormal); err != nil {
			return err
		}
	} else {
		go sendLiveCard(_ctx, u.ClientID, "", liveID)
	}
	return stopLive(ctx, l)
}

func StatLive(ctx context.Context, u *ClientUser, liveID string) (*LiveData, error) {
	var l LiveData
	err := session.Database(ctx).QueryRow(ctx, `
SELECT live_id,read_count,deliver_count,msg_count,user_count,start_at,end_at
FROM live_data WHERE live_id=$1
`, liveID).Scan(&l.LiveID, &l.ReadCount, &l.DeliverCount, &l.MsgCount, &l.UserCount, &l.StartAt, &l.EndAt)
	return &l, err
}

// 视频直播开始
func startLive(ctx context.Context, l *Live) error {
	// 直接开始
	query := durable.InsertQueryOrUpdate("live_data", "live_id", "start_at")
	if _, err := session.Database(ctx).Exec(ctx, query, l.LiveID, time.Now()); err != nil {
		return err
	}
	return updateLiveStatusByID(ctx, l.LiveID, LiveStatusLiving)
}

// 视频直播结束
func stopLive(ctx context.Context, l *Live) error {
	// 统计观看用户。 广播用户。 直播时长。 发言人数。 发言数量
	var startAt time.Time
	if err := session.Database(ctx).QueryRow(ctx, `SELECT start_at FROM live_data WHERE live_id=$1`, l.LiveID).Scan(&startAt); err != nil {
		return err
	}
	endAt := time.Now()
	if err := handleStatistics(ctx, l, startAt, endAt); err != nil {
		return err
	}
	if l.Category == LiveCategoryAudioAndImage {
		session.Database(ctx).Exec(ctx, `UPDATE live_replay SET live_id=$3 WHERE created_at>$1 AND created_at<$2`, startAt, endAt, l.LiveID)
	}
	return updateLiveStatusByID(ctx, l.LiveID, LiveStatusFinished)
}

func updateLiveStatusByID(ctx context.Context, liveID string, status int) error {
	_, err := session.Database(ctx).Exec(ctx, `UPDATE lives SET status=$2 WHERE live_id=$1`, liveID, status)
	return err
}

func handleStatistics(ctx context.Context, l *Live, startAt, endAt time.Time) error {
	var readCount, deliverCount, msgCount, userCount int
	if err := session.Database(ctx).QueryRow(ctx, `SELECT COUNT(1) FROM client_users WHERE client_id=$1 AND read_at > $2`, l.ClientID, startAt).Scan(&readCount); err != nil {
		return err
	}
	if err := session.Database(ctx).QueryRow(ctx, `SELECT COUNT(1) FROM client_users WHERE client_id=$1 AND deliver_at > $2`, l.ClientID, startAt).Scan(&deliverCount); err != nil {
		return err
	}
	if err := session.Database(ctx).QueryRow(ctx, `SELECT COUNT(1) FROM messages WHERE client_id=$1 AND created_at > $2 AND created_at < $3`, l.ClientID, startAt, endAt).Scan(&msgCount); err != nil {
		return err
	}
	if err := session.Database(ctx).QueryRow(ctx, `SELECT COUNT(distinct(user_id)) FROM messages WHERE client_id=$1 AND created_at > $2 AND created_at < $3`, l.ClientID, startAt, endAt).Scan(&userCount); err != nil {
		return err
	}
	_, err := session.Database(ctx).Exec(ctx, `
UPDATE live_data SET (read_count,deliver_count,msg_count,user_count,end_at)=($2,$3,$4,$5,$6) WHERE live_id=$1
`, l.LiveID, readCount, deliverCount, msgCount, userCount, endAt)
	return err
}

// 处理图文直播的聊天记录 并存入 replay 表中
func HandleAudioReplay(clientID string, msg *mixin.MessageView) {
	var id, mimeType string
	switch msg.Category {
	case mixin.MessageCategoryPlainText:
		msg.Data = string(tools.Base64Decode(msg.Data))
	case mixin.MessageCategoryPlainImage:
		var img mixin.ImageMessage
		if err := json.Unmarshal(tools.Base64Decode(msg.Data), &img); err != nil {
			session.Logger(_ctx).Println(err)
		}
		id = img.AttachmentID
		mimeType = img.MimeType
	case mixin.MessageCategoryPlainAudio:
		var audio mixin.AudioMessage
		if err := json.Unmarshal(tools.Base64Decode(msg.Data), &audio); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		id = audio.AttachmentID
		mimeType = audio.MimeType
		b, err := getBlobFromAttachmentID(id)
		if err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		fromName := fmt.Sprintf("%s.ogg", msg.MessageID)
		toName := fmt.Sprintf("%s.mp3", msg.MessageID)
		t, err := os.Create(fromName)
		if err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		if _, err := t.Write(b); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		cmd := exec.Command("ffmpeg", "-i", fromName, "-acodec", "libmp3lame", "-ac", "2", "-ab", "160", toName)
		if err := cmd.Start(); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		if err := cmd.Wait(); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}

		t.Close()
		if err := os.Remove(fromName); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		if err := UploadFileToQiniu(toName, "live-replay/"+msg.MessageID); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		if err := os.Remove(toName); err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		msg.Data = msg.MessageID
	case mixin.MessageCategoryPlainVideo:
		var video mixin.VideoMessage
		if err := json.Unmarshal(tools.Base64Decode(msg.Data), &video); err != nil {
			session.Logger(_ctx).Println(err)
		}
		id = video.AttachmentID
		mimeType = video.MimeType
	}
	if id != "" && mimeType != "" && msg.Category != mixin.MessageCategoryPlainAudio {
		b, err := getBlobFromAttachmentID(id)
		if err != nil {
			session.Logger(_ctx).Println(err)
		}
		err = UploadToQiniu(b, mimeType, "live-replay/"+msg.MessageID)
		if err != nil {
			session.Logger(_ctx).Println(err)
		}
		msg.Data = msg.MessageID
	}

	if err := createLiveReplay(_ctx, &LiveReplay{
		MessageID: msg.MessageID,
		ClientID:  clientID,
		Category:  msg.Category,
		Data:      msg.Data,
		CreatedAt: msg.CreatedAt,
	}); err != nil {
		session.Logger(_ctx).Println(err)
	}
}

func createLiveReplay(ctx context.Context, r *LiveReplay) error {
	query := durable.InsertQueryOrUpdate("live_replay", "message_id", "client_id,data,category,created_at,live_id")
	_, err := session.Database(ctx).Exec(ctx, query, r.MessageID, r.ClientID, r.Data, r.Category, r.CreatedAt, r.LiveID)
	return err
}

func UploadLiveImgToMixinStatistics(ctx context.Context, u *ClientUser, r *http.Request) (string, error) {
	if err := r.ParseMultipartForm(10 * 1024); err != nil {
		return "", err
	}
	image := r.MultipartForm.File["file"]
	file, err := image[0].Open()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return "", err
		//return
	}
	a, err := GetMixinClientByID(ctx, u.ClientID).CreateAttachment(ctx)
	if err != nil {
		return "", err
	}
	if err := mixin.UploadAttachment(ctx, a, data); err != nil {
		return "", err
	}
	return a.ViewURL, nil
}

func TopNews(ctx context.Context, u *ClientUser, newsID string, isCancel bool) error {
	t := time.Now()
	if isCancel {
		t, _ = time.Parse("2006-1-2", "1970-1-1")
	}
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	if _, err := session.Database(ctx).Exec(ctx, `UPDATE lives SET top_at=$2 WHERE live_id=$1`, newsID, t); err != nil {
		session.Logger(ctx).Println(err)
	}
	if _, err := session.Database(ctx).Exec(ctx, `UPDATE broadcast SET top_at=$2 WHERE message_id=$1`, newsID, t); err != nil {
		session.Logger(ctx).Println(err)
	}
	return nil
}

func getBlobFromAttachmentID(id string) ([]byte, error) {
	a, err := GetMixinClientByID(_ctx, GetFirstClient(_ctx).ClientID).ShowAttachment(_ctx, id)
	if err != nil {
		return nil, err
	}
	return session.Api(_ctx).RawGet(a.ViewURL), nil
}

func UploadFileToQiniu(name string, key string) error {
	formUploader, upToken := getQiniuUploader()
	ret := storage.PutRet{}
	putExtra := storage.PutExtra{}
	return formUploader.PutFile(context.Background(), &ret, upToken, key, name, &putExtra)
}

func UploadToQiniu(data []byte, mimeType, key string) error {
	formUploader, upToken := getQiniuUploader()
	ret := storage.PutRet{}
	putExtra := storage.PutExtra{
		MimeType: mimeType,
	}
	dataLen := int64(len(data))
	return formUploader.Put(context.Background(), &ret, upToken, key, bytes.NewReader(data), dataLen, &putExtra)
}

func getQiniuUploader() (*storage.FormUploader, string) {
	putPolicy := storage.PutPolicy{
		Scope: config.Config.Qiniu.Bucket,
	}
	mac := qbox.NewMac(config.Config.Qiniu.AccessKey, config.Config.Qiniu.SecretKey)
	upToken := putPolicy.UploadToken(mac)
	cfg := storage.Config{}
	// 空间对应的机房
	switch config.Config.Qiniu.Region {
	case "huadong":
		cfg.Zone = &storage.ZoneHuadong
	case "huabei":
		cfg.Zone = &storage.ZoneHuabei
	case "huanan":
		cfg.Zone = &storage.ZoneHuanan
	case "beimei":
		cfg.Zone = &storage.ZoneBeimei
	case "xinjiapo":
		cfg.Zone = &storage.ZoneXinjiapo
	case "fogcneast":
		cfg.Zone = &storage.ZoneFogCnEast1
	}
	// 是否使用https域名
	cfg.UseHTTPS = true
	// 上传是否使用CDN上传加速
	cfg.UseCdnDomains = false
	return storage.NewFormUploader(&cfg), upToken
}

func GetLiveReplayByLiveID(ctx context.Context, u *ClientUser, liveID, addr string) ([]*LiveReplay, error) {
	lrs := make([]*LiveReplay, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT category,data,created_at
FROM live_replay
WHERE live_id=$1
ORDER BY created_at
`, liveID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var lr LiveReplay
		if err := rows.Scan(&lr.Category, &lr.Data, &lr.CreatedAt); err != nil {
			return nil, err
		}
		lrs = append(lrs, &lr)
	}

	if _, err := session.Database(ctx).Exec(ctx, durable.InsertQuery("live_play", "live_id,user_id,addr"), liveID, u.UserID, addr); err != nil {
		session.Logger(ctx).Println(err)
	}
	return lrs, err
}

func sendLiveCard(ctx context.Context, clientID, url, liveID string) error {
	var msg string
	if url == "" {
		msg = config.Text.VideoLiveEnd
		if err := session.Database(ctx).QueryRow(ctx, `SELECT data FROM live_replay WHERE live_id=$1`, liveID).Scan(&url); err != nil {
			session.Logger(ctx).Println(err)
		}
	} else {
		msg = config.Text.VideoLiving
	}
	time.Sleep(2 * time.Minute)
	SendClientTextMsg(clientID, msg, "", false)
	if err := SendMessage(ctx, GetMixinClientByID(ctx, clientID).Client, &mixin.MessageRequest{
		ConversationID: mixin.UniqueConversationID(clientID, "b523c28b-1946-4b98-a131-e1520780e8af"),
		RecipientID:    "b523c28b-1946-4b98-a131-e1520780e8af",
		MessageID:      tools.GetUUID(),
		Category:       mixin.MessageCategoryPlainText,
		Data:           tools.Base64Encode([]byte(url)),
	}, false); err != nil {
		session.Logger(ctx).Println(err)
		return err
	}
	if url != "" {
		if err := createLiveReplay(ctx, &LiveReplay{tools.GetUUID(), clientID, liveID, "", url, time.Now()}); err != nil {
			session.Logger(ctx).Println(err)
			return err
		}
	}
	return nil
}
