package domain

import "time"

// FamilyPositionID 对应客户端家族职位编号：1族长、2长老、3..9自定义、10普通族人。
type FamilyPositionID uint8

const (
	FamilyLeader   FamilyPositionID = 1
	FamilyElder    FamilyPositionID = 2
	FamilyOrdinary FamilyPositionID = 10
)

type FamilyPosition struct {
	ID          FamilyPositionID
	Name        string
	Description string
	PermLevel   uint8
}

type FamilyMember struct {
	Char         CharID
	Name         string
	Level        int32
	Race         Race
	Position     FamilyPositionID
	PositionName string
	Contribution int64
}

type Family struct {
	ID         int64
	Name       string
	Level      uint8
	MemberCap  int
	Proclaim   string
	Resist     bool
	Reputation int64
	Wealth     int64
	MyPosition FamilyPositionID
	CanInvite  bool
	CanMail    bool
	Leader     CharID
	Members    []FamilyMember
	CreatedAt  time.Time
}

type FamilyBrowseEntry struct {
	ID         int64
	Name       string
	Level      uint8
	Members    int
	MemberCap  int
	Proclaim   string
	Reputation int64
	CreatedAt  time.Time
}

type FamilyStash struct {
	FamilyID int64
	Capacity int32
	TakePos  FamilyPositionID
	CanTake  bool
	IsLeader bool
	Entries  []FamilyStashEntry
}

type FamilyStashEntry struct {
	Index int32
	Stack Stack
	By    string
}

const (
	FamilyCreateMinLevel int32 = 40
	FamilyCreateHonor    int64 = 300
	FamilyCreateCost     int64 = 10_000_000 // 铜币口径：10金币
	FamilyMinJoinLevel   int32 = 12
)
