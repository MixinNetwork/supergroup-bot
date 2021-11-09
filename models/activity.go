package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/jackc/pgx/v4"
)

type Activity struct {
	ActivityIndex int    `json:"activity_index,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	Status        int    `json:"status,omitempty"`
	ImgURL        string `json:"img_url,omitempty"`
	ExpireImgURL  string `json:"expire_img_url,omitempty"`
	Action        string `json:"action,omitempty"`

	StartAt   time.Time `json:"start_at,omitempty"`
	ExpireAt  time.Time `json:"expire_at,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

var activitiesColumnsFull = []string{"activity_index", "client_id", "status", "img_url", "expire_img_url", "action", "start_at", "expire_at", "created_at"}

func activityFromRow(row durable.Row) (*Activity, error) {
	var a Activity
	err := row.Scan(&a.ActivityIndex, &a.ImgURL, &a.ExpireImgURL, &a.Action, &a.StartAt, &a.ExpireAt, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &a, err
}

func GetActivityByClientID(ctx context.Context, clientID string) ([]*Activity, error) {
	query := fmt.Sprintf("SELECT %s FROM activity WHERE client_id=$1 AND status=2", strings.Join(activitiesColumnsFull, ","))
	rows, err := session.Database(ctx).Query(ctx, query, clientID)
	if err != nil {
		return nil, err
	}

	var as []*Activity
	for rows.Next() {
		a, err := activityFromRow(rows)
		if err != nil {
			return nil, err
		}
		as = append(as, a)
	}
	return nil, err
}

func UpdateActivity(ctx context.Context, a *Activity) error {
	query := durable.InsertQueryOrUpdate("activity", "activity_index", "client_id,status,img_url,expire_img_url,action,start_at,expire_at")
	_, err := session.Database(ctx).Exec(ctx, query, a.ActivityIndex, a.ClientID, a.Status, a.ImgURL, a.ExpireImgURL, a.Action, a.StartAt, a.ExpireAt)
	return err
}
