package splunk

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splunk/go-splunk-client/pkg/authenticators"
	splunkclient "github.com/splunk/go-splunk-client/pkg/client"
	"github.com/stretchr/testify/assert"
)

// TestGetDimensions_QueryErrorDoesNotHang reproduces the scenario where the
// underlying Splunk query fails (here: an undecodable response body). Before
// the fix, the results channel was only ever closed inside the success
// callback, so a failed query left GetDimensions blocked forever on `range ch`.
func TestGetDimensions_QueryErrorDoesNotHang(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, w, _ := os.Pipe()
	logger := slog.New(slog.NewTextHandler(w, nil))

	client := &splunkclient.Client{
		URL:           server.URL,
		Authenticator: authenticators.Token{Token: "test"},
	}
	s := &Splunk{Client: client, Logger: logger}

	done := make(chan []string, 1)
	go func() {
		done <- s.GetDimensions("main", "some.metric")
	}()

	select {
	case dims := <-done:
		assert.Empty(t, dims)
	case <-time.After(2 * time.Second):
		t.Fatal("GetDimensions did not return within timeout: it deadlocked on an unclosed channel")
	}
}
