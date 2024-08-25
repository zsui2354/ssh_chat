package main

import (
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma"
	chromastyles "github.com/alecthomas/chroma/styles"
	"github.com/fatih/color"
	"github.com/jwalton/gchalk"
	"github.com/quackduck/term"
	"github.com/shurcooL/tictactoe"
)

type CMD struct {
	name     string
	run      func(line string, u *User)
	argsInfo string
	info     string
}

var (
	MainCMDs = []CMD{
		{"=`user`", dmCMD, "`msg`", "DM `user` with `msg`"}, // won't actually run, here just to show in docs
		{"users", usersCMD, "", "List users"},
		{"color", colorCMD, "`color`", "Change your name's color"},
		{"exit", exitCMD, "", "Leave the chat"},
		{"help", helpCMD, "", "Show help"},
		{"man", manCMD, "`cmd`", "Get help for a specific command"},
		{"emojis", emojisCMD, "", "See a list of emojis"},
		{"bell", bellCMD, "on|off|all", "ANSI bell on pings (on), never (off) or for every message (all)"},
		{"clear", clearCMD, "", "Clear the screen"},
		{"hang", hangCMD, "`char`|`word`", "Play hangman"}, // won't actually run, here just to show in docs
		{"tic", ticCMD, "`cell num`", "Play tic tac toe!"},
		{"devmonk", devmonkCMD, "", "Test your typing speed"},
		{"cd", cdCMD, "#`room`|`user`", "Join #room, DM user or run cd to see a list"}, // won't actually run, here just to show in docs
		{"tz", tzCMD, "`zone` [24h]", "Set your IANA timezone (like tz Asia/Dubai) and optionally set 24h"},
		{"nick", nickCMD, "`name`", "Change your username"},
		{"prompt", promptCMD, "`prompt`", "Change your prompt. Run `man prompt` for more info"},
		{"pronouns", pronounsCMD, "`@user`|`pronouns`", "Set your pronouns or get another user's"},
		{"theme", themeCMD, "`name`|list", "Change the syntax highlighting theme"},
		{"rest", commandsRestCMD, "", "Uncommon commands list"}}
	RestCMDs = []CMD{
		// {"people", peopleCMD, "", "See info about nice people who joined"},
		{"bio", bioCMD, "[`user`]", "Get a user's bio or set yours"},
		{"id", idCMD, "`user`", "Get a unique ID for a user (hashed key)"},
		{"admins", adminsCMD, "", "Print the ID (hashed key) for all admins"},
		{"eg-code", exampleCodeCMD, "[big]", "Example syntax-highlighted code"},
		{"lsbans", listBansCMD, "", "List banned IDs"},
		{"ban", banCMD, "`user` [`reason`] [`dur`]", "Ban <user> and optionally, with a reason or duration (admin)"},
		{"unban", unbanCMD, "IP|ID [dur]", "Unban a person (admin)"},
		{"mute", muteCMD, "`user`", "Mute <user> (admin)"},
		{"unmute", unmuteCMD, "`user`", "Unmute <user> (admin)"},
		{"kick", kickCMD, "`user`", "Kick <user> (admin)"},
		{"art", asciiArtCMD, "", "Show some panda art"},
		{"pwd", pwdCMD, "", "Show your current room"},
		//		{"sixel", sixelCMD, "<png url>", "Render an image in high quality"},
		{"shrug", shrugCMD, "", `¯\\\_(ツ)\_/¯`}, // won't actually run, here just to show in docs
		{"uname", unameCMD, "", "Show build info"},
		{"uptime", uptimeCMD, "", "Show server uptime"},
		{"8ball", eightBallCMD, "`question`", "Always tells the truth."},
		{"rmdir", rmdirCMD, "#`room`", "Remove an empty room"},
	}
	SecretCMDs = []CMD{
		{"ls", lsCMD, "???", "???"},
		{"cat", catCMD, "???", "???"},
		{"rm", rmCMD, "???", "???"},
		{"su", nickCMD, "???", "This is an alias of nick"},
		{"colour", colorCMD, "???", "This is an alias of color"}, // appease the british
		{":q", exitCMD, "", "This is an alias of exit"},          // appease the Vim user
		{":wq", exitCMD, "", "This is an alias of exit"},         // appease the Vim user, that wants to save
		{"neofetch", neofetchCMD, "???", "???"},                  //apease the Arch user (mostly)
	}

	unameCommit = ""
	unameTime   = ""
)

const (
	MaxRoomNameLen = 30
	MaxBioLen      = 300
)

func init() {
	MainCMDs = append(MainCMDs, CMD{"cmds", commandsCMD, "", "Show this message"}) // avoid initialization loop
}

// runCommands parses a line of raw input from a User and sends a message as
// required, running any commands the User may have called.
// It also accepts a boolean indicating if the line of input is from slack, in
// which case some commands will not be run (such as ./tz and ./exit)
func runCommands(line string, u *User) {
	line = rmBadWords(line)

	if u.IsMuted {
		u.writeln(u.Name, line)
		return
	}

	if line == "" {
		return
	}
	defer protectFromPanic()
	currCmd := strings.Fields(line)[0]
	if u.messaging != nil && currCmd != "=" && currCmd != "cd" && currCmd != "exit" && currCmd != "pwd" { // the commands allowed in a private dm room
		dmRoomCMD(line, u)
		return
	}
	if strings.HasPrefix(line, "=") && !u.isBridge {
		dmCMD(strings.TrimSpace(strings.TrimPrefix(line, "=")), u)
		return
	}

	// Now we know it is not a DM, so this is a safe place to add the hook for sending the event to plugins
	line = getMiddlewareResult(u, line)
	sendMessageToPlugins(line, u)

	switch currCmd {
	case "hang":
		hangCMD(strings.TrimSpace(strings.TrimPrefix(line, "hang")), u)
		return
	case "cd":
		cdCMD(strings.TrimSpace(strings.TrimPrefix(line, "cd")), u)
		return
	case "shrug":
		shrugCMD(strings.TrimSpace(strings.TrimPrefix(line, "shrug")), u)
		return
	case "mute":
		muteCMD(strings.TrimSpace(strings.TrimPrefix(line, "mute")), u)
		return
	}

	if u.isBridge {
		u.room.broadcastNoBridges(u.Name, line)
	} else {
		u.room.broadcast(u.Name, line)
	}

	devbotChat(u.room, line)

	args := strings.TrimSpace(strings.TrimPrefix(line, currCmd))

	if runPluginCMDs(u, currCmd, args) {
		return
	}

	if cmd, ok := getCMD(currCmd); ok {
		if cmd.argsInfo != "" || args == "" {
			cmd.run(args, u)
		}
	}
}

func dmCMD(rest string, u *User) {
	restSplit := strings.Fields(rest)
	if len(restSplit) < 2 {
		u.writeln(Devbot, "你得有个信息，伙计")
		return
	}
	peer, ok := findUserByName(u.room, restSplit[0])
	if !ok {
		u.writeln(Devbot, "没有这个人哈哈，你想私信谁？（您可能在错误的房间里)")
		return
	}
	msg := strings.TrimSpace(strings.TrimPrefix(rest, restSplit[0]))
	u.writeln(peer.Name+" <- ", msg)
	if u == peer {
		devbotRespond(u.room, []string{"你一定是真的寂寞，私信自己.",
			"别担心，我不会评头论足 :wink:",
			"真的?",
			"真是个白痴"}, 30)
		return
	}
	peer.writeln(u.Name+" -> ", msg)
}

func hangCMD(rest string, u *User) {
	if len([]rune(rest)) > 1 {
		if !u.isBridge {
			u.writeln(u.Name, "hang "+rest)
			u.writeln(Devbot, "(该词不会显示)")
		}
		hangGame = &hangman{rest, 15, " "} // default value of guesses so empty space is given away
		u.room.broadcast(Devbot, u.Name+" 开始了新的刽子手游戏！ 用 hang 猜字母 <letter>")
		u.room.broadcast(Devbot, "```\n"+hangPrint(hangGame)+"\nTries: "+strconv.Itoa(hangGame.triesLeft)+"\n```")
		return
	}
	if !u.isBridge {
		u.room.broadcast(u.Name, "hang "+rest)
	}
	if strings.Trim(hangGame.word, hangGame.guesses) == "" {
		u.room.broadcast(Devbot, "游戏已结束。使用 hang 开始新游戏 <word>")
		return
	}
	if len(rest) == 0 {
		u.room.broadcast(Devbot, "使用 hang 开始新游戏<word>，或使用 hang 开始猜测 <letter>")
		return
	}
	if hangGame.triesLeft == 0 {
		u.room.broadcast(Devbot, "不要再尝试了！这个词是 "+hangGame.word)
		return
	}
	if strings.Contains(hangGame.guesses, rest) {
		u.room.broadcast(Devbot, "您已经猜到了 "+rest)
		return
	}
	hangGame.guesses += rest
	if !(strings.Contains(hangGame.word, rest)) {
		hangGame.triesLeft--
	}
	display := hangPrint(hangGame)
	u.room.broadcast(Devbot, "```\n"+display+"\nTries: "+strconv.Itoa(hangGame.triesLeft)+"\n```")
	if strings.Trim(hangGame.word, hangGame.guesses) == "" {
		u.room.broadcast(Devbot, "没问题！这个词是 "+hangGame.word)
	} else if hangGame.triesLeft == 0 {
		u.room.broadcast(Devbot, "不要再尝试了！这个词是 "+hangGame.word)
	}
}

func clearCMD(_ string, u *User) {
	u.term.Write([]byte("\033[H\033[2J"))
}

func usersCMD(_ string, u *User) {
	u.room.broadcast("", printUsersInRoom(u.room))
}

func dmRoomCMD(line string, u *User) {
	u.writeln(u.messaging.Name+" <- ", line)
	if u == u.messaging {
		devbotRespond(u.room, []string{"你一定是真的寂寞，私信自己.",
			"别担心，我不会评判 :wink:",
			"真的?",
			"真是个白痴"}, 30)
		return
	}
	u.messaging.writeln(u.Name+" -> ", line)
}

// named devmonk at the request of a certain ced
func devmonkCMD(_ string, u *User) {
	sentences := []string{"我真的很想去上班，但我病得太重了，不能开车.", "栅栏搞不清楚它到底是要把东西挡在里面，还是要把东西挡在外面.", "他发现了彩虹的尽头，并对那里的一切感到惊讶.", "他得出结论，猪在猪天堂一定能飞.", "我只想告诉你，从你看孩子的眼神中，我看到了你对她的爱.", "我们不允许您携带宠物犰狳。.", "父亲在分娩时死亡.", "我用婴儿油涂满了我的朋友.", "草书是建造赛道的最佳方法.", "我妈妈为了装酷，说她喜欢的东西都和我一样.", "天空晴朗;星星闪烁.", "闪光灯摄影最适合在阳光充足的情况下使用.", "锈迹斑斑的钉子直立着，呈 45 度角，正等待着最合适的赤脚出现.", "人们不断告诉我 \"橙色\" 但我还是更喜欢 \"粉红色\".", "花生酱和果冻让这位老太太想起了她的过去.", "她总是对为什么世界必须是平的有一个有趣的观点.", "坚持用手肘剔牙的人真讨厌!", "Joe 发现交通锥是极好的扩音器.", "有人说，人们能很好地记住生命中的重要时刻，但却没有人记得自己的出生.", "紫色是森林中最好的城市.", "书就在桌子前面.", "每个人都对一夜之间出现的白色大飞艇感到好奇.", "他想知道她是否会喜欢他的脚趾甲收藏.", "仰卧起坐是结束一天的糟糕方式.", "他对女儿们发号施令，但她们只是回过头来瞪着他，一脸的好笑.", "她拿不准杯子是半空还是半满，于是一饮而尽.", "让他措手不及的是，空间里弥漫着烤牛排的香味.", "生活中没有什么比一块馅饼更美好的了.", "在探索了这座废弃的建筑后，他开始相信鬼魂的存在.", "这是一个日本娃娃.", "I've never seen a more beautiful brandy glass filled with wine.", "Don't piss in my garden and tell me you're trying to help my plants grow.", "She looked at the masterpiece hanging in the museum but all she could think is that her five-year-old could do better.", "Nobody loves a pig wearing lipstick.", "She always speaks to him in a loud voice.", "The teens wondered what was kept in the red shed on the far edge of the school grounds.", "I'll have you know I've written over fifty novels", "He didn't understand why the bird wanted to ride the bicycle.", "Potato wedges probably are not best for relationships.", "Baby wipes are made of chocolate stardust.", "Lucifer was surprised at the amount of life at Death Valley.", "She was too busy always talking about what she wanted to do to actually do any of it.", "The sudden rainstorm washed crocodiles into the ocean.", "I used to live in my neighbor's fishpond, but the aesthetic wasn't to my taste.", "He kept telling himself that one day it would all somehow make sense.", "The random sentence generator generated a random sentence about a random sentence.", "The reservoir water level continued to lower while we enjoyed our long shower.", "A song can make or ruin a person’s day if they let it get to them.", "He stomped on his fruit loops and thus became a cereal killer.", "I know many children ask for a pony, but I wanted a bicycle with rockets strapped to it."}
	text := sentences[rand.Intn(len(sentences))]
	u.writeln(Devbot, "好的，键入此文本: \n\n> "+text)
	u.term.SetPrompt("> ")
	defer u.formatPrompt()
	start := time.Now()
	line, err := u.term.ReadLine()
	if err == term.ErrPasteIndicator { // TODO: doesn't work for some reason?
		u.room.broadcast(Devbot, "SMH 你知道吗？ "+u.Name+" 试图在打字游戏中作弊?")
		return
	}
	dur := time.Since(start)

	accuracy := 100.0
	// analyze correctness
	if line != text {
		wrongWords := 0
		correct := strings.Fields(line)
		test := strings.Fields(text)
		if len(correct) > len(test) {
			wrongWords += len(correct) - len(test)
			correct = correct[:len(test)]
		} else {
			wrongWords += len(test) - len(correct)
			test = test[:len(correct)]
		}
		for i := 0; i < len(correct); i++ {
			if correct[i] != test[i] {
				wrongWords++
			}
		}
		accuracy -= 100 * float64(wrongWords) / float64(len(test))
		if accuracy < 0.0 {
			accuracy = 0.0
		}
	}

	u.room.broadcast(Devbot, "好 "+u.Name+", 你输入了 "+dur.Truncate(time.Second/10).String()+" 所以你的速度是 "+
		strconv.FormatFloat(
			float64(len(strings.Fields(text)))/dur.Minutes(), 'f', 1, 64,
		)+" wpm"+" with accuracy "+strconv.FormatFloat(accuracy, 'f', 1, 64)+"%",
	)
}

func ticCMD(rest string, u *User) {
	if rest == "" {
		u.room.broadcast(Devbot, "开始新的井字游戏！第一个玩家始终是 X.")
		u.room.broadcast(Devbot, "使用 tic 玩游戏 <cell num>")
		currentPlayer = tictactoe.X
		tttGame = new(tictactoe.Board)
		u.room.broadcast(Devbot, "```\n"+" 1 │ 2 │ 3\n───┼───┼───\n 4 │ 5 │ 6\n───┼───┼───\n 7 │ 8 │ 9\n"+"\n```")
		return
	}
	m, err := strconv.Atoi(rest)
	if err != nil {
		u.room.broadcast(Devbot, "确保你用的是数字，乐")
		return
	}
	if m < 1 || m > 9 {
		u.room.broadcast(Devbot, "移动是 1 到 9 之间的数字!")
		return
	}
	err = tttGame.Apply(tictactoe.Move(m-1), currentPlayer)
	if err != nil {
		u.room.broadcast(Devbot, err.Error())
		return
	}
	u.room.broadcast(Devbot, "```\n"+tttPrint(tttGame.Cells)+"\n```")
	if currentPlayer == tictactoe.X {
		currentPlayer = tictactoe.O
	} else {
		currentPlayer = tictactoe.X
	}
	if !(tttGame.Condition() == tictactoe.NotEnd) {
		u.room.broadcast(Devbot, tttGame.Condition().String())
		currentPlayer = tictactoe.X
		tttGame = new(tictactoe.Board)
	}
}

func exitCMD(_ string, u *User) {
	u.close(u.Name + " 已离开聊天")
}

func bellCMD(rest string, u *User) {
	switch rest {
	case "off":
		u.Bell = false
		u.PingEverytime = false
		u.room.broadcast("", "bell off (never)")
	case "on":
		u.Bell = true
		u.PingEverytime = false
		u.room.broadcast("", "bell on (pings)")
	case "all":
		u.Bell = true
		u.PingEverytime = true
		u.room.broadcast("", "bell all (every message)")
	case "", "status":
		if u.PingEverytime {
			u.room.broadcast("", "bell all (every message)")
		} else if u.Bell {
			u.room.broadcast("", "bell on (pings)")
		} else { // bell is off
			u.room.broadcast("", "bell off (never)")
		}
	default:
		u.room.broadcast(Devbot, "您的选项包括 off、on 和 all")
	}
}

func cdCMD(rest string, u *User) {
	defer u.formatPrompt()
	if u.messaging != nil {
		u.messaging = nil
		u.writeln(Devbot, "离开私人聊天")
		if rest == "" || rest == ".." {
			return
		}
	}
	if rest == ".." { // cd back into the main room
		u.room.broadcast(u.Name, "cd "+rest)
		if u.room != MainRoom {
			u.changeRoom(MainRoom)
		}
		return
	}
	if strings.HasPrefix(rest, "#") {
		u.room.broadcast(u.Name, "cd "+rest)
		if len(rest) > MaxRoomNameLen {
			rest = rest[0:MaxRoomNameLen]
			u.room.broadcast(Devbot, "房间名称的长度是有限的，所以我将其缩短为 "+rest+".")
		}
		if v, ok := Rooms[rest]; ok {
			u.changeRoom(v)
		} else {
			Rooms[rest] = &Room{rest, make([]*User, 0, 10), sync.RWMutex{}}
			u.changeRoom(Rooms[rest])
		}
		return
	}
	if rest == "" {
		u.room.broadcast(u.Name, "cd "+rest)
		type kv struct {
			roomName   string
			numOfUsers int
		}
		var ss []kv
		for k, v := range Rooms {
			ss = append(ss, kv{k, len(v.users)})
		}
		sort.Slice(ss, func(i, j int) bool {
			return ss[i].numOfUsers > ss[j].numOfUsers
		})
		roomsInfo := ""
		for _, kv := range ss {
			roomsInfo += Blue.Paint(kv.roomName) + ": " + printUsersInRoom(Rooms[kv.roomName]) + "  \n"
		}
		u.room.broadcast("", "聊天室和用户  \n"+strings.TrimSpace(roomsInfo))
		return
	}
	name := strings.Fields(rest)[0]
	if len(name) == 0 {
		u.writeln(Devbot, "你认为人们的名字是空的?")
		return
	}
	peer, ok := findUserByName(u.room, name)
	if !ok {
		u.writeln(Devbot, "没有这个人哈哈，你想私信谁？（您可能在错误的房间里)")
		return
	}
	u.messaging = peer
	u.writeln(Devbot, "现在在 DMs 中与 "+peer.Name+". 要离开，请使用 cd ..")
}

func tzCMD(tzArg string, u *User) {
	defer u.formatPrompt()
	if tzArg == "" {
		u.Timezone.Location = nil
		u.room.broadcast(Devbot, "启用的相对时间!")
		return
	}
	tzArgList := strings.Fields(tzArg)
	tz := tzArgList[0]
	switch strings.ToUpper(tz) {
	case "PST", "PDT":
		tz = "PST8PDT"
	case "CST", "CDT":
		tz = "CST6CDT"
	case "EST", "EDT":
		tz = "EST5EDT"
	case "MT":
		tz = "America/Phoenix"
	}
	var err error
	u.Timezone.Location, err = time.LoadLocation(tz)
	if err != nil {
		u.room.broadcast(Devbot, "你在那里有奇怪的时区，使用格式 大陆/城市，通常的美国时区（PST、PDT、EST、EDT...）或勾选 nodatime.org/TimeZones！")
		return
	}
	u.FormatTime24 = len(tzArgList) == 2 && tzArgList[1] == "24h"
	u.room.broadcast(Devbot, "更改了您的时区!")
}

func bioCMD(line string, u *User) {
	if line == "" {
		u.writeln(Devbot, "您当前的简历是:  \n> "+u.Bio)
		u.term.SetPrompt("> ")
		defer u.formatPrompt()
		for {
			input, err := u.term.ReadLine()
			if err != nil {
				return
			}
			input = strings.TrimSpace(input)
			if input != "" {
				if len(input) > MaxBioLen {
					u.writeln(Devbot, "你的简历太长了。它不应超过 "+strconv.Itoa(MaxBioLen)+" 字符.")
				}
				u.Bio = input
				// make sure it gets saved now so it stays even if the server crashes
				u.savePrefs() //nolint:errcheck // best effort
				return
			}
		}
	}
	target, ok := findUserByName(u.room, line)
	if !ok {
		u.room.broadcast(Devbot, "谁???")
		return
	}
	u.room.broadcast("", target.Bio)
}

func idCMD(line string, u *User) {
	victim, ok := findUserByName(u.room, line)
	if !ok {
		u.room.broadcast("", "未找到用户")
		return
	}
	u.room.broadcast("", victim.id)
}

func nickCMD(line string, u *User) {
	u.pickUsername(line) //nolint:errcheck // if reading input fails, the next repl will err out
}

func promptCMD(line string, u *User) {
	u.Prompt = line
	u.formatPrompt()
	if line == "" {
		u.writeln(Devbot, "(您的提示现在为空。您是否希望获取有关提示的更多信息？运行 'man prompt' 了解更多信息)")
	}
}

func listBansCMD(_ string, u *User) {
	msg := "Bans by ID:  \n"
	for i := 0; i < len(Bans); i++ {
		msg += Cyan.Cyan(strconv.Itoa(i+1)) + ". " + Bans[i].ID + "  \n"
	}
	u.room.broadcast(Devbot, msg)
}

func unbanCMD(toUnban string, u *User) {
	if !auth(u) {
		u.room.broadcast(Devbot, "未授权")
		return
	}

	if unbanIDorIP(toUnban) {
		u.room.broadcast(Devbot, "被解禁者: "+toUnban)
		saveBans()
	} else {
		u.room.broadcast(Devbot, "我找不到那个人")
	}
}

// unbanIDorIP unbans an ID or an IP, but does NOT save bans to the bans file.
// It returns whether the person was found, and so, whether the bans slice was modified.
func unbanIDorIP(toUnban string) bool {
	for i := 0; i < len(Bans); i++ {
		if Bans[i].ID == toUnban || Bans[i].Addr == toUnban { // allow unbanning by either ID or IP
			// remove this ban
			Bans = append(Bans[:i], Bans[i+1:]...)
			saveBans()
			return true
		}
	}
	return false
}

func banCMD(line string, u *User) {
	split := strings.Split(line, " ")
	if len(split) == 0 {
		u.room.broadcast(Devbot, "您要禁止哪个用户?")
		return
	}
	var victim *User
	var ok bool
	banner := u.Name
	banReason := "" // Initial ban reason is an empty string

	if split[0] == "devbot" {
		u.room.broadcast(Devbot, "你真的觉得你可以封禁我吗，渺小的人类?")
		victim = u // mwahahahaha - devbot
		banner = Devbot
	} else if !auth(u) {
		u.room.broadcast(Devbot, "未授权")
		return
	} else if victim, ok = findUserByName(u.room, split[0]); !ok {
		u.room.broadcast("", "未找到用户")
		return
	}

	if len(split) > 1 {
		dur, err := time.ParseDuration(split[len(split)-1])
		if err != nil {
			split[len(split)-1] = "" // there's no duration so don't trim anything from the reason
		}
		if len(split) > 2 {
			banReason = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, split[0]), split[len(split)-1]))
		}
		if err == nil { // there was a duration
			victim.ban(victim.Name + " 已被 " + banner + " 为 " + dur.String() + " " + banReason)
			go func(id string) {
				time.Sleep(dur)
				unbanIDorIP(id)
			}(victim.id) // evaluate id now, call unban with that value later
			return
		}
	}
	victim.ban(victim.Name + " 已被 " + banner + " " + banReason)
}

func kickCMD(line string, u *User) {
	victim, ok := findUserByName(u.room, line)
	if !ok {
		if line == "devbot" {
			u.room.broadcast(Devbot, "您将为此付出代价")
			u.close(u.Name + Red.Paint(" 已被踢出 ") + Devbot)
		} else {
			u.room.broadcast("", "未找到用户")
		}
		return
	}
	if !auth(u) && victim.id != u.id {
		u.room.broadcast(Devbot, "未授权")
		return
	}
	victim.close(victim.Name + Red.Paint(" 已被踢出 ") + u.Name)
}

func muteCMD(line string, u *User) {
	victim, ok := findUserByName(u.room, line)
	if !ok {
		u.room.broadcast("", "未找到用户")
		return
	}
	if !auth(u) && victim.id != u.id {
		u.room.broadcast(Devbot, "未授权")
		return
	}
	victim.IsMuted = true
}

func unmuteCMD(line string, u *User) {
	victim, ok := findUserByName(u.room, line)
	if !ok {
		u.room.broadcast("", "未找到用户")
		return
	}
	if !auth(u) && victim.id != u.id {
		u.room.broadcast(Devbot, "未授权")
		return
	}
	victim.IsMuted = false
}

func colorCMD(rest string, u *User) {
	if rest == "which" {
		u.room.broadcast(Devbot, u.Color+" "+u.ColorBG)
	} else if err := u.changeColor(rest); err != nil {
		u.room.broadcast(Devbot, err.Error())
	}
}

func adminsCMD(_ string, u *User) {
	msg := "管理员 ID:  \n"
	i := 1
	for id, info := range Config.Admins {
		if len(id) > 10 {
			id = id[:10] + "..."
		}
		msg += Cyan.Cyan(strconv.Itoa(i)) + ". " + id + "\t" + info + "  \n"
		i++
	}
	u.room.broadcast(Devbot, msg)
}

func helpCMD(_ string, u *User) {
	u.room.broadcast("", `欢迎来到 王果冻的聊天室! 聊天室通过 SSH 聊天： ssh apache.vyantaosheweining.top -p 2221
因为所有平台上都有 SSH 应用程序，甚至在移动设备上，所以您可以从任何地方加入。

运行 cmds 查看命令列表.

有趣的功能:
* 房间！运行 cd 查看所有房间，并使用 cd #foo 加入新房间.
* Markdown语法支持! 表格、标题、斜体和所有内容。只需使用 \\n 代替换行符.
* 代码语法突出显示。使用 Markdown 语法发送代码。运行 eg-code 查看示例.
* 直接消息！使用 =user <msg> 快速发送 DM,或通过运行 cd @user 留在 DM 中。.
* 时区支持，使用 tz Continent/City 设置您的时区.
* 内置井字游戏和刽子手！运行 tic 或 hang <word> 开始新游戏.
* 表情符号替换！\:rocket\: => :rocket: （就像在 Slack 和 Discord 上一样）

在替换换行符时，也可使用 https\://bulkseotools.com/add-remove-line-breaks.php.

加入项目Discord 服务器: https://discord.gg/yERQNTBbD5

命令：
   =<user>   <msg>           DM <user> 与 <msg>
   users                     列出用户
   color     <color>         更改您姓名的颜色
   exit                      离开聊天
   help                      显示帮助
   man       <cmd>           获取特定命令的帮助
   emojis                    查看表情符号列表
   bell      on|off|all      ANSI 钟声 (on), 从不 (off) 或每条消息 (all)
   clear                     清除屏幕
   hang      <char|word>     玩刽子手游戏
   tic       <cell num>      玩井字游戏！
   devmonk                   测试打字速度
   cd        #room|user      加入 #room, DM 用户或运行 cd 查看列表
   tz        <zone> [24h]    设置 IANA 时区 (like tz Asia/Dubai) 并可选择 24 小时
   nick      <name>          更改您的用户名
   pronouns  @user|pronouns  设置您的代词或获取其他用户的
   theme     <theme>|list    更改语法高亮主题
   rest                      不常用命令列表
   cmds                      显示此消息

其余部分：
   people                  查看有关加入的好人的信息
   id       <user>         获取用户的唯一 ID (hashed key)
   admins                  打印 ID (hashed key) 面向所有管理员
   eg-code  [big]          示例语法突出显示的代码
   lsbans                  列出禁止的 ID
   ban      <user>         禁令 <user> (admin)
   unban    <IP|ID> [dur]  取消禁止人员，并选择性地在一段时间内 (admin)
   kick     <user>         踢出 <user> (admin)
   art                     展示一些熊猫艺术
   pwd                     显示当前房间
   shrug                   ¯\_(ツ)_/¯
`)
}

func catCMD(line string, u *User) {
	if line == "" {
		u.room.broadcast("", "usage: cat [-benstuv] [file ...]")
	} else if line == "README.md" {
		helpCMD(line, u)
	} else {
		u.room.broadcast("", "cat: "+line+": 权限被拒绝")
	}
}

func rmCMD(line string, u *User) {
	if line == "" {
		u.room.broadcast("", `usage: rm [-f | -i] [-dPRrvW] file ...
unlink file`)
	} else {
		u.room.broadcast("", "rm: "+line+": 权限被拒绝, 笨蛋")
	}
}

func exampleCodeCMD(line string, u *User) {
	if line == "big" {
		u.room.broadcast(Devbot, "```go\npackage main\n\nimport \"fmt\"\n\nfunc sum(nums ...int) {\n    fmt.Print(nums, \" \")\n    total := 0\n    for _, num := range nums {\n        total += num\n    }\n    fmt.Println(total)\n}\n\nfunc main() {\n\n    sum(1, 2)\n    sum(1, 2, 3)\n\n    nums := []int{1, 2, 3, 4}\n    sum(nums...)\n}\n```")
		return
	}
	u.room.broadcast(Devbot, "\n```go\npackage main\nimport \"fmt\"\nfunc main() {\n   fmt.Println(\"Example!\")\n}\n```")
}

func init() { // add Matt Gleich's blackbird theme from https://github.com/blackbirdtheme/vscode/blob/master/themes/blackbird-midnight-color-theme.json#L175
	red := "#ff1131" // added saturation
	redItalic := "italic " + red
	white := "#fdf7cd"
	yellow := "#e1db3f"
	blue := "#268ef8"  // added saturation
	green := "#22e327" // added saturation
	gray := "#5a637e"
	teal := "#00ecd8"
	tealItalic := "italic " + teal

	chromastyles.Register(chroma.MustNewStyle("blackbird", chroma.StyleEntries{chroma.Text: white, chroma.Error: red, chroma.Comment: gray, chroma.Keyword: redItalic, chroma.KeywordNamespace: redItalic, chroma.KeywordType: tealItalic, chroma.Operator: blue, chroma.Punctuation: white, chroma.Name: white, chroma.NameAttribute: white, chroma.NameClass: green, chroma.NameConstant: tealItalic, chroma.NameDecorator: green, chroma.NameException: red, chroma.NameFunction: green, chroma.NameOther: white, chroma.NameTag: yellow, chroma.LiteralNumber: blue, chroma.Literal: yellow, chroma.LiteralDate: yellow, chroma.LiteralString: yellow, chroma.LiteralStringEscape: teal, chroma.GenericDeleted: red, chroma.GenericEmph: "italic", chroma.GenericInserted: green, chroma.GenericStrong: "bold", chroma.GenericSubheading: yellow, chroma.Background: "bg:#000000"}))
}

func themeCMD(line string, u *User) {
	// TODO: make this work with glamour
	u.room.broadcast(Devbot, "主题当前不起作用，因为 Devzat 正在切换到使用 glamour 进行渲染.")
	if line == "list" {
		u.room.broadcast(Devbot, "可用主题: "+strings.Join(chromastyles.Names(), ", "))
		return
	}
	for _, name := range chromastyles.Names() {
		if name == line {
			//markdown.CurrentTheme = chromastyles.Get(name)
			u.room.broadcast(Devbot, "主题设置为 "+name)
			return
		}
	}
	u.room.broadcast(Devbot, "那是什么主题？使用主题列表查看可用内容.")
}

func asciiArtCMD(_ string, u *User) {
	u.room.broadcast("", Art)
}

func pwdCMD(_ string, u *User) {
	if u.messaging != nil {
		u.writeln("", u.messaging.Name)
	} else {
		u.room.broadcast("", u.room.name)
	}
}

func shrugCMD(line string, u *User) {
	u.room.broadcast(u.Name, line+` ¯\\_(ツ)_/¯`)
}

func pronounsCMD(line string, u *User) {
	args := strings.Fields(line)

	if line == "" {
		u.room.broadcast(Devbot, "通过提供 em 或查询用户的代词来设置代词!")
		return
	}

	if len(args) == 1 && strings.HasPrefix(args[0], "@") {
		victim, ok := findUserByName(u.room, args[0][1:])
		if !ok {
			u.room.broadcast(Devbot, "那是谁?")
			return
		}
		u.room.broadcast(Devbot, victim.Name+"'的代词是 "+victim.displayPronouns())
		return
	}

	u.Pronouns = strings.Fields(strings.ReplaceAll(strings.ToLower(line), "\n", ""))
	//u.changeColor(u.Color) // refresh pronouns
	u.room.broadcast(Devbot, u.Name+" 现在过去了 "+u.displayPronouns())
}

func emojisCMD(_ string, u *User) {
	u.room.broadcast(Devbot, `完整列表请参见 https://github.com/ikatyang/emoji-cheat-sheet/  
下面是几个例子 (type :emoji_text: to use):  
:doughnut: doughnut  
:yum: yum  
:joy: joy  
:thinking: thinking  
:smile: smile  
:zipper_mouth_face: zipper_mouth_face  
:kangaroo: kangaroo  
:sleepy: sleepy  
:hot_pepper:  hot_pepper  
:face_with_thermometer: face_with_thermometer  
:dumpling: dumpling  
:sunglasses: sunglasses  
:skull: skull`)
}

func commandsRestCMD(_ string, u *User) {
	u.room.broadcast("", "其余的  \n"+autogenCommands(RestCMDs))
}

func manCMD(rest string, u *User) {
	if rest == "" {
		u.room.broadcast(Devbot, "您需要什么命令的帮助?")
		return
	}

	if rest == "prompt" {
		u.room.broadcast(Devbot, `prompt <prompt> 设置您的提示

你可以在其中使用一些 bash PS1 标签。
支持的标签包括：
* \u: 您的用户名
* \h, \H: devzat 的颜色与您的用户名相似
* \t, \T: 采用您首选格式的时间
* \w:  当前房间
* \W:  当前房间，#main 别名为 ~
* \S: 空格字符
* \$: $ 对于普通用户，# 对于管理员

默认提示符为 "\u:\S".`)
		return
	}

	if cmd, ok := getCMD(rest); ok {
		u.room.broadcast(Devbot, "用法: "+cmd.name+" "+cmd.argsInfo+"  \n"+cmd.info)
		return
	}
	// Plugin commands
	if c, ok := PluginCMDs[rest]; ok {
		u.room.broadcast(Devbot, "用法: "+rest+" "+c.argsInfo+"  \n"+c.info)
		return
	}

	u.room.broadcast("", "通过删除用户不登录的系统上不需要的包和内容，该系统已最小化.\n\n要恢复这些内容，包括手册页，您可以运行 'unminimize' 命令。您仍然需要确保已安装 'man-db' 软件包.")
}

func lsCMD(rest string, u *User) {
	if len(rest) > 0 && rest[0] == '#' {
		if r, ok := Rooms[rest]; ok {
			usersList := ""
			for _, us := range r.users {
				usersList += us.Name + Blue.Paint("/ ")
			}
			u.room.broadcast("", usersList)
			return
		}
	}
	if rest == "-i" { // show ids
		s := ""
		for _, us := range u.room.users {
			s += us.id + " " + us.Name + "  \n"
		}
		u.room.broadcast("", s)
		return
	}
	if rest != "" {
		u.room.broadcast("", "ls: "+rest+" 权限被拒绝")
		return
	}
	roomList := ""
	for _, r := range Rooms {
		roomList += Blue.Paint(r.name + "/ ")
	}
	usersList := ""
	for _, us := range u.room.users {
		usersList += us.Name + Blue.Paint("/ ")
	}
	usersList += Devbot + Blue.Paint("/ ")
	u.room.broadcast("", "README.md "+usersList+roomList)
}

func commandsCMD(_ string, u *User) {
	u.room.broadcast("", "Commands  \n"+autogenCommands(MainCMDs))
}

func unameCMD(rest string, u *User) {
	if unameCommit == "" || unameTime == "" {
		u.room.broadcast("", "没有可用的 uname 输出。构建 Devzat `"+color.HiYellowString(`go build -ldflags "-X 'main.unameCommit=$(git rev-parse HEAD)' -X 'main.unameTime=$(date)'"`)+"` to enable.")
		return
	}
	u.room.broadcast("", "Devzat ("+unameCommit+") "+unameTime)
}

func uptimeCMD(rest string, u *User) {
	uptime := time.Since(StartupTime)
	u.room.broadcast("", fmt.Sprintf("up %v days, %02d:%02d:%02d", int(uptime.Hours()/24), int(math.Mod(uptime.Hours(), 24)), int(math.Mod(uptime.Minutes(), 60)), int(math.Mod(uptime.Seconds(), 60))))
}

func neofetchCMD(_ string, u *User) {
	content, err := os.ReadFile(Config.DataDir + "/neofetch.txt")
	if err != nil {
		u.room.broadcast("", "Error reading "+Config.DataDir+"/neofetch.txt: "+err.Error())
		return
	}
	contentSplit := strings.Split(string(content), "\n")
	uptime := time.Since(StartupTime)
	uptimeStr := fmt.Sprintf("%v days, %v hours, %v minutes", int(uptime.Hours()/24), int(math.Mod(uptime.Hours(), 24)), int(math.Mod(uptime.Minutes(), 60)))
	memstats := runtime.MemStats{}
	runtime.ReadMemStats(&memstats)
	yellow := gchalk.RGB(255, 255, 0)
	userHost := yellow(os.Getenv("USER")) + "@" + yellow(os.Getenv("HOSTNAME"))
	colorSwatch1 := "\u001B[30m\u001B[40m   \u001B[31m\u001B[41m   \u001B[32m\u001B[42m   \u001B[33m\u001B[43m   \u001B[34m\u001B[44m   \u001B[35m\u001B[45m   \u001B[36m\u001B[46m   \u001B[37m\u001B[47m   \u001B[m"
	colorSwatch2 := "\u001B[38;5;8m\u001B[48;5;8m   \u001B[38;5;9m\u001B[48;5;9m   \u001B[38;5;10m\u001B[48;5;10m   \u001B[38;5;11m\u001B[48;5;11m   \u001B[38;5;12m\u001B[48;5;12m   \u001B[38;5;13m\u001B[48;5;13m   \u001B[38;5;14m\u001B[48;5;14m   \u001B[38;5;15m\u001B[48;5;15m   \u001B[m"
	properties := []struct {
		Key   string
		Value string
	}{
		{"", userHost},
		{"", strings.Repeat("-", len(userHost))},
		{"OS", "Devzat"},
		{"Uptime", uptimeStr},
		{"Packages", fmt.Sprint(len(PluginCMDs)+len(MainCMDs)+len(RestCMDs)) + " commands"},
		{"Shell", "devzat"},
		{"Memory", fmt.Sprintf("%v MiB alloc / %v MiB sys, %v GC cycles", memstats.Alloc/1024/1024, memstats.Sys/1024/1024, memstats.NumGC)},
		{"", ""},
		{"", colorSwatch1},
		{"", colorSwatch2},
	}
	result := ""
	for i, l := range contentSplit {
		result += l
		if i < len(properties) {
			p := properties[i]
			if p.Key != "" && p.Value != "" {
				result += "   " + yellow(p.Key) + ": " + p.Value
			} else if p.Value != "" {
				result += "   " + p.Value
			}
		}
		result += "  \n"
	}
	u.room.broadcast("", result)
}

func eightBallCMD(_ string, u *User) {
	responses := []string{
		"It is certain, ", "It is decidedly so, ", "Without a doubt, ", "Yes, definitely, ",
		"You may rely on it, ", "As I see it, yes, ", "Most likely, ", "Outlook good, ",
		"Yes, ", "Signs point to yes, ", "Reply hazy, try again, ", "Ask again later, ",
		"Better not tell you now, ", "Cannot predict now, ", "Concentrate and ask again, ",
		"Don't count on it, ", "My reply is no, ", "My sources say no, ", "Outlook not so good, ",
		"Very doubtful, ",
	}
	go func() {
		time.Sleep(time.Second * time.Duration(rand.Intn(10)))
		u.room.broadcast("8ball", responses[rand.Intn(len(responses))]+u.Name)
	}()
}

func rmdirCMD(rest string, u *User) {
	if rest == "#main" {
		u.room.broadcast("", "rmdir: failed to remove '"+rest+"': Operation not permitted")
	} else if room, ok := Rooms[rest]; ok {
		if len(room.users) == 0 {
			delete(Rooms, rest)
			u.room.broadcast("", "rmdir: removing directory, '"+rest+"'")
		} else {
			u.room.broadcast("", "rmdir: failed to remove '"+rest+"': Room not empty")
		}
	} else {
		u.room.broadcast("", "rmdir: failed to remove '"+rest+"': No such room")
	}
}
