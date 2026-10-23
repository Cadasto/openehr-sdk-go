package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_ElementNullReasonRefusesNil checks that an ELEMENT's null_reason
// refuses a nil child and a nil pointer with ErrTypeMismatch and stores
// nothing, as every other text attribute stored as given does, instead of
// storing a typed nil that reads back as a present reason.
func TestREQ107_ElementNullReasonRefusesNil(t *testing.T) {
	t.Parallel()
	for _, child := range []any{nil, (*rm.DVText)(nil), (*rm.DVCodedText)(nil)} {
		e := &rm.Element{}
		if err := EnsureSingle(e, "null_reason", child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("EnsureSingle(ELEMENT, null_reason, %#v) = %v, want ErrTypeMismatch", child, err)
		}
		if e.NullReason != nil {
			t.Errorf("ELEMENT.null_reason after refusing %#v = %#v, want none stored", child, e.NullReason)
		}
	}
}

// TestREQ107_IsmTransitionCareflowStepTakesValueOrPointer checks that an
// ISM_TRANSITION's careflow_step takes a DV_CODED_TEXT as a value or as a
// pointer, as its current_state and transition do, and refuses a nil
// pointer or a child of another type with ErrTypeMismatch.
func TestREQ107_IsmTransitionCareflowStepTakesValueOrPointer(t *testing.T) {
	t.Parallel()
	step := rm.DVCodedText{Value: "Prescribe", DefiningCode: rm.CodePhrase{CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"}}}
	for name, child := range map[string]any{"pointer": &step, "value": step} {
		i := &rm.IsmTransition{}
		if err := EnsureSingle(i, "careflow_step", child); err != nil {
			t.Errorf("EnsureSingle(ISM_TRANSITION, careflow_step, %s) = %v, want nil", name, err)
			continue
		}
		if i.CareflowStep == nil || i.CareflowStep.Value != step.Value || i.CareflowStep.DefiningCode != step.DefiningCode {
			t.Errorf("ISM_TRANSITION.careflow_step after writing a %s = %+v, want %+v", name, i.CareflowStep, step)
		}
	}
	for _, child := range []any{nil, (*rm.DVCodedText)(nil), &rm.DVText{Value: "Prescribe"}, "at0001"} {
		i := &rm.IsmTransition{}
		if err := EnsureSingle(i, "careflow_step", child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("EnsureSingle(ISM_TRANSITION, careflow_step, %#v) = %v, want ErrTypeMismatch", child, err)
		}
		if i.CareflowStep != nil {
			t.Errorf("ISM_TRANSITION.careflow_step after refusing %#v = %+v, want none stored", child, i.CareflowStep)
		}
	}
}
