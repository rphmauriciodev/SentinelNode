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
	viper.AddConfigPath(".")
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	err = viper.ReadInConfig()
	if err != nil {
		return
	}
	err = viper.Unmarshal(&config)
	return
}
