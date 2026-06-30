package bytecode

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

func Encode(program Program) ([]byte, error) {
	if program.Version == 0 {
		program.Version = Version
	}
	var output bytes.Buffer
	output.WriteString(Magic)
	if err := binary.Write(&output, binary.LittleEndian, program.Version); err != nil {
		return nil, err
	}
	writeUvarint(&output, uint64(program.Entry))
	writeUvarint(&output, uint64(len(program.Strings)))
	for _, value := range program.Strings {
		writeBytes(&output, []byte(value))
	}
	writeUvarint(&output, uint64(len(program.Functions)))
	for _, function := range program.Functions {
		writeBytes(&output, []byte(function.Name))
		writeUvarint(&output, uint64(function.Parameters))
		writeUvarint(&output, uint64(function.Locals))
		writeUvarint(&output, uint64(len(function.Code)))
		for _, instruction := range function.Code {
			output.WriteByte(byte(instruction.Opcode))
			writeVarint(&output, instruction.A)
			writeVarint(&output, instruction.B)
			writeUvarint(&output, uint64(instruction.Line))
		}
	}
	return output.Bytes(), nil
}

func Decode(encoded []byte) (Program, error) {
	reader := bufio.NewReader(bytes.NewReader(encoded))
	magic := make([]byte, len(Magic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return Program{}, fmt.Errorf("read bytecode header: %w", err)
	}
	if string(magic) != Magic {
		return Program{}, fmt.Errorf("invalid bytecode magic %q", string(magic))
	}
	var version uint16
	if err := binary.Read(reader, binary.LittleEndian, &version); err != nil {
		return Program{}, fmt.Errorf("read bytecode version: %w", err)
	}
	if version != Version {
		return Program{}, fmt.Errorf("unsupported bytecode version %d", version)
	}
	entry, err := binary.ReadUvarint(reader)
	if err != nil {
		return Program{}, fmt.Errorf("read bytecode entry: %w", err)
	}
	program := Program{Version: version, Entry: int(entry)}
	stringCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return Program{}, fmt.Errorf("read string count: %w", err)
	}
	if stringCount > 1_000_000 {
		return Program{}, fmt.Errorf("string count %d exceeds limit", stringCount)
	}
	for index := uint64(0); index < stringCount; index++ {
		value, err := readBytes(reader)
		if err != nil {
			return Program{}, fmt.Errorf("read string %d: %w", index, err)
		}
		program.Strings = append(program.Strings, string(value))
	}
	functionCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return Program{}, fmt.Errorf("read function count: %w", err)
	}
	if functionCount > 100_000 {
		return Program{}, fmt.Errorf("function count %d exceeds limit", functionCount)
	}
	for index := uint64(0); index < functionCount; index++ {
		name, err := readBytes(reader)
		if err != nil {
			return Program{}, fmt.Errorf("read function %d name: %w", index, err)
		}
		parameters, err := binary.ReadUvarint(reader)
		if err != nil {
			return Program{}, fmt.Errorf("read function %s parameters: %w", name, err)
		}
		locals, err := binary.ReadUvarint(reader)
		if err != nil {
			return Program{}, fmt.Errorf("read function %s locals: %w", name, err)
		}
		codeCount, err := binary.ReadUvarint(reader)
		if err != nil {
			return Program{}, fmt.Errorf("read function %s code count: %w", name, err)
		}
		if codeCount > 10_000_000 {
			return Program{}, fmt.Errorf("function %s instruction count %d exceeds limit", name, codeCount)
		}
		function := Function{Name: string(name), Parameters: int(parameters), Locals: int(locals)}
		for instructionIndex := uint64(0); instructionIndex < codeCount; instructionIndex++ {
			opcode, err := reader.ReadByte()
			if err != nil {
				return Program{}, fmt.Errorf("read function %s instruction %d: %w", name, instructionIndex, err)
			}
			a, err := binary.ReadVarint(reader)
			if err != nil {
				return Program{}, fmt.Errorf("read function %s operand A: %w", name, err)
			}
			b, err := binary.ReadVarint(reader)
			if err != nil {
				return Program{}, fmt.Errorf("read function %s operand B: %w", name, err)
			}
			line, err := binary.ReadUvarint(reader)
			if err != nil {
				return Program{}, fmt.Errorf("read function %s line: %w", name, err)
			}
			if Opcode(opcode) > OpPipeline {
				return Program{}, fmt.Errorf("unknown opcode %d in function %s", opcode, name)
			}
			function.Code = append(function.Code, Instruction{Opcode: Opcode(opcode), A: a, B: b, Line: int(line)})
		}
		program.Functions = append(program.Functions, function)
	}
	if program.Entry < 0 || program.Entry >= len(program.Functions) {
		return Program{}, fmt.Errorf("entry function index %d is out of range", program.Entry)
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		if err == nil {
			return Program{}, fmt.Errorf("bytecode contains trailing data")
		}
		return Program{}, fmt.Errorf("read bytecode trailer: %w", err)
	}
	return program, nil
}

func writeUvarint(writer io.ByteWriter, value uint64) {
	var buffer [binary.MaxVarintLen64]byte
	count := binary.PutUvarint(buffer[:], value)
	for _, current := range buffer[:count] {
		_ = writer.WriteByte(current)
	}
}

func writeVarint(writer io.ByteWriter, value int64) {
	var buffer [binary.MaxVarintLen64]byte
	count := binary.PutVarint(buffer[:], value)
	for _, current := range buffer[:count] {
		_ = writer.WriteByte(current)
	}
}

func writeBytes(writer *bytes.Buffer, value []byte) {
	writeUvarint(writer, uint64(len(value)))
	writer.Write(value)
}

func readBytes(reader *bufio.Reader) ([]byte, error) {
	length, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	const maximumByteString = 64 << 20
	if length > maximumByteString {
		return nil, fmt.Errorf("byte string length %d exceeds limit", length)
	}
	value := make([]byte, int(length))
	_, err = io.ReadFull(reader, value)
	return value, err
}
