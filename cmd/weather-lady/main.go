package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/caarlos0/env/v11"
	"github.com/sglre6355/weather-lady/internal/domain"
	"github.com/sglre6355/weather-lady/internal/infrastructure"
	"github.com/sglre6355/weather-lady/internal/infrastructure/database"
	"github.com/sglre6355/weather-lady/internal/presentation"
	"github.com/sglre6355/weather-lady/internal/usecase"
)

type config struct {
	DiscordToken      string `env:"DISCORD_TOKEN,required"`
	DatabaseURL       string `env:"DATABASE_URL,required"`
	WebCaptureAddress string `env:"WEB_CAPTURE_ADDRESS"    envDefault:"localhost:50051"`
}

func main() {
	cfg, err := env.ParseAs[config]()
	if err != nil {
		log.Fatalf("failed to parse environment variables: %v", err)
	}

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to access database handle: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("Error closing database connection: %v", err)
		}
	}()

	subscriptionStore := database.NewSubscriptionStore(db)
	if err := subscriptionStore.AutoMigrate(context.Background()); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}

	weatherService, err := infrastructure.NewWeatherService(cfg.WebCaptureAddress)
	if err != nil {
		log.Fatalf("Failed to create weather service: %v", err)
	}
	defer func() {
		if err := weatherService.Close(); err != nil {
			log.Printf("Error closing weather service: %v", err)
		}
	}()

	weatherUsecase := usecase.NewWeatherUsecase(weatherService)

	session, err := discordgo.New("Bot " + cfg.DiscordToken)
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
		usecase.WithSubscriptionStore(subscriptionStore),
		usecase.WithSubscriptionErrorHandler(
			func(sub domain.Subscription, stage usecase.SubscriptionErrorStage, err error) {
				log.Printf(
					"Subscription delivery failed (channel=%s stage=%s): %v",
					sub.ChannelID,
					stage,
					err,
				)
			},
		),
	)

	if err := subscriptionManager.LoadExisting(context.Background()); err != nil {
		if err := session.Close(); err != nil {
			log.Printf("Error closing Discord session after subscription restore failure: %v", err)
		}
		if err := weatherService.Close(); err != nil {
			log.Printf("Error closing weather service after subscription restore failure: %v", err)
		}
		log.Fatalf("Failed to restore saved subscriptions: %v", err)
	}

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
