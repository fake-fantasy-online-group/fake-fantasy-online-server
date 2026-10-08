// Package arch 不含任何产品代码, 只放**架构约束的可执行版本**。
//
// 为什么要有它: 分层规范写在文档里, 三个月后就没人记得了。现有的 world → proto
// 就是这么来的 —— 没人故意破坏分层, 只是某天需要发个包, 手边正好有 proto。
// 所以规范必须能跑, 跑不过就合不进去。
//
// 这个测试跟着 `go test ./...` 走, 也就跟着 `make check` 走。
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modPrefix = "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/"

// allow 是**唯一权威的依赖表**, 与 docs/架构/10-分层规范.md 第一节一一对应。
// 键是 internal/ 下的包路径, 值是它允许 import 的内部包(前缀匹配)。
// 改这张表之前先改文档 —— 文档是给人看的, 这张表是给 CI 看的, 两者不许分叉。
var allow = map[string][]string{
	// ── 模型层: 零内部依赖。这是整栋楼的地基, 它不许认识任何人 ──
	"domain": {},

	// ── 资源层 ──
	"data":  {"domain"}, // 静态配置(将来改名 gamedata)
	"store": {"domain"},

	// ── 游戏层: 认识模型与资源, **不认识协议和网络** ──
	"game/event":  {"domain"},
	"game/entity": {"domain", "game/event"},
	"game/scene":  {"domain", "game/entity", "game/event", "game/ai", "game/combat", "game/spawn", "data", "store"},
	"game/ai":     {"domain"}, // 纯决策: 不碰实体、不发事件、不掷骰
	"game/combat": {"domain", "game/entity", "game/event"},
	"game/spawn":  {"domain", "game/entity"}, // 只造实体, 不发事件
	// 队伍名册。**场景不依赖它** —— 场景里判"是不是队友"只比两个 PartyID 的值,
	// 名册是会话层在用。这条边一旦加上, 队伍就会被拖进场景 goroutine 的所有权模型里。
	"game/party": {"domain"},
	// 在线索引: 只回答"谁在哪张图、是哪个实体"。**不投递** ——
	// 投递要 Router(在 game/scene), 让它依赖过去就等于把全服索引焊在场景包上。
	"game/online": {"domain"},
	// 好友请求(待处理的社交意向)。**好友关系本身不在这里**, 在数据库里;
	// 也不在场景里 —— 场景一点都不需要好友。
	"game/social": {"domain"},
	// 交易握手与队伍名册同属跨场景、进程内的短期事实；库存仍由场景 Actor
	// 独占，只有校验后的值快照能进入这里。
	"game/trade": {"domain"},

	// ── 接入层 ──
	"wire":     {},       // 帧/握手/加密, 纯字节
	"crypto":   {},       //
	"net":      {"wire"}, // 连接读写循环
	"protocol": {"domain", "game/event", "wire"},
	"session":  {"domain", "protocol", "store", "wire", "net", "game/scene", "game/event", "game/party", "game/online", "game/social", "game/trade", "data"},
}

// redLines 是**永不豁免**的两条。上面那张表可以随架构演进调整,
// 这两条一旦松动, 整套分层就没有意义了。
var redLines = []struct {
	pkgPrefix string
	banned    []string
	why       string
}{
	{
		pkgPrefix: "game/",
		banned:    []string{"proto", "protocol", "session", "net", "gateway", "wire"},
		why:       "游戏逻辑不许认识协议与网络(不可逆决策 二): 游戏层发事件, 协议层翻字节",
	},
	{
		pkgPrefix: "domain",
		banned:    []string{""}, // 空前缀 = 任何内部包
		why:       "domain 是零依赖的纯模型层, 它认识谁, 谁就再也没法脱离它单测",
	},
}

// knownDebt 是**已知欠账**: 现存的违规, 允许存在但不许增加。
//
// 每一条都写清楚打算怎么还。这张表只能变短 —— 加一行需要在 code review 里
// 明确解释为什么这笔债值得欠。
//
// 当前为空。上一笔("world -> proto", 游戏逻辑直接拼协议字节)已经还清:
// world 包整体重做成了 game/scene, 事件由 protocol 编码。
var knownDebt = map[string]string{}

func TestLayering(t *testing.T) {
	root := internalRoot(t)
	pkgs := scan(t, root)
	if len(pkgs) < 5 {
		t.Fatalf("只扫到 %d 个包, 目录结构可能变了", len(pkgs))
	}

	seenDebt := map[string]bool{}
	for _, pkg := range sortedKeys(pkgs) {
		rules, known := allow[pkg]
		if !known {
			t.Errorf("包 %q 不在依赖表里。新建包必须同时在 docs/架构/10-分层规范.md "+
				"和 arch.allow 里登记它允许依赖谁 —— 不登记就等于没有分层", pkg)
			continue
		}
		for _, imp := range pkgs[pkg] {
			if allowed(imp, rules) {
				continue
			}
			edge := pkg + " -> " + imp
			if why, ok := knownDebt[edge]; ok {
				seenDebt[edge] = true
				t.Logf("已知欠账 %s (%s)", edge, why)
				continue
			}
			t.Errorf("违反分层: %s\n  %q 只允许依赖 %v\n  见 docs/架构/10-分层规范.md 第一节",
				edge, pkg, rules)
		}
	}

	// 欠账还清了就该从表里删掉, 否则这张表会变成永远没人看的装饰。
	for edge := range knownDebt {
		if !seenDebt[edge] {
			t.Errorf("欠账 %q 已经不存在了, 请从 knownDebt 里删掉这一行", edge)
		}
	}
}

func TestRedLines(t *testing.T) {
	root := internalRoot(t)
	pkgs := scan(t, root)

	for _, pkg := range sortedKeys(pkgs) {
		for _, rl := range redLines {
			if !matches(pkg, rl.pkgPrefix) {
				continue
			}
			for _, imp := range pkgs[pkg] {
				for _, b := range rl.banned {
					if b == "" || matches(imp, b) {
						t.Errorf("越过红线: %s -> %s\n  %s", pkg, imp, rl.why)
					}
				}
			}
		}
	}
}

// TestNoOpcodesInGameLayer 是"游戏层不认识协议"的第二道防线。
//
// import 检查挡得住 `protocol.EncodeXxx`, 挡不住有人在游戏逻辑里直接写
// `send(0x8011, ...)` —— 那种代码不 import 任何协议包, 却同样把协议钉死在了玩法里。
// 所以再扫一遍字面量: game/** 的源码里不许出现 0x8xxx 这种下行 opcode。
func TestNoOpcodesInGameLayer(t *testing.T) {
	root := internalRoot(t)
	// 0x8000~0x8fff 是下行 opcode 段; 0x1000~0x1fff 是上行。两段都不许出现。
	//
	// 只看**真实字面量**, 不看注释 —— 注释里写 "protocol 把事件翻成 0x8011 字节"
	// 恰恰是在解释分层, 不该被卡。所以走 AST 而不是正则扫文本。
	opcodeLit := regexp.MustCompile(`^0[xX][18][0-9a-fA-F]{3}$`)
	fset := token.NewFileSet()

	err := filepath.WalkDir(filepath.Join(root, "game"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.INT || !opcodeLit.MatchString(lit.Value) {
				return true
			}
			t.Errorf("%s:%d 游戏层出现了 opcode 字面量 %s\n"+
				"  游戏层发事件, 由 protocol 翻成字节。协议数字写进玩法就等于把协议焊死了",
				rel, fset.Position(lit.Pos()).Line, lit.Value)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 game/ 失败: %v", err)
	}
}

// TestInboundOpcodesLiveOnlyInProtocol 上行 opcode 只许出现在协议层。
//
// 以前 session 里是一个裸 switch(`case proto.CS_Move:` 一路排下去),
// 于是"哪个号对应哪个玩法"这件事散在会话逻辑里。现在解码是一张表,
// 号是**数据**不是代码 —— 抓到新号只需要加一行, 不需要改分支。
//
// 这条卡的是回退: 谁想图快在 session 里直接 `case 0x1234:` 接一个新协议,
// 会在这里被拦下。豁免名单只有一个包:
//
//	protocol  唯一编解码层, 号本来就该在这
func TestInboundOpcodesLiveOnlyInProtocol(t *testing.T) {
	root := internalRoot(t)
	allowed := map[string]bool{"protocol": true}
	inboundLit := regexp.MustCompile(`^0[xX]1[0-9a-fA-F]{3}$`)
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if allowed[top] {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.INT || !inboundLit.MatchString(lit.Value) {
				return true
			}
			t.Errorf("%s:%d 协议层之外出现了上行 opcode 字面量 %s\n"+
				"  上行解码是 protocol/decode.go 里的一张表, 号是数据不是代码。\n"+
				"  要接新协议请往那张表里加一行, 并给出实证来源。",
				filepath.ToSlash(rel), fset.Position(lit.Pos()).Line, lit.Value)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 internal/ 失败: %v", err)
	}
}

// ── 扫描 ──

// scan 返回 包路径 -> 它 import 的内部包(去重排序)。测试文件不算 ——
// 测试为了搭脚手架跨层引用是正常的, 卡它只会逼人写更差的测试。
func scan(t *testing.T, root string) map[string][]string {
	t.Helper()
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkg := filepath.ToSlash(rel)
		if pkg == "arch" {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		if out[pkg] == nil {
			out[pkg] = map[string]bool{}
		}
		for _, spec := range f.Imports {
			p, _ := strconv.Unquote(spec.Path.Value)
			if dep, ok := strings.CutPrefix(p, modPrefix); ok {
				out[pkg][dep] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 %s 失败: %v", root, err)
	}

	res := make(map[string][]string, len(out))
	for pkg, set := range out {
		res[pkg] = sortedSet(set)
	}
	return res
}

func internalRoot(t *testing.T) string {
	t.Helper()
	// 本文件在 internal/arch/, 上一级就是 internal/
	abs, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// allowed 报告 imp 是否落在允许列表里。前缀匹配, 但必须匹到完整路径段 ——
// 否则 "game" 会误放行 "gamedata"。
func allowed(imp string, rules []string) bool {
	for _, r := range rules {
		if matches(imp, r) {
			return true
		}
	}
	return false
}

func matches(pkg, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedSet(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
