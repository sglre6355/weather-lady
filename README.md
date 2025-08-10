# Weather Lady Discord Bot

A Discord bot that sends periodic weather forecast images to subscribed channels using a gRPC web capture service.

## Features

- `/subscribe` command to subscribe a channel for weather forecasts
- `/unsubscribe` command to remove all subscriptions from a channel
- `/latest-forecast` command to get current weather forecast on-demand
- Scheduled daily weather updates at specified times
- Captures weather forecast images from configurable URLs with custom CSS selectors
- Supports multiple subscriptions per channel (e.g., morning and evening forecasts)
- Default captures from tenki.jp weather forecast

## Setup

### Prerequisites

- Go 1.24.4 or later
- [Buf CLI](https://docs.buf.build/installation) for protobuf generation

### Build

1. Generate protobuf code:
   ```bash
   buf generate
   ```

2. Build the bot:
   ```bash
   go build -o weather-lady
   ```

### Run

1. Set environment variables:
   ```bash
   export DISCORD_TOKEN="your_discord_bot_token_here"
   export WEB_CAPTURE_ADDRESS="localhost:50051"  # Optional, defaults to localhost:50051
   ```

2. Start your gRPC web capture service on the specified address

3. Run the bot:
   ```bash
   ./weather-lady
   ```

## Commands

- **`/subscribe`**: Subscribe the current channel to receive weather forecasts
  - `time`: Time to send forecast (format: HH:MM, e.g., "08:00")
  - `message`: Custom message to send with the weather forecast
  - `url` (optional): Custom URL to capture weather data from
  - `selector` (optional): Custom CSS selector for the element to capture
  
- **`/unsubscribe`**: Remove all weather forecast subscriptions from the current channel

- **`/latest-forecast`**: Get the current weather forecast immediately (no parameters required)

## Usage Example

1. Run `/subscribe time:08:00 message:🌤️ Good morning! Here's your daily weather forecast!` to get weather forecasts every day at 8:00 AM
2. Run `/subscribe time:20:00 message:🌙 Evening weather update!` to also get weather forecasts at 8:00 PM  
3. Run `/subscribe time:12:00 message:☀️ Noon weather check! url:https://example.com/weather selector:.weather-map` for custom weather source
4. Run `/latest-forecast` to get the current weather forecast immediately
5. Run `/unsubscribe` to stop all weather updates for the channel

## Technical Details

- Uses discordgo library for Discord interactions
- Default captures weather forecast images from https://tenki.jp/ 
- Default targets the `#forecast-map-wrap` element for image capture
- Supports custom URLs and CSS selectors for flexible weather data sources
- Stores subscriptions in memory (not persistent across restarts)
- Uses gRPC to communicate with web capture service
- Built with buf for protobuf code generation
