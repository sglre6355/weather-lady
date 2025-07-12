package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
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

	bot, err := NewWeatherBot(discordToken, grpcAddress)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	err = bot.Start()
	if err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	err = bot.RegisterCommands()
	if err != nil {
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
