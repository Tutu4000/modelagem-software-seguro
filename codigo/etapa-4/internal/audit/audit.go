// Package audit grava eventos de auditoria em JSON, sem senha, token, cartão
// nem conteúdo de arquivo (C17, C18).
package audit

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type Event struct {
	Time   time.Time `json:"time"`
	Name   string    `json:"event"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	Result string    `json:"result"`
	Origin string    `json:"origin"`
}

type Logger struct {
	mu sync.Mutex
	w  io.Writer
}

func New(w io.Writer) *Logger { return &Logger{w: w} }

// Record grava uma linha por evento, com o horário em UTC.
func (l *Logger) Record(e Event) error {
	e.Time = time.Now().UTC()
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.w.Write(append(line, '\n'))
	return err
}
