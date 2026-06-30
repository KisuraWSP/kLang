package bytecode

import "fmt"

const (
	Magic   = "KBC1"
	Version = uint16(3)
)

type Opcode byte

const (
	OpConstNull Opcode = iota
	OpConstInt
	OpConstFloat
	OpConstBool
	OpConstString
	OpLoadLocal
	OpStoreLocal
	OpPop
	OpNegate
	OpNot
	OpAdd
	OpSubtract
	OpMultiply
	OpDivide
	OpFloorDivide
	OpModulo
	OpPower
	OpEqual
	OpNotEqual
	OpGreater
	OpGreaterEqual
	OpLess
	OpLessEqual
	OpJump
	OpJumpIfFalse
	OpJumpIfTrue
	OpCall
	OpPrint
	OpAssert
	OpReturn
	OpMakeList
	OpIndex
	OpStoreIndexLocal
	OpLength
	OpValidateRange
	OpIndexLocal
	OpLengthLocal
	OpConstFunction
	OpPipeline
)

func (opcode Opcode) String() string {
	names := [...]string{
		"CONST_NULL", "CONST_INT", "CONST_FLOAT", "CONST_BOOL", "CONST_STRING",
		"LOAD_LOCAL", "STORE_LOCAL", "POP", "NEGATE", "NOT", "ADD", "SUBTRACT",
		"MULTIPLY", "DIVIDE", "FLOOR_DIVIDE", "MODULO", "POWER", "EQUAL",
		"NOT_EQUAL", "GREATER", "GREATER_EQUAL", "LESS", "LESS_EQUAL", "JUMP",
		"JUMP_IF_FALSE", "JUMP_IF_TRUE", "CALL", "PRINT", "ASSERT", "RETURN",
		"MAKE_LIST", "INDEX", "STORE_INDEX_LOCAL", "LENGTH", "VALIDATE_RANGE",
		"INDEX_LOCAL", "LENGTH_LOCAL",
		"CONST_FUNCTION", "PIPELINE",
	}
	if int(opcode) >= len(names) {
		return fmt.Sprintf("OP_%d", opcode)
	}
	return names[opcode]
}

type PipelineMethod int64

const (
	PipelineIter PipelineMethod = iota
	PipelineFilter
	PipelineMap
	PipelineSkip
	PipelineTake
	PipelineCollect
	PipelineSort
	PipelineFold
	PipelineAny
	PipelineAll
	PipelineForEach
)

type Instruction struct {
	Opcode Opcode
	A      int64
	B      int64
	Line   int
}

type Function struct {
	Name       string
	Parameters int
	Locals     int
	Code       []Instruction
}

type Program struct {
	Version   uint16
	Entry     int
	Strings   []string
	Functions []Function
}

type Diagnostic struct {
	File    string
	Line    int
	Column  int
	Message string
	Hint    string
}
