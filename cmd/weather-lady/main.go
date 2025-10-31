package main

import (
	"context"
	"log/slog"
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
	DatabaseDSN       string `env:"DATABASE_DSN,required"`
	WebCaptureAddress string `env:"WEB_CAPTURE_ADDRESS"    envDefault:"localhost:50051"`
}

func run() int {
	cfg, err := env.ParseAs[config]()
	if err != nil {
		slog.Error("failed to parse environment variables", slog.Any("error", err))
		return 1
	}

	db, err := database.Open(cfg.DatabaseDSN)
	if err != nil {
		slog.Error("failed to connect to database", slog.Any("error", err))
		return 1
	}

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("failed to access database handle", slog.Any("error", err))
		return 1
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			slog.Error("failed to close database connection", slog.Any("error", err))
		}
	}()

	subscriptionStore := database.NewSubscriptionStore(db)
	if err := subscriptionStore.AutoMigrate(context.Background()); err != nil {
		slog.Error("failed to run database migrations", slog.Any("error", err))
		return 1
	}

	weatherService, err := infrastructure.NewWeatherService(cfg.WebCaptureAddress)
	if err != nil {
		slog.Error("failed to create weather service", slog.Any("error", err))
		return 1
	}
	defer func() {
		if err := weatherService.Close(); err != nil {
			slog.Error("failed to close weather service connection", slog.Any("error", err))
		}
	}()

	weatherUsecase := usecase.NewWeatherUsecase(weatherService)

	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		slog.Error("failed to create Discord session", slog.Any("error", err))
		return 1
	}
	defer func() {
		if err := session.Close(); err != nil {
			slog.Error(
				"failed to close Discord session",
				"error",
				err,
			)
		}
	}()

	forecastSender := presentation.NewDiscordForecastSender(session)

	subscriptionManager := usecase.NewSubscriptionManager(
		weatherUsecase,
		forecastSender,
		usecase.WithSubscriptionStore(subscriptionStore),
		usecase.WithSubscriptionErrorHandler(
			func(sub domain.Subscription, stage usecase.SubscriptionErrorStage, err error) {
				slog.Error(
					"subscription delivery failed",
					slog.String("channel", sub.ChannelID),
					slog.Any("stage", stage),
					slog.Any("error", err),
				)
			},
		),
	)

	if err := subscriptionManager.LoadExisting(context.Background()); err != nil {
		slog.Error("failed to restore saved subscriptions", "error", err)
		return 1
	}

	bot, err := presentation.NewWeatherBot(session, subscriptionManager, weatherUsecase)
	if err != nil {
		slog.Error("failed to create bot", "error", err)
		return 1
	}

	if err := bot.Start(); err != nil {
		bot.Stop()
		slog.Error("failed to start bot", "error", err)
		return 1
	}

	if err := bot.RegisterCommands(); err != nil {
		bot.Stop()
		slog.Error("failed to register commands", "error", err)
		return 1
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Termination signal received, shutting down...")
	bot.Stop()
	slog.Info("Bot successfully terminated")

	return 0
}

func main() {
	os.Exit(run())
}
