package presentation

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/sglre6355/weather-lady/internal/domain"
	"github.com/sglre6355/weather-lady/internal/usecase"
)

const (
	defaultForecastURL      = "https://tenki.jp/#forecast-public-date-entry-2"
	defaultForecastSelector = "#forecast-map-wrap"
	latestForecastURL       = "https://tenki.jp/"
)

// WeatherBot wires Discord events to application use cases.
type WeatherBot struct {
	session        *discordgo.Session
	subscriptions  *usecase.SubscriptionManager
	weatherCapture usecase.ForecastCapture
}

// NewWeatherBot constructs a bot instance with all supporting services wired up.
func NewWeatherBot(session *discordgo.Session, subscriptions *usecase.SubscriptionManager, capture usecase.ForecastCapture) (*WeatherBot, error) {
	if session == nil {
		return nil, fmt.Errorf("discord session cannot be nil")
	}
	if subscriptions == nil {
		return nil, fmt.Errorf("subscription manager cannot be nil")
	}
	if capture == nil {
		return nil, fmt.Errorf("weather capture use case cannot be nil")
	}

	bot := &WeatherBot{
		session:        session,
		subscriptions:  subscriptions,
		weatherCapture: capture,
	}

	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteractionCreate)

	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	return bot, nil
}

// Start establishes the connection to Discord.
func (b *WeatherBot) Start() error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	log.Println("Weather bot is running!")
	return nil
}

// Stop releases all resources and stops scheduled deliveries.
func (b *WeatherBot) Stop() {
	if b.subscriptions != nil {
		b.subscriptions.Shutdown()
	}

	if b.session != nil {
		if err := b.session.Close(); err != nil {
			log.Printf("Error closing Discord session: %v", err)
		}
	}
}

func (b *WeatherBot) onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
}

func (b *WeatherBot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	switch i.ApplicationCommandData().Name {
	case "subscribe":
		b.handleSubscribeWeather(s, i)
	case "unsubscribe":
		b.handleUnsubscribeWeather(s, i)
	case "latest-forecast":
		b.handleCurrentWeather(s, i)
	}
}

// RegisterCommands recreates the slash commands used by the bot.
func (b *WeatherBot) RegisterCommands() error {
	existingCommands, err := b.session.ApplicationCommands(b.session.State.User.ID, "")
	if err != nil {
		log.Printf("Error getting existing commands: %v", err)
	} else {
		for _, cmd := range existingCommands {
			if err := b.session.ApplicationCommandDelete(b.session.State.User.ID, "", cmd.ID); err != nil {
				log.Printf("Error deleting command %s: %v", cmd.Name, err)
			}
		}
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "subscribe",
			Description: "Subscribe this channel to receive weather forecasts",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "time",
					Description: "Time to send weather forecast (format: HH:MM, e.g., 08:00)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message",
					Description: "Custom message to send with the weather forecast",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "url",
					Description: "URL to capture weather data from",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "selector",
					Description: "CSS selector for the element to capture",
					Required:    false,
				},
			},
		},
		{
			Name:        "unsubscribe",
			Description: "Unsubscribe this channel from weather forecasts",
		},
		{
			Name:        "latest-forecast",
			Description: "Show latest weather forecast",
		},
	}

	for _, cmd := range commands {
		if _, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", cmd); err != nil {
			return fmt.Errorf("failed to create command %s: %w", cmd.Name, err)
		}
	}

	return nil
}

func (b *WeatherBot) handleSubscribeWeather(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, option := range i.ApplicationCommandData().Options {
		opt := option
		options[opt.Name] = opt
	}

	timeOption, ok := options["time"]
	if !ok {
		b.respondWithError(s, i, "Time option is required")
		return
	}

	parsedTime, err := time.Parse("15:04", timeOption.StringValue())
	if err != nil {
		b.respondWithError(s, i, "Invalid time format. Please use HH:MM format (e.g., 08:00)")
		return
	}

	messageOption, ok := options["message"]
	if !ok {
		b.respondWithError(s, i, "Message option is required")
		return
	}

	url := defaultForecastURL
	if option, ok := options["url"]; ok && option.StringValue() != "" {
		url = option.StringValue()
	}

	selector := defaultForecastSelector
	if option, ok := options["selector"]; ok && option.StringValue() != "" {
		selector = option.StringValue()
	}

	sub := domain.Subscription{
		ChannelID:       i.ChannelID,
		GuildID:         i.GuildID,
		Time:            parsedTime,
		URL:             url,
		ElementSelector: selector,
		Message:         messageOption.StringValue(),
	}

	if err := b.subscriptions.Add(sub); err != nil {
		log.Printf("Failed to add subscription for channel %s: %v", i.ChannelID, err)
		b.respondWithError(s, i, "Failed to subscribe channel to weather forecasts")
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Successfully subscribed this channel to receive weather forecasts at %s daily from %s", timeOption.StringValue(), url),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *WeatherBot) handleUnsubscribeWeather(s *discordgo.Session, i *discordgo.InteractionCreate) {
	count := b.subscriptions.Remove(i.ChannelID)

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Removed %d weather forecast subscription(s) from this channel", count),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *WeatherBot) handleCurrentWeather(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	imageData, err := b.weatherCapture.CaptureForecast(ctx, latestForecastURL, defaultForecastSelector)
	if err != nil {
		if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "Failed to capture weather forecast",
		}); err != nil {
			log.Printf("Error sending followup: %v", err)
		}
		return
	}

	if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: "Here's the latest weather forecast! ☀️",
		Files: []*discordgo.File{
			{
				Name:        "weather_forecast.png",
				ContentType: "image/png",
				Reader:      bytes.NewReader(imageData),
			},
		},
	}); err != nil {
		log.Printf("Error sending followup: %v", err)
	}
}

func (b *WeatherBot) respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}
