package webtemplate_test

// REQ-106 — deterministic Marshal + round-trip goldens. Marshalling the same
// compiled template twice MUST be byte-identical, and MUST match the checked-in
// golden (regenerate with -update).

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

var update = flag.Bool("update", false, "update webtemplate golden files")

func goldenFixtures() map[string]string {
	tmpl := "../../../testkit/cassettes/templates/"
	return map[string]string{
		"minimal_evaluation":  tmpl + "minimal_evaluation.en.v1.opt",
		"minimal_observation": tmpl + "minimal_observation.en.v1.opt",
		"minimal_instruction": tmpl + "minimal_instruction.en.v1.opt",
		"constrain_test":      referenceDir + "/constrain_test.opt",
	}
}

func TestMarshalDeterministicAndGolden(t *testing.T) {
	for name, opt := range goldenFixtures() {
		t.Run(name, func(t *testing.T) {
			c := compileFixture(t, opt)
			a, err := webtemplate.Marshal(c)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			b, err := webtemplate.Marshal(c)
			if err != nil {
				t.Fatalf("marshal (2nd): %v", err)
			}
			if !bytes.Equal(a, b) {
				t.Fatal("non-deterministic Marshal output")
			}

			var pretty bytes.Buffer
			if err := json.Indent(&pretty, a, "", "  "); err != nil {
				t.Fatalf("indent: %v", err)
			}
			pretty.WriteByte('\n')

			golden := filepath.Join("testdata/webtemplate", name+".json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, pretty.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update): %v", err)
			}
			if !bytes.Equal(pretty.Bytes(), want) {
				t.Errorf("output != golden %s (run with -update to regenerate)", golden)
			}
		})
	}
}
