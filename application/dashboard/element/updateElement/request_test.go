package updateelement

import (
	"encoding/json"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/element/component"
)

func TestRequest_Validate(t *testing.T) {
	t.Run("valid request with item component", func(t *testing.T) {
		req := Request{
			UUID: "element-uuid-123",
			Body: &itemComponentRequest{
				Type:        component.ComponentTypeItem,
				ContentUUID: "test-uuid",
				ContentType: "article",
			},
			Venues: []string{"venue1"},
		}

		errs := req.Validate()

		if len(errs) != 0 {
			t.Errorf("Validate() returned %d errors, want 0: %v", len(errs), errs)
		}
	})

	t.Run("valid request with jumbotron component", func(t *testing.T) {
		req := Request{
			UUID: "element-uuid-123",
			Body: &jumbotronComponentRequest{
				Type: component.ComponentTypeJumbotron,
				Item: itemComponentRequest{
					Type:        component.ComponentTypeItem,
					ContentUUID: "content-uuid-123",
					ContentType: "content-type-123",
				},
			},
			Venues: []string{"venue1"},
		}

		errs := req.Validate()

		if len(errs) != 0 {
			t.Errorf("Validate() returned %d errors, want 0: %v", len(errs), errs)
		}
	})

	t.Run("valid request with cards component", func(t *testing.T) {
		req := Request{
			UUID: "element-uuid-123",
			Body: &cardsComponentRequest{
				Type:       component.ComponentTypeCards,
				Title:      "test-title",
				IsCarousel: true,
				Items: []itemComponentRequest{
					{
						Type:        component.ComponentTypeItem,
						ContentUUID: "test-uuid",
						ContentType: "article",
					},
				},
			},
			Venues: []string{"venue1"},
		}

		errs := req.Validate()

		if len(errs) != 0 {
			t.Errorf("Validate() returned %d errors, want 0: %v", len(errs), errs)
		}
	})

	t.Run("valid request with featured component", func(t *testing.T) {
		req := Request{
			UUID: "element-uuid-123",
			Body: &featuredComponentRequest{
				Type: component.ComponentTypeFeatured,
				Main: itemComponentRequest{
					Type:        component.ComponentTypeItem,
					ContentUUID: "main-uuid",
					ContentType: "article",
				},
				Aside: []itemComponentRequest{
					{
						Type:        component.ComponentTypeItem,
						ContentUUID: "aside-uuid-1",
						ContentType: "article",
					},
					{
						Type:        component.ComponentTypeItem,
						ContentUUID: "aside-uuid-2",
						ContentType: "article",
					},
				},
			},
			Venues: []string{"venue1", "venue2"},
		}

		errs := req.Validate()

		if len(errs) != 0 {
			t.Errorf("Validate() returned %d errors, want 0: %v", len(errs), errs)
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
			json:    `{"uuid":"element-uuid-123","body":{"type":"jumbotron","item":{"type":"item","content_uuid":"test-uuid","content_type":"article"}},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				jumbotron, ok := r.Body.(*jumbotronComponentRequest)
				return ok && jumbotron.Type == component.ComponentTypeJumbotron && r.UUID == "element-uuid-123" && len(r.Venues) == 1
			},
		},
		{
			name:    "unmarshals featured component",
			json:    `{"uuid":"element-uuid-123","body":{"type":"featured","main":{"type":"item","content_uuid":"main-uuid","content_type":"article"},"aside":[{"type":"item","content_uuid":"aside-uuid","content_type":"article"}]},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				featured, ok := r.Body.(*featuredComponentRequest)
				return ok && featured.Type == component.ComponentTypeFeatured && r.UUID == "element-uuid-123" && len(r.Venues) == 1
			},
		},
		{
			name:    "unmarshals item component",
			json:    `{"uuid":"element-uuid-123","body":{"type":"item","content_uuid":"test-uuid","content_type":"article"},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				item, ok := r.Body.(*itemComponentRequest)
				return ok && item.Type == component.ComponentTypeItem && r.UUID == "element-uuid-123" && len(r.Venues) == 1
			},
		},
		{
			name:    "unmarshals cards component",
			json:    `{"uuid":"element-uuid-123","body":{"type":"cards","title":"test-title","is_carousel":true,"items":[{"type":"item","content_uuid":"test-uuid","content_type":"article"}]},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				cards, ok := r.Body.(*cardsComponentRequest)
				return ok && cards.Type == component.ComponentTypeCards && r.UUID == "element-uuid-123" && len(r.Venues) == 1
			},
		},
		{
			name:    "returns error for unsupported component type",
			json:    `{"uuid":"element-uuid-123","body":{"type":"unsupported"},"venues":["venue1"]}`,
			wantErr: true,
			check:   func(r *Request) bool { return true },
		},
		{
			name:    "returns error for malformed json",
			json:    `{"body":}`,
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
