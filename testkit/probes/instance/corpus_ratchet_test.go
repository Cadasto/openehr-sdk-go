package instanceprobes_test

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

const (
	reasonRefusalString       = "refusal_string"
	reasonRefusalAttribute    = "refusal_attribute"
	reasonRefusalOther        = "refusal_other"
	reasonPlaceholderTime     = "placeholder_time"
	reasonPlaceholderLanguage = "placeholder_language"
	reasonOrdinalSymbol       = "ordinal_symbol"
	reasonRandomInterval      = "random_interval"
	reasonFloorCluster        = "floor_cluster"
	reasonFloorElement        = "floor_element"
	reasonFloorAction         = "floor_action"
	reasonValidatorOther      = "validator_other"

	entryGenerate = "generate"
	entryBuilder  = "builder"
)

// ratchetNow is the clock every corpus call shares.
var ratchetNow = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

// ratchetFailure is one axis of the corpus ratchet. The reason carries a
// locator, so two distinct failures on the same axis do not share a key.
type ratchetFailure struct {
	template string
	entry    string
	policy   string
	fill     string
	reason   string
}

// corpusCompileFailures names vendored OPTs whose ParseFile or Compile
// fails. An allowlisted name that compiles, or a compile failure that
// is not listed, fails the test.
var corpusCompileFailures = []string{
	"definition/body_weight",
	"templates/social",
}

// corpusRatchet is the failure set measured on this tree. Policy and fill
// are the diagnostic strings from instance.Policy and instance.ValueFill.
// RandomFill uses math/rand/v2.NewPCG(1, 2), a fresh source per call.
// Each reason is category plus a stable locator (type, attribute, code
// and path, or JSON path).
var corpusRatchet = []ratchetFailure{
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "builder", policy: "minimal", fill: "example", reason: "floor_action:required:/content[0]/items[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "builder", policy: "minimal", fill: "example", reason: "floor_action:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/ism_transition/current_state"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "floor_action:required:/content[0]/items[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "floor_action:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/items[5]/data/events[0]/data/items[8]/value/symbol"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[1]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[1]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[2]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[2]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[3]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[3]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[4]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[4]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[5]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[5]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[4]/data/events[0]/data/items[2]/value/lower"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[4]/data/events[0]/data/items[2]/value/upper"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[4]/data/events[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[4]/data/origin"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[5]/data/events[0]/data/items[5]/value"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[5]/data/events[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[5]/data/origin"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[0]/items[5]/data/events[0]/data/items[13]/value/value"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/ism_transition/current_state"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-OBSERVATION.conformance_observation.v0]/data/events[at0002]/data/items[at0025]/value/value"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "floor_action:required:/content[0]/items[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "floor_action:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/items[5]/data/events[0]/data/items[8]/value/symbol"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[1]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[1]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[2]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[2]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[3]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[3]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[4]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[4]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[5]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[5]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[4]/data/events[0]/data/items[2]/value/lower"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[4]/data/events[0]/data/items[2]/value/upper"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[4]/data/events[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[4]/data/origin"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[5]/data/events[0]/data/items[5]/value"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[5]/data/events[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[5]/data/origin"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[0]/items[5]/data/events[0]/data/items[13]/value/value"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/ism_transition/current_state"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-OBSERVATION.conformance_observation.v0]/data/events[at0002]/data/items[at0025]/value/value"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "example", reason: "floor_action:required:/content[0]/items[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "example", reason: "floor_action:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/ism_transition/current_state"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "random", reason: "floor_action:required:/content[0]/items[0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "random", reason: "floor_action:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0]/ism_transition/current_state"},
	{template: "templates/BMI", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/BMI", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/BMI", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/BMI", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/BMI", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/other_context/items[2]/value"},
	{template: "templates/BMI", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[1]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[1]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[2]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[2]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/other_context/items[2]/value"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[1]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[1]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[2]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[2]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/other_context/items[2]/value"},
	{template: "templates/BMI", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/other_context/items[2]/value"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/other_context/items[2]/value"},
	{template: "templates/BMI", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[0]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[1]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[2]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[3]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[1]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[2]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[3]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/time"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/time"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Demonstration.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[0]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[1]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[2]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[3]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[1]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[2]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[3]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[0]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[1]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[2]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[3]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[1]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[2]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[3]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[0]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[1]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[2]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/data/events[3]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[1]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[2]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[3]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[2]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[3]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[0]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[1]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[2]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "floor_cluster:cardinality:/content[0]/data/events[3]/data/items[2]/items[0]/items"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[1]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[2]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[3]/data/items[1]/items[11]/value/symbol"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[2]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[8]/value"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/lower"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/data/items[1]/items[9]/value/upper"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[3]/time"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Demonstration.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Episode.v2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Episode.v2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Episode.v2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Episode.v2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Episode.v2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Episode.v2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Episode.v2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Episode.v2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Episode.v2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Episode.v2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Episode.v2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Episode.v2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Episode.v2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Episode.v2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Episode.v2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[7]/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0002]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0032]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0089]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[7]/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0002]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0032]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0089]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[7]/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0002]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0032]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0089]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[7]/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0002]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0032]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0089]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[7]/items[4]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0002]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0032]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/items[at0089]/name/value"},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-SECTION.allergies_adverse_reactions_rcp.v1]/items[openEHR-EHR-EVALUATION.adverse_reaction_risk.v1]/data/items[at0009]/name/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[2]/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/name/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[2]/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/name/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[2]/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/name/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/data/items[2]/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/name/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/data/items[2]/value"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/name/value"},
	{template: "templates/IDCR Problem List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR Problem List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR Problem List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[3]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[3]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[3]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[3]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[3]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/items[0]/data/items[4]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/items[0]/protocol/items[0]/value"},
	{template: "templates/IDCR Problem List.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Referral Request.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Referral Request.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Referral Request.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Referral Request.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/details/items[11]/items[2]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/details/items[11]/items[3]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/details/items[11]/items[8]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/details/items[12]/items[3]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:cardinality:/relationships"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/details/items[11]/items[2]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/details/items[11]/items[3]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/details/items[11]/items[8]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/details/items[12]/items[3]/value"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:cardinality:/relationships"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:cardinality:/relationships"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:cardinality:/relationships"},
	{template: "templates/Test_dv_boolean.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:required:/content[0]/data/events[0]/data/items[0]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[0]/data/events[0]/data/items[0]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[0]/data/events[0]/data/items[0]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:required:/content[0]/data/events[0]/data/items[0]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:required:/content[0]/data/events[0]/data/items[0]/value/value"},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/value"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0030]/value/id"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0030]/value/id"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0030]/value/id"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0030]/value/id"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0030]/value/id"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0037]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0037]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0037]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0037]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0037]/value/symbol"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/formalism"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/formalism"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/formalism"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/formalism"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0028]/value/formalism"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/value/value"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/alternative_types.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/alternative_types.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/alternative_types.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/alternative_types.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/alternative_types.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/body_weight", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/body_weight", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/body_weight", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/body_weight", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/body_weight", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/body_weight", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/body_weight", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[1]/time"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/body_weight", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "floor_action:required:/content[2]/time"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "floor_action:required:/content[openEHR-EHR-ACTION.validation_action_test.v0]/time"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[6]/data/rows[0]/items[0]/archetype_node_id"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[6]/data/rows[0]/items[0]/name"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[3]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[3]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[4]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[4]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[5]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[5]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[6]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[6]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[7]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[7]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[7]/data/events[0]/time"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[7]/data/origin"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[4]/data/item"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v0]/data/item"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v3]/data/rotated"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v0]/name/value"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v1]/name/value"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v2]/name/value"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v3]/name/value"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "floor_action:required:/content[2]/time"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "floor_action:required:/content[openEHR-EHR-ACTION.validation_action_test.v0]/time"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[6]/data/rows[0]/items[0]/archetype_node_id"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[6]/data/rows[0]/items[0]/name"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[3]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[3]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[4]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[4]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[5]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[5]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[6]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[6]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[7]/encoding"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[7]/language"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[7]/data/events[0]/time"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[7]/data/origin"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[4]/data/item"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v0]/data/item"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v3]/data/rotated"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v0]/name/value"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v1]/name/value"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v2]/name/value"},
	{template: "templates/clinical_content_validation", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v3]/name/value"},
	{template: "templates/clinical_notes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/clinical_notes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/clinical_notes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/clinical_notes.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[2]/activities[0]/description/items[2]/items[1]/value"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[2]/activities[0]/description/items[2]/items[2]/value"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[2]/activities[0]/description/items[2]/items[1]/value"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[2]/activities[0]/description/items[2]/items[2]/value"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/clinical_notes.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/family_history.v.1.2.3", entry: "builder", policy: "minimal", fill: "example", reason: "refusal_other:slot_fill:/content[openEHR-EHR-EVALUATION.family_history.v2]/data/items[at0003]/items[at0024]/items[at0027]"},
	{template: "templates/family_history.v.1.2.3", entry: "generate", policy: "example", fill: "example", reason: "refusal_other:slot_fill:/content[openEHR-EHR-EVALUATION.family_history.v2]/data/items[at0003]/items[at0024]/items[at0027]"},
	{template: "templates/family_history.v.1.2.3", entry: "generate", policy: "example", fill: "random", reason: "refusal_other:slot_fill:/content[openEHR-EHR-EVALUATION.family_history.v2]/data/items[at0003]/items[at0024]/items[at0027]"},
	{template: "templates/family_history.v.1.2.3", entry: "generate", policy: "minimal", fill: "example", reason: "refusal_other:slot_fill:/content[openEHR-EHR-EVALUATION.family_history.v2]/data/items[at0003]/items[at0024]/items[at0027]"},
	{template: "templates/family_history.v.1.2.3", entry: "generate", policy: "minimal", fill: "random", reason: "refusal_other:slot_fill:/content[openEHR-EHR-EVALUATION.family_history.v2]/data/items[at0003]/items[at0024]/items[at0027]"},
	{template: "templates/minimal_action_2", entry: "builder", policy: "minimal", fill: "example", reason: "floor_action:required:/content[0]/time"},
	{template: "templates/minimal_action_2", entry: "builder", policy: "minimal", fill: "example", reason: "floor_action:required:/content[openEHR-EHR-ACTION.minimal_2.v1]/time"},
	{template: "templates/minimal_action_2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_action_2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_action_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-ACTION.minimal_2.v1]/description/items[at0002]/value/type"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "example", reason: "floor_action:required:/content[0]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "example", reason: "floor_action:required:/content[openEHR-EHR-ACTION.minimal_2.v1]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-ACTION.minimal_2.v1]/description/items[at0002]/value/type"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "random", reason: "floor_action:required:/content[0]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "random", reason: "floor_action:required:/content[openEHR-EHR-ACTION.minimal_2.v1]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-ACTION.minimal_2.v1]/description/items[at0002]/value/type"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "example", reason: "floor_action:required:/content[0]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "example", reason: "floor_action:required:/content[openEHR-EHR-ACTION.minimal_2.v1]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-ACTION.minimal_2.v1]/description/items[at0002]/value/type"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "random", reason: "floor_action:required:/content[0]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "random", reason: "floor_action:required:/content[openEHR-EHR-ACTION.minimal_2.v1]/time"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_action_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-ACTION.minimal_2.v1]/description/items[at0002]/value/type"},
	{template: "templates/minimal_admin.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/items[0]/value/symbol"},
	{template: "templates/minimal_admin.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_admin.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/items[0]/value/symbol"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/items[0]/value/symbol"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/items[0]/value/symbol"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/items[0]/value/symbol"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_admin.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_evaluation.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_evaluation.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_evaluation.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_instruction.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_instruction.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_instruction.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:required:/content[0]/activities[0]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-INSTRUCTION.minimal.v1]/activities[at0001]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[0]/activities[0]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-INSTRUCTION.minimal.v1]/activities[at0001]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[0]/activities[0]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "example", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-INSTRUCTION.minimal.v1]/activities[at0001]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:required:/content[0]/activities[0]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:required:/content[openEHR-EHR-INSTRUCTION.minimal.v1]/activities[at0001]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:required:/content[0]/activities[0]/action_archetype_id"},
	{template: "templates/minimal_instruction.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:required:/content[openEHR-EHR-INSTRUCTION.minimal.v1]/activities[at0001]/action_archetype_id"},
	{template: "templates/minimal_observation.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_observation.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_observation.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/minimal_observation.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/minimal_observation.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/my_spanish_template_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/my_spanish_template_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/my_spanish_template_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/my_spanish_template_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/my_spanish_template_v0", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/my_spanish_template_v0", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/nested.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/items[0]/activities[0]/description/items[0]/value/symbol"},
	{template: "templates/nested.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/nested.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/nested.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-SECTION.nested.v1]/items[openEHR-EHR-INSTRUCTION.nested.v1]/activities[at0001]/description/items[at0002]/value"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/items[0]/activities[0]/description/items[0]/value/symbol"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-SECTION.nested.v1]/items[openEHR-EHR-INSTRUCTION.nested.v1]/activities[at0001]/description/items[at0002]/value"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/items[0]/activities[0]/description/items[0]/value/symbol"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-SECTION.nested.v1]/items[openEHR-EHR-INSTRUCTION.nested.v1]/activities[at0001]/description/items[at0002]/value"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/items[0]/activities[0]/description/items[0]/value/symbol"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-SECTION.nested.v1]/items[openEHR-EHR-INSTRUCTION.nested.v1]/activities[at0001]/description/items[at0002]/value"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/items[0]/activities[0]/description/items[0]/value/symbol"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/encoding"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/items[0]/language"},
	{template: "templates/nested.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-SECTION.nested.v1]/items[openEHR-EHR-INSTRUCTION.nested.v1]/activities[at0001]/description/items[at0002]/value"},
	{template: "templates/persistent_minimal.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/persistent_minimal.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/persistent_minimal.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/persistent_minimal.en.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/persistent_minimal.en.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0011]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0013]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0015]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/items[at0019]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0017]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0021]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0027]/name/value"},
	{template: "templates/test_template_rename_node_2", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-OBSERVATION.test_template_rename_node.v1]/data/events[at0002]/data/items[at0029]/name/value"},
	{template: "templates/vital_signs", entry: "builder", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/protocol/items[0]/items"},
	{template: "templates/vital_signs", entry: "builder", policy: "minimal", fill: "example", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "builder", policy: "minimal", fill: "example", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/vital_signs", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/vital_signs", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_cluster:cardinality:/content[0]/protocol/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_cluster:cardinality:/content[1]/data/events[0]/state/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_cluster:cardinality:/content[1]/protocol/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[2]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[2]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[2]/protocol/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[2]/protocol/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[3]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "floor_element:required:/content[3]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[3]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[3]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[1]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[1]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[2]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[2]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[3]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[3]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_cluster:cardinality:/content[0]/protocol/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_cluster:cardinality:/content[1]/data/events[0]/state/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_cluster:cardinality:/content[1]/protocol/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[2]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[2]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[2]/protocol/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[2]/protocol/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[3]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "floor_element:required:/content[3]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[3]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[3]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[1]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[1]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[2]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[2]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[3]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[3]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "example", reason: "floor_cluster:cardinality:/content[0]/protocol/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "example", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "example", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "random", reason: "floor_cluster:cardinality:/content[0]/protocol/items[0]/items"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "random", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/archetype_node_id"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "random", reason: "floor_element:required:/content[0]/data/events[0]/state/items[0]/name"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "templates/vital_signs", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/Corona_Anamnese", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/Corona_Anamnese", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/Corona_Anamnese", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/Corona_Anamnese", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/Corona_Anamnese", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "example", fill: "example", reason: "refusal_other:slot_fill:/content[openEHR-EHR-SECTION.adhoc.v1,'Allgemeine Angaben']/items[openEHR-EHR-INSTRUCTION.service_request.v1]/protocol/items[at0141]"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "example", fill: "random", reason: "refusal_other:slot_fill:/content[openEHR-EHR-SECTION.adhoc.v1,'Allgemeine Angaben']/items[openEHR-EHR-INSTRUCTION.service_request.v1]/protocol/items[at0141]"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/Corona_Anamnese", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/GECCO_Diagnose", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/items[2]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/items[4]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/GECCO_Diagnose", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "builder", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/items[2]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/items[4]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.absence.v2]/data/items[at0002]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.absence.v2]/data/items[at0005]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.absence.v2]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.exclusion_specific.v1]/data/items[at0003]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.exclusion_specific.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/items[2]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/items[4]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.absence.v2]/data/items[at0002]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.absence.v2]/data/items[at0005]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.absence.v2]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.exclusion_specific.v1]/data/items[at0003]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.exclusion_specific.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "example", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/items[2]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/items[4]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "example", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/items[2]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/items[4]/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/name/value"},
	{template: "webtemplate/GECCO_Diagnose", entry: "generate", policy: "minimal", fill: "random", reason: "validator_other:rm_type_mismatch:/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/name/value"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[2]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[3]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[4]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[5]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "builder", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[2]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[3]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[4]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[5]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[1]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[2]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[3]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_language:/content[3]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[1]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[1]/data/events[1]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[1]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[3]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/content[3]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0006]/items[at0008]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0006]/items[at0010]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0006]/items[at0011]/items[at0013]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0014]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0015]/items[at0017]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "example", reason: "validator_other:primitive_out_of_range:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0014]/value/numerator"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[2]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[3]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[4]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[5]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[1]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[2]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[3]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_language:/content[3]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[1]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[1]/data/events[1]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[1]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[3]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/content[3]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0006]/items[at0008]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0006]/items[at0010]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0006]/items[at0011]/items[at0013]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0014]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_not_in_list:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0015]/items[at0017]/value/type"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "example", fill: "random", reason: "validator_other:primitive_out_of_range:/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0014]/value/numerator"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[2]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[3]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[4]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[5]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "example", reason: "placeholder_time:/context/start_time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[0]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[1]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[2]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[3]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[4]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "ordinal_symbol:required:/content[0]/data/events[0]/data/items[5]/value/symbol"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/encoding"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_language:/content[0]/language"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/events[0]/time"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/content[0]/data/origin"},
	{template: "webtemplate/constrain_test", entry: "generate", policy: "minimal", fill: "random", reason: "placeholder_time:/context/start_time"},
}

func TestREQ107_CorpusRatchet(t *testing.T) {
	// PROBE-027 checks Generate then ValidateComposition on a few OPTs.
	// This ratchet records that distance for every vendored OPT (REQ-107).
	ctx := t.Context()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("ListAllOPTs returned no templates")
	}

	var compileFails []string
	observed := make(map[ratchetFailure]struct{})
	for _, ref := range refs {
		opt, err := template.ParseFile(ref.Path)
		if err != nil {
			compileFails = append(compileFails, ref.Name)
			continue
		}
		compiled, err := templatecompile.Compile(opt)
		if err != nil {
			compileFails = append(compileFails, ref.Name)
			continue
		}
		rootType := ""
		if compiled.Root() != nil {
			rootType = compiled.Root().RMTypeName()
		}
		compositionRoot := rootType == "COMPOSITION"

		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
				opts := instance.Options{
					Policy:      policy,
					Language:    "en",
					Now:         ratchetNow,
					ValueFill:   fill,
					ValueSource: mrand.NewPCG(1, 2),
				}
				if compositionRoot {
					opts.Territory = "NL"
					opts.Composer = fixedComposer()
				}
				out, err := instance.Generate(ctx, compiled, opts)
				if err != nil {
					reason := generateReason(t, err)
					recordReason(observed, ref.Name, entryGenerate, policy, fill, reason)
					continue
				}
				reasons := outputReasons(t, out, compiled, compositionRoot, fill)
				recordReasons(observed, ref.Name, entryGenerate, policy, fill, reasons)
			}
		}

		// REQ-101 is composition-only. A non-COMPOSITION root is not built.
		if !compositionRoot {
			continue
		}
		b, err := composition.NewBuilder(ctx, compiled,
			composition.WithLanguage("en"),
			composition.WithTerritory("NL"),
			composition.WithComposer(fixedComposer()),
			composition.WithNow(ratchetNow),
		)
		if err != nil {
			reason := generateReason(t, err)
			recordReason(observed, ref.Name, entryBuilder, instance.Minimal, instance.ExampleFill, reason)
			continue
		}
		comp, err := b.Build()
		if err != nil {
			reason := generateReason(t, err)
			recordReason(observed, ref.Name, entryBuilder, instance.Minimal, instance.ExampleFill, reason)
			continue
		}
		reasons := outputReasons(t, comp, compiled, true, instance.ExampleFill)
		recordReasons(observed, ref.Name, entryBuilder, instance.Minimal, instance.ExampleFill, reasons)
	}

	compileUnlisted, compileStale := diffStrings(setOf(compileFails), setOf(corpusCompileFailures))
	unlisted, stale := diffRatchet(observed, ratchetSet(t, corpusRatchet))
	if len(compileUnlisted) == 0 && len(compileStale) == 0 && len(unlisted) == 0 && len(stale) == 0 {
		return
	}

	var buf strings.Builder
	if len(compileUnlisted) > 0 || len(compileStale) > 0 {
		buf.WriteString("compile-failure allowlist mismatch\n")
		buf.WriteString("  unlisted: ")
		buf.WriteString(strings.Join(compileUnlisted, ", "))
		buf.WriteString("\n  now compiles: ")
		buf.WriteString(strings.Join(compileStale, ", "))
		buf.WriteByte('\n')
	}
	writeRows(&buf, "unlisted failures", unlisted)
	writeRows(&buf, "listed cases that now pass", stale)
	t.Fatalf("REQ-107 corpus ratchet\n%s", buf.String())
}

func fixedComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

func recordReason(dst map[ratchetFailure]struct{}, template, entry string, policy instance.Policy, fill instance.ValueFill, reason string) {
	dst[ratchetFailure{
		template: template,
		entry:    entry,
		policy:   policy.String(),
		fill:     fill.String(),
		reason:   reason,
	}] = struct{}{}
}

func recordReasons(dst map[ratchetFailure]struct{}, template, entry string, policy instance.Policy, fill instance.ValueFill, reasons map[string]struct{}) {
	for reason := range reasons {
		recordReason(dst, template, entry, policy, fill, reason)
	}
}

func outputReasons(t *testing.T, root any, compiled *templatecompile.Compiled, compositionRoot bool, fill instance.ValueFill) map[string]struct{} {
	t.Helper()
	reasons := make(map[string]struct{})
	note := func(res validation.Result) {
		if res.OK {
			return
		}
		for _, iss := range res.Issues {
			if iss.Severity != validation.Error {
				continue
			}
			reasons[issueReason(iss, fill)] = struct{}{}
		}
	}
	note(validation.ValidateRM(root))
	if compositionRoot {
		comp, err := instance.AsComposition(root)
		if err != nil {
			reasons[generateReason(t, err)] = struct{}{}
			return reasons
		}
		note(validation.ValidateComposition(comp, compiled))
	} else {
		// REQ-110: non-COMPOSITION roots use the generic validator.
		note(validation.Validate(root, compiled))
	}
	placeholders, err := placeholderReasons(root)
	if err != nil {
		t.Fatalf("placeholder scan: %v", err)
	}
	for reason := range placeholders {
		reasons[reason] = struct{}{}
	}
	return reasons
}

func generateReason(t *testing.T, err error) string {
	t.Helper()
	switch {
	case errors.Is(err, rmwrite.ErrUnknownRMType):
		tok, ok := lastQuotedToken(err.Error())
		if !ok {
			t.Fatalf("unknown RM type without a quoted locator: %v", err)
		}
		if tok == "STRING" {
			return reasonRefusalString + ":" + tok
		}
		return reasonRefusalOther + ":" + tok
	case errors.Is(err, rmwrite.ErrUnknownAttribute):
		tok, ok := lastQuotedToken(err.Error())
		if !ok {
			t.Fatalf("unknown attribute without a quoted locator: %v", err)
		}
		return reasonRefusalAttribute + ":" + tok
	case errors.Is(err, rmwrite.ErrTypeMismatch):
		tok, ok := lastQuotedToken(err.Error())
		if !ok {
			t.Fatalf("type mismatch without a quoted locator: %v", err)
		}
		return reasonRefusalOther + ":type_mismatch:" + tok
	case errors.Is(err, instance.ErrSlotFillUnsupported):
		path, ok := trailingPath(err.Error())
		if !ok {
			t.Fatalf("slot fill without a path: %v", err)
		}
		return reasonRefusalOther + ":slot_fill:" + path
	case errors.Is(err, instance.ErrTypeMismatch):
		return reasonRefusalOther + ":type_mismatch"
	default:
		t.Fatalf("unclassified generate error: %v", err)
		return ""
	}
}

// issueReason keys a validator finding by category, code, and path.
// The category is chosen from the code and the path's last attribute.
// Detail text is not read.
func issueReason(iss validation.Issue, fill instance.ValueFill) string {
	last := lastAttr(iss.Path)
	cat := reasonValidatorOther
	switch {
	case last == "symbol" && iss.Code == "required":
		cat = reasonOrdinalSymbol
	case fill == instance.RandomFill && iss.Code == "primitive_unit_unknown" && (last == "lower" || last == "upper"):
		cat = reasonRandomInterval
	case last == "items" && (iss.Code == "cardinality" || iss.Code == "required"):
		cat = reasonFloorCluster
	case iss.Code == "required" && (last == "name" || last == "archetype_node_id"):
		cat = reasonFloorElement
	case iss.Code == "required" && (last == "time" || last == "ism_transition"):
		cat = reasonFloorAction
	}
	return cat + ":" + iss.Code + ":" + iss.Path
}

func placeholderReasons(root any) (map[string]struct{}, error) {
	raw, err := canjson.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("canjson.Marshal: %w", err)
	}
	var tree any
	if err := jsonv2.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	if _, ok := tree.(map[string]any); !ok {
		return nil, fmt.Errorf("canonical JSON decoded as %T, want map[string]any", tree)
	}
	reasons := make(map[string]struct{})
	walkPlaceholder(tree, "", reasons)
	return reasons, nil
}

func walkPlaceholder(v any, path string, reasons map[string]struct{}) {
	switch n := v.(type) {
	case map[string]any:
		loc := path
		if loc == "" {
			loc = "/"
		}
		if typ, _ := n["_type"].(string); typ == "DV_DATE_TIME" && stringField(n, "value") == "example" {
			reasons[reasonPlaceholderTime+":"+loc] = struct{}{}
		}
		if attr := lastAttr(loc); (attr == "language" || attr == "encoding") && languageExample(n) {
			reasons[reasonPlaceholderLanguage+":"+loc] = struct{}{}
		}
		for k, child := range n {
			walkPlaceholder(child, joinKey(loc, k), reasons)
		}
	case []any:
		base := path
		if base == "" {
			base = "/"
		}
		for i, child := range n {
			walkPlaceholder(child, fmt.Sprintf("%s[%d]", base, i), reasons)
		}
	case string:
		if n != "example" {
			return
		}
		switch lastAttr(path) {
		case "time", "origin":
			reasons[reasonPlaceholderTime+":"+path] = struct{}{}
		case "language", "encoding":
			reasons[reasonPlaceholderLanguage+":"+path] = struct{}{}
		}
	}
}

// languageExample reports an ENTRY language or encoding whose code or
// value is exactly "example".
func languageExample(n map[string]any) bool {
	if stringField(n, "code_string") == "example" {
		return true
	}
	_, hasCode := n["code_string"]
	return !hasCode && stringField(n, "value") == "example"
}

func stringField(n map[string]any, key string) string {
	s, _ := n[key].(string)
	return s
}

func joinKey(parent, key string) string {
	if parent == "" || parent == "/" {
		return "/" + key
	}
	return parent + "/" + key
}

// lastQuotedToken returns the last double-quoted identifier in msg.
// The token is one run of letters, digits, and underscores, so a value
// such as "example.com" does not match.
func lastQuotedToken(msg string) (string, bool) {
	var last string
	found := false
	for i := 0; i < len(msg); i++ {
		if msg[i] != '"' {
			continue
		}
		j := i + 1
		for j < len(msg) && isTokenByte(msg[j]) {
			j++
		}
		if j > i+1 && j < len(msg) && msg[j] == '"' {
			last = msg[i+1 : j]
			found = true
			i = j
		}
	}
	return last, found
}

func isTokenByte(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// trailingPath returns the AQL path at the end of a slot-fill error.
// A name predicate may contain a space; a tab means the suffix is not a path.
func trailingPath(msg string) (string, bool) {
	const mark = ": /"
	i := strings.LastIndex(msg, mark)
	if i < 0 {
		return "", false
	}
	p := msg[i+len(mark)-1:]
	if p == "/" || strings.Contains(p, "\t") {
		return "", false
	}
	return p, true
}

func lastAttr(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.IndexByte(path, '['); i >= 0 {
		path = path[:i]
	}
	return path
}

func (r ratchetFailure) line() string {
	return r.template + "\t" + r.entry + "\t" + r.policy + "\t" + r.fill + "\t" + r.reason
}

func ratchetSet(t *testing.T, rows []ratchetFailure) map[ratchetFailure]struct{} {
	t.Helper()
	out := make(map[ratchetFailure]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := out[row]; ok {
			t.Errorf("duplicate ratchet row %s", row.line())
		}
		out[row] = struct{}{}
	}
	return out
}

func diffRatchet(got, want map[ratchetFailure]struct{}) (unlisted, stale []ratchetFailure) {
	for row := range got {
		if _, ok := want[row]; !ok {
			unlisted = append(unlisted, row)
		}
	}
	for row := range want {
		if _, ok := got[row]; !ok {
			stale = append(stale, row)
		}
	}
	slices.SortFunc(unlisted, cmpRatchet)
	slices.SortFunc(stale, cmpRatchet)
	return unlisted, stale
}

func cmpRatchet(a, b ratchetFailure) int {
	if c := strings.Compare(a.template, b.template); c != 0 {
		return c
	}
	if c := strings.Compare(a.entry, b.entry); c != 0 {
		return c
	}
	if c := strings.Compare(a.policy, b.policy); c != 0 {
		return c
	}
	if c := strings.Compare(a.fill, b.fill); c != 0 {
		return c
	}
	return strings.Compare(a.reason, b.reason)
}

func setOf(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

func diffStrings(got, want map[string]struct{}) (unlisted, stale []string) {
	for name := range got {
		if _, ok := want[name]; !ok {
			unlisted = append(unlisted, name)
		}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			stale = append(stale, name)
		}
	}
	slices.Sort(unlisted)
	slices.Sort(stale)
	return unlisted, stale
}

func writeRows(buf *strings.Builder, title string, rows []ratchetFailure) {
	if len(rows) == 0 {
		return
	}
	buf.WriteString(title)
	buf.WriteString(":\n")
	for _, row := range rows {
		buf.WriteString("  ")
		buf.WriteString(row.line())
		buf.WriteByte('\n')
	}
}
