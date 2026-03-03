package admin

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

func writeSSEEvent(c *gin.Context, event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, string(raw)); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
