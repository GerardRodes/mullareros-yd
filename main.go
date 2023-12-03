package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	argHttpPort        = flag.String("http-port", "5000", "")
	argOutDir          = flag.String("out-dir", "/tmp/yt-dlp", "")
	argDownloadThreads = flag.Int("download-threads", 2, "")
	argMaxAge          = flag.Duration("max-age", time.Hour*4, "")
	isProd             = false
)

func init() {
	time.Local = time.UTC
	isProd = !strings.Contains(os.Args[0], "go-build")

	_ = os.Mkdir(*argOutDir, os.ModePerm)
}

func main() {
	if isProd {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Print("hi")
	defer log.Print("bye")

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		t := time.NewTimer(time.Second * 10)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				clearOldFiles()
			}
		}
	}()

	if err := serve(ctx); err != nil {
		log.Error().Err(err).Msg("group error")
	}

	wg.Wait()
}

func clearOldFiles() {
	dirs, err := os.ReadDir(*argOutDir)
	if err != nil {
		log.Error().Err(err).Msg("read dir")
		return
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}

		if globalState.Has(d.Name()) {
			continue
		}

		info, err := d.Info()
		if err != nil {
			log.Error().Err(err).Msg("get file info")
			continue
		}

		if info.ModTime().After(time.Now().Add(*argMaxAge * -1)) {
			continue
		}

		log.Printf("deleting %s", d.Name())
		if err := os.RemoveAll(filepath.Join(*argOutDir, d.Name())); err != nil {
			log.Error().Err(err).Msg("remove all")
		}
	}
}
