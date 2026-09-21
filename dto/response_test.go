package dto

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWithMetaUsesStandardResponseEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	WithMeta(context, http.StatusOK, []string{"employee"}, "Employees retrieved", &Meta{
		Page:    2,
		PerPage: 20,
		Total:   41,
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var response BaseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !response.Success || response.Message != "Employees retrieved" {
		t.Fatalf("response = %#v, want successful employees response", response)
	}
	if response.Meta == nil || response.Meta.Page != 2 || response.Meta.PerPage != 20 || response.Meta.Total != 41 {
		t.Fatalf("meta = %#v, want page 2/per_page 20/total 41", response.Meta)
	}
}

func TestHandleErrorUsesStandardResponseEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	HandleError(context, ErrBadRequest)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	var response BaseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Success || response.Message != ErrBadRequest.Error() {
		t.Fatalf("response = %#v, want failed bad-request response", response)
	}
}
