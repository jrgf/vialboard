package httpapi

import (
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	vial "github.com/jrgf/go-vial"
)

func TestEventStreamRefreshesWriteDeadline(t *testing.T) {
	app := vial.New()
	app.Get("/stream", func(c *vial.Context) error {
		write := func(value string) error {
			if !writeEventStream(c.Response(), func() error {
				_, err := io.WriteString(c.Response(), value)
				return err
			}) {
				return errors.New("stream write failed")
			}
			return nil
		}
		if err := write("start"); err != nil {
			return err
		}
		time.Sleep(75 * time.Millisecond)
		return write("done")
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
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "startdone" {
		t.Fatalf("stream body = %q", body)
	}
}
