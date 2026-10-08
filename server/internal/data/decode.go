package data

import (
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
)

var gbkDec = simplifiedchinese.GBK.NewDecoder()

func gbk(s string) string {
	out, err := gbkDec.Bytes([]byte(s))
	if err != nil {
		return s
	}
	return string(out)
}

func atoi32(s string) int32 {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return int32(n)
}
