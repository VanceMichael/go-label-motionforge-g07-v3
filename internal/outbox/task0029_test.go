package outbox

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

type task0029Client struct{ body *task0029Body }

func (c task0029Client) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 503, Body: c.body}, nil
}

type task0029Body struct {
	*bytes.Reader
	closed bool
}

func (b *task0029Body) Close() error { b.closed = true; return nil }

func TestTask0029CanceledWebhookClosesBodyAndPreservesEvent(t *testing.T) {
	body := &task0029Body{Reader: bytes.NewReader([]byte("retry later"))}
	sender := NewSender(task0029Client{body: body})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sender.Send(ctx, "http://webhook.motion.test/events", "event-task0029", []byte(`{"event":"capture"}`))
	if err == nil {
		t.Fatal("webhook failure was reported as success")
	}
	if !body.closed {
		t.Fatal("webhook response body was not closed")
	}
	var _ io.Reader = body
}
