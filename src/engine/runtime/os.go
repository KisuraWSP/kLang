package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

func osValue() Value {
	lineSeparator := "\n"
	if runtime.GOOS == "windows" {
		lineSeparator = "\r\n"
	}
	return objectValue("OS", map[string]Value{
		"name":                StringValue(runtime.GOOS),
		"arch":                StringValue(runtime.GOARCH),
		"cpu_count":           IntValue(runtime.NumCPU()),
		"path_separator":      StringValue(string(os.PathSeparator)),
		"path_list_separator": StringValue(string(os.PathListSeparator)),
		"line_separator":      StringValue(lineSeparator),
	})
}

func (runtimeState *Runtime) callOSBuiltin(name string, args []Value) (Value, error) {
	if name == "OS" {
		if len(args) != 0 {
			return NullValue(), Error{Message: fmt.Sprintf("OS expects 0 arguments, got %d", len(args))}
		}
		return osValue(), nil
	}
	if len(args) == 0 || !isObjectType(args[0], "OS") {
		return NullValue(), Error{Message: name + " expects OS as its first argument"}
	}

	switch name {
	case "os_current_dir":
		if err := osArgumentCount(name, args, 1); err != nil {
			return NullValue(), err
		}
		path, err := os.Getwd()
		if err != nil {
			return osResultError(name, err), nil
		}
		return ResultOkValue(StringValue(path)), nil
	case "os_change_dir":
		if err := osArgumentCount(name, args, 2); err != nil {
			return NullValue(), err
		}
		path, err := osStringArgument(name, args[1], "path")
		if err != nil {
			return NullValue(), err
		}
		if changeErr := os.Chdir(path); changeErr != nil {
			return osResultError(name, changeErr), nil
		}
		return ResultOkValue(BoolValue(true)), nil
	case "os_temp_dir":
		if err := osArgumentCount(name, args, 1); err != nil {
			return NullValue(), err
		}
		return StringValue(os.TempDir()), nil
	case "os_home_dir":
		if err := osArgumentCount(name, args, 1); err != nil {
			return NullValue(), err
		}
		path, err := os.UserHomeDir()
		if err != nil {
			return osResultError(name, err), nil
		}
		return ResultOkValue(StringValue(path)), nil
	case "os_hostname":
		if err := osArgumentCount(name, args, 1); err != nil {
			return NullValue(), err
		}
		hostname, err := os.Hostname()
		if err != nil {
			return osResultError(name, err), nil
		}
		return ResultOkValue(StringValue(hostname)), nil
	case "os_process_id":
		if err := osArgumentCount(name, args, 1); err != nil {
			return NullValue(), err
		}
		return IntValue(os.Getpid()), nil
	case "os_get_env":
		if err := osArgumentCount(name, args, 2); err != nil {
			return NullValue(), err
		}
		key, err := osStringArgument(name, args[1], "name")
		if err != nil {
			return NullValue(), err
		}
		value, exists := os.LookupEnv(key)
		if !exists {
			return OptionNoneValue(), nil
		}
		return OptionSomeValue(StringValue(value)), nil
	case "os_set_env":
		if err := osArgumentCount(name, args, 3); err != nil {
			return NullValue(), err
		}
		key, err := osStringArgument(name, args[1], "name")
		if err != nil {
			return NullValue(), err
		}
		value, err := osStringArgument(name, args[2], "value")
		if err != nil {
			return NullValue(), err
		}
		if setErr := os.Setenv(key, value); setErr != nil {
			return osResultError(name, setErr), nil
		}
		return ResultOkValue(BoolValue(true)), nil
	case "os_unset_env":
		if err := osArgumentCount(name, args, 2); err != nil {
			return NullValue(), err
		}
		key, err := osStringArgument(name, args[1], "name")
		if err != nil {
			return NullValue(), err
		}
		if unsetErr := os.Unsetenv(key); unsetErr != nil {
			return osResultError(name, unsetErr), nil
		}
		return ResultOkValue(BoolValue(true)), nil
	case "os_environment":
		if err := osArgumentCount(name, args, 1); err != nil {
			return NullValue(), err
		}
		environment := map[string]Value{}
		entries := os.Environ()
		sort.Strings(entries)
		for _, entry := range entries {
			key, value, found := strings.Cut(entry, "=")
			if found {
				environment[key] = StringValue(value)
			}
		}
		return Value{Kind: ValueMap, Data: environment}, nil
	case "os_execute":
		if err := osArgumentCount(name, args, 3); err != nil {
			return NullValue(), err
		}
		command, err := osStringArgument(name, args[1], "command")
		if err != nil {
			return NullValue(), err
		}
		commandArgs, err := stringList(args[2])
		if err != nil {
			return NullValue(), Error{Message: "os_execute arguments expect List[String]"}
		}
		process := exec.Command(command, commandArgs...)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		process.Stdout = &stdout
		process.Stderr = &stderr
		runErr := process.Run()
		exitCode := 0
		success := runErr == nil
		if runErr != nil {
			var exitError *exec.ExitError
			if !errors.As(runErr, &exitError) {
				return osResultError(name, runErr), nil
			}
			exitCode = exitError.ExitCode()
		}
		return ResultOkValue(TableValue(map[string]Value{
			"command":   StringValue(command),
			"stdout":    StringValue(stdout.String()),
			"stderr":    StringValue(stderr.String()),
			"exit_code": IntValue(exitCode),
			"success":   BoolValue(success),
		})), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unknown OS operation %q", name)}
	}
}

func osArgumentCount(name string, args []Value, expected int) error {
	if len(args) != expected {
		return Error{Message: fmt.Sprintf("%s expects %d argument(s), got %d", name, expected, len(args))}
	}
	return nil
}

func osStringArgument(name string, value Value, label string) (string, error) {
	if value.Kind != ValueString {
		return "", Error{Message: fmt.Sprintf("%s %s expects String, got %s", name, label, runtimeTypeName(value))}
	}
	return value.Data.(string), nil
}

func osResultError(operation string, err error) Value {
	return ResultErrValue(StringValue(fmt.Sprintf("%s failed: %s", operation, err)))
}
