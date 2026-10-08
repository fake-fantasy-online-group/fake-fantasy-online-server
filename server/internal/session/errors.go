package session

import "errors"

// ErrNoKeys 表示握手时未能取得会话密钥。
var ErrNoKeys = errors.New("session: 无会话密钥")
