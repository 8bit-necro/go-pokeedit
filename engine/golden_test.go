package engine

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pe "github.com/8bit-necro/go-pokeedit"
)

// The files in ../testdata/pkhex were produced by the original PKHeX.Core
// bridge: <name>.sav.gz is a save it generated and <name>.inspect.json is what
// its "inspect" action reported for it. These tests assert that this library
// reads those saves identically and writes them back unchanged.

func loadGolden(t *testing.T, name string) (raw []byte, game string, want map[string]any) {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "testdata", "pkhex", name+".sav.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err = io.ReadAll(zr); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("..", "testdata", "pkhex", name+".inspect.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Game    string         `json:"game"`
		Summary map[string]any `json:"summary"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return raw, doc.Game, doc.Summary
}

func goldenNames(t *testing.T) []string {
	files, err := filepath.Glob(filepath.Join("..", "testdata", "pkhex", "*.sav.gz"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files: %v", err)
	}
	var out []string
	for _, f := range files {
		out = append(out, strings.TrimSuffix(filepath.Base(f), ".sav.gz"))
	}
	return out
}

func TestInspectMatchesPKHeX(t *testing.T) {
	for _, name := range goldenNames(t) {
		t.Run(name, func(t *testing.T) {
			raw, game, want := loadGolden(t, name)
			req, _ := json.Marshal(map[string]any{
				"action": "inspect", "game": game, "save": base64.StdEncoding.EncodeToString(raw),
			})
			out, ok := Run(req)
			if !ok {
				t.Fatalf("engine error: %s", out)
			}
			var got struct {
				Summary map[string]any `json:"summary"`
			}
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Summary, want) {
				for k := range want {
					if !reflect.DeepEqual(got.Summary[k], want[k]) {
						t.Errorf("summary[%q] differs:\n got: %.300v\nwant: %.300v", k, got.Summary[k], want[k])
					}
				}
				t.Fatal("summary differs from PKHeX")
			}
		})
	}
}

func TestUnmodifiedSaveRoundTripsExactly(t *testing.T) {
	for _, name := range goldenNames(t) {
		t.Run(name, func(t *testing.T) {
			raw, game, _ := loadGolden(t, name)
			sav, err := pe.Open(raw, pe.OpenOptions{Game: pe.Game(game)})
			if err != nil {
				t.Fatal(err)
			}
			out, err := sav.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, raw) {
				t.Fatalf("rewriting an unmodified save changed it")
			}
		})
	}
}
