package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"image"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/acarl005/stripansi"
	"github.com/caarlos0/sshmarshal"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/disintegration/imaging"
	//"github.com/eliukblau/pixterm/pkg/ansimage"
	"github.com/fatih/color"
	"github.com/gliderlabs/ssh"
	//markdown "github.com/quackduck/go-term-markdown"
	cryptoSSH "golang.org/x/crypto/ssh"
)

var (
	Art  = getASCIIArt()
	CMDs []*[]CMD // slice of pointers to slices of commands (this is so updates to sub-slices are reflected in the main slice even if append is used)
)

func init() {
	CMDs = []*[]CMD{&MainCMDs, &RestCMDs, &SecretCMDs}
}

func getCMD(name string) (CMD, bool) {
	for _, cmds := range CMDs {
		if cmds == nil {
			Log.Println("CMD 中的 nil 分段") // should never happen
			continue
		}
		for _, cmd := range *cmds {
			if cmd.name == name {
				return cmd, true
			}
		}
	}
	return CMD{}, false
}

func getASCIIArt() string {
	sep := string(os.PathSeparator)
	b, _ := os.ReadFile(Config.DataDir + sep + "art.txt")
	if b == nil {
		return "抱歉，没有找到任何艺术作品，请打你的开发者一巴掌，告诉他们添加一个 " + Config.DataDir + sep + "art.txt 文件"
	}
	return string(b)
}

func printUsersInRoom(r *Room) string {
	names := ""
	admins := ""
	for _, us := range r.users {
		if auth(us) {
			admins += us.Name + " "
			continue
		}
		names += us.Name + " "
	}
	if len(names) > 0 {
		names = names[:len(names)-1] // cut extra space at the end
	}
	names = "[" + names + "]"
	if len(admins) > 0 {
		admins = admins[:len(admins)-1]
	}
	admins = "[" + admins + "]"
	return names + " Admins: " + admins
}

func lenString(a string) int {
	return len([]rune(stripansi.Strip(a)))
}

func autogenCommands(cmds []CMD) string {
	b := new(bytes.Buffer)
	w := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	for _, cmd := range cmds {
		w.Write([]byte("   " + Chalk.Bold(cmd.name) + "\t" + cmd.argsInfo + "\t_" + cmd.info + "_  \n")) //nolint:errcheck // bytes.Buffer is never going to err out
	}
	w.Flush()
	return b.String()
}

// check if a User is an admin
func auth(u *User) bool {
	_, ok := Config.Admins[u.id]
	return ok
}

func keepSessionAlive(s ssh.Session) {
	for {
		time.Sleep(time.Minute * 3)
		_, err := s.SendRequest("keepalive@devzat", true, nil)
		if err != nil {
			return
		}
	}
}

func protectFromPanic() {
	if i := recover(); i != nil {
		MainRoom.broadcast(Devbot, "给我打王果冻那小子一巴掌，服务器差点崩溃，也告诉他这个事: "+fmt.Sprint(i)+", stack: "+string(debug.Stack()))
	}
}

// removes arrows, spaces and non-ascii-printable characters
func cleanName(name string) string {
	s := ""
	name = strings.ReplaceAll(strings.TrimSpace(strings.Split(name, "\n")[0]), // use one trimmed line
		" ", "-")
	if len([]rune(name)) > 27 {
		name = string([]rune(name)[:27])
	}
	for i := 0; i < len(name); i++ {
		if 33 <= name[i] && name[i] <= 126 { // ascii printables only: '!' to '~'
			s += string(name[i])
		}
	}
	return s
}

func mdRender(a string, beforeMessageLen int, lineWidth int, imageCache map[string]image.Image) string {
	glamourStyle := glamour.DarkStyleConfig
	glamourStyle.Document.Color = nil
	glamourStyle.Document.Margin = nil
	glamourStyle.Image = ansi.StylePrimitive{Format: "\n<img>{{.text}}</img>\n"}
	glamourStyle.ImageText.Format = "Image: {{.text}}"
	glamourStyle.Table.StyleBlock.StylePrimitive.Prefix = "\x1b[0m" // ansi reset: hack to stop space in front of table from being erased
	r, _ := glamour.NewTermRenderer(glamour.WithEmoji(), glamour.WithStyles(glamourStyle), glamour.WithWordWrap(lineWidth-beforeMessageLen), glamour.WithPreservedNewLines())
	md, err := r.Render(a)
	if err != nil {
		MainRoom.broadcast(Devbot, err.Error())
		return ""
	}
	md = addLeftPad(strings.TrimSuffix(replaceImgs(md, lineWidth, imageCache), "\n"), beforeMessageLen)
	return md
}

func replaceImgs(md string, width int, cache map[string]image.Image) string {
	if !strings.Contains(md, "<img>") {
		return md
	}
	start := strings.Index(md, "<img>")
	end := strings.Index(md, "</img>")
	if end == -1 {
		return md
	}
	imgStart := start + 5
	imgEnd := end
	imgText := md[imgStart:imgEnd]
	imgText = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(imgText), "\n", ""), " ", "")

	if img, ok := cache[imgText]; ok {
		imgText = imgRender(img, width/2)
		return replaceImgs(md[:start]+imgText+md[end+6:], width, cache)
	}

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(imgText)
	if err != nil {
		return replaceImgs(md[:start]+imgText+" (获取图像时出错)"+md[end+6:], width, cache)
	}
	if res.StatusCode != http.StatusOK {
		return replaceImgs(md[:start]+imgText+"(error: http: "+http.StatusText(res.StatusCode)+")"+md[end+6:], width, cache)
	}
	limitReader := io.LimitReader(res.Body, 30*1024*1024) // 30 megabyte limit
	// https://github.com/golang/go/issues/12512#issuecomment-137981217
	header := new(bytes.Buffer)
	config, _, err := image.DecodeConfig(io.TeeReader(limitReader, header))
	if err != nil || config.Width > 4032*2 || config.Height > 3024*2 {
		return replaceImgs(md[:start]+imgText+" (无效或太大而无法渲染)"+md[end+6:], width, cache)
	}
	img, _, err := image.Decode(io.MultiReader(header, limitReader))
	if err != nil {
		return replaceImgs(md[:start]+imgText+" (错误解码图像)"+md[end+6:], width, cache)
	}
	if cache != nil {
		cache[imgText] = img
	}
	imgText = imgRender(img, width/2)

	return replaceImgs(md[:start]+imgText+md[end+6:], width, cache)
}

func imgRender(img image.Image, width int) string {
	var builder strings.Builder
	img = imaging.Fit(img, width, math.MaxInt32, imaging.Lanczos)
	for y := 0; y < img.Bounds().Dy(); y += 2 {
		for x := 0; x < img.Bounds().Dx(); x++ {
			r1, g1, b1, _ := img.At(x, y).RGBA()
			r2, g2, b2, _ := img.At(x, y+1).RGBA()
			builder.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r1/256, g1/256, b1/256, r2/256, g2/256, b2/256))
		}
		builder.WriteString("\x1b[0m\n")
	}
	return builder.String()
}

func addLeftPad(a string, pad int) string {
	split := strings.Split(a, "\n")
	for i := 1; i < len(split); i++ { // skip first line
		split[i] = strings.Repeat(" ", pad) + split[i]
	}
	a = strings.Join(split, "\n")
	return a
}

// Returns true and the User with the same name if the username is taken, false and nil otherwise
func userDuplicate(r *Room, a string) (*User, bool) {
	for i := range r.users {
		if stripansi.Strip(r.users[i].Name) == stripansi.Strip(a) {
			return r.users[i], true
		}
	}
	return nil, false
}

func saveBans() {
	f, err := os.Create(Config.DataDir + string(os.PathSeparator) + "bans.json")
	if err != nil {
		Log.Println(err)
		return
	}
	defer f.Close()
	j := json.NewEncoder(f)
	j.SetIndent("", "   ")
	err = j.Encode(Bans)
	if err != nil {
		MainRoom.broadcast(Devbot, "error 保存封禁: "+err.Error())
		Log.Println(err)
		return
	}
}

func readBans() {
	f, err := os.Open(Config.DataDir + string(os.PathSeparator) + "bans.json")
	if err != nil {
		if !os.IsNotExist(err) {
			Log.Println(err)
		}
		return
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&Bans)
	if err != nil {
		MainRoom.broadcast(Devbot, "error 加载封禁: "+err.Error())
		Log.Println(err)
		return
	}
}

func findUserByName(r *Room, name string) (*User, bool) {
	r.usersMutex.RLock()
	defer r.usersMutex.RUnlock()
	for _, u := range r.users {
		if stripansi.Strip(u.Name) == name || "@"+stripansi.Strip(u.Name) == name {
			return u, true
		}
	}
	return nil, false
}

func remove(s []*User, a *User) []*User {
	for j := range s {
		if s[j] == a { // https://github.com/golang/go/wiki/SliceTricks#delete
			copy(s[j:], s[j+1:])
			s[len(s)-1] = nil
			return s[:len(s)-1]
		}
	}
	return s
}

func devbotChat(room *Room, line string) {
	if strings.Contains(line, "devbot") {
		if strings.HasPrefix(line, "kick ") || strings.HasPrefix(line, "ban ") { // devbot already replied in the command function
			return
		}
		if strings.Contains(line, "how are you") || strings.Contains(line, "how you") {
			devbotRespond(room, []string{"How are _you_",
				"一如既往的精彩",
				"老规矩, 解决量子引力问题 :微笑:",
				"你好?",
				"关于星系间奶牛的思考",
				"数学在其他宇宙中会有所不同吗？",
				""}, 99)
			return
		}
		if strings.Contains(line, "thank") {
			devbotRespond(room, []string{"别客气",
				"没问题",
				"是的，别担心",
				":微笑:",
				"不用担心",
				"不客气，伙计!",
				"哈哈"}, 93)
			return
		}
		if strings.Contains(line, "good") || strings.Contains(line, "cool") || strings.Contains(line, "awesome") || strings.Contains(line, "amazing") {
			devbotRespond(room, []string{"谢谢 哈哈", ":sunglasses:", ":墨镜:", "哈哈", "haha", "谢谢 哈哈", "哎呀"}, 93)
			return
		}
		if strings.Contains(line, "bad") || strings.Contains(line, "idiot") || strings.Contains(line, "stupid") {
			devbotRespond(room, []string{"真是个白痴，欺负一个机器人", ":(", ":生气:", ":愤怒:", ":哭:", "我正忙着呢", "闭嘴", "运行 ./help, 你需要它."}, 60)
			return
		}
		if strings.Contains(line, "shut up") {
			devbotRespond(room, []string{"NO YOU", "You shut up", "what an idiot, bullying a bot"}, 90)
			return
		}
		devbotRespond(room, []string{"嗨，我是 开发者机器人", "嘿", "你好 ：火箭:", "是?", "开发者机器人 来救援!", ":浪:"}, 90)
	}
	if line == "./help" || line == "/help" || strings.Contains(line, "help me") {
		devbotRespond(room, []string{"运行 help 以获取帮助!",
			"寻求帮助?",
			"使用 cmds 查看可用命令或查看帮助 :star:"}, 100)
	}
	if line == "easter" {
		devbotRespond(room, []string{"eggs?", "bunny?"}, 100)
	}
	if strings.Contains(line, "rm -rf") {
		devbotRespond(room, []string{"rm -rf you", "我听说 rm -rf / 真的可以释放一些空间!\n\n 你应该在你的电脑上试试", "evil"}, 100)
		return
	}
	if strings.Contains(line, "where") && strings.Contains(line, "repo") {
		devbotRespond(room, []string{"软件仓库在 github.com/zsui2354/devzat!", ":star: github.com/zsui2354/devzat :star:", "# github.com/zsui2354/devzat"}, 100)
	}
	if strings.Contains(line, "rocket") || strings.Contains(line, "spacex") || strings.Contains(line, "tesla") {
		devbotRespond(room, []string{
			":rocket:",
			"我喜欢火箭",
			"太空探索技术公司 SpaceX",
			"埃隆·马斯克"}, 80)
	}
	if strings.Contains(line, "elon") {
		devbotRespond(room, []string{"当某件事足够重要时，即使赔率对您不利，您也会这样做 - Elon",
			"我确实认为，如果你有一个引人注目的产品，就会有很大的潜力 - Elon",
			"如果你想创建一家公司，这就像烤蛋糕。你必须让所有成分的比例合适 - Elon",
			"耐心是一种美德，我正在学习耐心。这是一个惨痛的教训 - Elon"}, 75)
	}
	if !strings.Contains(line, "start") && strings.Contains(line, "star") {
		devbotRespond(room, []string{"有人说 :star:?",
			"如果您喜欢 Devzat，请在 github.com/quackduck/devzat!",
			":star: github.com/quackduck/devzat", ":star:"}, 90)
	}
	if strings.Contains(line, "cool project") || strings.Contains(line, "this is cool") || strings.Contains(line, "this is so cool") {
		devbotRespond(room, []string{"谢谢 :slight_smile:!",
			" 如果你喜欢 Devzat，请在 github.com/quackduck/devzat!",
			"Star Devzat here: github.com/quackduck/devzat"}, 90)
	}
}

func devbotRespond(room *Room, messages []string, chance int) {
	if chance == 100 || chance > rand.Intn(100) {
		go func() {
			time.Sleep(time.Second / 2)
			pick := messages[rand.Intn(len(messages))]
			room.broadcast(Devbot, pick)
		}()
	}
}

func shasum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func holidaysCheck(u *User) {
	currentMonth := time.Now().Month()
	today := time.Now().Day()

	type holiday struct {
		month time.Month
		day   int
		name  string
		image string
	}

	holidayList := []holiday{
		{time.February, 14, "❤️ - Valentine's Day", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/81/heavy-black-heart_2764.png"},
		{time.March, 17, "☘️ - St. Patrick's Day", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/shamrock_2618-fe0f.png"},
		{time.April, 22, "🌎 - Earth Day", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/globe-showing-americas_1f30e.png"},
		{time.May, 8, "👩 - Mother's Day", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/woman_1f469.png"},
		{time.June, 19, "👨 - Father's Day", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/man_1f468.png"},
		{time.September, 11, "👴 - Grandparents' Day", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/old-woman_1f475.png"},
		{time.October, 31, "🎃 - Halloween", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/jack-o-lantern_1f383.png"},
		{time.December, 25, "🎅 - Christmas", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/santa-claus_1f385.png"},
		{time.December, 31, "🍾 - New Year's Eve", "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/325/bottle-with-popping-cork_1f37e.png"},
	}

	for _, h := range holidayList {
		if currentMonth == h.month && today == h.day {
			u.writeln("", "!["+h.name+"]("+h.image+")")
			time.Sleep(time.Second)
			clearCMD("", u)
			break
		}
	}
}

func printPrettyDuration(d time.Duration) string {
	s := d.Round(time.Minute).String()
	s = s[:len(s)-2] // cut off "0s" at the end
	if s == "" {     // we cut off the seconds so if there's nothing in the string it means it was made of only seconds.
		s = "< 1m"
	}
	return s
}

func fmtTime(u *User, lastStamp time.Time) string {
	if u.Timezone.Location == nil {
		diff := lastStamp.Sub(u.joinTime)
		if diff < 0 {
			return printPrettyDuration(-diff) + " earlier"
		}
		return printPrettyDuration(diff) + " in"
	}
	if u.FormatTime24 {
		return lastStamp.In(u.Timezone.Location).Format("15:04")
	}
	return lastStamp.In(u.Timezone.Location).Format("3:04")
}

// Check if the private key is there and if it is not, try to create it.
func checkKey(keyPath string) {
	_, err := os.Stat(keyPath)
	if err == nil {
		// Key exists, everything is fine and dandy.
		return
	}
	if !os.IsNotExist(err) { // the error is not a not-exist error. i.e. the file exists but there's some other problem with it
		Log.Printf("检查 SSH 密钥时出错 [%v]: %v\n", keyPath, err)
		return
	}

	Log.Printf("生成新的 SSH 服务器私钥 %v\n", keyPath)
	privkey, pubkey, err := genKey()
	if err != nil {
		Log.Printf("生成密钥对时出错: %v\n", err)
		return
	}
	privkeyFile, err := os.Create(keyPath)
	if err != nil {
		Log.Printf("为私钥创建文件时出错: %v\n", err)
		return
	}
	defer privkeyFile.Close()
	blk, err := sshmarshal.MarshalPrivateKey(privkey, "")
	if err != nil {
		Log.Printf("封送私钥时出错: %v\n", err)
		return
	}
	if err := pem.Encode(privkeyFile, blk); err != nil {
		Log.Printf("对私钥进行编码时出错: %v\n", err)
		return
	}
	Log.Println("已成功生成密钥!\n虽然公钥对于服务器操作不是必需的，但保存它可能很有用:\n" + color.YellowString(string(cryptoSSH.MarshalAuthorizedKey(pubkey))))
}

func genKey() (ed25519.PrivateKey, ssh.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, err
	}
	sshPubKey, err := cryptoSSH.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	return priv, sshPubKey, nil
}
