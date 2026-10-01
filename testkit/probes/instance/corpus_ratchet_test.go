package instanceprobes_test

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

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

// ratchetFailure is one axis of the corpus ratchet. The table is a set
// of template, entry, policy, fill, and reason. A listed row whose axis
// no longer fails for that reason is stale. An observed failure with no
// row is unlisted.
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
var corpusRatchet = []ratchetFailure{
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalAttribute},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalAttribute},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalAttribute},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalAttribute},
	{template: "flat-conformance/conformance_ehrbase.de.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalAttribute},
	{template: "templates/Address.v2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalAttribute},
	{template: "templates/Address.v2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalAttribute},
	{template: "templates/Address.v2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalAttribute},
	{template: "templates/Address.v2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalAttribute},
	{template: "templates/BMI", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/BMI", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/BMI", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/BMI", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/BMI", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/BMI", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/BMI", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/BMI", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/BMI", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/BMI", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Demonstration.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonFloorCluster},
	{template: "templates/Demonstration.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Demonstration.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Demonstration.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonFloorCluster},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonFloorCluster},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonFloorCluster},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonFloorCluster},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Demonstration.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Episode.v2", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Episode.v2", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Episode.v2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/IDCR -  Adverse Reaction List.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/IDCR - Laboratory Test Report.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/IDCR Problem List.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/IDCR Problem List.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/IDCR Problem List.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Referral Request.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Referral Request.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Referral Request.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/TestPerson.v2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalAttribute},
	{template: "templates/TestPerson.v2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalAttribute},
	{template: "templates/TestPerson.v2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalAttribute},
	{template: "templates/TestPerson.v2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalAttribute},
	{template: "templates/Test_dv_boolean.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_false_true.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_boolean_true_false.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_coded_text_with_local_codes.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_count_range_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_date_time_validity_kind_constraint_v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonValidatorOther},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonValidatorOther},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonValidatorOther},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonValidatorOther},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ehr_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonValidatorOther},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_identifier_pattern_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_count_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_lower_upper_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_interval_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_multimedia_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_ordinal_with_constraints.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_parsable_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_proportion_open_constraint_precision_1.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_quantity_property_units_constrained_with_magnitude_range.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_list_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/Test_dv_text_pattern_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/Test_dv_uri_open_constraint.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/alternative_types.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/alternative_types.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/alternative_types.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/body_weight", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/body_weight", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/body_weight", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/body_weight", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/body_weight", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/body_weight", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/body_weight", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/body_weight", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/body_weight", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/body_weight", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/clinical_content_validation", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/clinical_content_validation", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/clinical_notes.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/clinical_notes.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/clinical_notes.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/cluster-slot.ehrbase.org.v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/family_history.v.1.2.3", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalOther},
	{template: "templates/family_history.v.1.2.3", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalOther},
	{template: "templates/family_history.v.1.2.3", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalOther},
	{template: "templates/family_history.v.1.2.3", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalOther},
	{template: "templates/family_history.v.1.2.3", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalOther},
	{template: "templates/minimal_action_2", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonFloorAction},
	{template: "templates/minimal_action_2", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_action_2", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonValidatorOther},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonFloorAction},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonValidatorOther},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonFloorAction},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonValidatorOther},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonFloorAction},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonValidatorOther},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonFloorAction},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_action_2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonValidatorOther},
	{template: "templates/minimal_admin.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/minimal_admin.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/minimal_admin.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_evaluation.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_evaluation.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_evaluation.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_evaluation.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_evaluation.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_instruction.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/minimal_instruction.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/minimal_instruction.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/minimal_instruction.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/minimal_instruction.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/minimal_observation.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_observation.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/minimal_observation.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/my_spanish_template_v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/my_spanish_template_v0", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/my_spanish_template_v0", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/nested.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/nested.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/nested.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonValidatorOther},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonValidatorOther},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonValidatorOther},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonValidatorOther},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonOrdinalSymbol},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/nested.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonValidatorOther},
	{template: "templates/persistent_minimal.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/persistent_minimal.en.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/persistent_minimal.en.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/terminology_test2.ehrbase.org.v1", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/test_template_rename_node", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node_2", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node_2", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node_2", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node_2", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "templates/test_template_rename_node_2", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "templates/vital_signs", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonFloorCluster},
	{template: "templates/vital_signs", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonFloorElement},
	{template: "templates/vital_signs", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/vital_signs", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "example", reason: reasonFloorCluster},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "example", reason: reasonFloorElement},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "random", reason: reasonFloorCluster},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "random", reason: reasonFloorElement},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonFloorCluster},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonFloorElement},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonFloorCluster},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonFloorElement},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "templates/vital_signs", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "webtemplate/Corona_Anamnese", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/Corona_Anamnese", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "webtemplate/Corona_Anamnese", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "webtemplate/Corona_Anamnese", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "webtemplate/Corona_Anamnese", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/Corona_Anamnese", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "webtemplate/Corona_Anamnese", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/Corona_Anamnese", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
	{template: "webtemplate/GECCO_Diagnose", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "webtemplate/GECCO_Diagnose", entry: entryGenerate, policy: "example", fill: "example", reason: reasonRefusalString},
	{template: "webtemplate/GECCO_Diagnose", entry: entryGenerate, policy: "example", fill: "random", reason: reasonRefusalString},
	{template: "webtemplate/GECCO_Diagnose", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonRefusalString},
	{template: "webtemplate/GECCO_Diagnose", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonRefusalString},
	{template: "webtemplate/constrain_test", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "webtemplate/constrain_test", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/constrain_test", entry: entryBuilder, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "example", reason: reasonOrdinalSymbol},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "example", reason: reasonPlaceholderTime},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "example", reason: reasonValidatorOther},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "random", reason: reasonOrdinalSymbol},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "random", reason: reasonPlaceholderTime},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "example", fill: "random", reason: reasonValidatorOther},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonOrdinalSymbol},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "minimal", fill: "example", reason: reasonPlaceholderTime},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonOrdinalSymbol},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderLanguage},
	{template: "webtemplate/constrain_test", entry: entryGenerate, policy: "minimal", fill: "random", reason: reasonPlaceholderTime},
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
					recordReason(observed, ref.Name, entryGenerate, policy, fill, generateReason(err))
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
			recordReason(observed, ref.Name, entryBuilder, instance.Minimal, instance.ExampleFill, generateReason(err))
			continue
		}
		comp, err := b.Build()
		if err != nil {
			recordReason(observed, ref.Name, entryBuilder, instance.Minimal, instance.ExampleFill, generateReason(err))
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
			reasons[reasonRefusalOther] = struct{}{}
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

func generateReason(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "unknown RM type") && strings.Contains(msg, `"STRING"`) {
		return reasonRefusalString
	}
	if strings.Contains(msg, "unknown attribute") {
		return reasonRefusalAttribute
	}
	return reasonRefusalOther
}

func issueReason(iss validation.Issue, fill instance.ValueFill) string {
	switch {
	case placeholderTimeIssue(iss):
		return reasonPlaceholderTime
	case placeholderLanguageIssue(iss):
		return reasonPlaceholderLanguage
	case pathHasAttr(iss.Path, "symbol"):
		return reasonOrdinalSymbol
	case fill == instance.RandomFill && randomIntervalIssue(iss):
		return reasonRandomInterval
	case floorClusterIssue(iss):
		return reasonFloorCluster
	case floorElementIssue(iss):
		return reasonFloorElement
	case floorActionIssue(iss):
		return reasonFloorAction
	default:
		return reasonValidatorOther
	}
}

func placeholderTimeIssue(iss validation.Issue) bool {
	if !strings.Contains(iss.Detail, `"example"`) {
		return false
	}
	if pathHasAttr(iss.Path, "time") || pathHasAttr(iss.Path, "origin") {
		return true
	}
	return lastAttr(iss.Path) == "value" && strings.Contains(iss.Detail, "date-time")
}

func placeholderLanguageIssue(iss validation.Issue) bool {
	if !pathHasAttr(iss.Path, "language") && !pathHasAttr(iss.Path, "encoding") {
		return false
	}
	d := iss.Detail
	return strings.Contains(d, "local::example") || strings.Contains(d, `"example"`)
}

func randomIntervalIssue(iss validation.Issue) bool {
	if iss.Code == "rm_invariant" && strings.Contains(iss.Detail, "DV_INTERVAL") && strings.Contains(iss.Detail, "lower") {
		return true
	}
	return iss.Code == "primitive_unit_unknown" && (pathHasAttr(iss.Path, "lower") || pathHasAttr(iss.Path, "upper"))
}

func floorClusterIssue(iss validation.Issue) bool {
	if lastAttr(iss.Path) != "items" {
		return false
	}
	if iss.Code != "cardinality" && iss.Code != "required" {
		return false
	}
	return detailType(iss.Detail) == "CLUSTER"
}

func floorElementIssue(iss validation.Issue) bool {
	attr := lastAttr(iss.Path)
	if attr != "name" && attr != "archetype_node_id" {
		return false
	}
	return iss.Code == "required" && detailType(iss.Detail) == "ELEMENT"
}

func floorActionIssue(iss validation.Issue) bool {
	attr := lastAttr(iss.Path)
	if attr != "time" && attr != "ism_transition" {
		return false
	}
	return iss.Code == "required" && detailType(iss.Detail) == "ACTION"
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

func walkPlaceholder(v any, parentKey string, reasons map[string]struct{}) {
	switch n := v.(type) {
	case map[string]any:
		if typ, _ := n["_type"].(string); typ == "DV_DATE_TIME" {
			if s, _ := n["value"].(string); s == "example" {
				reasons[reasonPlaceholderTime] = struct{}{}
			}
		}
		if (parentKey == "time" || parentKey == "origin") && stringField(n, "value") == "example" {
			reasons[reasonPlaceholderTime] = struct{}{}
		}
		if (parentKey == "language" || parentKey == "encoding") && codePhraseExample(n) {
			reasons[reasonPlaceholderLanguage] = struct{}{}
		}
		for k, child := range n {
			walkPlaceholder(child, k, reasons)
		}
	case []any:
		for _, child := range n {
			walkPlaceholder(child, parentKey, reasons)
		}
	case string:
		if n != "example" {
			return
		}
		if parentKey == "time" || parentKey == "origin" {
			reasons[reasonPlaceholderTime] = struct{}{}
		}
		if parentKey == "language" || parentKey == "encoding" {
			reasons[reasonPlaceholderLanguage] = struct{}{}
		}
	}
}

// codePhraseExample reports an ENTRY language or encoding whose code is
// the placeholder "example" (local::example on the wire), or a bare
// string value "example" with no code phrase.
func codePhraseExample(n map[string]any) bool {
	if stringField(n, "code_string") == "example" {
		return true
	}
	_, hasCode := n["code_string"]
	_, hasType := n["_type"]
	return stringField(n, "value") == "example" && !hasCode && !hasType
}

func stringField(n map[string]any, key string) string {
	s, _ := n[key].(string)
	return s
}

func pathHasAttr(path, attr string) bool {
	for seg := range strings.SplitSeq(path, "/") {
		if i := strings.IndexByte(seg, '['); i >= 0 {
			seg = seg[:i]
		}
		if seg == attr {
			return true
		}
	}
	return false
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

func detailType(detail string) string {
	const marker = " on "
	i := strings.LastIndex(detail, marker)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(detail[i+len(marker):])
	if j := strings.IndexAny(rest, " \t,;."); j >= 0 {
		rest = rest[:j]
	}
	return rest
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
		if _, ok := out[name]; ok {
			continue
		}
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
