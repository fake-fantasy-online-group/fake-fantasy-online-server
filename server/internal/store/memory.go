package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Memory 是 Store 的内存实现, 供 A0 骨架跑通与单元测试使用。
// 生产用 PostgreSQL 实现(接同一接口)。这里刻意做成线程安全, 语义与真实存储对齐,
// 以便上层逻辑在两种实现下行为一致。
type Memory struct {
	endgameStates   map[int64]domain.EndgameState
	endgameReceipts map[endgameReceiptKey]domain.EndgameReceipt
	endgameRuns     map[string]domain.EndgameRunRecord
	mu              sync.RWMutex
	accounts        map[string]*Account            // username -> account
	accByID         map[int64]*Account             //
	chars           map[int64]*domain.Character    // id -> char
	charName        map[string]int64               // name -> id
	bags            map[int64]*domain.Bag          // charID -> 背包
	warehouses      map[int64]*domain.Warehouse    // charID -> 角色独立仓库
	wardrobes       map[int64]*domain.Wardrobe     // charID -> 永久外观收藏
	stalls          map[int64]*domain.Stall        // charID -> 摆摊托管物权
	worn            map[int64]*domain.EquipSet     // charID -> 身上穿的
	changeSets      map[int64]*domain.ChangeSet    // charID -> 快速换装备用装备
	skills          map[int64]domain.Learned       // charID -> 学会的技能
	quests          map[int64]domain.QuestLog      // charID -> 任务本
	pets            map[int64][]domain.PetInstance // charID -> 养的宠物
	friends         map[int64]map[int64]string     // charID -> 好友 id -> 备注
	blocks          map[int64]map[int64]uint8      // charID -> 被屏蔽角色 id -> 客户端 scope
	seenTips        map[int64]map[int32]struct{}   // charID -> 已展示的 NewbieTipBox tipId
	nextAcc         int64
	nextChar        int64
}

func NewMemory() *Memory {
	return &Memory{
		endgameStates:   map[int64]domain.EndgameState{},
		endgameReceipts: map[endgameReceiptKey]domain.EndgameReceipt{},
		endgameRuns:     map[string]domain.EndgameRunRecord{},
		accounts:        map[string]*Account{},
		accByID:         map[int64]*Account{},
		chars:           map[int64]*domain.Character{},
		charName:        map[string]int64{},
		bags:            map[int64]*domain.Bag{},
		warehouses:      map[int64]*domain.Warehouse{},
		wardrobes:       map[int64]*domain.Wardrobe{},
		stalls:          map[int64]*domain.Stall{},
		worn:            map[int64]*domain.EquipSet{},
		changeSets:      map[int64]*domain.ChangeSet{},
		skills:          map[int64]domain.Learned{},
		quests:          map[int64]domain.QuestLog{},
		pets:            map[int64][]domain.PetInstance{},
		friends:         map[int64]map[int64]string{},
		blocks:          map[int64]map[int64]uint8{},
		seenTips:        map[int64]map[int32]struct{}{},
	}
}

func (m *Memory) AccountByName(_ context.Context, username string) (*Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.accounts[username]
	if !ok {
		return nil, nil // 不存在返回 (nil,nil)，由 session 层返回“请先注册”
	}
	cp := *a
	return &cp, nil
}

func (m *Memory) CreateAccount(_ context.Context, username, passHash string) (*Account, error) {
	return m.createAccount(username, passHash, "")
}

func (m *Memory) CreateRegisteredAccount(_ context.Context, username, passHash, secHash string) (*Account, error) {
	return m.createAccount(username, passHash, secHash)
}

func (m *Memory) createAccount(username, passHash, secHash string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[username]; ok {
		return nil, ErrDup
	}
	m.nextAcc++
	a := &Account{ID: m.nextAcc, Username: username, PassHash: passHash, SecHash: secHash}
	m.accounts[username] = a
	m.accByID[a.ID] = a
	cp := *a
	return &cp, nil
}

func (m *Memory) UpdateAccountPasswordHash(_ context.Context, accountID int64, passHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.PassHash = passHash
	return nil
}

func (m *Memory) CompareAndSwapAccountCredentials(ctx context.Context, accountID int64, oldPass, oldSec, newPass, newSec string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return (&txView{m: m}).CompareAndSwapAccountCredentials(ctx, accountID, oldPass, oldSec, newPass, newSec)
}

func (m *Memory) UpdateAccountSecurityHash(_ context.Context, accountID int64, securityHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.SecHash = securityHash
	return nil
}

// SetBanned 是按用户名设置封禁的测试辅助入口。生产 GM 命令使用下面按稳定账号
// id 操作的 SetAccountBanned，避免用户名规范变化时封错目标。
func (m *Memory) SetBanned(username string, banned bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.accounts[username]; ok {
		a.Banned = banned
		a.BannedUntil = time.Time{}
	}
}

func (m *Memory) SetAccountBanned(_ context.Context, accountID int64, banned bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.Banned = banned
	a.BannedUntil = time.Time{}
	return nil
}

func (m *Memory) SetAccountBannedUntil(_ context.Context, accountID int64, until time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.Banned = false
	a.BannedUntil = until
	return nil
}

func (m *Memory) CharsByAccount(_ context.Context, accountID int64) ([]*domain.Character, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*domain.Character
	for _, c := range m.chars {
		if c.AccountID == accountID {
			cp := *c
			if a := m.accByID[c.AccountID]; a != nil {
				cp.Caiyu = a.Caiyu
			}
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Memory) CharByName(_ context.Context, name string) (*domain.Character, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.charName[name]
	if !ok {
		return nil, domain.ErrCharNotFound
	}
	cp := *m.chars[id]
	if a := m.accByID[cp.AccountID]; a != nil {
		cp.Caiyu = a.Caiyu
	}
	return &cp, nil
}

func (m *Memory) RenameCharacter(_ context.Context, charID int64, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, taken := m.charName[newName]; taken && existing != charID {
		return domain.ErrNameTaken
	}
	c, ok := m.chars[charID]
	if !ok {
		return domain.ErrCharNotFound
	}
	delete(m.charName, c.Name)
	c.Name = newName
	m.charName[newName] = charID
	return nil
}

func (m *Memory) CreateChar(_ context.Context, c *domain.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, taken := m.charName[c.Name]; taken {
		return domain.ErrNameTaken
	}
	m.nextChar++
	c.ID = m.nextChar
	cp := *c
	m.chars[c.ID] = &cp
	m.charName[c.Name] = c.ID
	return nil
}

// LoadBag 内存实现: 按角色返回存过的背包, 没有就给个空的。
func (m *Memory) LoadBag(_ context.Context, charID int64, slots int) (*domain.Bag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.bags[charID]; ok && b != nil {
		return b, nil
	}
	return domain.NewBag(slots), nil
}

func (m *Memory) LoadWarehouse(_ context.Context, charID int64, rule domain.WarehouseRule) (*domain.Warehouse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w := m.warehouses[charID]; w != nil {
		return w, nil
	}
	if !rule.Valid() {
		return nil, fmt.Errorf("store: 非法个人仓库规则")
	}
	return domain.NewWarehouse(rule.InitialPages, rule.SlotsPerPage, rule.MaxStack), nil
}

func (m *Memory) LoadWardrobe(_ context.Context, charID int64, rule domain.WardrobeRule) (*domain.Wardrobe, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w := m.wardrobes[charID]; w != nil {
		return w, nil
	}
	if !rule.Valid() {
		return nil, fmt.Errorf("store: 非法衣柜规则")
	}
	return domain.NewWardrobe(0), nil
}

func (m *Memory) LoadStall(_ context.Context, charID int64) (*domain.Stall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stalls[charID].Clone(), nil
}

// LoadEquips 内存实现。
func (m *Memory) LoadEquips(_ context.Context, charID int64) (*domain.EquipSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.worn[charID]; ok && w != nil {
		return w, nil
	}
	return domain.NewEquipSet(), nil
}

func (t *txView) LoadEquips(ctx context.Context, charID int64) (*domain.EquipSet, error) {
	return t.m.LoadEquips(ctx, charID)
}

func (m *Memory) LoadChangeSet(_ context.Context, charID int64) (*domain.ChangeSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if set := m.changeSets[charID]; set != nil {
		return set, nil
	}
	return domain.NewChangeSet(), nil
}

func (t *txView) LoadChangeSet(ctx context.Context, charID int64) (*domain.ChangeSet, error) {
	return t.m.LoadChangeSet(ctx, charID)
}

// LoadSkills 内存实现。
func (m *Memory) LoadSkills(_ context.Context, charID int64) (domain.Learned, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.skills[charID]; ok && l != nil {
		return l, nil
	}
	return domain.Learned{}, nil
}

func (t *txView) LoadSkills(ctx context.Context, charID int64) (domain.Learned, error) {
	return t.m.LoadSkills(ctx, charID)
}

// LoadQuests 内存实现。
func (m *Memory) LoadQuests(_ context.Context, charID int64) (domain.QuestLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.quests[charID]; ok && l != nil {
		return l, nil
	}
	return domain.QuestLog{}, nil
}

func (t *txView) LoadQuests(ctx context.Context, charID int64) (domain.QuestLog, error) {
	return t.m.LoadQuests(ctx, charID)
}

// LoadPets 内存实现。没有时返回 nil, 与 Postgres 一致。
func (m *Memory) LoadPets(_ context.Context, charID int64) ([]domain.PetInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return domain.ClonePets(m.pets[charID]), nil
}

func (t *txView) LoadPets(ctx context.Context, charID int64) ([]domain.PetInstance, error) {
	return t.m.LoadPets(ctx, charID)
}

// ── 好友(内存实现)。双向, 与 Postgres 同语义 ──

func (m *Memory) LoadFriends(_ context.Context, charID int64) ([]domain.Friend, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Friend
	for id, remark := range m.friends[charID] {
		name := ""
		if c := m.chars[id]; c != nil {
			name = c.Name
		}
		out = append(out, domain.Friend{Char: domain.CharID(id), Name: name, Remark: remark})
	}
	// 内存实现没有 created_at, 按 id 排一下让结果稳定 ——
	// 不排的话 map 遍历顺序每次都不同, 测试会偶发失败
	sort.Slice(out, func(i, j int) bool { return out[i].Char < out[j].Char })
	return out, nil
}

func (m *Memory) AddFriend(_ context.Context, a, b int64) error {
	if a == b {
		return fmt.Errorf("store: 不能加自己为好友")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pair := range [2][2]int64{{a, b}, {b, a}} {
		if m.friends[pair[0]] == nil {
			m.friends[pair[0]] = map[int64]string{}
		}
		if _, ok := m.friends[pair[0]][pair[1]]; !ok {
			m.friends[pair[0]][pair[1]] = ""
		}
	}
	return nil
}

func (m *Memory) RemoveFriend(_ context.Context, a, b int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.friends[a], b)
	delete(m.friends[b], a)
	return nil
}

func (m *Memory) SetFriendRemark(_ context.Context, charID, friendID int64, remark string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.friends[charID][friendID]; ok {
		m.friends[charID][friendID] = remark
	}
	return nil
}

func (m *Memory) CountFriends(_ context.Context, charID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.friends[charID]), nil
}

func (m *Memory) LoadBlocks(_ context.Context, charID int64) ([]domain.BlockEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.BlockEntry, 0, len(m.blocks[charID]))
	for id, scope := range m.blocks[charID] {
		name := ""
		if c := m.chars[id]; c != nil {
			name = c.Name
		}
		out = append(out, domain.BlockEntry{Char: domain.CharID(id), Name: name, Scope: scope})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Char < out[j].Char })
	return out, nil
}

func (m *Memory) UpsertBlock(_ context.Context, charID, blockedCharID int64, scope uint8) error {
	if charID == blockedCharID || scope > 1 {
		return fmt.Errorf("store: 无效屏蔽关系")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.chars[charID] == nil || m.chars[blockedCharID] == nil {
		return domain.ErrCharNotFound
	}
	if m.blocks[charID] == nil {
		m.blocks[charID] = map[int64]uint8{}
	}
	m.blocks[charID][blockedCharID] = scope
	return nil
}

func (m *Memory) RemoveBlock(_ context.Context, charID, blockedCharID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blocks[charID], blockedCharID)
	return nil
}

func (t *txView) LoadFriends(ctx context.Context, charID int64) ([]domain.Friend, error) {
	return t.m.LoadFriends(ctx, charID)
}
func (t *txView) AddFriend(ctx context.Context, a, b int64) error {
	return t.m.AddFriend(ctx, a, b)
}
func (t *txView) RemoveFriend(ctx context.Context, a, b int64) error {
	return t.m.RemoveFriend(ctx, a, b)
}
func (t *txView) SetFriendRemark(ctx context.Context, c, f int64, r string) error {
	return t.m.SetFriendRemark(ctx, c, f, r)
}
func (t *txView) CountFriends(ctx context.Context, charID int64) (int, error) {
	return t.m.CountFriends(ctx, charID)
}

// SaveSnapshot 内存实现: 角色行与背包在同一把锁内一起存。
func (m *Memory) SaveSnapshot(_ context.Context, snap domain.Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveSnapshotLocked(snap)
}

// saveSnapshotLocked 是 Memory 与 txView 共用的无锁内核；调用方已持有 m.mu。
// txView 若回调公开 SaveSnapshot 会重入同一把锁并永久卡死，所以事务视图不能
// 再转调 m.SaveSnapshot。
func (m *Memory) saveSnapshotLocked(snap domain.Snapshot) error {
	if snap.Char == nil {
		return nil
	}
	if _, ok := m.chars[snap.Char.ID]; !ok {
		return domain.ErrCharNotFound
	}
	if snap.Char.Caiyu < 0 {
		return fmt.Errorf("store: 彩玉余额不能为负")
	}
	if err := validateEndgameSnapshot(snap); err != nil {
		return err
	}
	cloned := snap.Clone()
	if a := m.accByID[snap.Char.AccountID]; a != nil {
		a.Caiyu = snap.Char.Caiyu
		cloned.Char.Caiyu = 0
	}
	m.chars[snap.Char.ID] = cloned.Char
	if snap.Bag != nil {
		if m.bags == nil {
			m.bags = map[int64]*domain.Bag{}
		}
		m.bags[snap.Char.ID] = cloned.Bag
	}
	if snap.Worn != nil {
		if m.worn == nil {
			m.worn = map[int64]*domain.EquipSet{}
		}
		m.worn[snap.Char.ID] = cloned.Worn
	}
	if snap.ChangeSet != nil {
		if m.changeSets == nil {
			m.changeSets = map[int64]*domain.ChangeSet{}
		}
		m.changeSets[snap.Char.ID] = cloned.ChangeSet
	}
	if snap.Warehouse != nil {
		if m.warehouses == nil {
			m.warehouses = map[int64]*domain.Warehouse{}
		}
		m.warehouses[snap.Char.ID] = cloned.Warehouse
	}
	if snap.Wardrobe != nil {
		if m.wardrobes == nil {
			m.wardrobes = map[int64]*domain.Wardrobe{}
		}
		m.wardrobes[snap.Char.ID] = cloned.Wardrobe
	}
	if snap.Stall == nil {
		delete(m.stalls, snap.Char.ID)
	} else {
		m.stalls[snap.Char.ID] = cloned.Stall
	}
	if snap.Char.Skills != nil {
		if m.skills == nil {
			m.skills = map[int64]domain.Learned{}
		}
		m.skills[snap.Char.ID] = cloned.Char.Skills
	}
	if snap.Char.Quests != nil {
		if m.quests == nil {
			m.quests = map[int64]domain.QuestLog{}
		}
		m.quests[snap.Char.ID] = cloned.Char.Quests
	}
	// 宠物无条件存(哪怕是空的) —— 只在非空时存的话, 放生最后一只宠存不进去。
	m.pets[snap.Char.ID] = domain.ClonePets(snap.Char.Pets)
	return nil
}

func (t *txView) LoadBag(ctx context.Context, charID int64, slots int) (*domain.Bag, error) {
	return t.m.LoadBag(ctx, charID, slots)
}

func (t *txView) LoadWarehouse(ctx context.Context, charID int64, rule domain.WarehouseRule) (*domain.Warehouse, error) {
	return t.m.LoadWarehouse(ctx, charID, rule)
}

func (t *txView) LoadWardrobe(ctx context.Context, charID int64, rule domain.WardrobeRule) (*domain.Wardrobe, error) {
	return t.m.LoadWardrobe(ctx, charID, rule)
}

func (t *txView) LoadStall(ctx context.Context, charID int64) (*domain.Stall, error) {
	return t.m.LoadStall(ctx, charID)
}

func (t *txView) SaveSnapshot(_ context.Context, snap domain.Snapshot) error {
	return t.m.saveSnapshotLocked(snap)
}

func (m *Memory) SaveChar(_ context.Context, c *domain.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.chars[c.ID]; !ok {
		return domain.ErrCharNotFound
	}
	cp := *c
	m.chars[c.ID] = &cp
	return nil
}

func (m *Memory) LoadSeenTips(_ context.Context, charID int64) ([]int32, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.chars[charID]; !ok {
		return nil, domain.ErrCharNotFound
	}
	out := make([]int32, 0, len(m.seenTips[charID]))
	for tipID := range m.seenTips[charID] {
		out = append(out, tipID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (m *Memory) MarkTipSeen(_ context.Context, charID int64, tipID int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.chars[charID]; !ok {
		return domain.ErrCharNotFound
	}
	if m.seenTips[charID] == nil {
		m.seenTips[charID] = map[int32]struct{}{}
	}
	m.seenTips[charID][tipID] = struct{}{}
	return nil
}

func (m *Memory) DeleteChar(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.chars[id]
	if !ok {
		return domain.ErrCharNotFound
	}
	delete(m.charName, c.Name)
	delete(m.chars, id)
	delete(m.warehouses, id)
	delete(m.wardrobes, id)
	delete(m.stalls, id)
	delete(m.changeSets, id)
	delete(m.blocks, id)
	for owner := range m.blocks {
		delete(m.blocks[owner], id)
	}
	delete(m.seenTips, id)
	return nil
}

// WithTx: 内存实现没有真事务, 用大锁近似"原子块"。真实实现走 DB 事务。
func (m *Memory) WithTx(ctx context.Context, fn func(Store) error) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	backup := m.transactionCopy()
	committed := false
	defer func() {
		if !committed {
			m.restoreTransaction(backup)
		}
	}()
	err = fn(&txView{m})
	committed = err == nil
	return err
}

func (m *Memory) Close() error { return nil }

// txView 在 WithTx 内暴露不再自行加锁的视图(锁已被 WithTx 持有), 避免重入死锁。
type txView struct{ m *Memory }

func (t *txView) AccountByName(_ context.Context, u string) (*Account, error) {
	a, ok := t.m.accounts[u]
	if !ok {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}
func (t *txView) CreateAccount(_ context.Context, u, h string) (*Account, error) {
	return t.createAccount(u, h, "")
}
func (t *txView) CreateRegisteredAccount(_ context.Context, u, h, s string) (*Account, error) {
	return t.createAccount(u, h, s)
}
func (t *txView) createAccount(u, h, s string) (*Account, error) {
	if _, ok := t.m.accounts[u]; ok {
		return nil, ErrDup
	}
	t.m.nextAcc++
	a := &Account{ID: t.m.nextAcc, Username: u, PassHash: h, SecHash: s}
	t.m.accounts[u] = a
	t.m.accByID[a.ID] = a
	cp := *a
	return &cp, nil
}
func (t *txView) UpdateAccountPasswordHash(_ context.Context, accountID int64, passHash string) error {
	a, ok := t.m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.PassHash = passHash
	return nil
}

func (t *txView) CompareAndSwapAccountCredentials(_ context.Context, accountID int64, oldPass, oldSec, newPass, newSec string) (bool, error) {
	a := t.m.accByID[accountID]
	if a == nil || a.PassHash != oldPass || a.SecHash != oldSec {
		return false, nil
	}
	a.PassHash, a.SecHash = newPass, newSec
	return true, nil
}
func (t *txView) UpdateAccountSecurityHash(_ context.Context, accountID int64, securityHash string) error {
	a, ok := t.m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.SecHash = securityHash
	return nil
}
func (t *txView) SetAccountBanned(_ context.Context, accountID int64, banned bool) error {
	a, ok := t.m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.Banned = banned
	a.BannedUntil = time.Time{}
	return nil
}
func (t *txView) SetAccountBannedUntil(_ context.Context, accountID int64, until time.Time) error {
	a, ok := t.m.accByID[accountID]
	if !ok {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	a.Banned = false
	a.BannedUntil = until
	return nil
}
func (t *txView) CharsByAccount(_ context.Context, id int64) ([]*domain.Character, error) {
	var out []*domain.Character
	for _, c := range t.m.chars {
		if c.AccountID == id {
			cp := *c
			if a := t.m.accByID[c.AccountID]; a != nil {
				cp.Caiyu = a.Caiyu
			}
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (t *txView) CharByName(_ context.Context, n string) (*domain.Character, error) {
	id, ok := t.m.charName[n]
	if !ok {
		return nil, domain.ErrCharNotFound
	}
	cp := *t.m.chars[id]
	if a := t.m.accByID[cp.AccountID]; a != nil {
		cp.Caiyu = a.Caiyu
	}
	return &cp, nil
}
func (t *txView) CreateChar(_ context.Context, c *domain.Character) error {
	if _, taken := t.m.charName[c.Name]; taken {
		return domain.ErrNameTaken
	}
	t.m.nextChar++
	c.ID = t.m.nextChar
	cp := *c
	t.m.chars[c.ID] = &cp
	t.m.charName[c.Name] = c.ID
	return nil
}
func (t *txView) SaveChar(_ context.Context, c *domain.Character) error {
	if _, ok := t.m.chars[c.ID]; !ok {
		return domain.ErrCharNotFound
	}
	cp := *c
	t.m.chars[c.ID] = &cp
	return nil
}
func (t *txView) LoadSeenTips(_ context.Context, charID int64) ([]int32, error) {
	if _, ok := t.m.chars[charID]; !ok {
		return nil, domain.ErrCharNotFound
	}
	out := make([]int32, 0, len(t.m.seenTips[charID]))
	for tipID := range t.m.seenTips[charID] {
		out = append(out, tipID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
func (t *txView) MarkTipSeen(_ context.Context, charID int64, tipID int32) error {
	if _, ok := t.m.chars[charID]; !ok {
		return domain.ErrCharNotFound
	}
	if t.m.seenTips[charID] == nil {
		t.m.seenTips[charID] = map[int32]struct{}{}
	}
	t.m.seenTips[charID][tipID] = struct{}{}
	return nil
}
func (t *txView) DeleteChar(_ context.Context, id int64) error {
	c, ok := t.m.chars[id]
	if !ok {
		return domain.ErrCharNotFound
	}
	delete(t.m.charName, c.Name)
	delete(t.m.chars, id)
	delete(t.m.warehouses, id)
	delete(t.m.changeSets, id)
	delete(t.m.seenTips, id)
	return nil
}
func (t *txView) WithTx(ctx context.Context, fn func(Store) error) error { return fn(t) }
func (t *txView) Close() error                                           { return nil }
