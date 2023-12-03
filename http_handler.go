package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/rs/zerolog/log"

	_ "embed"
)

//go:embed index.html
var indexHTML []byte

var handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	log.Print("new request", r.URL.String())
	if len(r.URL.String()) < 1 {
		// todo:
		return
	}

	var err error
	switch {
	case strings.HasSuffix(r.URL.Path, "favicon.ico"):
		w.WriteHeader(http.StatusNotFound)
		return
	case r.URL.Path == "/state":
		err = handlerState(w, r)
	case r.URL.Path == "/clear":
		err = handlerClear(w, r)
	case strings.HasPrefix(r.URL.Path, "/download/"):
		http.ServeFile(w, r, filepath.Join(*argOutDir, r.URL.Path[len("/download/"):]))
		return
	case strings.HasPrefix(r.URL.Path, "/yt-dlp/"):
		err = handlerDownload(w, r)
	default:
		w.WriteHeader(http.StatusOK)
		w.Header().Add("content-type", "text/html")
		_, _ = w.Write(indexHTML)
		return
	}

	if err != nil && errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("handler")
	}
})

func handlerState(w http.ResponseWriter, r *http.Request) error {
	globalState.l.RLock()
	defer globalState.l.RUnlock()
	spew.Fdump(w, globalState.records)
	return nil
}

func handlerClear(w http.ResponseWriter, r *http.Request) (outErr error) {
	dirs, err := os.ReadDir(*argOutDir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}

		if globalState.Has(d.Name()) {
			log.Printf("skipping %s", d.Name())
			continue
		}

		log.Printf("deleting %s", d.Name())
		_, _ = w.Write([]byte(d.Name()))
		_, _ = w.Write([]byte("\n"))
		if err := os.RemoveAll(filepath.Join(*argOutDir, d.Name())); err != nil {
			log.Error().Err(err).Msg("remove all")
		}
	}
	return
}

func handlerDownload(w http.ResponseWriter, r *http.Request) (outErr error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("SSE not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")

	ctx := r.Context()

	durl := r.URL.String()[len("/yt-dlp/"):]

	id, err := GetID(ctx, durl)
	if err != nil {
		return fmt.Errorf("get id: %w", err)
	}
	log.Printf("got id: %s", id)

	var rec *Record
	if r := globalState.Get(id); r != nil {
		log.Print("found on state")
		rec = r
	} else {
		rec = &Record{
			ID: id,
		}
		globalState.Put(rec)
		log.Print("created record")

		go func() {
			defer globalState.Delete(id)

			if err := Download(durl, rec); err != nil {
				log.Error().Err(err).Msg("download")
			}
		}()
	}

	l := make(chan string)
	rec.AddListener(l)
	defer rec.RemoveListener(l)

	if e := rec.LastLog(); e != "" {
		e += "\n\n"
		_, err = w.Write(append(
			[]byte("data: "),
			[]byte(e)...,
		))
		if err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("ctx: %w", err)
			}
		case e, ok := <-l:
			if !ok {
				e = "finished"
			}

			e += "\n\n"
			_, err = w.Write(append(
				[]byte("data: "),
				[]byte(e)...,
			))
			if err != nil {
				return fmt.Errorf("write: %w", err)
			}

			flusher.Flush()
			if !ok {
				_, err = w.Write([]byte(`
event: end
data: ` + rec.Filepath + "\n\n"))

				if err != nil {
					return fmt.Errorf("write: %w", err)
				}

				return
			}
		}
	}
}
