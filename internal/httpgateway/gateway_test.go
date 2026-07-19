package httpgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"authservice/pkg/models"
	"authservice/pkg/service"
	authv1 "authservice/proto/auth/v1"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newTestGateway wires the real services over an in-memory DB behind the
// JSON shim, exactly as cmd/server does.
func newTestGateway(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("PLATFORM_CLIENT_SECRET", "platform-secret")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.GetAllModels()...); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}
	if err := service.BootstrapPlatform(context.Background(), db); err != nil {
		t.Fatalf("BootstrapPlatform failed: %v", err)
	}
	return New(nil, true,
		Service{Desc: &authv1.AuthService_ServiceDesc, Impl: service.NewAuthServiceServer(db)},
		Service{Desc: &authv1.PlatformService_ServiceDesc, Impl: service.NewPlatformServiceServer(db)},
	)
}

func postJSON(t *testing.T, h http.Handler, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON (%d): %s", rec.Code, rec.Body.String())
	}
	return rec.Code, out
}

func TestGateway_PlatformFlow(t *testing.T) {
	h := newTestGateway(t)

	// camelCase protojson request/response on the console contract paths
	code, reg := postJSON(t, h, "/auth.v1.PlatformService/RegisterDeveloper",
		`{"email":"dev@example.com","password":"password123","orgName":"acme"}`)
	if code != http.StatusOK || reg["success"] != true {
		t.Fatalf("RegisterDeveloper failed: %d %v", code, reg)
	}
	if reg["developerId"] == "" || reg["orgId"] == "" {
		t.Fatalf("expected camelCase developerId/orgId fields, got %v", reg)
	}

	code, login := postJSON(t, h, "/auth.v1.PlatformService/DeveloperLogin",
		`{"email":"dev@example.com","password":"password123"}`)
	if code != http.StatusOK || login["success"] != true {
		t.Fatalf("DeveloperLogin failed: %d %v", code, login)
	}
	token, _ := login["accessToken"].(string)
	if token == "" {
		t.Fatalf("expected camelCase accessToken in response, got %v", login)
	}
	// EmitUnpopulated: false booleans must still be present
	if _, ok := login["isSuperadmin"]; !ok {
		t.Fatalf("expected isSuperadmin to be emitted even when false")
	}

	// AuthService is served on the same shim
	code, health := postJSON(t, h, "/auth.v1.AuthService/HealthCheck", ``)
	if code != http.StatusOK || health["status"] != "SERVING" {
		t.Fatalf("HealthCheck via gateway failed: %d %v", code, health)
	}
}

func TestGateway_Errors(t *testing.T) {
	h := newTestGateway(t)

	// Unknown method
	if code, _ := postJSON(t, h, "/auth.v1.PlatformService/Nope", `{}`); code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown method, got %d", code)
	}

	// Malformed JSON body
	if code, _ := postJSON(t, h, "/auth.v1.PlatformService/DeveloperLogin", `{oops`); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", code)
	}

	// GET rejected
	req := httptest.NewRequest(http.MethodGet, "/auth.v1.PlatformService/DeveloperLogin", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", rec.Code)
	}

	// CORS preflight (permissive in dev)
	req = httptest.NewRequest(http.MethodOptions, "/auth.v1.PlatformService/DeveloperLogin", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected permissive CORS preflight, got %d %v", rec.Code, rec.Header())
	}
}
