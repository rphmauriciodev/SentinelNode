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
	viper.SetDefault("MQTT_BROKER", "ssl://localhost:8883")
	viper.SetDefault("MQTT_CLIENT_ID", "ingestion-engine-01")
	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", "5435")
	viper.SetDefault("DB_USER", "sentinel_user")
	viper.SetDefault("DB_PASSWORD", "sentinel_password")
	viper.SetDefault("DB_NAME", "sentinel_db")
	viper.SetDefault("DB_SSLMODE", "disable")
	viper.SetDefault("MEDIA_GATEWAY_URL", "http://localhost:8080/record")

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
