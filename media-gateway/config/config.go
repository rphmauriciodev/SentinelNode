package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Port             string `mapstructure:"PORT"`
	RecordingsDir    string `mapstructure:"RECORDINGS_DIR"`
	DefaultCameraURL string `mapstructure:"DEFAULT_CAMERA_URL"`
	DefaultDeviceID  string `mapstructure:"DEFAULT_DEVICE_ID"`
	ToleranceSeconds int    `mapstructure:"TOLERANCE_SECONDS"`
}

func LoadConfig() (config Config, err error) {
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("RECORDINGS_DIR", "recordings")
	viper.SetDefault("DEFAULT_DEVICE_ID", "esp32-cam-01")
	viper.SetDefault("TOLERANCE_SECONDS", 30)

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
