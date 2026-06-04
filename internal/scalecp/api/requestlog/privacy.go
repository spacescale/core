package requestlog

import (
	"fmt"
	"strings"
)

const (
	maxUserAgentLogLen  = 255
	maxPanicValueLogLen = 200
)

func userAgentLogAttr(rawUserAgent string) (string, string, bool) {
	ua := strings.TrimSpace(rawUserAgent)
	if ua == "" {
		return "", "", false
	}
	return "user_agent", truncateLogString(ua, maxUserAgentLogLen), true
}

func truncateLogString(input string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(input) <= maxLen {
		return input
	}
	return input[:maxLen]
}

func panicValueLogValue(recovered any) string {
	return truncateLogString(fmt.Sprint(recovered), maxPanicValueLogLen)
}
