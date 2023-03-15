package service

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"x-ui/config"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/util/common"
	"x-ui/xray"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var bot *tgbotapi.BotAPI
var adminIds []int64
var isRunning bool

type LoginStatus byte

const (
	LoginSuccess LoginStatus = 1
	LoginFail    LoginStatus = 0
)

type Tgbot struct {
	inboundService InboundService
	settingService SettingService
	serverService  ServerService
	lastStatus     *Status
}

func (t *Tgbot) NewTgbot() *Tgbot {
	return new(Tgbot)
}

func (t *Tgbot) Start() error {
	tgBottoken, err := t.settingService.GetTgBotToken()
	if err != nil || tgBottoken == "" {
		logger.Warning("Get TgBotToken failed:", err)
		return err
	}

	tgBotid, err := t.settingService.GetTgBotChatId()
	if err != nil {
		logger.Warning("Get GetTgBotChatId failed:", err)
		return err
	}

	for _, adminId := range strings.Split(tgBotid, ",") {
		id, err := strconv.Atoi(adminId)
		if err != nil {
			logger.Warning("Failed to get IDs from GetTgBotChatId:", err)
			return err
		}
		adminIds = append(adminIds, int64(id))
	}

	bot, err = tgbotapi.NewBotAPI(tgBottoken)
	if err != nil {
		fmt.Println("Get tgbot's api error:", err)
		return err
	}
	bot.Debug = false

	// listen for TG bot income messages
	if !isRunning {
		logger.Info("✅ ربات شروع به کار کرد.")
		go t.OnReceive()
		isRunning = true
	}

	return nil
}

func (t *Tgbot) IsRunnging() bool {
	return isRunning
}

func (t *Tgbot) Stop() {
	bot.StopReceivingUpdates()
	logger.Info("⛔️ ربات متوقف شد.")
	isRunning = false
	adminIds = nil
}

func (t *Tgbot) OnReceive() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 10

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		tgId := update.FromChat().ID
		chatId := update.FromChat().ChatConfig().ChatID
		isAdmin := checkAdmin(tgId)
		if update.Message == nil {
			if update.CallbackQuery != nil {
				t.asnwerCallback(update.CallbackQuery, isAdmin)
			}
		} else {
			if update.Message.IsCommand() {
				t.answerCommand(update.Message, chatId, isAdmin)
			} else {
				t.aswerChat(update.Message.Text, chatId, isAdmin)
			}
		}
	}
}

func (t *Tgbot) answerCommand(message *tgbotapi.Message, chatId int64, isAdmin bool) {
	msg := ""
	// Extract the command from the Message.
	switch message.Command() {
	case "help":
		msg = "<b>✅ با این ربات خیلی راحت می تونی حجم مصرفی اکانتت رو استعلام کنی!</b>\n\n <b>♻️ لطفا انتخاب کنید : </b>"
	case "creator":
		msg = "<b>👨🏻‍💻 این ربات توسط @MR_PROGR4MMER ساخته شده است، درصورت داشتن هر گونه مشکل پیام دهید.</b>"
	case "start":
		msg = "<b>سلام</b> <i>" + message.From.FirstName + "</i> <b>عزیز</b>👋"
		if isAdmin {
			msg += "\n<b>🤖 به مدیریت ربات استعلام حجم وی تو ری خوش آمدید.</b>"
		}
		msg += "\n\n<b>🤖 به ربات استعلام حجم وی تو ری خوش آمدید.</b>\n<b>♻️ لطفا انتخاب کنید : </b>"
	case "status":
		msg = "<b>👀 من هنوز زنده م و دارم خدمات ارائه میدم</b>"
	case "usage":
		if isAdmin {
			t.searchClient(chatId, message.CommandArguments())
		} else {
			msg = "<b>❌ شما مجاز به این عملیات نمی باشید 👮‍♀️✋🏻</b>"
		}
	default:
		msg = "<b>❌ دستور وارد شده درست نمی باشد لطفا بر روی دستور زیر کلیک نمایید.</b> \n /help - /help - /help"
	}
	t.SendAnswer(chatId, msg, isAdmin)
}

func (t *Tgbot) aswerChat(message string, chatId int64, isAdmin bool) {
	t.SendAnswer(chatId, "<b>🕵️‍♂️متوجه نشدم!!!!!</b>\n<b>♻️ از منو زیر انتخاب کنید : </b>", isAdmin)
}

func (t *Tgbot) asnwerCallback(callbackQuery *tgbotapi.CallbackQuery, isAdmin bool) {
	// Respond to the callback query, telling Telegram to show the user
	// a message with the data received.
	callback := tgbotapi.NewCallback(callbackQuery.ID, callbackQuery.Data)
	if _, err := bot.Request(callback); err != nil {
		logger.Warning(err)
	}

	switch callbackQuery.Data {
	case "get_usage":
		t.SendMsgToTgbot(callbackQuery.From.ID, t.getServerUsage())
	case "inbounds":
		t.SendMsgToTgbot(callbackQuery.From.ID, t.getInboundUsages())
	case "exhausted_soon":
		t.SendMsgToTgbot(callbackQuery.From.ID, t.getExhausted())
	case "get_backup":
		t.sendBackup(callbackQuery.From.ID)
	case "client_traffic":
		t.getClientUsage(callbackQuery.From.ID, callbackQuery.From.UserName)
	case "commands":
		t.SendMsgToTgbot(callbackQuery.From.ID, "📌 برای اطلاع از وضعیت اکانت، کافیه اسم را با دستور زیر به ربات بفرستید : \r\n \r\n<code>/usage نام اکانت</code>")
	}
}

func checkAdmin(tgId int64) bool {
	for _, adminId := range adminIds {
		if adminId == tgId {
			return true
		}
	}
	return false
}



func (t *Tgbot) SendAnswer(chatId int64, msg string, isAdmin bool) {
	var numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 اطلاعات سرور", "get_usage"),
			tgbotapi.NewInlineKeyboardButtonData("📤 بکاپ دیتابیس", "get_backup"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 سرویس ها", "inbounds"),
			tgbotapi.NewInlineKeyboardButtonData("👤 اکانت ها", "exhausted_soon"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📜 دستورات", "commands"),
			tgbotapi.NewInlineKeyboardButtonURL("🚀 تست سرعت", "https://pcmag.speedtestcustom.com"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("👨🏻‍💻 برنامه نویس 👨🏻‍💻", "https://t.me/MR_PROGR4MMER"),
		),
	)
	var numericKeyboardClient = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("♻️ استعلام حجم ♻️", "client_traffic"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🚀 تست سرعت", "https://pcmag.speedtestcustom.com"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("☎️ پشتیبان ☎️", "https://t.me/mohammadrezach1376"),
		),
	)
	msgConfig := tgbotapi.NewMessage(chatId, msg)
	msgConfig.ParseMode = "HTML"
	if isAdmin {
		msgConfig.ReplyMarkup = numericKeyboard
	} else {
		msgConfig.ReplyMarkup = numericKeyboardClient
	}
	_, err := bot.Send(msgConfig)
	if err != nil {
		logger.Warning("خطا در ارتباط با تلگرام :", err)
	}
}

func (t *Tgbot) SendMsgToTgbot(tgid int64, msg string) {
	var allMessages []string
	limit := 2000
	// paging message if it is big
	if len(msg) > limit {
		messages := strings.Split(msg, "\r\n \r\n")
		lastIndex := -1
		for _, message := range messages {
			if (len(allMessages) == 0) || (len(allMessages[lastIndex])+len(message) > limit) {
				allMessages = append(allMessages, message)
				lastIndex++
			} else {
				allMessages[lastIndex] += "\r\n \r\n" + message
			}
		}
	} else {
		allMessages = append(allMessages, msg)
	}
	for _, message := range allMessages {
		info := tgbotapi.NewMessage(tgid, message)
		info.ParseMode = "HTML"
		_, err := bot.Send(info)
		if err != nil {
			logger.Warning("خطا در ارتباط با تلگرام :", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (t *Tgbot) SendMsgToTgbotAdmins(msg string) {
	for _, adminId := range adminIds {
		t.SendMsgToTgbot(adminId, msg)
	}
}

func (t *Tgbot) SendReport() {
	runTime, err := t.settingService.GetTgbotRuntime()
	if err == nil && len(runTime) > 0 {
		t.SendMsgToTgbotAdmins("<b>🔁 وضعیت کرون جاب : </b>" + runTime + "\r\n<b>⏰ تاریخ و ساعت : </b>" + time.Now().Format("2006-01-02 15:04:05"))
	}
	info := t.getServerUsage()
	t.SendMsgToTgbotAdmins(info)
	exhausted := t.getExhausted()
	t.SendMsgToTgbotAdmins(exhausted)
	backupEnable, err := t.settingService.GetTgBotBackup()
	if err == nil && backupEnable {
		for _, adminId := range adminIds {
			t.sendBackup(int64(adminId))
		}
	}
}

func (t *Tgbot) getServerUsage() string {
	var info string
	//get hostname
	name, err := os.Hostname()
	if err != nil {
		logger.Error("get hostname error:", err)
		name = ""
	}
	info = fmt.Sprintf("<b>💻 نام سرور : </b>%s\r\n", name)
	//get ip address
	var ip string
	var ipv6 string
	netInterfaces, err := net.Interfaces()
	if err != nil {
		logger.Error("net.Interfaces failed, err:", err.Error())
		info += "<b>🌐 آی پی : ناشناس</b>\r\n \r\n"
	} else {
		for i := 0; i <len(netInterfaces); i++ {
			if (netInterfaces[i].Flags & net.FlagUp) != 0 {
				addrs, _ := netInterfaces[i].Addrs()

				for _, address := range addrs {
					if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
						if ipnet.IP.To4() != nil {
							ip += ipnet.IP.String() + " "
						} else if ipnet.IP.To16() != nil && !ipnet.IP.IsLinkLocalUnicast() {
							ipv6 += ipnet.IP.String() + " "
						}
					}
				}
			}
		}
		info += fmt.Sprintf("<b>🌐آی پی : </b>%s\r\n<b>🌐آی پی ورژن 6 : </b>%s\r\n", ip, ipv6)
	}

	// get latest status of server
	t.lastStatus = t.serverService.GetStatus(t.lastStatus)
	info += fmt.Sprintf("<b>🔌 آپتایم سرور: </b>%d روز\r\n", int(t.lastStatus.Uptime/86400))
	info += fmt.Sprintf("<b>📈 سرعت بارگذاری سرور: </b>%.1f, %.1f, %.1f\r\n", t.lastStatus.Loads[0], t.lastStatus.Loads[1], t.lastStatus.Loads[2])
	info += fmt.Sprintf("<b>📋 وضعیت رام سرور : </b>%s/%s\r\n", common.FormatTraffic(int64(t.lastStatus.Mem.Current)), common.FormatTraffic(int64(t.lastStatus.Mem.Total)))
	info += fmt.Sprintf("<b>🔹 تعداد تی سی پی : </b>%d\r\n", t.lastStatus.TcpCount)
	info += fmt.Sprintf("<b>🔸 تعداد یو دی پی : </b>%d\r\n", t.lastStatus.UdpCount)
	info += fmt.Sprintf("<b>🚦 کل حجم مصرفی : </b>%s (↑%s,↓%s)\r\n", common.FormatTraffic(int64(t.lastStatus.NetTraffic.Sent+t.lastStatus.NetTraffic.Recv)), common.FormatTraffic(int64(t.lastStatus.NetTraffic.Sent)), common.FormatTraffic(int64(t.lastStatus.NetTraffic.Recv)))
	info += fmt.Sprintf("<b>ℹ وضعیت پنل : </b>%s", t.lastStatus.Xray.State)

	return info
}

func (t *Tgbot) UserLoginNotify(username string, ip string, time string, status LoginStatus) {
	if username == "" || ip == "" || time == "" {
		logger.Warning("UserLoginNotify failed,invalid info")
		return
	}
	var msg string
	// Get hostname
	name, err := os.Hostname()
	if err != nil {
		logger.Warning("get hostname error:", err)
		return
	}
	if status == LoginSuccess {
		msg = fmt.Sprintf("<b>✅با موفقیت به پنل وارد شدید.\r\n📝 نام سرور :</b>%s\r\n", name)
	} else if status == LoginFail {
		msg = fmt.Sprintf("<b>❌ورود به پنل ناموفق بود.\r\n📝 نام سرور :</b>%s\r\n", name)
	}
	msg += fmt.Sprintf("<b>⏰ ساعت : </b>%s\r\n", time)
	msg += fmt.Sprintf("<b>👤 یوزرنیم وارد شده : </b>%s\r\n", username)
	msg += fmt.Sprintf("<b>🖥 آی پی دستگاه : </b>%s\r\n", ip)
	t.SendMsgToTgbotAdmins(msg)
}

func (t *Tgbot) getInboundUsages() string {
	info := ""
	// get traffic
	inbouds, err := t.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("GetAllInbounds run failed:", err)
		info += "❌ خطا در دریافت سرویس ها"
	} else {
		// NOTE:If there no any sessions here,need to notify here
		// TODO:Sub-node push, automatic conversion format
		for _, inbound := range inbouds {
			info += fmt.Sprintf("<b>📍اسم سرویس : </b>%s\r\n<b>🔢 پورت : </b>%d\r\n", inbound.Remark, inbound.Port)
			info += fmt.Sprintf("<b>🧮 کل حجم مصرفی : </b>%s (↑%s,↓%s)\r\n", common.FormatTraffic((inbound.Up + inbound.Down)), common.FormatTraffic(inbound.Up), common.FormatTraffic(inbound.Down))
			if inbound.ExpiryTime == 0 {
				info += "<b>📅 تاریخ انقضا : ♾ نامحدود </b>\r\n \r\n"
			} else {
				info += fmt.Sprintf("<b>📅 تاریخ انقضا : </b>%s\r\n \r\n", time.Unix((inbound.ExpiryTime/1000), 0).Format("2006-01-02 15:04:05"))
			}
		}
	}
	return info
}

func (t *Tgbot) getClientUsage(chatId int64, tgUserName string) {
	traffics, err := t.inboundService.GetClientTrafficTgBot(tgUserName)
	if err != nil {
		logger.Warning(err)
		msg := "‼️ خطایی رخ داد!"
		t.SendMsgToTgbot(chatId, msg)
		return
	}
	if len(traffics) == 0 {
		msg := "<b>‼️ هیچ سروری برای شما پیدا نشد!\n✅ لطفا به پشتیبان اطلاع دهید تا آیدی شما را به کانفیگتون اضافه کند.\n\n🆔 آید شما : </b><b>@" + tgUserName + "</b>"
		t.SendMsgToTgbot(chatId, msg)
	}
	for _, traffic := range traffics {
		expiryTime := ""
		if traffic.ExpiryTime == 0 {
			expiryTime = "♾ نامحدود"
		} else {
			expiryTime = time.Unix((traffic.ExpiryTime / 1000), 0).Format("2006-01-02 15:04:05")
		}
		total := ""
		if traffic.Total == 0 {
			total = "♾ نامحدود"
		} else {
			total = common.FormatTraffic((traffic.Total))
		}
		output := fmt.Sprintf("<b>🔰 وضعیت اکانت : </b>%t\r\n<b>👤 نام اکانت : </b>%s\r\n<b>🔼 حجم آپلود شده↑ : </b>%s\r\n<b>🔽 حجم دانلود شده↓ : </b>%s\r\n<b>🔄 مجموع مصرف : </b>%s / %s\r\n<b>📅 تاریخ انقضا : </b>%s\r\n",
			traffic.Enable, traffic.Email, common.FormatTraffic(traffic.Up), common.FormatTraffic(traffic.Down), common.FormatTraffic((traffic.Up + traffic.Down)),
			total, expiryTime)
		t.SendMsgToTgbot(chatId, output)
	}
	t.SendAnswer(chatId, "<b>♻️ لطفا انتخاب کنید : </b>", false)
}

func (t *Tgbot) searchClient(chatId int64, email string) {
	traffics, err := t.inboundService.GetClientTrafficByEmail(email)
	if err != nil {
		logger.Warning(err)
		msg := "‼️ خطایی رخ داد!"
		t.SendMsgToTgbot(chatId, msg)
		return
	}
	if len(traffics) == 0 {
		msg := "<b>👀 چیزی پیدا نشد!</b>"
		t.SendMsgToTgbot(chatId, msg)
		return
	}
	for _, traffic := range traffics {
		expiryTime := ""
		if traffic.ExpiryTime == 0 {
			expiryTime = "♾ نامحدود"
		} else {
			expiryTime = time.Unix((traffic.ExpiryTime / 1000), 0).Format("2006-01-02 15:04:05")
		}
		total := ""
		if traffic.Total == 0 {
			total = "♾ نامحدود"
		} else {
			total = common.FormatTraffic((traffic.Total))
		}
		output := fmt.Sprintf("<b>🔰 وضعیت اکانت : </b>%t\r\n<b>👤 نام اکانت : </b>%s\r\n<b>🔼 حجم آپلود شده↑ : </b>%s\r\n<b>🔽 حجم دانلود شده↓ : </b>%s\r\n<b>🔄 مجموع مصرف : </b>%s / %s\r\n<b>📅 تاریخ انقضا : </b>%s\r\n",
			traffic.Enable, traffic.Email, common.FormatTraffic(traffic.Up), common.FormatTraffic(traffic.Down), common.FormatTraffic((traffic.Up + traffic.Down)),
			total, expiryTime)
		t.SendMsgToTgbot(chatId, output)
	}
}

func (t *Tgbot) getExhausted() string {
	trDiff := int64(0)
	exDiff := int64(0)
	now := time.Now().Unix() * 1000
	var exhaustedInbounds []model.Inbound
	var exhaustedClients []xray.ClientTraffic
	var disabledInbounds []model.Inbound
	var disabledClients []xray.ClientTraffic
	output := ""
	TrafficThreshold, err := t.settingService.GetTgTrafficDiff()
	if err == nil && TrafficThreshold > 0 {
		trDiff = int64(TrafficThreshold) * 1073741824
	}
	ExpireThreshold, err := t.settingService.GetTgExpireDiff()
	if err == nil && ExpireThreshold > 0 {
		exDiff = int64(ExpireThreshold) * 84600
	}
	inbounds, err := t.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("❌ خطا در آپلود لیست سرویس ها", err)
	}
	for _, inbound := range inbounds {
		if inbound.Enable {
			if (inbound.ExpiryTime > 0 && (now-inbound.ExpiryTime < exDiff)) ||
				(inbound.Total > 0 && (inbound.Total-inbound.Up+inbound.Down < trDiff)) {
				exhaustedInbounds = append(exhaustedInbounds, *inbound)
			}
			if len(inbound.ClientStats) > 0 {
				for _, client := range inbound.ClientStats {
					if client.Enable {
						if (client.ExpiryTime > 0 && (now-client.ExpiryTime < exDiff)) ||
							(client.Total > 0 && (client.Total-client.Up+client.Down < trDiff)) {
							exhaustedClients = append(exhaustedClients, client)
						}
					} else {
						disabledClients = append(disabledClients, client)
					}
				}
			}
		} else {
			disabledInbounds = append(disabledInbounds, *inbound)
		}
	}
	output += fmt.Sprintf("<b>🔍 آمار کل سرویس ها : </b>\r\n<b>🛑 تعداد غیرفعال : </b>%d\r\n<b>👤 تعداد اکانت ها : </b>%d\r\n \r\n", len(disabledInbounds), len(exhaustedInbounds))
	if len(disabledInbounds)+len(exhaustedInbounds) > 0 {
		output += "📚 لیست سرویس ها : \r\n"
		for _, inbound := range exhaustedInbounds {
			output += fmt.Sprintf("<b>📍اسم سرویس : </b>%s\r\n<b>🔢 پورت : </b>%d\r\n<b>🧮 کل حجم مصرفی : </b>%s (↑%s,↓%s)\r\n", inbound.Remark, inbound.Port, common.FormatTraffic((inbound.Up + inbound.Down)), common.FormatTraffic(inbound.Up), common.FormatTraffic(inbound.Down))
			if inbound.ExpiryTime == 0 {
				output += "<b>📅 تاریخ انقضا : ♾ نامحدود </b>\r\n \r\n"
			} else {
				output += fmt.Sprintf("<b>📅 تاریخ انقضا : </b>%s\r\n \r\n", time.Unix((inbound.ExpiryTime/1000), 0).Format("2006-01-02 15:04:05"))
			}
		}
	}
	output += fmt.Sprintf("<b>🔍 آمار کل کاربران : </b>\r\n<b>🛑 تعداد غیرفعال : </b>%d\r\n<b>👤 تعداد اکانت ها : </b>%d\r\n \r\n", len(disabledClients), len(exhaustedClients))
	if len(disabledClients)+len(exhaustedClients) > 0 {
		output += "<b>📝 لیست کاربران : </b>\r\n"
		for _, traffic := range exhaustedClients {
			expiryTime := ""
			if traffic.ExpiryTime == 0 {
				expiryTime = "♾ نامحدود"
			} else {
				expiryTime = time.Unix((traffic.ExpiryTime / 1000), 0).Format("2006-01-02 15:04:05")
			}
			total := ""
			if traffic.Total == 0 {
				total = "♾ نامحدود"
			} else {
				total = common.FormatTraffic((traffic.Total))
			}
			output += fmt.Sprintf("<b>🔰 وضعیت اکانت : </b>%t\r\n<b>👤 نام اکانت : </b>%s\r\n<b>🔼 حجم آپلود شده↑ : </b>%s\r\n<b>🔽 حجم دانلود شده↓ : </b>%s\r\n<b>🔄 مجموع مصرف : </b>%s / %s\r\n<b>📅 تاریخ انقضا : </b>%s\r\n",
				traffic.Enable, traffic.Email, common.FormatTraffic(traffic.Up), common.FormatTraffic(traffic.Down), common.FormatTraffic((traffic.Up + traffic.Down)),
				total, expiryTime)
		}
	}

	return output
}

func (t *Tgbot) sendBackup(chatId int64) {
	sendingTime := time.Now().Format("2006-01-02 15:04:05")
	t.SendMsgToTgbot(chatId, "<b>🕰 تایم بکاپ : </b>"+sendingTime)
	file := tgbotapi.FilePath(config.GetDBPath())
	msg := tgbotapi.NewDocument(chatId, file)
	_, err := bot.Send(msg)
	if err != nil {
		logger.Warning("<b>❌ خطایی در آپلود دیتابیس رخ داد!</b>", err)
	}
}
