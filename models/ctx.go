package models

import (
	"context"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
)

var _ctx context.Context

func init() {
	_ctx = session.WithDatabase(context.Background(), durable.NewDatabase(context.Background()))
	_ctx = session.WithRedis(_ctx, durable.NewRedis(context.Background()))
	initAllDDL()
}

var initModal = []string{
	airdrop_DDL,
	login_log_DDL,
	client_member_auth_DDL,
	assets_DDL,
	exinOTCAsset_DDL,
	exinLocalAsset_DDL,
	claim_DDL,
	client_asset_level_DDL,
	client_asset_lp_check_DDL,
	client_white_url_DDL,
	broadcast_DDL,
	client_DDL,
	client_block_user_DDL,
	block_user_DDL,
	client_replay_DDL,
	bot_user_DDL,
	daily_data_DDL,
	distribute_messages_DDL,
	guess_DDL,
	guess_record_DDL,
	guess_result_DDL,
	live_play_DDL,
	live_replay_DDL,
	live_data_DDL,
	lives_DDL,
	lottery_record_DDL,
	messages_DDL,
	properties_DDL,
	power_DDL,
	power_record_DDL,
	snapshots_DDL,
	transfer_pendding_DDL,
	swap_DDL,
	users_DDL,
	invitation_DDL,
}

func initAllDDL() {
	for _, v := range initModal {
		if _, err := session.Database(_ctx).Exec(_ctx, v); err != nil {
			session.Logger(_ctx).Println(err)
		}
	}
	initClientMemberAuth(_ctx)
}
