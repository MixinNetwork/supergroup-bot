package models

import (
	"context"
	"fmt"
	"time"

	"github.com/MixinNetwork/supergroup/session"
	"github.com/fox-one/mixin-sdk-go"
)

const client_member_auth_DDL = `
CREATE TABLE IF NOT EXISTS client_member_auth (
	client_id varchar(36) NOT NULL,
	user_status SMALLINT NOT NULL,
	plain_text bool NOT NULL,
	lucky_coin bool NOT NULL,
	plain_sticker bool NOT NULL,
	plain_image bool NOT NULL,
	plain_video bool NOT NULL,
	plain_post bool NOT NULL,
	plain_data bool NOT NULL,
	plain_live bool NOT NULL,
	plain_contact bool NOT NULL,
	plain_transcript bool NOT NULL,
	app_card bool NOT NULL DEFAULT false,
	url bool NOT NULL,
	updated_at timestamp NOT NULL DEFAULT now(),
	PRIMARY KEY (client_id, user_status)
);
alter table client_member_auth add if not exists app_card bool DEFAULT false;
`

type ClientMemberAuth struct {
	ClientID        string    `json:"client_id"`
	UserStatus      int       `json:"user_status"`
	PlainText       bool      `json:"plain_text"`
	PlainSticker    bool      `json:"plain_sticker"`
	PlainImage      bool      `json:"plain_image"`
	PlainVideo      bool      `json:"plain_video"`
	PlainPost       bool      `json:"plain_post"`
	PlainData       bool      `json:"plain_data"`
	PlainLive       bool      `json:"plain_live"`
	PlainContact    bool      `json:"plain_contact"`
	PlainTranscript bool      `json:"plain_transcript"`
	AppCard         bool      `json:"app_card"`
	URL             bool      `json:"url"`
	LuckyCoin       bool      `json:"lucky_coin"`
	UpdatedAt       time.Time `json:"updated_at"`

	Limit int `json:"limit,omitempty"`
}

func initClientMemberAuth(ctx context.Context) {
	cs, err := getAllClient(ctx)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}

	for _, clientID := range cs {
		session.Database(ctx).Exec(ctx, `INSERT INTO client_member_auth(client_id,user_status,plain_text,plain_sticker,lucky_coin,plain_image,plain_video,plain_post,plain_data,plain_live,plain_contact,plain_transcript,url,app_card) VALUES($1, 1, true, true, true, false, false, false, false, false, false, false, false, false) ON CONFLICT (client_id, user_status) DO NOTHING;`, clientID)
		session.Database(ctx).Exec(ctx, `INSERT INTO client_member_auth(client_id,user_status,plain_text,plain_sticker,lucky_coin,plain_image,plain_video,plain_post,plain_data,plain_live,plain_contact,plain_transcript,url,app_card) VALUES($1, 2, true, true, true, true, false, false, false, false, false, false, false, false) ON CONFLICT (client_id, user_status) DO NOTHING;`, clientID)
		session.Database(ctx).Exec(ctx, `INSERT INTO client_member_auth(client_id,user_status,plain_text,plain_sticker,lucky_coin,plain_image,plain_video,plain_post,plain_data,plain_live,plain_contact,plain_transcript,url,app_card) VALUES($1, 5, true, true, true, true, true, true, true, true, true, true, false, false) ON CONFLICT (client_id, user_status) DO NOTHING;`, clientID)
	}
}

func GetClientMemberAuth(ctx context.Context, u *ClientUser) (map[int]ClientMemberAuth, error) {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return nil, session.ForbiddenError(ctx)
	}
	cmas := make(map[int]ClientMemberAuth)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT client_id,user_status,plain_text,plain_sticker,lucky_coin,plain_image,plain_video,app_card,
plain_post,plain_data,plain_live,plain_contact,plain_transcript,url,updated_at
FROM client_member_auth
WHERE client_id=$1
`, u.ClientID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var cma ClientMemberAuth
		if err := rows.Scan(&cma.ClientID, &cma.UserStatus, &cma.PlainText, &cma.PlainSticker,
			&cma.LuckyCoin, &cma.PlainImage, &cma.PlainVideo, &cma.AppCard, &cma.PlainPost, &cma.PlainData,
			&cma.PlainLive, &cma.PlainContact, &cma.PlainTranscript, &cma.URL, &cma.UpdatedAt); err != nil {
			return nil, err
		}
		cma.Limit = statusLimitMap[cma.UserStatus]
		cmas[cma.UserStatus] = cma
	}
	return cmas, nil
}

func UpdateClientMemberAuth(ctx context.Context, u *ClientUser, auth ClientMemberAuth) error {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return session.ForbiddenError(ctx)
	}
	if !checkUserStatusIsValid(auth.UserStatus) {
		return session.BadDataError(ctx)
	}

	query := `
UPDATE client_member_auth SET 
plain_text=$3, plain_sticker=$4, lucky_coin=$5, plain_image=$6, plain_video=$7, plain_post=$8,
plain_data=$9, plain_live=$10, plain_contact=$11, plain_transcript=$12, url=$13, app_card=$14, updated_at=now()
WHERE client_id=$1 AND user_status=$2
`
	_, err := session.Database(ctx).Exec(ctx, query, u.ClientID, auth.UserStatus,
		true, auth.PlainSticker, auth.LuckyCoin, auth.PlainImage, auth.PlainVideo, auth.PlainPost,
		auth.PlainData, auth.PlainLive, auth.PlainContact, auth.PlainTranscript, auth.URL, auth.AppCard)
	return err
}

func checkHasClientMemberAuth(ctx context.Context, clientID, category string, userStatus int) bool {
	if category == mixin.MessageCategoryPlainText {
		return true
	}
	if !checkCategoryIsValid(category) {
		session.Logger(ctx).Println(category)
		return false
	}
	if userStatus > 5 {
		userStatus = 5
	}
	var hasAuth bool
	query := fmt.Sprintf(`SELECT %s FROM client_member_auth WHERE client_id=$1 AND user_status=$2`, category)
	if err := session.Database(ctx).QueryRow(ctx, query, clientID, userStatus).Scan(&hasAuth); err != nil {
		session.Logger(ctx).Println(err)
		return false
	}
	return hasAuth
}

func checkUserStatusIsValid(userStatus int) bool {
	return userStatus == ClientUserStatusFresh ||
		userStatus == ClientUserStatusAudience ||
		userStatus == ClientUserStatusLarge
}

var defaultAuth = map[string]bool{
	mixin.MessageCategoryPlainText:    true,
	mixin.MessageCategoryPlainSticker: true,
	mixin.MessageCategoryPlainImage:   true,
	mixin.MessageCategoryPlainVideo:   true,
	mixin.MessageCategoryPlainPost:    true,
	mixin.MessageCategoryPlainData:    true,
	mixin.MessageCategoryPlainLive:    true,
	mixin.MessageCategoryPlainContact: true,
	mixin.MessageCategoryAppCard:      true,
	"PLAIN_TRANSCRIPT":                true,
	"lucky_coin":                      true,
	"url":                             true,
}

func checkCategoryIsValid(key string) bool {
	return defaultAuth[key]
}
