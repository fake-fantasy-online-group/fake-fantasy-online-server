package domain

// AnnouncementPool is the server-owned rolling text configuration.
type AnnouncementLine struct {
	ID   int32
	Text string
}
type AnnouncementPool struct {
	Lines                         []AnnouncementLine
	Speed, IntervalSec, PeriodMin int32
	Enabled                       bool
}
