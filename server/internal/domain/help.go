package domain

// HelpTopic 是客户端帮助面板的一条标题与正文。主题来自 PostgreSQL
// gamedata.ov_help；协议层只负责把这两个已闭合字段写成 0x8055。
type HelpTopic struct {
	Title   string
	Content string
}
