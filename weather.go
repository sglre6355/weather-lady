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

	web_capture "github.com/sglre6355/weather-lady/gen/proto/web_capture/v1"
)

type WeatherService struct {
	grpcClient web_capture.WebCaptureServiceClient
	grpcConn   *grpc.ClientConn
}

func NewWeatherService(grpcAddress string) (*WeatherService, error) {
	conn, err := grpc.NewClient(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	client := web_capture.NewWebCaptureServiceClient(conn)

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

func (ws *WeatherService) CaptureWeatherForecast(ctx context.Context, url, elementSelector string) ([]byte, error) {
	req := &web_capture.CaptureElementRequest{
		Url:             url,
		ElementSelector: elementSelector,
		ImageFormat:     web_capture.ImageFormat_IMAGE_FORMAT_PNG,
	}

	resp, err := ws.grpcClient.CaptureElement(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to capture weather forecast: %w", err)
	}

	return resp.ImageData, nil
}

func (b *WeatherBot) sendWeatherForecast(channelID, url, elementSelector, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if b.weatherService == nil {
		log.Printf("Weather service not initialized for channel %s", channelID)
		return
	}

	imageData, err := b.weatherService.CaptureWeatherForecast(ctx, url, elementSelector)
	if err != nil {
		log.Printf("Failed to capture weather forecast for channel %s: %v", channelID, err)
		return
	}

	_, err = b.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: message,
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
