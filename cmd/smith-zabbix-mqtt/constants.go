package main

const (
	Version    = "0.0.6"
	envLocal   = "local"
	envDev     = "dev"
	Driver     = "smith-zabbix-mqtt"
	AppNameRus = "Zabbix2MQTT"
	AppNameEng = "Zabbix2MQTT"

	DEFAULT_CONFIG_PATH = "/etc/smith-zabbix-mqtt/config.yaml"
	MOSQUITTO_SOCK_FILE = "/var/run/mosquitto/mosquitto.sock"
	DEFAULT_BROKER_URL  = "tcp://localhost:1883"

	SEVERITY_UNDEFINED = -1
)

// MQTT
const (
	QOS         = 1
	META_SUFFIX = "/meta"
)

// Controls виртуальных устройств
const (
	CTRL_UPTIME         = "uptime"         // Имя для контрола uptime
	CTRL_TOTAL_TRIGGERS = "total_triggers" // Имя для контрола total_triggers
)
