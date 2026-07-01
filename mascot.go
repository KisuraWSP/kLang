package main

import (
	"fmt"
	"os"
	"strings"
)

type mascotMood string

const (
	mascotHelp    mascotMood = "help"
	mascotSuccess mascotMood = "success"
	mascotError   mascotMood = "error"
)

func printMascot(out *os.File, mood mascotMood) {
	if !mascotEnabled(out) {
		return
	}

	face := "[^_^]"
	message := "Kibi: Ready when you are—pick a command and let's build something neat!"
	switch mood {
	case mascotSuccess:
		message = "Kibi: Nice work! Your program finished safely."
	case mascotError:
		face = "[o_o]"
		message = "Kibi: We hit a snag, but the diagnostic above points the way. You've got this!"
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "   %s\n", face)
	fmt.Fprintln(out, `  /|___|\`)
	fmt.Fprintln(out, `    / \`)
	fmt.Fprintln(out, message)
	fmt.Fprintln(out)
}

func mascotEnabled(out *os.File) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KLANG_MASCOT"))) {
	case "always", "1", "true", "yes":
		return true
	case "never", "0", "false", "no":
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	info, err := out.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
