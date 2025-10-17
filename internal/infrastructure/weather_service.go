package infrastructure

import (
	"context"
	"fmt"

	web_capture "github.com/sglre6355/weather-lady/gen/web_capture/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// WeatherService wraps the gRPC client used to capture weather forecasts.
type WeatherService struct {
	grpcClient web_capture.WebCaptureServiceClient
	grpcConn   *grpc.ClientConn
}

// NewWeatherService connects to the remote capture service and returns a usable client wrapper.
func NewWeatherService(grpcAddress string) (*WeatherService, error) {
	conn, err := grpc.NewClient(
		grpcAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	client := web_capture.NewWebCaptureServiceClient(conn)

	return &WeatherService{
		grpcClient: client,
		grpcConn:   conn,
	}, nil
}

// Close tears down the underlying gRPC connection.
func (ws *WeatherService) Close() error {
	if ws.grpcConn != nil {
		return ws.grpcConn.Close()
	}
	return nil
}

// CaptureWeatherForecast captures the requested element and returns the rendered binary contents.
func (ws *WeatherService) CaptureWeatherForecast(
	ctx context.Context,
	url, elementSelector string,
) ([]byte, error) {
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
