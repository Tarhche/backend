package createelement

import (
	"encoding/json"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/element/component"
)

func TestRequest_Validate(t *testing.T) {
	t.Run("always returns valid", func(t *testing.T) {
		req := Request{
			Type:   "jumbotron",
			Body:   component.Jumbotron{},
			Venues: []string{"venue1"},
		}

		valid, errs := req.Validate()

		if !valid {
			t.Errorf("Validate() valid = false, want true")
		}

		if len(errs) != 0 {
			t.Errorf("Validate() returned %d errors, want 0", len(errs))
		}
	})
}

func TestRequest_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*Request) bool
	}{
		{
			name:    "unmarshals jumbotron component",
			json:    `{"type":"jumbotron","body":{"title":"Test","subtitle":"Subtitle"},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				_, ok := r.Body.(component.Jumbotron)
				return ok && r.Type == "jumbotron"
			},
		},
		{
			name:    "unmarshals featured component",
			json:    `{"type":"featured","body":{},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				_, ok := r.Body.(component.Featured)
				return ok && r.Type == "featured"
			},
		},
		{
			name:    "unmarshals item component",
			json:    `{"type":"item","body":{},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				_, ok := r.Body.(component.Item)
				return ok && r.Type == "item"
			},
		},
		{
			name:    "returns error for unsupported component type",
			json:    `{"type":"unsupported","body":{},"venues":["venue1"]}`,
			wantErr: true,
			check:   func(r *Request) bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req Request
			err := json.Unmarshal([]byte(tt.json), &req)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !tt.check(&req) {
				t.Errorf("UnmarshalJSON() did not unmarshal correctly")
			}
		})
	}
}
