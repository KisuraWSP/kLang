package diagnostic

type Severity string

const (
	SeverityError   Severity = "ERROR"
	SeverityWarning Severity = "WARNING"
	SeverityNote    Severity = "NOTE"
	SeverityHelp    Severity = "HELP"
)

type Phase string

const (
	PhaseSource  Phase = "SOURCE"
	PhaseModule  Phase = "MODULE"
	PhaseParse   Phase = "PARSE"
	PhaseType    Phase = "TYPE"
	PhaseRuntime Phase = "RUNTIME"
	PhaseBackend Phase = "BACKEND"
	PhaseJS      Phase = "JS"
	PhaseWASM    Phase = "WASM"
)

const (
	CodeSourceFailure       = "K0001"
	CodeImportResolution    = "K1001"
	CodeSyntax              = "K1002"
	CodeUnknownIdentifier   = "K2001"
	CodeUnknownFunction     = "K2002"
	CodeUnknownType         = "K2003"
	CodeTypeMismatch        = "K2101"
	CodeStaticSemantics     = "K2201"
	CodeTransactionSafety   = "K3101"
	CodeRuntimeFailure      = "K4001"
	CodeRuntimeAssertion    = "K4002"
	CodeTransactionConflict = "K4101"
	CodeBackendUnsupported  = "K5001"
	CodeBackendFailure      = "K5002"
)

type Span struct {
	File        string `json:"file,omitempty"`
	StartLine   int    `json:"start_line,omitempty"`
	StartColumn int    `json:"start_column,omitempty"`
	EndLine     int    `json:"end_line,omitempty"`
	EndColumn   int    `json:"end_column,omitempty"`
}

func (span Span) Valid() bool {
	return span.File != "" || span.StartLine > 0 || span.StartColumn > 0
}

type Label struct {
	Span    Span   `json:"span"`
	Message string `json:"message,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type TextEdit struct {
	Span        Span   `json:"span"`
	Replacement string `json:"replacement,omitempty"`
	Message     string `json:"message,omitempty"`
}

type StackFrame struct {
	Function string `json:"function"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

// Diagnostic is shared by source loading, parsing, type checking, runtime
// execution, and compiler backends. The flat location fields are retained for
// API compatibility; Primary is the canonical structured representation.
type Diagnostic struct {
	Code      string   `json:"code"`
	Severity  Severity `json:"severity"`
	Phase     Phase    `json:"phase"`
	File      string   `json:"file,omitempty"`
	Line      int      `json:"line,omitempty"`
	Column    int      `json:"column,omitempty"`
	EndLine   int      `json:"end_line,omitempty"`
	EndColumn int      `json:"end_column,omitempty"`

	Message    string `json:"message"`
	Rule       string `json:"rule,omitempty"`
	FeatureID  string `json:"feature_id,omitempty"`
	Hint       string `json:"hint,omitempty"`
	SourceLine string `json:"source_line,omitempty"`

	Primary     Span         `json:"primary"`
	Labels      []Label      `json:"labels,omitempty"`
	Notes       []string     `json:"notes,omitempty"`
	Helps       []string     `json:"helps,omitempty"`
	Suggestions []string     `json:"suggestions,omitempty"`
	Fixes       []TextEdit   `json:"fixes,omitempty"`
	Frames      []StackFrame `json:"frames,omitempty"`

	ExpectedType string `json:"expected_type,omitempty"`
	FoundType    string `json:"found_type,omitempty"`
}

func Normalize(value Diagnostic) Diagnostic {
	if value.Severity == "" {
		value.Severity = SeverityError
	}
	if value.Primary.Valid() {
		if value.File == "" {
			value.File = value.Primary.File
		}
		if value.Line == 0 {
			value.Line = value.Primary.StartLine
		}
		if value.Column == 0 {
			value.Column = value.Primary.StartColumn
		}
		if value.EndLine == 0 {
			value.EndLine = value.Primary.EndLine
		}
		if value.EndColumn == 0 {
			value.EndColumn = value.Primary.EndColumn
		}
	} else {
		value.Primary = Span{
			File:        value.File,
			StartLine:   value.Line,
			StartColumn: value.Column,
			EndLine:     value.EndLine,
			EndColumn:   value.EndColumn,
		}
	}
	if value.EndLine == 0 && value.Line > 0 {
		value.EndLine = value.Line
		value.Primary.EndLine = value.Line
	}
	if value.EndColumn == 0 && value.Column > 0 {
		value.EndColumn = value.Column
		value.Primary.EndColumn = value.Column
	}
	return value
}

type Carrier interface {
	Diagnostic() Diagnostic
}
