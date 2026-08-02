package httpapi

import (
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	vial "github.com/jrgf/go-vial"
)

func TestNotificationStreamCanDisableWriteTimeout(t *testing.T) {
	app := vial.New()
	app.Get("/stream", func(c *vial.Context) error {
		if err := disableWriteDeadline(c.Response()); err != nil {
			return err
		}
		if _, err := fmt.Fprint(c.Response(), "start"); err != nil {
			return err
		}
		c.Response().(interface{ Flush() }).Flush()
		time.Sleep(75 * time.Millisecond)
		_, err := fmt.Fprint(c.Response(), "done")
		return err
	})

	server := httptest.NewUnstartedServer(app)
	server.Config.WriteTimeout = 20 * time.Millisecond
	server.Start()
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "startdone" {
		t.Fatalf("stream body = %q", body)
	}
}
