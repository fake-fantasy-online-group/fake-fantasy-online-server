package session

import (
	"regexp"
	"unicode"
	"unicode/utf8"
)

// Redaction is kept byte-for-byte equivalent to the client's
// Foundation/ClientLogRedactor.cs (same rule order). Shared samples live in
// testdata/client_log_redaction_cases.json, generated from the client code.
//
// .NET regex semantics reproduced here:
//   - \s is Unicode whitespace ([\f\n\r\t\v\x85\p{Z}]), so U+3000 counts;
//   - key/value and Authorization accept the full-width colon '：';
//   - \b around the hex rule uses Unicode word characters;
//   - the phone rule uses digit look-behind/look-ahead (no consumed neighbours);
//   - base64-like runs are masked only when they mix ASCII letters and digits,
//     so stack-trace paths and type names stay readable.
const clientLogSpace = `\s\x{85}\p{Z}`

var (
	clientLogAuthorization = regexp.MustCompile(`(?i)(authorization[` + clientLogSpace + `]*[:=：][` + clientLogSpace + `]*)(bearer[` +
		clientLogSpace + `]+|basic[` + clientLogSpace + `]+)?[^` + clientLogSpace + `,;"'&]+`)
	clientLogKeyValue = regexp.MustCompile(`(?i)((?:token|ticket|session|password|passwd|pwd|secret|密码)[A-Za-z_]*["']?[` +
		clientLogSpace + `]*[:=：][` + clientLogSpace + `]*["']?)[^` + clientLogSpace + `,;"'&]+`)
	clientLogEmail  = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	clientLogHexRun = regexp.MustCompile(`[0-9a-fA-F]{32,}`)
	clientLogBase64 = regexp.MustCompile(`[A-Za-z0-9+/_\-]{32,}={0,2}`)
)

// RedactClientLog masks credentials, long hex/base64 secrets, e-mail
// addresses and mainland mobile numbers.
func RedactClientLog(b []byte) []byte {
	b = clientLogAuthorization.ReplaceAll(b, []byte("${1}***"))
	b = clientLogKeyValue.ReplaceAll(b, []byte("${1}***"))
	b = clientLogEmail.ReplaceAll(b, []byte("[email]"))
	b = redactClientLogHex(b)
	b = clientLogBase64.ReplaceAllFunc(b, func(m []byte) []byte {
		if mixesLettersAndDigits(m) {
			return []byte("[b64]")
		}
		return m
	})
	return redactClientLogPhones(b)
}

// dotnetWordRune matches .NET \w: [\p{L}\p{Mn}\p{Nd}\p{Pc}].
func dotnetWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Nd, r) || unicode.Is(unicode.Pc, r)
}

// redactClientLogHex reproduces \b[0-9a-fA-F]{32,}\b: a maximal hex run is
// masked only when both neighbours are non-word characters. A shorter
// backtracked match can never end on a boundary inside the run.
func redactClientLogHex(b []byte) []byte {
	locs := clientLogHexRun.FindAllIndex(b, -1)
	if len(locs) == 0 {
		return b
	}
	out := make([]byte, 0, len(b))
	last := 0
	for _, l := range locs {
		if l[0] > 0 {
			if r, _ := utf8.DecodeLastRune(b[:l[0]]); dotnetWordRune(r) {
				continue
			}
		}
		if l[1] < len(b) {
			if r, _ := utf8.DecodeRune(b[l[1]:]); dotnetWordRune(r) {
				continue
			}
		}
		out = append(out, b[last:l[0]]...)
		out = append(out, "[hex]"...)
		last = l[1]
	}
	return append(out, b[last:]...)
}

// redactClientLogPhones reproduces
// (?<![0-9])(?:\+?86[\- ]?)?1[3-9][0-9]{9}(?![0-9]) with .NET backtracking
// order: at each start position try the prefix forms greedily, then without.
func redactClientLogPhones(b []byte) []byte {
	var out []byte
	last := 0
	for i := 0; i < len(b); {
		if i > 0 && b[i-1] >= '0' && b[i-1] <= '9' {
			i++
			continue
		}
		end := clientLogPhoneEnd(b[i:])
		if end < 0 {
			i++
			continue
		}
		out = append(out, b[last:i]...)
		out = append(out, "[phone]"...)
		i += end
		last = i
	}
	if out == nil {
		return b
	}
	return append(out, b[last:]...)
}

// clientLogPhoneEnd returns the match length at the start of s, or -1.
func clientLogPhoneEnd(s []byte) int {
	for _, prefix := range []string{`\+86[\- ]`, `\+86`, `86[\- ]`, `86`, ``} {
		re := clientLogPhonePrefixes[prefix]
		if loc := re.FindIndex(s); loc != nil {
			if loc[1] == len(s) || s[loc[1]] < '0' || s[loc[1]] > '9' {
				return loc[1]
			}
		}
	}
	return -1
}

var clientLogPhonePrefixes = func() map[string]*regexp.Regexp {
	m := map[string]*regexp.Regexp{}
	for _, p := range []string{`\+86[\- ]`, `\+86`, `86[\- ]`, `86`, ``} {
		m[p] = regexp.MustCompile(`^` + p + `1[3-9][0-9]{9}`)
	}
	return m
}()

func mixesLettersAndDigits(b []byte) bool {
	letter, digit := false, false
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
			letter = true
		}
		if letter && digit {
			return true
		}
	}
	return false
}
