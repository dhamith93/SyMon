package stringops

import (
	"strings"
)

// Reverse reverses string array
func Reverse(s []string) []string {
	for i := 0; i < len(s)/2; i++ {
		j := len(s) - i - 1
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// StringArrToJSONArr returns json arr of given string arr
func StringArrToJSONArr(data []string) string {
	var sb strings.Builder
	sb.WriteString("[")

	for i, s := range data {
		sb.WriteString(s)

		if i < (len(data) - 1) {
			sb.WriteString(",")
		}
	}

	sb.WriteString("]")
	return sb.String()
}
