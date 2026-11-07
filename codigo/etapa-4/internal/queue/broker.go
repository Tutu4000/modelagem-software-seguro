package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Nomes declarados em deploy/rabbitmq/topologia.json. Os usuários não podem
// declarar nada (E4.1).
const (
	ExchangeTasks   = "tarefas"
	QueueTasks      = "tarefas"
	ExchangeResults = "resultados"
	QueueResults    = "resultados"
)

// TaskMessage é tudo o que o worker recebe: só um arquivo já validado, o hash
// para conferir e uma URL de leitura só dele (E4.2, C16).
type TaskMessage struct {
	TaskID uuid.UUID `json:"tarefa_id"`
	FileID uuid.UUID `json:"arquivo_id"`
	SHA256 string    `json:"sha256"`
	URL    string    `json:"url"`
}

type ResultMessage struct {
	TaskID    uuid.UUID  `json:"tarefa_id"`
	Status    string     `json:"status"` // StatusDone ou StatusFailed
	VolumeMM3 float64    `json:"volume_mm3,omitempty"`
	BoxMM     [3]float64 `json:"caixa_mm"`
}

const (
	StatusWaiting    = "aguardando"
	StatusProcessing = "processando"
	StatusDone       = "concluida"
	StatusFailed     = "falhou"
)

// Broker é uma conexão com o RabbitMQ. Quando ela cai, o processo deve sair e
// o Docker o reinicia: é mais simples e previsível que reconectar.
type Broker struct {
	conn *amqp.Connection

	mu sync.Mutex
	ch *amqp.Channel // em modo de confirmação, só para publicar
}

func Dial(ctx context.Context, host, user string, password []byte) (*Broker, error) {
	u := url.URL{Scheme: "amqp", Host: host, User: url.UserPassword(user, string(password)), Path: "/"}
	var (
		conn *amqp.Connection
		err  error
	)
	for range 30 {
		if conn, err = amqp.Dial(u.String()); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("RabbitMQ em %s como %s: %w", host, user, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		conn.Close()
		return nil, err
	}
	return &Broker{conn: conn, ch: ch}, nil
}

// Publish só retorna depois da confirmação do RabbitMQ: a mensagem persistente
// já está na fila quorum.
func (b *Broker) Publish(ctx context.Context, exchange string, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	confirm, err := b.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, exchange, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
	if err != nil {
		return err
	}
	ok, err := confirm.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("o RabbitMQ recusou a mensagem")
	}
	return nil
}

// Consume abre um canal próprio, com no máximo prefetch mensagens sem ack.
func (b *Broker) Consume(queue string, prefetch int) (<-chan amqp.Delivery, error) {
	ch, err := b.conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return nil, err
	}
	return ch.Consume(queue, "", false, false, false, false, nil)
}

// Closed recebe um valor quando a conexão cai.
func (b *Broker) Closed() <-chan *amqp.Error {
	return b.conn.NotifyClose(make(chan *amqp.Error, 1))
}

func (b *Broker) Close() error { return b.conn.Close() }
