// Package audit grava eventos de auditoria em JSON, sem senha, token, cartão
// nem conteúdo de arquivo (C17, C18).
package audit

import "io"

type Logger struct {
	w io.Writer
}

func New(w io.Writer) *Logger { return &Logger{w: w} }
