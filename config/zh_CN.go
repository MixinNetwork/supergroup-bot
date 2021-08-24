package config

var zh_CN_Text = text{
	Desc:                "项目介绍",
	Join:                "加入社群",
	Home:                "社群主页",
	News:                "资讯",
	Transfer:            "交易",
	Activity:            "活动",
	Auth:                "授权检查",
	Forward:             "帮转",
	Mute:                "禁言",
	Block:               "拉黑",
	JoinMsg:             "{name} 加入了群组",
	AuthSuccess:         "授权成功",
	PrefixLeaveMsg:      "【留言】",
	LeaveGroup:          "您已退出社群。",
	OpenChatStatus:      "您已恢复群聊消息接收。",
	CloseChatStatus:     "您已停止接受群聊消息，但仍会收到公告信息。",
	MuteOpen:            "社群已禁言",
	MuteClose:           "社群禁言已解除",
	Muting:              "禁言中",
	VideoLiving:         "视频直播开始了。",
	VideoLiveEnd:        "视频直播结束了。",
	Living:              "图文直播开始，社群已禁言。",
	LiveEnd:             "图文直播已结束，社群禁言已解除。",
	WelcomeUpdate:       "入群欢迎语已更新",
	StopMessage:         "由于您长时间未接收本群消息，为了避免您下次访问时需要同步成千上万条消息，我们暂停了给你发消息。",
	StopClose:           "开始接收消息",
	StopBroadcast:       "查看近期公告",
	StickerWarning:      "【提醒】请不要连续发表情刷屏，继续刷屏可能会被禁言甚至拉黑。",
	StatusSet:           "{full_name}({identity_number}) 已成为本社群{status}",
	StatusCancel:        "{full_name}({identity_number}) 已取消{status}资格",
	StatusAdmin:         "管理员",
	StatusGuest:         "嘉宾",
	Reward:              "{send_name} 给 {reward_name} 打赏了 {amount} {symbol}",
	From:                "来自 {identity_number}",
	MemberCentre:        "会员中心",
	PayForFresh:         "🎉恭喜付费获得 1 年期初级会员！会员资格需要到期才能续费。\n\n你可以发文字、贴纸和红包类型的消息，每分钟 10 条消息。发广告、私信骚扰群友、引战、挑事会被禁言甚至拉黑。",
	PayForLarge:         "🎉恭喜付费获得 1 年期资深会员！会员资格需要到期才能续费。\n\n你可以发文字、贴纸、红包、图片、视频类型的消息，每分钟 20 条消息。发广告、私信骚扰群友、引战、挑事会被禁言甚至拉黑。",
	AuthForFresh:        "🎉恭喜你免费获得初级会员的资格！注意社群会定期访问并检查您的资产是否满足持仓要求，请放心我们不会存储您的资产信息更不会用于其他用途。\n\n你可以发文字、贴纸和红包类型的消息，每分钟 10 条消息。发广告、私信骚扰群友、引战、挑事会被禁言甚至拉黑。",
	AuthForLarge:        "🎉恭喜你免费获得资深会员的资格！注意社群会定期访问并检查您的资产是否满足持仓要求，请放心我们不会存储您的资产信息更不会用于其他用途。\n\n你可以发文字、贴纸、红包、图片、视频类型的消息，每分钟 20 条消息。发广告、私信骚扰群友、引战、挑事会被禁言甚至拉黑。",
	LimitReject:         "【提醒】发言次数超过限额！你最近 1 分钟已发送 {limit} 条消息，请稍后再发，继续发言刷屏可能会被禁言甚至拉黑。\n\n「小提示」持仓越高每分钟发言次数越多，消息类型越丰富。",
	MutedReject:         "⚠️警告️⚠️ 你被禁言了 {muted_time} 小时，还剩 {hours} 小时 {minutes} 分钟，继续发言可能会被禁言更长时间甚至拉黑。",
	URLReject:           "【提醒】你没有发链接的权限！非广告可找管理员帮忙转发，继续发带广告的链接可能会被禁言甚至拉黑。",
	QrcodeReject:        "【提醒】你没有发链接的权限！非广告可找管理员帮忙转发，继续发带广告的链接可能会被禁言甚至拉黑。",
	URLAdmin:            "【操作提醒】检测到有人发链接！",
	LanguageReject:      "⚠️ 警告⚠️ 这里是中文社群请讲中文，继续发发其他语言的消息可能会被禁言甚至拉黑。",
	LanguageAdmin:       "【操作提醒】检测到有人发其他语言的消息！",
	BalanceReject:       "本社群已开启会员发言，想发言参与讨论请到会员中心付费购买或授权免费领取会员资格。\n\n非会员可以接收所有聊天消息但是不能参与聊天，你所有的发言只有管理员能看到，可以给管理员留言咨询问题。",
	CategoryReject:      "【提醒】你没有发{category}的权限！继续发{category}可能会被禁言甚至拉黑。",
	CategoryRejectTips:  "\n\n「小提示」持仓越高每分钟发言次数越多，消息类型越丰富。",
	NotOpenSpeakJoinMsg: "你可以发文字、贴纸和红包类型的消息，每分钟 5 条消息。发广告、私信骚扰群友、引战、挑事会被禁言甚至拉黑。",
	Forbid:              "【提醒】本社群禁止发{category}消息！",
	BotCard:             "机器人卡片",
	Category: map[string]string{
		"PLAIN_TEXT":       "文字",
		"PLAIN_POST":       "文章",
		"PLAIN_IMAGE":      "图片",
		"PLAIN_STICKER":    "表情",
		"PLAIN_LIVE":       "直播",
		"PLAIN_VIDEO":      "视频",
		"APP_CARD":         "卡片",
		"PLAIN_LOCATION":   "位置",
		"PLAIN_DATA":       "文件",
		"PLAIN_CONTACT":    "联系人卡片",
		"PLAIN_AUDIO":      "音频",
		"PLAIN_TRANSCRIPT": "聊天记录",
	},
}
