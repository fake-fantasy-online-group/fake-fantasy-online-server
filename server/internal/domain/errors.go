package domain

import (
	"errors"
	"unicode/utf8"
)

var (
	ErrInvalidRace    = errors.New("domain: 非法职业")
	ErrInvalidGender  = errors.New("domain: 非法性别")
	ErrInvalidHead    = errors.New("domain: 非法发型")
	ErrInvalidHair    = errors.New("domain: 非法发色")
	ErrNameEmpty      = errors.New("domain: 角色名为空")
	ErrNameTooLong    = errors.New("domain: 角色名过长")
	ErrNameBadChar    = errors.New("domain: 角色名含非法字符")
	ErrCharNotFound   = errors.New("domain: 角色不存在")
	ErrNameTaken      = errors.New("domain: 角色名已被占用")
	ErrSlotTaken      = errors.New("domain: 槽位已占用")
	ErrPetNameEmpty   = errors.New("domain: 宠物名为空")
	ErrPetNameTooLong = errors.New("domain: 宠物名过长")
	ErrPetNameBadChar = errors.New("domain: 宠物名含非法字符")
)

const maxNameBytes = 24 // 实测客户端角色名为长度前缀 UTF-8, 服务端限制到 24 字节(约8汉字)

// validateName 应用建号命名规则。规则来自客户端约束与运营需要, 集中在此便于评审/调整。
func validateName(name string) error {
	if name == "" {
		return ErrNameEmpty
	}
	if len(name) > maxNameBytes {
		return ErrNameTooLong
	}
	if !utf8.ValidString(name) {
		return ErrNameBadChar
	}
	for _, r := range name {
		// 禁止控制字符与空白(防止显示异常/伪造)
		if r < 0x20 || r == 0x7f {
			return ErrNameBadChar
		}
	}
	return nil
}

// ValidateCharacterName 暴露与建角完全相同的命名规则给改名入口。
func ValidateCharacterName(name string) error { return validateName(name) }

// ValidatePetName mirrors the official Win05 text field: trim is owned by the caller and the
// editable value contains at most ten characters. Pet names are not globally unique.
func ValidatePetName(name string) error {
	if name == "" {
		return ErrPetNameEmpty
	}
	if !utf8.ValidString(name) {
		return ErrPetNameBadChar
	}
	if utf8.RuneCountInString(name) > 10 {
		return ErrPetNameTooLong
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return ErrPetNameBadChar
		}
	}
	return nil
}
