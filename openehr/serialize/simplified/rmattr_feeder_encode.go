package simplified

// REQ-140 — the `_feeder_audit` family, encode side: the exact inverse of
// rmattr_feeder.go.
//
// Every nested value goes back out through the emitter that owns its shape — a
// party through C2's [partyRMAttr], an item id and an `original_content` through
// [emitLeafValue] — so the two directions cannot spell one datatype differently.
// The `|raw` carrier behind those leaf positions is what keeps the emitters total:
// an `original_content` whose charset the suffix set cannot hold rides
// `original_content|raw` rather than being refused.
//
// Two shapes are typed errors instead, because the grammar has no carrier for them
// anywhere: a populated `other_details` (deferred — see rmattr_feeder.go) and an
// empty RM-mandatory `system_id`, which decode requires and would read back as a
// fabricated blank.

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// feederAuditRMAttr writes a LOCATABLE's `_feeder_audit` under path. A nil audit
// writes nothing.
func feederAuditRMAttr(out map[string]any, path string, fa *rm.FeederAudit) error {
	if fa == nil {
		return nil
	}
	if err := identifierListRMAttr(out, path, "originating_system_item_id", fa.OriginatingSystemItemIds); err != nil {
		return err
	}
	if err := identifierListRMAttr(out, path, "feeder_system_item_id", fa.FeederSystemItemIds); err != nil {
		return err
	}
	if err := feederAuditDetailsRMAttr(out, path+"/originating_system_audit", fa.OriginatingSystemAudit); err != nil {
		return err
	}
	if fa.FeederSystemAudit != nil {
		if err := feederAuditDetailsRMAttr(out, path+"/feeder_system_audit", *fa.FeederSystemAudit); err != nil {
			return err
		}
	}
	return originalContentRMAttr(out, path, fa.OriginalContent)
}

// identifierListRMAttr writes one of FEEDER_AUDIT's two DV_IDENTIFIER lists,
// indexed by list position, through the DV_IDENTIFIER leaf emitter. The FLAT
// segment is singular where the RM attribute is plural — the reference's spelling.
func identifierListRMAttr(out map[string]any, base, seg string, ids []rm.DVIdentifier) error {
	for i, id := range ids {
		if _, err := emitLeafValue(out, feederItemIDPath(base, seg, i), id, "DV_IDENTIFIER", false, false); err != nil {
			return err
		}
	}
	return nil
}

// feederAuditDetailsRMAttr writes one FEEDER_AUDIT_DETAILS: its three own suffixes
// and its three party positions.
func feederAuditDetailsRMAttr(out map[string]any, path string, d rm.FeederAuditDetails) error {
	if d.OtherDetails != nil && !rm.IsTypedNil(d.OtherDetails) {
		return fmt.Errorf("%w: %q carries a FEEDER_AUDIT_DETAILS.other_details (an ITEM_STRUCTURE), which the reference's suffix set has no channel for and the pinned corpus never writes — deferred rather than dropped (REQ-140, see deviations.md)",
			ErrUnsupportedDatatype, path+"/other_details")
	}
	if d.SystemID == "" {
		return fmt.Errorf("%w: %q is RM-mandatory on FEEDER_AUDIT_DETAILS but empty; decode requires it and would read a blank back",
			ErrUnsupportedDatatype, path+"|system_id")
	}
	out[path+"|system_id"] = d.SystemID
	if d.VersionID != nil {
		out[path+"|version_id"] = *d.VersionID
	}
	if d.Time != nil {
		if err := bareDateTimeRMAttr(out, path+"|time", *d.Time); err != nil {
			return err
		}
	}
	if err := partyRMAttr(out, path+"/location", d.Location); err != nil {
		return err
	}
	if err := partyProxyRMAttr(out, path+"/subject", d.Subject); err != nil {
		return err
	}
	return partyRMAttr(out, path+"/provider", d.Provider)
}

// originalContentRMAttr writes FEEDER_AUDIT.original_content under whichever of the
// two key names its concrete DV_ENCAPSULATED subtype claims — the reference's
// choice-by-key-name, which is the only discriminator either spelling carries.
//
// Each goes out through its datatype's leaf emitter, so the `|raw` carrier behind it
// takes a value the suffix set cannot capture (a charset, a language, a thumbnail).
// A DV_ENCAPSULATED subtype outside the two the RM declares has no key name at all
// and is a typed error.
func originalContentRMAttr(out map[string]any, base string, content rm.DVEncapsulated) error {
	if content == nil || rm.IsTypedNil(content) {
		return nil
	}
	if p, ok := as[rm.DVParsable](content); ok {
		_, err := emitLeafValue(out, base+"/original_content", p, "DV_PARSABLE", false, false)
		return err
	}
	if m, ok := as[rm.DVMultimedia](content); ok {
		_, err := emitLeafValue(out, base+"/original_content_multimedia", m, "DV_MULTIMEDIA", false, false)
		return err
	}
	return fmt.Errorf("%w: %q cannot carry a %T; the choice is by key name and the grammar spells only DV_PARSABLE (`original_content`) and DV_MULTIMEDIA (`original_content_multimedia`)",
		ErrUnsupportedDatatype, base+"/original_content", content)
}
