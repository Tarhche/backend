package createelement

import (
	"encoding/json"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/element/component"
)

func TestRequest_Validate(t *testing.T) {
	t.Run("valid request with item component", func(t *testing.T) {
		req := Request{
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
			Body: &jumbotronComponentRequest{
				Type: component.ComponentTypeJumbotron,
				Item: itemComponentRequest{
					Type:        component.ComponentTypeItem,
					ContentUUID: "test-uuid",
					ContentType: "article",
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

	t.Run("valid request with stack component", func(t *testing.T) {
		req := Request{
			Body: &stackComponentRequest{
				Type:             component.ComponentTypeStack,
				HighlightCurrent: true,
				VisibleNeighbors: 2,
				Items: []itemComponentRequest{
					{
						Type:        component.ComponentTypeItem,
						ContentUUID: "test-uuid-1",
						ContentType: "article",
					},
					{
						Type:        component.ComponentTypeItem,
						ContentUUID: "test-uuid-2",
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

	t.Run("invalid request with stack component", func(t *testing.T) {
		req := Request{
			Body: &stackComponentRequest{
				Type: component.ComponentTypeStack,
				Items: []itemComponentRequest{
					{
						Type:        component.ComponentTypeItem,
						ContentType: "article",
					},
				},
			},
			Venues: []string{"venue1"},
		}

		errs := req.Validate()

		if _, ok := errs["body.items.0.content_uuid"]; !ok {
			t.Errorf("Validate() returned %v, want an error on body.items.0.content_uuid", errs)
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
			json:    `{"body":{"type":"jumbotron","item":{"type":"item","content_uuid":"test-uuid","content_type":"article"}},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				jumbotron, ok := r.Body.(*jumbotronComponentRequest)
				return ok && jumbotron.Type == component.ComponentTypeJumbotron && len(r.Venues) == 1
			},
		},
		{
			name:    "unmarshals featured component",
			json:    `{"body":{"type":"featured","main":{"type":"item","content_uuid":"main-uuid","content_type":"article"},"aside":[{"type":"item","content_uuid":"aside-uuid","content_type":"article"}]},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				featured, ok := r.Body.(*featuredComponentRequest)
				return ok && featured.Type == component.ComponentTypeFeatured && len(r.Venues) == 1
			},
		},
		{
			name:    "unmarshals item component",
			json:    `{"body":{"type":"item","content_uuid":"test-uuid","content_type":"article"},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				item, ok := r.Body.(*itemComponentRequest)
				return ok && item.Type == component.ComponentTypeItem && len(r.Venues) == 1
			},
		},
		{
			name:    "unmarshals cards component",
			json:    `{"body":{"type":"cards","title":"test-title","is_carousel":true,"items":[{"type":"item","content_uuid":"test-uuid","content_type":"article"}]},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				cards, ok := r.Body.(*cardsComponentRequest)
				return ok && cards.Type == component.ComponentTypeCards && len(r.Venues) == 1
			},
		},
		{
			name:    "unmarshals stack component",
			json:    `{"body":{"type":"stack","highlight_current":true,"visible_neighbors":3,"items":[{"type":"item","content_uuid":"test-uuid","content_type":"article"}]},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				stack, ok := r.Body.(*stackComponentRequest)
				return ok && stack.Type == component.ComponentTypeStack &&
					stack.HighlightCurrent && stack.VisibleNeighbors == 3 &&
					len(stack.Items) == 1 && len(r.Venues) == 1
			},
		},
		{
			// Not a decode error: an unsupported type leaves the body unset and
			// Validate reports it, so the caller gets a translated message.
			name:    "leaves an unsupported component type for validation",
			json:    `{"body":{"type":"unsupported"},"venues":["venue1"]}`,
			wantErr: false,
			check: func(r *Request) bool {
				return r.Body == nil && r.Validate()["body.type"] == "invalid_value"
			},
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
