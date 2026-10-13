package config

import (
	"errors"
	"regexp"
	"strings"
)

var placeholderRE = regexp.MustCompile(`\{([A-Za-z_][A-Za-z_0-9]*)\}`)

func ParametersRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	for i := 0; i < len(pattern); {
		if end := literalEnd(pattern, i); end > i {
			b.WriteString(pattern[i:end])
			i = end
			continue
		}
		if pattern[i] == '{' {
			loc := placeholderRE.FindStringSubmatchIndex(pattern[i:])
			if len(loc) >= 4 && loc[0] == 0 {
				name := pattern[i+loc[2] : i+loc[3]]
				b.WriteString("(?P<" + name + ">[^/]+)")
				i += loc[1]
				continue
			}
		}
		b.WriteByte(pattern[i])
		i++
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, errors.New("invalid path parameter pattern")
	}
	seen := map[string]bool{}
	for _, name := range re.SubexpNames() {
		if name == "" {
			continue
		}
		if seen[name] {
			return nil, errors.New("duplicate path parameter")
		}
		seen[name] = true
	}
	return re, nil
}
func Parameters(re *regexp.Regexp, path string) map[string]string {
	result := map[string]string{}
	if re == nil {
		return result
	}
	matches := re.FindStringSubmatch(path)
	if matches == nil {
		return result
	}
	for i, name := range re.SubexpNames() {
		if name != "" {
			result[name] = matches[i]
		}
	}
	return result
}
func HeaderValue(h Header, path string) (string, bool) {
	if h.RE == nil {
		return h.Value, true
	}
	if !h.RE.MatchString(path) {
		return "", false
	}
	params := Parameters(h.RE, path)
	v := strings.ReplaceAll(strings.ReplaceAll(h.Value, "{{", "\x00"), "}}", "\x01")
	v = placeholderRE.ReplaceAllStringFunc(v, func(token string) string { return params[token[1:len(token)-1]] })
	return strings.ReplaceAll(strings.ReplaceAll(v, "\x00", "{"), "\x01", "}"), true
}

func literalEnd(pattern string, i int) int {
	if pattern[i] == '\\' && i+1 < len(pattern) {
		if pattern[i+1] != 'Q' {
			return i + 2
		}
		end := strings.Index(pattern[i+2:], `\E`)
		if end < 0 {
			return len(pattern)
		}
		return i + end + 4
	}
	if pattern[i] != '[' {
		return i
	}
	end := i + 1
	for end < len(pattern) {
		if pattern[end] == '\\' {
			end += 2
			continue
		}
		if pattern[end] == ']' {
			return end + 1
		}
		end++
	}
	return len(pattern)
}
