package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Fadil-Tao/paddock/internal/runner"
	"github.com/Fadil-Tao/paddock/internal/transport"
	"github.com/Fadil-Tao/paddock/internal/transport/handlers"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
)


func main(){ 
	c, err := client.New(client.FromEnv)
	if err != nil {
			log.Fatal().Err(err).Msg("failed to connect to Docker")
	}

	d := runner.NewDockerRunner(c)
	h :=  handlers.NewSandboxHandler(d)
	router := transport.NewHttpRouter(h)

	server := &http.Server{
			Addr:    ":8000",
			Handler: router.Route(),
	}

	go func() {
			log.Info().Msg("server started on :8000")
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Error().Err(err).Msg("server error")
			}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("shutdown error")
	}
	log.Info().Msg("graceful shutdown complete")
}