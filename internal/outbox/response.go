package outbox

import (
	"io"
)

func readWebhookBody(body io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(body, limit+1))
}
