package config

import "github.com/spf13/viper"

type Config struct {
	MQTTBroker      string `mapstructure:"MQTT_BROKER"`
	MQTTClientID    string `mapstructure:"MQTT_CLIENT_ID"`
	DBHost          string `mapstructure:"DB_HOST"`
	DBPort          string `mapstructure:"DB_PORT"`
	DBUser          string `mapstructure:"DB_USER"`
	DBPassword      string `mapstructure:"DB_PASSWORD"`
	DBName          string `mapstructure:"DB_NAME"`
	DBSslMode       string `mapstructure:"DB_SSLMODE"`
	MediaGatewayURL string `mapstructure:"MEDIA_GATEWAY_URL"`
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
