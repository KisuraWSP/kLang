package file

import (
	"fmt"
	"testing"
)

func TestCheckForExistingFile(t *testing.T) {
	if !FileExists("tests/test1.klang") {
		t.Error("This file exists")
	}
}

func TestCheckForNonExistingFile(t *testing.T) {
	result := FileExists("tests/var.klang")
	fmt.Printf("\n--> Debug: Checking 'tests/var.klang'. Result was: %v\n", result)

	if result {
		t.Error("This file Doesn't Exist, but FileExists returned true")
	}
}
