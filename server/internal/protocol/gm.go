package protocol

// GMCommandText 用客户端已有的 0x8008 服务端文本动作回显 GM 执行结果。
// displayMode=1 是普通聊天提示模式；权限与命令结果只由服务端决定。
func GMCommandText(text string) []byte {
	if !wireString(text) {
		return nil
	}
	return NewW(SCServerText).Str(text).U8(1).Bytes()
}
