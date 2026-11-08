package orders

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/notify"
	"nuggetmachine/internal/partners"
)

// The texts the customer reads, in Portuguese. They carry the short order ID,
// the status and the tracking code, and never the address, the price details
// or the support conversation: those stay behind the login. The only
// exception is the address change, whose e-mail must show what is being
// confirmed (C21).

func shortID(o Order) string { return o.ID.String()[:8] }

func NoticeReview(o Order, approved bool) notify.Message {
	if approved {
		return notify.New(o.ContactEmail, "Pedido "+shortID(o)+" aprovado",
			"O seu pedido "+shortID(o)+" passou pela revisão técnica e está aguardando pagamento.\n\n"+
				"Acesse a plataforma para pagar e acompanhar o pedido.")
	}
	return notify.New(o.ContactEmail, "Pedido "+shortID(o)+" não aprovado",
		"O seu pedido "+shortID(o)+" não foi aprovado na revisão técnica.\n\n"+
			"O laudo com o motivo está na página do pedido, na plataforma.")
}

func NoticePaid(o Order) notify.Message {
	return notify.New(o.ContactEmail, "Pagamento do pedido "+shortID(o)+" confirmado",
		"Recebemos o pagamento do pedido "+shortID(o)+". Ele entra na fila de produção.")
}

func NoticeShipped(o Order) notify.Message {
	tracking := ""
	if o.TrackingCode != nil {
		tracking = "\n\nCódigo de rastreio: " + *o.TrackingCode
	}
	return notify.New(o.ContactEmail, "Pedido "+shortID(o)+" enviado",
		"O pedido "+shortID(o)+" foi coletado pela transportadora."+tracking)
}

func NoticeDelivered(o Order) notify.Message {
	return notify.New(o.ContactEmail, "Pedido "+shortID(o)+" entregue",
		"A transportadora informou a entrega do pedido "+shortID(o)+".\n\n"+
			"Se algo estiver errado, abra um chamado na plataforma.")
}

// NoticeShipmentProblem is RF15: the carrier reported a problem.
func NoticeShipmentProblem(o Order) notify.Message {
	return notify.New(o.ContactEmail, "Problema na entrega do pedido "+shortID(o),
		"A transportadora informou um problema na entrega do pedido "+shortID(o)+".\n\n"+
			"Vamos acompanhar com ela. Se precisar, abra um chamado na plataforma.")
}

func NoticeCancelled(o Order, refund *Payment) notify.Message {
	text := "O pedido " + shortID(o) + " foi cancelado a seu pedido."
	switch {
	case refund == nil:
	case refund.Status == "refunded":
		text += "\n\nO estorno do pagamento foi feito no gateway."
	default:
		text += "\n\nO estorno do pagamento está em andamento."
	}
	text += "\n\nSe você não pediu o cancelamento, abra um chamado na plataforma."
	return notify.New(o.ContactEmail, "Pedido "+shortID(o)+" cancelado", text)
}

// NoticeAddressCode carries the code that only the customer, logged in, can
// type to confirm the change asked to support.
func NoticeAddressCode(o Order, a partners.Address, code string, expires time.Time) notify.Message {
	return notify.New(o.ContactEmail, "Confirme a troca de endereço do pedido "+shortID(o),
		"O suporte registrou um pedido de troca do endereço de entrega do pedido "+shortID(o)+" para:\n\n"+
			a.Recipient+"\n"+a.Street+", "+a.Number+"\n"+a.City+" - "+a.State+"\n"+a.PostalCode+"\n\n"+
			"Para confirmar, entre na plataforma e informe o código "+code+" na página do pedido. "+
			"O código vale até "+expires.In(brt).Format("02/01/2006 15:04")+" (horário de Brasília).\n\n"+
			"Se você não pediu essa troca, não informe o código e abra um chamado: o endereço só muda com ele.")
}

func NoticeAddressChanged(o Order) notify.Message {
	return notify.New(o.ContactEmail, "Endereço do pedido "+shortID(o)+" alterado",
		"O endereço de entrega do pedido "+shortID(o)+" foi alterado depois da sua confirmação.\n\n"+
			"Se não foi você, abra um chamado na plataforma imediatamente.")
}

var brt = time.FixedZone("BRT", -3*60*60)

// Notifier sends the notices after the commit. An informational notice that
// fails does not undo what already happened: it goes to the log and to the
// notify.failed event.
type Notifier struct {
	Sender notify.Sender
	Audit  *audit.Logger
	Log    *slog.Logger
}

func (n Notifier) Send(ctx context.Context, m notify.Message, target, origin string) {
	if n.Sender == nil || strings.TrimSpace(m.To) == "" {
		return
	}
	err := n.Sender.Send(ctx, m)
	if err == nil {
		return
	}
	n.Log.Error("notification not queued", "message", m.ID, "error", err)
	if rerr := n.Audit.Record(audit.Event{
		Name: "notify.failed", Actor: "system", Action: "notify", Target: target,
		Result: "failed", Reason: fmt.Sprintf("message %s: %v", m.ID, err), Origin: origin,
	}); rerr != nil {
		n.Log.Error("audit failed", "error", rerr)
	}
}
