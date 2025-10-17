package usecase

import (
	"context"
)

// ForecastProvider captures weather snapshots as raw bytes.
type ForecastProvider interface {
	CaptureWeatherForecast(ctx context.Context, url, elementSelector string) ([]byte, error)
}

// WeatherUsecase exposes weather-oriented application actions.
type WeatherUsecase struct {
	provider ForecastProvider
}

// NewWeatherUsecase wraps the provider to expose higher-level operations.
func NewWeatherUsecase(provider ForecastProvider) *WeatherUsecase {
	return &WeatherUsecase{provider: provider}
}

// CaptureForecast requests a rendered forecast from the provider.
func (u *WeatherUsecase) CaptureForecast(
	ctx context.Context,
	url, elementSelector string,
) ([]byte, error) {
	return u.provider.CaptureWeatherForecast(ctx, url, elementSelector)
}
