package config

import (
	"log"

	"github.com/caarlos0/env/v11"
)

type Conf struct {
	Server ConfServer
	DB     ConfDB
}

type ConfServer struct {
	URL    string `env:"PADDOCK_URL,required"`
	Port   string `env:"PADDOCK_API_PORT,required"`
	ApiKey string `env:"PADDOCK_API_KEY,required"`
}

type ConfDB struct {
	Path string `env:"PADDOCK_DB_PATH" envDefault:"./data/paddock.db"`
}

func New() *Conf {
	var c Conf
	if err := env.Parse(&c); err != nil {
		log.Fatalf("Failed to decode: %s", err)
	}

	return &c
}
