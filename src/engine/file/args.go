package file

import "slices"

func GetFilePath(argument []string) string {
	for index, arg := range argument {
		if arg == "--file" && index+1 < len(argument) {
			return argument[index+1]
		}
	}
	return ""
}

func GetTestsPath(argument []string) string {
	for index, arg := range argument {
		if arg == "--tests" && index+1 < len(argument) {
			return argument[index+1]
		}
	}
	return ""
}

func GetProgramPath(argument []string) string {
	for index, arg := range argument {
		if arg == "--program" && index+1 < len(argument) {
			return argument[index+1]
		}
	}
	return ""
}

func HasRunFlag(argument []string) bool {
	return slices.Contains(argument, "--run")
}
