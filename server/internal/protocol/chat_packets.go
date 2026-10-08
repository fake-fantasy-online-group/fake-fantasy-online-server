package protocol

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// OpChat 是客户端发送聊天消息时使用的线协议 opcode。
// ReqChat 单独放在聊天协议文件中，避免聊天业务细节继续挤进主解码表。
const (
	OpChat  uint16  = 0x1058
	ReqChat ReqKind = 250
)

func init() {
	reqNames[ReqChat] = "Chat"
}

// decChat 解 0x1058：U8 channel + Str text + U8 sharesCount，随后每条分享为
// U8 kind + U8 tab + I32 slot。1.5.8 在分享段后固定追加 I32 itemId + U8 skin；
// 普通聊天写 0/0，SendHorn 复用 0x1058 并写实际值。当前业务只转发纯文本，但必须完整吃掉分享尾，
// 否则畸形包可能借“未读取的剩余字节”绕过边界校验。
func DecodeChat(payload []byte) (Request, bool) {
	r := newReader(payload)
	channel := r.u8()
	message := r.str()
	shareCount := r.u8()
	if shareCount > 4 {
		return Request{}, false
	}
	shares := make([]ShareRef, 0, shareCount)
	for i := uint8(0); i < shareCount; i++ {
		shares = append(shares, ShareRef{Kind: r.u8(), Tab: r.u8(), Slot: r.i32()})
	}
	itemID, skin := r.i32(), r.u8()
	if !r.done() || message == "" {
		return Request{}, false
	}
	return Request{Kind: ReqChat, U8: channel, S1: message, ID32: uint32(itemID),
		Flag: skin, Shares: shares}, true
}

// ChannelChat 编码 0x8039 的无分享文本形态。客户端把分享列表作为可选尾读取，
// 所以纯文本包在 text 后结束即可。
type ChatShareView struct {
	Kind                                 uint8
	ItemID                               int32
	Quality                              uint8
	Name                                 string
	Desc                                 string
	Pet                                  *domain.PetItemInfo
	PPAiUsed, PPAiCap, PPShUsed, PPShCap int32
}

func ChannelChat(sender int32, name string, theme, channel uint8, message string, shares ...ChatShareView) []byte {
	if name == "" || message == "" || !wireString(name) || !wireString(message) {
		return nil
	}
	if len(shares) > 255 {
		return nil
	}
	w := NewW(0x8039).
		I32(sender).
		Str(name).
		U8(theme).
		U8(channel).
		Str(message)
	if len(shares) == 0 {
		return w.Bytes()
	}
	w.U8(uint8(len(shares)))
	for _, share := range shares {
		if !appendChatShare(w, share) {
			return nil
		}
	}
	return w.Bytes()
}

// EntityBubble 编码 0x8019：地图内实体号 + 头顶气泡文本。它与 0x8039
// 聊天栏消息使用同一份已校验正文，但客户端消费入口不同，二者不能互相替代。
func EntityBubble(sender int32, message string) []byte {
	if sender <= 0 || message == "" || !wireString(message) {
		return nil
	}
	return NewW(0x8019).I32(sender).Str(message).Bytes()
}
