package config

var en_Text = text{
	Desc:            "Project Description",
	Join:            "Join the community",
	Home:            "Community Homepage",
	News:            "Information",
	Transfer:        "Trade",
	Activity:        "Event",
	Auth:            "Authorize to check",
	Forward:         "Forward",
	Mute:            "Mute",
	Block:           "Block",
	JoinMsg:         "{name} join the group",
	AuthSuccess:     "Authorization successful",
	PrefixLeaveMsg:  "【message】",
	LeaveGroup:      "You have quited the group",
	OpenChatStatus:  "You have resumed receiving group chat messages.",
	CloseChatStatus: "You have stopped receiving group chat messages, but you will still receive announcements.",
	MuteOpen:        "The community has been muted",
	MuteClose:       "The community has been unmuted",
	Muting:          "Muting",
	VideoLiving:     "The video livestreaming has started.",
	VideoLiveEnd:    "The video livestreaming has ended.",
	Living:          "The message livestreaming has started, the group has been muted.",
	LiveEnd:         "The message livestreaming has ended, the group has been unmuted.",
	WelcomeUpdate:   "The welcome message has been updated",
	StopMessage:     "Since you have not received messages from this group for a long time, we have suspended sending you messages in order to avoid synchronizing thousands of messages on your next visit.",
	StopClose:       "Start receiving messages",
	StopBroadcast:   "View recent announcements",
	StickerWarning:  "【Reminder】Please do not continuously post stickers, otherwise you will be banned.",
	StatusSet:       "{full_name}({identity_number}) has been group {status}",
	StatusCancel:    "{full_name}({identity_number}) has been disqualified from {status}",
	StatusAdmin:     "Admin",
	StatusGuest:     "Lecturer",
	Reward:          "{send_name} gave {reward_name} {amount} {symbol} as gifts.",
	From:            "From {identity_number}",
	MemberCentre:    "Membership",
	PayForFresh:     "🎉Congratulations, you get 1-year primary membership! Membership needs to be renewed only when it expires.\n\nYou could send texts, stickers, and LuckyCoin to the group. Sending limitation is 10 messages per minute. If you send ads, filthy languages, provocations, cause trouble in the group, or send private messages to harass group members, you will be muted or even blocked from the group.",
	PayForLarge:     "🎉Congratulations, you get 1-year premium membership! Membership needs to be renewed only when it expires.\n\nYou could send texts, stickers, images, videos to the group. Sending limitation is 20 messages per minute. If you send ads, filthy languages, provocations, cause trouble in the group, or send private messages to harass group members, you will be muted or even blocked from the group.",
	AuthForFresh:    "🎉Congratulations, you get 1-year primary membership freely! Please note that the group will check your assets regularly to see if you meet the position requirement.\n\nYou could send texts, stickers, and LuckyCoin to the group. Sending limitation is 10 messages per minute. If you send ads, filthy languages, provocations, cause trouble in the group, or send private messages to harass group members, you will be muted or even blocked from the group.",
	AuthForLarge:    "🎉Congratulations, you get 1-year premium membership freely! Please note that the group will check your assets regularly to see if you meet the position requirement.\n\nYou could send texts, stickers, images, videos to the group. Sending limitation is 20 messages per minute. If you send ads, filthy languages, provocations, cause trouble in the group, or send private messages to harass group members, you will be muted or even blocked from the group.",
	LimitReject:     "【Reminder】Sending times exceeded the limit! You have send {limit} messages in the last 1 minute, please retry later, continue to send messages may be muted or blocked.\n\n「Hint」More balance on the wallet, more types of messages could be sent.",
	MutedReject:     "⚠️Warning⚠️ You're muted for {muted_time} hours, {hours} hours {minutes} minutes left, continue sending messages may be muted for a longer time or even blocked.",
	URLReject:       "【Reminder】You do not have permission to post links! If it's not an ad, please ask group admin to forward it, continue sending Ad links may be muted or even blocked.",
	URLAdmin:        "【Operation reminder】Detected someone is sending links!",
	LanguageReject:  "⚠️ Warning ⚠️ This is the English group; please speak English here, keep speaking other languages may be muted or even blocked.",
	LanguageAdmin:   "【Caution】Detecting someone was sending messages in another language.",
	BalanceReject:   "This group has been switched on membership status. To send messages or participate in the chat, please pay for the membership or authorize assets checking for free membership.\n\nNon-membership participants could receive all messages but cannot participate in the chat. Only the group admins could receive and see your messages, so you could ask them questions by leaving messages to the group.",
	CategoryReject:  "【Reminder】You do not have permission to post {category}! Continue to send {category} may be muted or even blocked.\n\n「Hint」More balance on the wallet, more messages per minute and more types of messages could be sent.",
	Forbid:          "【Reminder】It's not allowed to send {category} messages!",
	BotCard:         "Bot card",
	Category: map[string]string{
		"PLAIN_TEXT":     "Text",
		"PLAIN_POST":     "Article",
		"PLAIN_IMAGE":    "Image",
		"PLAIN_STICKER":  "Sticker",
		"PLAIN_LIVE":     "Livestreaming",
		"PLAIN_VIDEO":    "Video",
		"APP_CARD":       "Card",
		"PLAIN_LOCATION": "Position",
		"PLAIN_DATA":     "Document",
		"PLAIN_CONTACT":  "Contact",
		"PLAIN_AUDIO":    "Audio",
	},
}
