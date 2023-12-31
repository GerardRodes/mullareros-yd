package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

type (
	Record struct {
		ID        string        `json:"id"`
		Filepath  string        `json:"filepath"`
		Subtitles []string      `json:"subtitles"`
		lastLog   string        `json:"-"`
		totalLogs atomic.Uint32 `json:"-"`
		listeners []chan string `json:"-"`
		l         sync.RWMutex  `json:"-"`
	}
)

func (r *Record) isDoneFlagPath() string {
	return filepath.Join(*argOutDir, r.ID, "done")
}

func (r *Record) IsDone() bool {
	_, err := os.Stat(r.isDoneFlagPath())
	return !errors.Is(err, os.ErrNotExist)
}

func (r *Record) AddListener(l chan string) {
	r.l.Lock()
	defer r.l.Unlock()

	r.listeners = append(r.listeners, l)
}

func (r *Record) RemoveListener(l chan string) {
	r.l.Lock()
	defer r.l.Unlock()

	for i := range r.listeners {
		if r.listeners[i] == l {
			r.listeners = append(r.listeners[:i], r.listeners[i+1:]...)
		}
	}
}

func (r *Record) LastLog() string {
	r.l.RLock()
	defer r.l.RUnlock()

	return r.lastLog
}

func (r *Record) Started() bool {
	return r.totalLogs.Load() > 0
}

func (r *Record) Done() {
	r.l.Lock()
	defer r.l.Unlock()

	f, err := os.Create(r.isDoneFlagPath())
	if err != nil {
		log.Error().Err(err).Msg("cannot create is done flag")
	}
	f.Close()

	for i := range r.listeners {
		close(r.listeners[i])
	}
}

func (r *Record) SendLog(e string) {
	r.l.RLock()
	defer r.l.RUnlock()

	for i := range r.listeners {
		r.listeners[i] <- e
	}
	r.lastLog = e
	r.totalLogs.Add(1)
}

var globalState = &State{
	records: make([]*Record, 0, 10),
}

type State struct {
	records []*Record
	l       sync.RWMutex
}

func (s *State) Keys() (out []string) {
	s.l.RLock()
	defer s.l.RUnlock()

	for _, r := range s.records {
		out = append(out, r.ID)
	}
	return
}

func (s *State) Has(id string) bool {
	s.l.RLock()
	defer s.l.RUnlock()

	for _, r := range s.records {
		if r.ID == id {
			return true
		}
	}
	return false
}

func (s *State) Get(id string) *Record {
	s.l.RLock()
	defer s.l.RUnlock()

	for _, r := range s.records {
		if r.ID == id {
			return r
		}
	}

	return nil
}

func (s *State) Put(r *Record) {
	s.l.Lock()
	defer s.l.Unlock()

	s.records = append(s.records, r)
}

func (s *State) Delete(id string) {
	s.l.Lock()
	defer s.l.Unlock()

	i := -1
	for idx, r := range s.records {
		if r.ID == id {
			i = idx
			break
		}
	}

	if i == -1 {
		return
	}

	s.records = append(s.records[:i], s.records[i+1:]...)
}
