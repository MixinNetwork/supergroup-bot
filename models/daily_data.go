package models

import (
	"context"
	"fmt"
	"time"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/robfig/cron/v3"
)

const daily_data_DDL = `
CREATE TABLE IF NOT EXISTS daily_data (
  client_id     VARCHAR(36) NOT NULL,
  date          DATE NOT NULL,
  users         INTEGER NOT NULL DEFAULT 0,
  active_users  INTEGER NOT NULL DEFAULT 0,
  messages      INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(client_id, date)
);
`

type DailyData struct {
	ClientID    string `json:"client_id,omitempty"`
	Date        string `json:"date,omitempty"`
	Users       int    `json:"users"`
	ActiveUsers int    `json:"active_users"`
	Messages    int    `json:"messages"`
}

func updateDailyData(ctx context.Context, d *DailyData) error {
	query := durable.InsertQueryOrUpdate("daily_data", "client_id,date", "users,messages,active_users")
	_, err := session.Database(ctx).Exec(ctx, query, d.ClientID, d.Date, d.Users, d.Messages, d.ActiveUsers)
	return err
}

func ScriptToUpdateDailyData(ctx context.Context) error {
	// 统计每一个大群的每天用户数和每天的消息数量
	clients, err := getAllClient(ctx)
	if err != nil {
		return err
	}
	for _, client := range clients {
		if err := initGroupDailyData(ctx, client); err != nil {
			session.Logger(ctx).Println(err)
		}
	}

	return nil
}

func initGroupDailyData(ctx context.Context, clientID string) error {
	// 1. 获取第一个用户加入的时间
	var startAt time.Time
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT created_at FROM client_users
WHERE client_id=$1
ORDER BY created_at
LIMIT 1
`, clientID).Scan(&startAt); err != nil {
		return err
	}
	startAt, _ = time.Parse("2006-1-2", startAt.Format("2006-1-2"))
	if startAt.IsZero() {
		session.Logger(ctx).Println(startAt)
		return nil
	}
	// 2. 开始循环， 直到今天为止
	for {
		if startAt.Add(time.Hour * 24).After(time.Now()) {
			break
		}
		d, err := statisticsGroupDailyData(ctx, clientID, startAt)
		if err != nil {
			return err
		}
		if err := updateDailyData(ctx, d); err != nil {
			return err
		}
		startAt = startAt.Add(time.Hour * 24)
	}

	return nil
}

func statisticsGroupDailyData(ctx context.Context, clientID string, startAt time.Time) (*DailyData, error) {
	endAt := startAt.Add(time.Hour * 24)
	var users, messages, activeUsers int
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM client_users 
WHERE client_id=$1 AND created_at>$2 AND created_at<$3`, clientID, startAt, endAt).Scan(&users); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM messages
WHERE client_id=$1 AND created_at>$2 AND created_at<$3`, clientID, startAt, endAt).Scan(&messages); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, fmt.Sprintf(`
SELECT count(1) FROM client_users 
WHERE client_id=$1 
AND $2-deliver_at<interval '%f %s'
AND created_at<$2
`, config.NotActiveCheckTime, "hours"), clientID, endAt).Scan(&activeUsers); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}

	return &DailyData{
		ClientID:    clientID,
		Users:       users,
		Messages:    messages,
		ActiveUsers: activeUsers,
		Date:        startAt.Format("2006-1-2"),
	}, nil
}

type DailyDataResp struct {
	List     []*DailyData `json:"list"`
	Today    *DailyData   `json:"today"`
	HighUser int          `json:"high_user"`
}

func GetDailyDataByClientID(ctx context.Context, u *ClientUser) (*DailyDataResp, error) {
	if !checkIsAdmin(ctx, u.ClientID, u.UserID) {
		return nil, session.ForbiddenError(ctx)
	}
	res := DailyDataResp{
		List:     make([]*DailyData, 0),
		Today:    nil,
		HighUser: 0,
	}
	rows, err := session.Database(ctx).Query(ctx, ` SELECT to_char(date, 'YYYY-MM-DD') date, users, messages, active_users FROM daily_data WHERE client_id=$1 ORDER BY date `, u.ClientID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var d DailyData
		if err := rows.Scan(&d.Date, &d.Users, &d.Messages, &d.ActiveUsers); err != nil {
			return nil, err
		}
		res.List = append(res.List, &d)
	}
	var d DailyData
	startAt, _ := time.Parse("2006-1-2", time.Now().Format("2006-1-2"))

	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM client_users 
WHERE client_id=$1 AND created_at>$2`, u.ClientID, startAt).Scan(&d.Users); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM messages
WHERE client_id=$1 AND created_at>$2`, u.ClientID, startAt).Scan(&d.Messages); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM client_users 
WHERE client_id=$1 AND created_at>$2`, u.ClientID, startAt).Scan(&d.Users); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, `
SELECT count(1) FROM client_users 
WHERE client_id=$1 AND priority=1`, u.ClientID).Scan(&res.HighUser); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}
	if err := session.Database(ctx).QueryRow(ctx, fmt.Sprintf(`
SELECT count(1) FROM client_users 
WHERE client_id=$1 
AND $2-deliver_at<interval '%f %s'
`, config.NotActiveCheckTime, "hours"), u.ClientID, startAt).Scan(&d.ActiveUsers); err != nil {
		session.Logger(ctx).Println(err)
		return nil, err
	}

	d.Date = time.Now().Format("2006-01-02")
	res.Today = &d
	return &res, nil
}

func StartDailyDataJob() {
	c := cron.New(cron.WithLocation(time.UTC))
	_, err := c.AddFunc("16 0 * * *", func() {
		cs, err := getAllClient(_ctx)
		if err != nil {
			session.Logger(_ctx).Println(err)
			return
		}
		now, _ := time.Parse("2006-1-2", time.Now().Format("2006-1-2"))
		yesterday := now.Add(-24 * time.Hour)
		for _, clientID := range cs {
			dd, err := statisticsGroupDailyData(_ctx, clientID, yesterday)
			if err != nil {
				session.Logger(_ctx).Println(err)
				continue
			}
			if err := updateDailyData(_ctx, dd); err != nil {
				session.Logger(_ctx).Println(err)
			}
		}
	})
	if err != nil {
		session.Logger(_ctx).Println(err)
		SendMsgToDeveloper(_ctx, "", "定时任务。。。出问题了。。。")
		return
	}
	c.Start()
}
