package presentation

import (
	"bytes"
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// DiscordForecastSender pushes weather snapshots to a Discord channel.
type DiscordForecastSender struct {
	session *discordgo.Session
}

// NewDiscordForecastSender wires a Discord session to the forecast dispatch interface expected by the use case layer.
func NewDiscordForecastSender(session *discordgo.Session) *DiscordForecastSender {
	return &DiscordForecastSender{session: session}
}

// SendForecast posts the supplied image and message to the target Discord channel.
func (s *DiscordForecastSender) SendForecast(
	ctx context.Context,
	channelID string,
	imageData []byte,
	message string,
) error {
	if s.session == nil {
		return fmt.Errorf("discord session is not initialised")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	payload := &discordgo.MessageSend{
		Content: message,
		Files: []*discordgo.File{
			{
				Name:        "weather_forecast.png",
				ContentType: "image/png",
				Reader:      bytes.NewReader(imageData),
			},
		},
	}

	if _, err := s.session.ChannelMessageSendComplex(channelID, payload); err != nil {
		return fmt.Errorf("failed to send forecast message: %w", err)
	}

	return nil
}
