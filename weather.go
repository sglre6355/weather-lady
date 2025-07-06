package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	webcapture "github.com/sglre6355/weather-lady/webcapture"
)

type WeatherService struct {
	grpcClient webcapture.WebCaptureServiceClient
	grpcConn   *grpc.ClientConn
}

func NewWeatherService(grpcAddress string) (*WeatherService, error) {
	conn, err := grpc.NewClient(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	client := webcapture.NewWebCaptureServiceClient(conn)

	return &WeatherService{
		grpcClient: client,
		grpcConn:   conn,
	}, nil
}

func (ws *WeatherService) Close() error {
	if ws.grpcConn != nil {
		return ws.grpcConn.Close()
	}
	return nil
}

func (ws *WeatherService) CaptureWeatherForecast(ctx context.Context) ([]byte, error) {
	req := &webcapture.CaptureElementRequest{
		Url:             "https://tenki.jp/#forecast-public-date-entry-2",
		ElementSelector: "#forecast-map-wrap",
		ImageFormat:     webcapture.ImageFormat_IMAGE_FORMAT_PNG,
	}

	resp, err := ws.grpcClient.CaptureElement(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to capture weather forecast: %w", err)
	}

	return resp.ImageData, nil
}

func (b *WeatherBot) sendWeatherForecast(channelID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if b.weatherService == nil {
		log.Printf("Weather service not initialized for channel %s", channelID)
		return
	}

	imageData, err := b.weatherService.CaptureWeatherForecast(ctx)
	if err != nil {
		log.Printf("Failed to capture weather forecast for channel %s: %v", channelID, err)
		return
	}

	_, err = b.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: "🌤️ Here's your daily weather forecast!",
		Files: []*discordgo.File{
			{
				Name:   "weather_forecast.png",
				Reader: io.NopCloser(bytes.NewReader(imageData)),
			},
		},
	})

	if err != nil {
		log.Printf("Failed to send weather forecast to channel %s: %v", channelID, err)
	}
}
