package domain

type SocialPerson struct {
	Char  CharID
	Name  string
	Level int32
}

type ApprenticeSnapshot struct {
	Master      *SocialPerson
	Peers       []SocialPerson
	Apprentices []SocialPerson
}

const (
	ApprenticeMaxLevel int32 = 29
	MasterMinLevel     int32 = 31
	DefaultApprentices       = 5
	MaximumApprentices       = 50
)
