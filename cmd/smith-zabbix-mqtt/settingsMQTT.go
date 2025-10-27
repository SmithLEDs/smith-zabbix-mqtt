package main

import (
	"log/slog"
	"time"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func setupMQTT(cfg *config.Config, log *slog.Logger) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Mqtt.Address).
		SetClientID(cfg.Mqtt.ClientID).
		SetOrderMatters(false).         // Allow out of order messages (use this option unless in order delivery is essential)
		SetConnectTimeout(time.Second). // Minimal delays on connect
		SetWriteTimeout(time.Second).   // Minimal delays on writes
		SetKeepAlive(10 * time.Second). // Keepalive every 10 seconds so we quickly detect network outages
		SetPingTimeout(time.Second).    // local broker so response should be quick
		SetConnectRetry(false).         // Automate connection management (will keep trying to connect and will reconnect if network drops)
		SetAutoReconnect(true)

	if cfg.Mqtt.Auth {
		opts.SetUsername(cfg.Mqtt.Login).SetPassword(cfg.Mqtt.Password)
	}

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		log.Error(
			"MQTT connection lost",
			slog.String("error", err.Error()),
		)
	}

	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		log.Warn("MQTT attempting to reconnect")
	}

	return opts
}
