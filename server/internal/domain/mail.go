package domain

import "time"

// Native ov_item 3309 (script "paper") and ov_help "发送邮件" / "群发邮件":
// one sheet is spent per letter recipient. Opening or cancelling the editor is
// not a send. Parcel material/slot rules are deliberately unchanged here.
const MailLetterPaperID ItemID = 3309

type MailRule struct {
	Capacity   int32
	ExpireDays int32
	MaxAttach  int32
}

func (r MailRule) Valid() bool {
	return r.Capacity > 0 && r.ExpireDays > 0 && r.MaxAttach > 0 && r.MaxAttach <= 255
}

type MailAttachment struct {
	Ordinal int32
	Stack   Stack
}

type Mail struct {
	ID          int64
	RecipientID int64
	SenderID    int64
	SenderName  string
	Title       string
	Body        string
	Money       int64
	Caiyu       int64
	Type        uint8
	Read        bool
	Returned    bool
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Attachments []MailAttachment
}

func (m Mail) CanClaim() bool {
	return m.Money > 0 || m.Caiyu > 0 || len(m.Attachments) > 0
}

func (m Mail) CanReturn() bool {
	return m.SenderID > 0 && !m.Returned && m.CanClaim()
}

type MailDraft struct {
	RecipientName string
	Title         string
	Body          string
	Money         int64
	Caiyu         int64
	Type          uint8
	Attachments   []MailAttachment
}
