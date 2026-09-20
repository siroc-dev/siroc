package update

import "strings"

const DefaultChannel = "https://get.siroc.dev"

func ResolveChannel(channel string) string {
	channel = strings.TrimRight(strings.TrimSpace(channel), "/")
	if channel == "" {
		return DefaultChannel
	}
	return channel
}
