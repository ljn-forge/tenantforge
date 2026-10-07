package platform

import (
	"errors"
	"math"
	"testing"

	"github.com/ljn-forge/tenantforge/extension"
)

func TestDiagnosticsNeverExposeProviderOrHandlerDetails(t *testing.T) {
	provider := errors.New("private credentials and token must never be public")
	if safeDiagnostic(stageStorage, provider) != "storage_unavailable" {
		t.Fatal("provider error not categorized")
	}
	_, e := invokeJob(extension.JobDefinition{Run: func(extension.JobContext) (map[string]any, error) { panic("private secret") }}, nil)
	if e == nil || e.Error() != "job handler failed" {
		t.Fatal("handler panic detail escaped")
	}
	if validateJobResult(extension.JobDefinition{ResultFields: []Field{{Key: "count", Type: "number"}}}, map[string]any{"count": math.NaN()}) == nil {
		t.Fatal("non-JSON result accepted")
	}
}
