package store

import "errors"

// ErrDup 唯一约束冲突(账号名/角色名已存在)。具体实现将 DB 唯一冲突映射到此。
var ErrDup = errors.New("store: 唯一约束冲突")
