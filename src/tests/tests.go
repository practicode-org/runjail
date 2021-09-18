package tests

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/practicode-org/worker/src/api"
)

func CheckExitCode(check api.TestCheck, exitCode int) (bool, error) {
	if check.Type == "exit_code" {
		desiredExitCode, err := strconv.Atoi(check.Arg)
		if err != nil {
			return false, fmt.Errorf("failed to perform %s check: %v", check.Type, err)
		}
		return exitCode == desiredExitCode, nil
	}
	return true, nil
}

func CheckSourceCode(check api.TestCheck, sourceTexts []string) (bool, error) {
	if check.Type == "text_contains" {
		for _, text := range sourceTexts {
			if strings.Contains(text, check.Arg) {
				return true, nil
			}
		}
		return false, nil
	} else if check.Type == "text_excludes" {
		for _, text := range sourceTexts {
			if strings.Contains(text, check.Arg) {
				return false, nil
			}
		}
		return true, nil
	}
	return true, nil
}
