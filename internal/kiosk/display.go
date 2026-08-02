package kiosk

import (
	"fmt"
	"os"
	"os/exec"
)

func runDisplayCommands(display string, cmds [][]string) error {
	for _, args := range cmds {
		if len(args) == 0 {
			continue
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%v: %w", args, err)
		}
	}
	return nil
}
