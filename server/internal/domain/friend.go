package domain

// 好友。
//
// ⚠️ **原始数据里没有好友配置表。** 全库 139 张表，列名里带 friend/buddy/relation 的
// 一个都没有。所以下面的上限与规则**全部是服务端定的**。
//
// 但机制的形状是有实证的 —— 三件道具的描述文本给出了三条：
//
//	3083 飞鸽传书（好友）  「你的信息将会发送到你的所有**在线好友**」→ 在线好友是一等概念
//	3309 信纸            「无论你的好友是否在线，都可以给对方发送消息」→ 离线留言
//	3310 包袱            「无论你的好友是否在线，都可以给对方寄送物品和金钱」→ 离线寄物
//
// MVP 只做关系本身与上下线通知；留言和寄物属于邮件系统，另算。

// Friend 是好友列表里的一条。
type Friend struct {
	Char CharID
	Name string
	// Remark 备注名。玩家给好友起的名字，空表示用角色名。
	Remark string
}

// BlockEntry 是一条单向屏蔽关系。Scope 保留 1.5.8 客户端的原始
// 0/1 值；当前服务端对两种值都执行消息和交互拦截，不猜测
// 未经证明的更细业务名。
type BlockEntry struct {
	Char  CharID
	Name  string
	Scope uint8
}

// DisplayName 返回该显示的名字：起过备注就用备注。
func (f Friend) DisplayName() string {
	if f.Remark != "" {
		return f.Remark
	}
	return f.Name
}

// MaxFriends 好友数上限。
//
// **服务端定。** 定 100 的理由：上限存在的意义是挡住"脚本加满全服"，
// 而不是限制正常社交 —— 一个人认识 100 个人已经远超实际。
// 太小反而会让老玩家不停地删了加、加了删。
const MaxFriends = 100

// FriendReject 是好友操作被拒的原因。
type FriendReject uint8

const (
	FriendOK         FriendReject = iota
	FriendSelf                    // 不能加自己
	FriendAlready                 // 已经是好友了
	FriendListFull                // 自己的列表满了
	FriendTargetFull              // 对方的列表满了
	FriendNotFound                // 没这个人
	FriendNotFriend               // 不是好友（删除时）
	FriendNoRequest               // 没有待处理的请求
	FriendOffline                 // 对方不在线（加好友要当面同意）
)

// CanAddFriend 判断能不能加。
//
// mine/theirs 是双方当前的好友数 —— **两边都要查**：
// 只查自己的话，一个列表满了的人会被人不停地加，而他自己什么都做不了。
func CanAddFriend(self, target CharID, mine, theirs int, already bool) FriendReject {
	if self == target {
		return FriendSelf
	}
	if already {
		return FriendAlready
	}
	if mine >= MaxFriends {
		return FriendListFull
	}
	if theirs >= MaxFriends {
		return FriendTargetFull
	}
	return FriendOK
}
