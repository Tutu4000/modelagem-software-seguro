// Comando worker consome as tarefas e processa os STL. Roda na rede de
// processamento, sem banco, sem internet e sem chave do armazenamento (FC3).
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"nuggetmachine/internal/config"
	"nuggetmachine/internal/processing"
	"nuggetmachine/internal/queue"
	"nuggetmachine/internal/stl"
)

// errRetry marca falhas passageiras: a tarefa volta para a fila, até o limite
// de entregas da fila quorum (E4.5).
var errRetry = errors.New("falha passageira")

type worker struct {
	broker  *queue.Broker
	http    *http.Client
	timeout time.Duration
	log     *slog.Logger
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("o worker parou", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	var env config.Env
	host := env.String("AMQP_HOST", "rabbitmq:5672")
	user := env.String("AMQP_USER", "worker")
	password := env.Secret("AMQP_PASSWORD")
	timeout, err := time.ParseDuration(env.String("TASK_TIMEOUT", "60s"))
	if err != nil {
		return fmt.Errorf("TASK_TIMEOUT: %w", err)
	}
	if err := env.Err(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	broker, err := queue.Dial(ctx, host, user, password)
	if err != nil {
		return err
	}
	defer broker.Close()
	closed := broker.Closed()
	deliveries, err := broker.Consume(queue.QueueTasks, 1)
	if err != nil {
		return err
	}
	w := worker{broker: broker, http: &http.Client{Timeout: timeout}, timeout: timeout, log: log}
	log.Info("worker no ar")

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-closed:
			return fmt.Errorf("conexão com o RabbitMQ caiu: %v", err)
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("o canal de tarefas fechou")
			}
			w.handle(ctx, d)
		}
	}
}

func (w worker) handle(ctx context.Context, d amqp.Delivery) {
	var m queue.TaskMessage
	if err := json.Unmarshal(d.Body, &m); err != nil {
		w.log.Error("tarefa ilegível enviada para a fila de falhas", "error", err)
		_ = d.Reject(false)
		return
	}
	log := w.log.With("tarefa", m.TaskID, "arquivo", m.FileID)

	taskCtx, cancel := context.WithTimeout(ctx, w.timeout)
	res, err := w.process(taskCtx, m)
	cancel()

	switch {
	case err == nil:
		err = w.broker.Publish(ctx, queue.ExchangeResults, queue.ResultMessage{
			TaskID: m.TaskID, Status: queue.StatusDone, VolumeMM3: res.VolumeMM3, BoxMM: res.BoxMM,
		})
		if err != nil {
			log.Error("falha ao publicar o resultado", "error", err)
			_ = d.Nack(false, true)
			return
		}
		log.Info("tarefa concluída", "volume_mm3", res.VolumeMM3)
		_ = d.Ack(false)
	case errors.Is(err, errRetry):
		log.Warn("tarefa devolvida à fila", "error", err)
		_ = d.Nack(false, true)
	default:
		// Falha definitiva: a API fica sabendo, e a mensagem vai para a fila de
		// falhas para análise.
		log.Error("tarefa falhou", "error", err)
		err := w.broker.Publish(ctx, queue.ExchangeResults, queue.ResultMessage{TaskID: m.TaskID, Status: queue.StatusFailed})
		if err != nil {
			log.Error("falha ao publicar a falha", "error", err)
			_ = d.Nack(false, true)
			return
		}
		_ = d.Reject(false)
	}
}

func (w worker) process(ctx context.Context, m queue.TaskMessage) (processing.Result, error) {
	var want [32]byte
	if n, err := hex.Decode(want[:], []byte(m.SHA256)); err != nil || n != len(want) {
		return processing.Result{}, fmt.Errorf("sha256 inválido na mensagem")
	}
	data, err := w.download(ctx, m.URL)
	if err != nil {
		return processing.Result{}, err
	}
	return processing.Analyze(ctx, data, want)
}

// download lê no máximo o limite do upload. Um 4xx, como uma URL vencida, é
// definitivo. O prazo da tarefa vencido também: ela é encerrada (E4.4).
func (w worker) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := w.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", errRetry, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: armazenamento respondeu %s", errRetry, resp.Status)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("armazenamento respondeu %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, stl.MaxSize+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", errRetry, err)
	}
	if len(data) > stl.MaxSize {
		return nil, fmt.Errorf("arquivo acima de %d bytes", stl.MaxSize)
	}
	return data, nil
}
