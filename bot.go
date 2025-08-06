package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type WeatherBot struct {
	Session        *discordgo.Session
	subscriptions  map[string][]Subscription
	weatherService *WeatherService
	mu             sync.RWMutex
}

type Subscription struct {
	ChannelID       string
	GuildID         string
	Time            time.Time
	Ticker          *time.Ticker
	StopChan        chan struct{}
	URL             string
	ElementSelector string
	Message         string
}

func NewWeatherBot(token string, grpcAddress string) (*WeatherBot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	weatherService, err := NewWeatherService(grpcAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create weather service: %w", err)
	}

	bot := &WeatherBot{
		Session:        session,
		subscriptions:  make(map[string][]Subscription),
		weatherService: weatherService,
	}

	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteractionCreate)

	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	return bot, nil
}

func (b *WeatherBot) Start() error {
	err := b.Session.Open()
	if err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	log.Println("Weather bot is running!")
	return nil
}

func (b *WeatherBot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, subs := range b.subscriptions {
		for _, sub := range subs {
			if sub.Ticker != nil {
				sub.Ticker.Stop()
			}
			if sub.StopChan != nil {
				close(sub.StopChan)
			}
		}
	}

	if b.weatherService != nil {
		if err := b.weatherService.Close(); err != nil {
			log.Printf("Error closing weather service: %v", err)
		}
	}

	if err := b.Session.Close(); err != nil {
		log.Printf("Error closing Discord session: %v", err)
	}
}

func (b *WeatherBot) onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
}

func (b *WeatherBot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "subscribe":
		b.handleSubscribeWeather(s, i)
	case "unsubscribe":
		b.handleUnsubscribeWeather(s, i)
	case "current-weather":
		b.handleCurrentWeather(s, i)
	}
}

func (b *WeatherBot) RegisterCommands() error {
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
			Name:        "current-weather",
			Description: "Show current weather forecast",
			Options: []*discordgo.ApplicationCommandOption{
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
	}

	for _, cmd := range commands {
		_, err := b.Session.ApplicationCommandCreate(b.Session.State.User.ID, "", cmd)
		if err != nil {
			return fmt.Errorf("failed to create command %s: %w", cmd.Name, err)
		}
	}

	return nil
}

func (b *WeatherBot) handleSubscribeWeather(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	timeStr := options[0].StringValue()

	parsedTime, err := time.Parse("15:04", timeStr)
	if err != nil {
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Invalid time format. Please use HH:MM format (e.g., 08:00)",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		}); err != nil {
			log.Printf("Error responding to interaction: %v", err)
		}
		return
	}

	var url, selector, message string
	url = "https://tenki.jp/#forecast-public-date-entry-2"
	selector = "#forecast-map-wrap"

	for _, option := range options {
		switch option.Name {
		case "message":
			message = option.StringValue()
		case "url":
			if option.StringValue() != "" {
				url = option.StringValue()
			}
		case "selector":
			if option.StringValue() != "" {
				selector = option.StringValue()
			}
		}
	}

	channelID := i.ChannelID
	guildID := i.GuildID

	err = b.AddSubscription(channelID, guildID, parsedTime, url, selector, message)
	if err != nil {
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Failed to subscribe channel to weather forecasts",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		}); err != nil {
			log.Printf("Error responding to interaction: %v", err)
		}
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Successfully subscribed this channel to receive weather forecasts at %s daily from %s", timeStr, url),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *WeatherBot) handleUnsubscribeWeather(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := i.ChannelID

	count := b.RemoveSubscriptions(channelID)

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
	options := i.ApplicationCommandData().Options

	var url, selector string
	url = "https://tenki.jp/#forecast-public-date-entry-2"
	selector = "#forecast-map-wrap"

	for _, option := range options {
		switch option.Name {
		case "url":
			if option.StringValue() != "" {
				url = option.StringValue()
			}
		case "selector":
			if option.StringValue() != "" {
				selector = option.StringValue()
			}
		}
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if b.weatherService == nil {
		if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "Weather service not available",
		}); err != nil {
			log.Printf("Error sending followup: %v", err)
		}
		return
	}

	imageData, err := b.weatherService.CaptureWeatherForecast(ctx, url, selector)
	if err != nil {
		if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "Failed to capture weather forecast",
		}); err != nil {
			log.Printf("Error sending followup: %v", err)
		}
		return
	}

	if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: "Current weather forecast:",
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

func (b *WeatherBot) AddSubscription(channelID, guildID string, scheduledTime time.Time, url, elementSelector, message string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	stopChan := make(chan struct{})

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(),
		scheduledTime.Hour(), scheduledTime.Minute(), 0, 0, now.Location())

	var nextRun time.Time
	if today.After(now) {
		nextRun = today
	} else {
		nextRun = today.Add(24 * time.Hour)
	}

	subscription := Subscription{
		ChannelID:       channelID,
		GuildID:         guildID,
		Time:            scheduledTime,
		StopChan:        stopChan,
		URL:             url,
		ElementSelector: elementSelector,
		Message:         message,
	}

	b.subscriptions[channelID] = append(b.subscriptions[channelID], subscription)

	go b.scheduleWeatherUpdate(channelID, nextRun, stopChan, url, elementSelector, message)

	return nil
}

func (b *WeatherBot) RemoveSubscriptions(channelID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, exists := b.subscriptions[channelID]
	if !exists {
		return 0
	}

	count := len(subs)

	for _, sub := range subs {
		if sub.Ticker != nil {
			sub.Ticker.Stop()
		}
		if sub.StopChan != nil {
			close(sub.StopChan)
		}
	}

	delete(b.subscriptions, channelID)
	return count
}

func (b *WeatherBot) scheduleWeatherUpdate(channelID string, nextRun time.Time, stopChan chan struct{}, url, elementSelector, message string) {
	timer := time.NewTimer(time.Until(nextRun))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			b.sendWeatherForecast(channelID, url, elementSelector, message)
			timer.Reset(24 * time.Hour)
		case <-stopChan:
			return
		}
	}
}
