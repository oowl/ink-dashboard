package layout

import "testing"

func TestNormalizeDocumentKeepsExplicitEmptyArtboard(t *testing.T) {
	doc := NormalizeDocument(Document{
		Version: 1,
		Artboards: map[string]Artboard{
			"portrait": {Width: 600, Height: 800, Components: []Component{}},
		},
	})

	if len(doc.Artboards["portrait"].Components) != 0 {
		t.Fatal("expected explicit empty portrait artboard to stay empty")
	}
	if len(doc.Artboards["landscape"].Components) == 0 {
		t.Fatal("expected missing landscape artboard to use defaults")
	}
}

func TestNormalizeDocumentDropsUnknownComponents(t *testing.T) {
	doc := NormalizeDocument(Document{
		Version: 1,
		Artboards: map[string]Artboard{
			"portrait": {
				Width:  600,
				Height: 800,
				Components: []Component{
					{ID: "bad", Type: "unknown", X: 1, Y: 2, W: 3, H: 4},
					{ID: "notes", Type: "notes", X: -10, Y: 900, W: 20, H: 20},
				},
			},
		},
	})

	components := doc.Artboards["portrait"].Components
	if len(components) != 1 || components[0].Type != "notes" {
		t.Fatalf("unexpected components after normalization: %#v", components)
	}
	if components[0].X != 0 || components[0].Y != 680 || components[0].W != 180 || components[0].H != 120 {
		t.Fatalf("expected notes component to be clamped to min size and bounds, got %#v", components[0])
	}
}

func TestNormalizeDocumentTrimsComponentProps(t *testing.T) {
	doc := NormalizeDocument(Document{
		Version: 1,
		Artboards: map[string]Artboard{
			"portrait": {
				Width:  600,
				Height: 800,
				Components: []Component{
					{
						ID:   "calendar",
						Type: "calendar",
						X:    10,
						Y:    20,
						W:    300,
						H:    220,
						Props: map[string]string{
							" data_key ":   " events ",
							"source_key":   "   ",
							" custom.flag": " true ",
						},
					},
				},
			},
		},
	})

	props := doc.Artboards["portrait"].Components[0].Props
	if props["data_key"] != "events" || props["custom.flag"] != "true" {
		t.Fatalf("expected props to be trimmed and preserved, got %#v", props)
	}
	if _, ok := props["source_key"]; ok {
		t.Fatalf("expected blank source_key to be dropped, got %#v", props)
	}
}
