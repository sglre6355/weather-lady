package domain

import "time"

// Subscription represents a daily forecast delivery configuration for a Discord channel.
type Subscription struct {
	ChannelID       string
	GuildID         string
	Time            time.Time
	URL             string
	ElementSelector string
	Message         string
}
