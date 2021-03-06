package services

import (
	"context"
	"time"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/MixinNetwork/supergroup/models"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/MixinNetwork/supergroup/tools"
)

type AssetsCheckService struct{}

func (service *AssetsCheckService) Run(ctx context.Context) error {
	for {
		now := time.Now()
		if err := startAssetCheck(ctx); err != nil {
			session.Logger(ctx).Println(err)
		}
		tools.PrintTimeDuration("资产检查...", now)
		time.Sleep(config.AssetsCheckTime)
	}
}

func startAssetCheck(ctx context.Context) error {
	// 获取所有的用户
	allClientUser, err := models.GetAllClientNeedAssetsCheckUser(ctx, true)
	if err != nil {
		return err
	}
	// 检查所有的用户是否活跃
	checkUserIsActive(ctx, allClientUser)
	allClientUser, err = models.GetAllClientNeedAssetsCheckUser(ctx, false)
	if err != nil {
		return err
	}
	if len(allClientUser) == 0 {
		return nil
	}
	var allUser []string
	_allUser := make(map[string]bool)
	for _, user := range allClientUser {
		_allUser[user.UserID] = true
	}
	for k := range _allUser {
		allUser = append(allUser, k)
	}
	foxUserAssetMap, _ := models.GetAllUserFoxShares(ctx, allUser)
	exinUserAssetMap, _ := models.GetAllUserExinShares(ctx, allUser)

	for _, user := range allClientUser {
		curStatus, err := models.GetClientUserStatus(ctx, user, foxUserAssetMap[user.UserID], exinUserAssetMap[user.UserID])
		if err != nil {
			session.Logger(ctx).Println(err)
			if err := models.UpdateClientUserPriorityAndStatus(ctx, user.ClientID, user.UserID, models.ClientUserPriorityLow, models.ClientUserStatusAudience); err != nil {
				session.Logger(ctx).Println(err)
			}
			return nil
		}
		priority := models.ClientUserPriorityLow
		if curStatus != models.ClientUserStatusAudience {
			priority = models.ClientUserPriorityHigh
		}
		if err := models.UpdateClientUserPriorityAndStatus(ctx, user.ClientID, user.UserID, priority, curStatus); err != nil {
			session.Logger(ctx).Println(err)
		}
	}
	return nil
}

func checkUserIsActive(ctx context.Context, allClientUser []*models.ClientUser) {
	lms, err := models.GetClientLastMsg(ctx)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	for _, user := range allClientUser {
		if err := models.CheckUserIsActive(ctx, user, lms[user.ClientID]); err != nil {
			session.Logger(ctx).Println(err)
		}
	}
}
