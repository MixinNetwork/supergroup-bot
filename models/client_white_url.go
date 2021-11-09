package models

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
)

const client_white_url_DDL = `
CREATE TABLE IF NOT EXISTS client_white_url (
	client_id  VARCHAR(36) NOT NULL,
	white_url  VARCHAR DEFAULT '',
	created_at timestamp with time zone default now(),
	PRIMARY KEY(client_id,white_url)
);
`

type ClientWhiteURL struct {
	ClientID  string    `json:"client_id,omitempty"`
	WhiteURL  string    `json:"white_url,omitempty"`
	CreatedAt time.Time `json:"created_atomitempty"`
}

func UpdateClientWhiteURL(ctx context.Context, clientID, whiteURL string) error {
	query := durable.InsertQueryOrUpdate("client_white_url", "client_id,white_url", "")
	_, err := session.Database(ctx).Exec(ctx, query, clientID, whiteURL)
	return err
}

func CheckUrlIsWhiteURL(ctx context.Context, clientID, targetURL string) bool {
	ws, err := GetClientWhiteURLByClientID(ctx, clientID)
	if err != nil {
		session.Logger(ctx).Println(err)
		return false
	}
	if strings.HasPrefix(targetURL, "http") {
		targetURLObj, err := url.Parse(targetURL)
		if err != nil {
			return false
		}
		for _, w := range ws {
			if targetURLObj.Host == w.WhiteURL {
				return true
			}
		}
	} else {
		for _, w := range ws {
			if strings.HasPrefix(targetURL, w.WhiteURL) {
				return true
			}
		}
	}
	return false
}

func GetClientWhiteURLByClientID(ctx context.Context, clientID string) ([]*ClientWhiteURL, error) {
	var result []*ClientWhiteURL
	rows, err := session.Database(ctx).Query(ctx, ` SELECT white_url FROM client_white_url WHERE client_id = $1 `, clientID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var item ClientWhiteURL
		err := rows.Scan(&item.WhiteURL)
		if err != nil {
			return nil, err
		}
		result = append(result, &item)
	}
	return result, err
}

func DailyUpdateClientWhiteURL() {
	for {
		cs, err := getAllClient(_ctx)
		if err != nil {
			session.Logger(_ctx).Println(err)
			return
		}

		for _, c := range cs {
			me, err := GetMixinClientByID(_ctx, c).UserMe(_ctx)
			if err != nil {
				session.Logger(_ctx).Println(err)
				continue
			}
			for _, u := range me.App.ResourcePatterns {
				whiteURL, err := url.Parse(u)
				if err != nil {
					session.Logger(_ctx).Println(err)
					continue
				}
				err = UpdateClientWhiteURL(_ctx, c, whiteURL.Host)
				if err != nil {
					session.Logger(_ctx).Println(err)
				}
			}
		}
		time.Sleep(time.Hour * 24)
	}
}
