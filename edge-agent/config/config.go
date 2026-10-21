package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	MQTTBroker   string `mapstructure:"MQTT_BROKER"`
	MQTTClientID string `mapstructure:"MQTT_CLIENT_ID"`
	DeviceID     string `mapstructure:"DEVICE_ID"`
}

func LoadConfig() (config Config, err error) {
	viper.SetDefault("MQTT_BROKER", "ssl://localhost:8883")
	viper.SetDefault("MQTT_CLIENT_ID", "edge-agent-01")
	viper.SetDefault("DEVICE_ID", "edge-agent-01")

	viper.AddConfigPath(".")
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	if err = viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return config, err
		}
		err = nil
	}
	err = viper.Unmarshal(&config)
	return config, err
}
