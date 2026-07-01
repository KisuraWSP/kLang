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
	File        string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

func (span Span) Valid() bool {
	return span.File != "" || span.StartLine > 0 || span.StartColumn > 0
}

type Label struct {
	Span    Span
	Message string
	Primary bool
}

type TextEdit struct {
	Span        Span
	Replacement string
	Message     string
}

type StackFrame struct {
	Function string
	File     string
	Line     int
	Column   int
}

// Diagnostic is shared by source loading, parsing, type checking, runtime
// execution, and compiler backends. The flat location fields are retained for
// API compatibility; Primary is the canonical structured representation.
type Diagnostic struct {
	Code      string
	Severity  Severity
	Phase     Phase
	File      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int

	Message    string
	Rule       string
	Hint       string
	SourceLine string

	Primary     Span
	Labels      []Label
	Notes       []string
	Helps       []string
	Suggestions []string
	Fixes       []TextEdit
	Frames      []StackFrame

	ExpectedType string
	FoundType    string
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
