package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRequestCancellationAndTimeoutAreNotInternalFailures(t *testing.T) {
	for _, item := range []struct {
		name   string
		cause  error
		status int
		code   string
	}{{"client abort", context.Canceled, 499, "cancelled"}, {"server deadline", context.DeadlineExceeded, 504, "timeout"}} {
		t.Run(item.name, func(t *testing.T) {
			err := fmt.Errorf("query interrupted: %w", item.cause)
			if code := errorCode(err); code != item.code {
				t.Fatalf("interrupted request log code %q, want %q", code, item.code)
			}
			response := httptest.NewRecorder()
			writeError(response, "test-request", err)
			if response.Code != item.status {
				t.Fatalf("interrupted request status %d, want %d", response.Code, item.status)
			}
			var envelope struct {
				Error struct{ Code, Message string }
			}
			if e := json.Unmarshal(response.Body.Bytes(), &envelope); e != nil {
				t.Fatal(e)
			}
			if envelope.Error.Code != item.code {
				t.Fatal("incorrect safe response code")
			}
			if envelope.Error.Message == err.Error() {
				t.Fatal("query details leaked")
			}
		})
	}
}

func TestRequestContextClassifiesProviderCancellationWithoutLeakingDetails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &pgconn.PgError{Code: "57014", Message: "private provider query details"}
	if code := errorCode(requestError(ctx, provider)); code != "cancelled" {
		t.Fatalf("cancelled provider code %q", code)
	}
	business := forbidden()
	if !errors.Is(requestError(context.Background(), business), business) {
		t.Fatal("live request business error changed")
	}
	if errorCode(&pgconn.PgError{Code: "40P01", Message: "private query"}) != "conflict" {
		t.Fatal("response and log classifications diverged")
	}
}
