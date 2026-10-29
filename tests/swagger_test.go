package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	appRouter "github.com/ERBM-Capstone/ERBM-backend/internal/router"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
)

func TestSwaggerUsesServingOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := jwtmanager.NewManager(
		"swagger-test-access-key-with-at-least-32-bytes",
		"swagger-test-refresh-key-with-at-least-32-bytes",
		"swagger-test", time.Minute, time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	r := appRouter.New(nil, tokens, false, service.LoginPolicy{}, service.ActivationConfig{}, nil)

	for _, origin := range []string{"http://localhost:8080", "https://erbm-backend.onrender.com"} {
		t.Run(origin, func(t *testing.T) {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, origin+"/swagger/doc.json", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("Swagger document status = %d, want %d", response.Code, http.StatusOK)
			}

			var document struct {
				Host        string                                `json:"host"`
				Schemes     []string                              `json:"schemes"`
				BasePath    string                                `json:"basePath"`
				Paths       map[string]map[string]json.RawMessage `json:"paths"`
				Definitions map[string]struct {
					Properties map[string]json.RawMessage `json:"properties"`
					Required   []string                   `json:"required"`
				} `json:"definitions"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
				t.Fatalf("decode Swagger document: %v", err)
			}
			if document.Host != "" || len(document.Schemes) != 0 {
				t.Errorf("Swagger overrides the serving origin: host = %q, schemes = %v", document.Host, document.Schemes)
			}
			if document.BasePath != "/" {
				t.Errorf("Swagger base path = %q, want /", document.BasePath)
			}
			if _, exists := document.Paths["/api/v1/auth/login"]["post"]; !exists {
				t.Error("Swagger must expose POST /api/v1/auth/login without duplicating the API prefix")
			}
			for _, name := range []string{"dto.CreateProjectRequest", "dto.UpdateProjectRequest", "dto.ProjectResponse"} {
				schema, exists := document.Definitions[name]
				if !exists {
					t.Fatalf("missing project schema %s", name)
				}
				if _, exists := schema.Properties["budget"]; exists {
					t.Errorf("%s still exposes the removed budget field", name)
				}
				for _, field := range schema.Required {
					if field == "budget" {
						t.Errorf("%s still requires the removed budget field", name)
					}
				}
			}
		})
	}
}
