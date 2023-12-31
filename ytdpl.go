package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/rs/zerolog/log"

	"golang.org/x/sync/errgroup"
)

var rePrefix = regexp.MustCompile(`^\[\w*\]\s`)

func Download(durl string, rec *Record) error {
	log.Printf("downloading: %s", durl)

	args := []string{
		durl,
		"--newline",
		"--concurrent-fragments", fmt.Sprintf("%d", *argDownloadThreads),
		"--restrict-filenames",
		"--trim-filenames", "150",
		"--embed-subs",
		"--write-subs",
		"--sub-langs", "en.*",
		"--write-auto-subs",
		"--no-playlist",
		"--output", path.Join(*argOutDir, "%(id)s", "%(title).200B.%(ext)s"),
	}

	{
		purl, _ := url.Parse(durl)
		if strings.HasSuffix(purl.Host, "mitele.es") {
			args = append(args, "--fixup", "never")
		}
	}

	cmd := exec.Command("yt-dlp", args...)

	rc, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}

	errc, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}

	g, _ := errgroup.WithContext(context.Background())

	started := make(chan struct{})
	g.Go(func() error {
		rec.SendLog("starting...")
		if err := cmd.Start(); err != nil {
			errdata, _ := io.ReadAll(errc)
			return fmt.Errorf("start: %w: %s", err, string(errdata))
		}
		close(started)

		if err := cmd.Wait(); err != nil {
			errdata, _ := io.ReadAll(errc)
			return fmt.Errorf("wait: %w: %s", err, string(errdata))
		}

		rec.Done()
		return nil
	})

	g.Go(func() error {
		<-started

		rec.SendLog("start")

		scanner := bufio.NewScanner(rc)
		subtitles := map[string]struct{}{}
		for scanner.Scan() {
			line := scanner.Text()

			rec.SendLog(rePrefix.ReplaceAllString(line, ""))
			if idx := strings.Index(line, *argOutDir); idx != -1 {
				fp := line[idx:]

				if strings.Contains(line, "subtitles to:") {
					subtitles[fp[len(*argOutDir):]] = struct{}{}
					continue
				}

				dotidx := strings.LastIndex(fp, ".")
				if dotidx == -1 {
					panic("cannot parse file loc")
				}

				for i, c := range fp[dotidx+1:] {
					if !(unicode.IsLetter(c) || unicode.IsNumber(c)) {
						fp = fp[:dotidx+i+1]
						break
					}
				}

				fp = fp[len(*argOutDir):]
				base := filepath.Base(fp)
				fp = filepath.Join("/download", fp[:len(fp)-len(base)], url.PathEscape(base))
				if len(rec.Filepath) == 0 || len(fp) < len(rec.Filepath) {
					rec.Filepath = fp
				}
			}
		}

		for k := range subtitles {
			rec.Subtitles = append(rec.Subtitles, filepath.Join("/download", k))
		}
		log.Print("ended scanning")

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scanner: %w", err)
		}

		return nil
	})

	return g.Wait()
}

func GetID(ctx context.Context, durl string) (string, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", durl, "-O", "id")
	out := bytes.NewBuffer(nil)
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run: %w", err)
	}

	return strings.TrimSpace(out.String()), nil
}
