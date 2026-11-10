package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// maxJSONBody is far above any request body of the API besides the upload,
// which does not use JSON.
const maxJSONBody = 64 << 10

// DecodeJSON reads one JSON object and refuses fields the type does not have:
// a client that sends "total_cents" or "status" gets an error instead of
// having it silently ignored (CA03, mass assignment).
func DecodeJSON(c *gin.Context, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONBody))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("the body has more than one JSON value")
	}
	return nil
}
