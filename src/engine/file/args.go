package file

import "slices"

func GetFilePath(argument []string) string {
	for index, arg := range argument {
		if arg == "--file" {
			return argument[index+1]
		}
	}
	return ""
}

func HasRunFlag(argument []string) bool {
	return slices.Contains(argument, "--run")
}
