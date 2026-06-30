package bytecode

import (
	"reflect"
	"strings"
	"testing"

	"kLang/src/engine/ir"
)

func TestCodecRoundTrip(t *testing.T) {
	original := Program{
		Version: Version,
		Entry:   0,
		Strings: []string{"hello"},
		Functions: []Function{{
			Name: "Main", Locals: 1,
			Code: []Instruction{
				{Opcode: OpConstString, A: 0, Line: 2},
				{Opcode: OpPrint, A: 1, Line: 2},
				{Opcode: OpPop, Line: 2},
				{Opcode: OpConstInt, A: 42, Line: 3},
				{Opcode: OpReturn, Line: 3},
			},
		}},
	}
	encoded, err := Encode(original)
	if err != nil {
		t.Fatalf("encode bytecode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode bytecode: %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("round trip mismatch:\nwant %#v\ngot  %#v", original, decoded)
	}
}

func TestCompilerAndVMExecuteTypedCoreLoop(t *testing.T) {
	position := ir.Position{File: "main.klang", Line: 1, Column: 1}
	identifier := func(name string) ir.Expression {
		return ir.Expression{Kind: ir.ExpressionIdentifier, Name: name}
	}
	literal := func(value string) ir.Expression {
		return ir.Expression{Kind: ir.ExpressionLiteral, Type: "Int", Value: value}
	}
	binary := func(left ir.Expression, operator string, right ir.Expression) ir.Expression {
		return ir.Expression{Kind: ir.ExpressionBinary, Operator: operator, Left: &left, Right: &right}
	}
	sumCall := ir.Expression{Kind: ir.ExpressionCall, Name: "Sum", Arguments: []ir.Expression{literal("5")}}
	resultIdentifier := identifier("result")
	program := ir.Program{
		Name: "vm-test", EntryPoint: "Main",
		Functions: []ir.Function{
			{
				Pos: position, Name: "Sum", ReturnType: "Int",
				Params: []ir.Binding{{Name: "limit", Type: "Int"}},
				Body: []ir.Statement{
					{Pos: position, Kind: ir.StatementVariable, Binding: ir.Binding{Name: "total", Type: "Int", Mutable: true}, Value: literal("0")},
					{Pos: position, Kind: ir.StatementVariable, Binding: ir.Binding{Name: "index", Type: "Int", Mutable: true}, Value: literal("0")},
					{
						Pos: position, Kind: ir.StatementWhile,
						Condition: binary(identifier("index"), "<", identifier("limit")),
						Body: []ir.Statement{
							{Pos: position, Kind: ir.StatementAssignment, Operator: "+=", Target: identifier("total"), Value: identifier("index")},
							{Pos: position, Kind: ir.StatementAssignment, Operator: "+=", Target: identifier("index"), Value: literal("1")},
						},
					},
					{Pos: position, Kind: ir.StatementReturn, Value: identifier("total")},
				},
			},
			{
				Pos: position, Name: "Main", ReturnType: "Int",
				Body: []ir.Statement{
					{Pos: position, Kind: ir.StatementVariable, Binding: ir.Binding{Name: "result", Type: "Int"}, Value: sumCall},
					{Pos: position, Kind: ir.StatementExpression, Value: ir.Expression{Kind: ir.ExpressionCall, Name: "print", Arguments: []ir.Expression{resultIdentifier}}},
					{Pos: position, Kind: ir.StatementReturn, Value: identifier("result")},
				},
			},
		},
	}
	compiled, diagnostics := Compile(program)
	if len(diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %#v", diagnostics)
	}
	encoded, err := Encode(compiled)
	if err != nil {
		t.Fatalf("encode bytecode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode bytecode: %v", err)
	}
	result, err := NewVM().Execute(decoded)
	if err != nil {
		t.Fatalf("execute bytecode: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Int != 10 {
		t.Fatalf("expected result 10, got %#v", result.Value)
	}
	if !reflect.DeepEqual(result.Output, []string{"10"}) {
		t.Fatalf("expected printed 10, got %#v", result.Output)
	}
	if result.Instructions == 0 {
		t.Fatal("expected executed instruction count")
	}
}

func TestVMReportsDivisionByZeroWithFunctionAndLine(t *testing.T) {
	program := Program{
		Version: Version,
		Functions: []Function{{
			Name: "Main",
			Code: []Instruction{
				{Opcode: OpConstInt, A: 1, Line: 7},
				{Opcode: OpConstInt, A: 0, Line: 7},
				{Opcode: OpDivide, Line: 7},
				{Opcode: OpReturn, Line: 7},
			},
		}},
	}
	_, err := NewVM().Execute(program)
	if err == nil || err.Error() != "bytecode runtime error at Main:7 [ip=2]: division by zero" {
		t.Fatalf("unexpected runtime error: %v", err)
	}
}

func TestDecoderRejectsTrailingData(t *testing.T) {
	encoded, err := Encode(Program{
		Version: Version,
		Functions: []Function{{
			Name: "Main",
			Code: []Instruction{{Opcode: OpConstNull}, {Opcode: OpReturn}},
		}},
	})
	if err != nil {
		t.Fatalf("encode bytecode: %v", err)
	}
	encoded = append(encoded, 0xff)
	if _, err := Decode(encoded); err == nil {
		t.Fatal("expected trailing bytecode data to be rejected")
	}
}

func TestVMEnforcesCallDepthLimit(t *testing.T) {
	program := Program{
		Version: Version,
		Functions: []Function{{
			Name: "Main",
			Code: []Instruction{{Opcode: OpCall, A: 0}, {Opcode: OpReturn}},
		}},
	}
	vm := NewVM()
	vm.CallDepthLimit = 4
	if _, err := vm.Execute(program); err == nil || !strings.Contains(err.Error(), "call depth limit exceeded") {
		t.Fatalf("expected call depth error, got %v", err)
	}
}

func TestVMReportsListBoundsErrors(t *testing.T) {
	program := Program{
		Version: Version,
		Functions: []Function{{
			Name: "Main", Locals: 1,
			Code: []Instruction{
				{Opcode: OpConstInt, A: 1, Line: 4},
				{Opcode: OpMakeList, A: 1, Line: 4},
				{Opcode: OpStoreLocal, A: 0, Line: 4},
				{Opcode: OpConstInt, A: 2, Line: 5},
				{Opcode: OpIndexLocal, A: 0, Line: 5},
				{Opcode: OpReturn, Line: 5},
			},
		}},
	}
	if _, err := NewVM().Execute(program); err == nil || !strings.Contains(err.Error(), "List index 2 is out of bounds") {
		t.Fatalf("expected List bounds error, got %v", err)
	}
}
