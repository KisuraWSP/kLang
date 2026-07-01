package bytecode

import "testing"

func FuzzBytecodeDecodeNeverPanics(f *testing.F) {
	valid, err := Encode(Program{
		Version: Version,
		Functions: []Function{{
			Name: "Main",
			Code: []Instruction{{Opcode: OpConstInt, A: 0}, {Opcode: OpReturn}},
		}},
	})
	if err != nil {
		f.Fatalf("encode seed: %v", err)
	}
	f.Add([]byte{})
	f.Add([]byte("KBC1"))
	f.Add(valid)
	f.Fuzz(func(t *testing.T, encoded []byte) {
		_, _ = Decode(encoded)
	})
}
