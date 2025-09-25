package main

import (
	"fmt"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	QOS = 1
)

func setupMQTT() *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()

	opts.AddBroker(cfg.Mqtt.Address)
	opts.SetClientID(cfg.Mqtt.ClientID)

	if cfg.Mqtt.Auth {
		opts.SetUsername(cfg.Mqtt.Login)
		opts.SetPassword(cfg.Mqtt.Password)
	}

	opts.SetOrderMatters(false)       // Allow out of order messages (use this option unless in order delivery is essential)
	opts.ConnectTimeout = time.Second // Minimal delays on connect
	opts.WriteTimeout = time.Second   // Minimal delays on writes
	opts.KeepAlive = 10               // Keepalive every 10 seconds so we quickly detect network outages
	opts.PingTimeout = time.Second    // local broker so response should be quick

	// Automate connection management (will keep trying to connect and will reconnect if network drops)
	opts.ConnectRetry = false
	opts.AutoReconnect = true

	opts.DefaultPublishHandler = func(_ mqtt.Client, msg mqtt.Message) {
		fmt.Printf("UNEXPECTED MESSAGE: %s\n", msg)
	}

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		log.Error(
			"MQTT",
			slog.String("connection lost", err.Error()),
		)
	}

	opts.OnConnect = func(c mqtt.Client) {
		log.Info("MQTT connection established")
	}

	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		log.Warn("MQTT attempting to reconnect")
	}

	return opts
}
