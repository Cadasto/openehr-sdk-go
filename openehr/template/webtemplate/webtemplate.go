package webtemplate

import (
	"encoding/json"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// defaultVersion is the EHRbase openEHR_SDK WebTemplate schema version
// this package mirrors (REQ-106, ADR-0014).
const defaultVersion = "2.3"

// WebTemplate is the root of the exported document (REQ-106).
type WebTemplate struct {
	TemplateID      string   `json:"templateId"`
	Version         string   `json:"version"`
	DefaultLanguage string   `json:"defaultLanguage"`
	Languages       []string `json:"languages"`
	Tree            *Node    `json:"tree"`
}

// Node is one element of the WebTemplate tree (REQ-106). A max of -1
// denotes an unbounded upper occurrence.
type Node struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name,omitempty"`
	LocalizedName         string            `json:"localizedName,omitempty"`
	RMType                string            `json:"rmType"`
	NodeID                string            `json:"nodeId,omitempty"`
	Min                   int               `json:"min"`
	Max                   int               `json:"max"`
	LocalizedNames        map[string]string `json:"localizedNames,omitempty"`
	LocalizedDescriptions map[string]string `json:"localizedDescriptions,omitempty"`
	AQLPath               string            `json:"aqlPath"`
	Inputs                []Input           `json:"inputs,omitempty"`
	Children              []*Node           `json:"children,omitempty"`
}

// Input is one logical form input under a leaf Node (REQ-106).
type Input struct {
	Suffix      string          `json:"suffix,omitempty"`
	Type        string          `json:"type"`
	List        []InputListItem `json:"list,omitempty"`
	ListOpen    bool            `json:"listOpen,omitempty"`
	Validation  *Validation     `json:"validation,omitempty"`
	Terminology string          `json:"terminology,omitempty"`
}

// InputListItem is one entry of a coded or ordinal input's list.
type InputListItem struct {
	Value           string            `json:"value"`
	Label           string            `json:"label,omitempty"`
	Ordinal         *int              `json:"ordinal,omitempty"`
	LocalizedLabels map[string]string `json:"localizedLabels,omitempty"`
}

// Validation carries a numeric or temporal constraint on an input.
type Validation struct {
	Range     *Range `json:"range,omitempty"`
	Precision *Range `json:"precision,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

// Range is a numeric interval with optional inclusive/exclusive operators.
type Range struct {
	Min   *float64 `json:"min,omitempty"`
	MinOp string   `json:"minOp,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	MaxOp string   `json:"maxOp,omitempty"`
}

type config struct {
	version         string
	defaultLanguage string
	languages       []string
}

// Option customises Build and Marshal.
type Option func(*config)

// WithVersion overrides the emitted schema version (default "2.3").
func WithVersion(v string) Option { return func(c *config) { c.version = v } }

// WithDefaultLanguage overrides the default language code (otherwise taken
// from the compiled template).
func WithDefaultLanguage(code string) Option {
	return func(c *config) { c.defaultLanguage = code }
}

// WithLanguages overrides the language set emitted for localized text.
func WithLanguages(codes ...string) Option {
	return func(c *config) { c.languages = codes }
}

// ErrEmptyTemplate is returned when the compiled template has no root.
var ErrEmptyTemplate = errors.New("webtemplate: compiled template has no root")

// Marshal builds and JSON-encodes the WebTemplate (REQ-106).
func Marshal(c *templatecompile.Compiled, opts ...Option) ([]byte, error) {
	wt, err := Build(c, opts...)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wt)
}
