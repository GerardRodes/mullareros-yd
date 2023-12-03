package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

func serve(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	server := http.Server{
		Addr:        ":" + *argHttpPort,
		ReadTimeout: time.Second,
		Handler:     handler,
	}

	g.Go(func() error {
		log.Info().Msgf("listening at :%s", *argHttpPort)

		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("cannot init http server: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		log.Info().Msg("shutting down...")

		if err := ctx.Err(); !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("ctx error")
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown http server: %w", err)
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("group err: %w", err)
	}

	return nil
}
