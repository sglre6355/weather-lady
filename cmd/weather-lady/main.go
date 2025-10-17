package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"

	"github.com/sglre6355/weather-lady/internal/domain"
	"github.com/sglre6355/weather-lady/internal/infrastructure"
	"github.com/sglre6355/weather-lady/internal/presentation"
	"github.com/sglre6355/weather-lady/internal/usecase"
)

func main() {
	discordToken := os.Getenv("DISCORD_TOKEN")
	if discordToken == "" {
		log.Fatal("DISCORD_TOKEN environment variable is required")
	}

	grpcAddress := os.Getenv("WEB_CAPTURE_ADDRESS")
	if grpcAddress == "" {
		grpcAddress = "localhost:50051"
	}

	weatherService, err := infrastructure.NewWeatherService(grpcAddress)
	if err != nil {
		log.Fatalf("Failed to create weather service: %v", err)
	}
	defer func() {
		if err := weatherService.Close(); err != nil {
			log.Printf("Error closing weather service: %v", err)
		}
	}()

	weatherUsecase := usecase.NewWeatherUsecase(weatherService)

	session, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		if err := weatherService.Close(); err != nil {
			log.Printf("Error closing weather service after Discord failure: %v", err)
		}
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	forecastSender := presentation.NewDiscordForecastSender(session)

	subscriptionManager := usecase.NewSubscriptionManager(
		weatherUsecase,
		forecastSender,
		usecase.WithSubscriptionErrorHandler(func(sub domain.Subscription, stage usecase.SubscriptionErrorStage, err error) {
			log.Printf("Subscription delivery failed (channel=%s stage=%s): %v", sub.ChannelID, stage, err)
		}),
	)

	bot, err := presentation.NewWeatherBot(session, subscriptionManager, weatherUsecase)
	if err != nil {
		if err := session.Close(); err != nil {
			log.Printf("Error closing Discord session after bot initialisation failure: %v", err)
		}
		if err := weatherService.Close(); err != nil {
			log.Printf("Error closing weather service after bot initialisation failure: %v", err)
		}
		log.Fatalf("Failed to create bot: %v", err)
	}

	if err := bot.Start(); err != nil {
		bot.Stop()
		if err := weatherService.Close(); err != nil {
			log.Printf("Error closing weather service after bot start failure: %v", err)
		}
		log.Fatalf("Failed to start bot: %v", err)
	}

	if err := bot.RegisterCommands(); err != nil {
		bot.Stop()
		if err := weatherService.Close(); err != nil {
			log.Printf("Error closing weather service after command registration failure: %v", err)
		}
		log.Fatalf("Failed to register commands: %v", err)
	}

	log.Println("Weather Lady bot is now running. Press CTRL-C to exit.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down...")
	bot.Stop()
	log.Println("Bot stopped successfully.")
}
