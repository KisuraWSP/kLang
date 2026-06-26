package runtime

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

func fileValue(path string) Value {
	return objectValue("File", map[string]Value{
		"path":      StringValue(path),
		"name":      StringValue(filepath.Base(path)),
		"extension": StringValue(filepath.Ext(path)),
	})
}

func filePath(value Value, operation string) (string, error) {
	if !isObjectType(value, "File") {
		return "", Error{Message: fmt.Sprintf("%s expects File, got %s", operation, runtimeTypeName(value))}
	}
	pathValue, ok := value.Data.(ObjectData).Fields["path"]
	if !ok || pathValue.Kind != ValueString {
		return "", Error{Message: operation + " received an invalid File value"}
	}
	return pathValue.Data.(string), nil
}

func (runtime *Runtime) callFileBuiltin(name string, args []Value) (Value, error) {
	if name == "File" {
		if len(args) != 1 || args[0].Kind != ValueString {
			return NullValue(), Error{Message: "File expects one String path"}
		}
		return fileValue(args[0].Data.(string)), nil
	}

	expected := 1
	if name == "file_write" || name == "file_append" {
		expected = 2
	}
	if len(args) != expected {
		return NullValue(), Error{Message: fmt.Sprintf("%s expects %d argument(s), got %d", name, expected, len(args))}
	}
	path, err := filePath(args[0], name)
	if err != nil {
		return NullValue(), err
	}

	switch name {
	case "file_read":
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fileResultError(name, path, readErr), nil
		}
		return ResultOkValue(StringValue(string(content))), nil
	case "file_read_lines":
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fileResultError(name, path, readErr), nil
		}
		scanner := bufio.NewScanner(bytes.NewReader(content))
		scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
		lines := make([]Value, 0)
		for scanner.Scan() {
			lines = append(lines, StringValue(scanner.Text()))
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return fileResultError(name, path, scanErr), nil
		}
		return ResultOkValue(Value{Kind: ValueList, Data: lines}), nil
	case "file_write", "file_append":
		if args[1].Kind != ValueString {
			return NullValue(), Error{Message: fmt.Sprintf("%s content expects String, got %s", name, runtimeTypeName(args[1]))}
		}
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if name == "file_append" {
			flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		handle, openErr := os.OpenFile(path, flags, 0o644)
		if openErr != nil {
			return fileResultError(name, path, openErr), nil
		}
		written, writeErr := handle.WriteString(args[1].Data.(string))
		closeErr := handle.Close()
		if writeErr != nil {
			return fileResultError(name, path, writeErr), nil
		}
		if closeErr != nil {
			return fileResultError(name, path, closeErr), nil
		}
		return ResultOkValue(IntValue(written)), nil
	case "file_exists":
		_, statErr := os.Stat(path)
		if statErr == nil {
			return ResultOkValue(BoolValue(true)), nil
		}
		if os.IsNotExist(statErr) {
			return ResultOkValue(BoolValue(false)), nil
		}
		return fileResultError(name, path, statErr), nil
	case "file_size":
		info, statErr := os.Stat(path)
		if statErr != nil {
			return fileResultError(name, path, statErr), nil
		}
		if info.IsDir() {
			return ResultErrValue(StringValue(fmt.Sprintf("%s %q failed: path is a directory", name, path))), nil
		}
		return ResultOkValue(IntValue(int(info.Size()))), nil
	case "file_create":
		handle, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if createErr != nil {
			return fileResultError(name, path, createErr), nil
		}
		if closeErr := handle.Close(); closeErr != nil {
			return fileResultError(name, path, closeErr), nil
		}
		return ResultOkValue(fileValue(path)), nil
	case "file_remove":
		if removeErr := os.Remove(path); removeErr != nil {
			return fileResultError(name, path, removeErr), nil
		}
		return ResultOkValue(BoolValue(true)), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unknown File operation %q", name)}
	}
}

func fileResultError(operation string, path string, err error) Value {
	return ResultErrValue(StringValue(fmt.Sprintf("%s %q failed: %s", operation, path, err)))
}
