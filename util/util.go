package util

import (
    "os/exec"
)

// Execute executes the given system command
func Execute(command string, isUsingPipes bool, params ...string) string {
    if isUsingPipes {
        cmd := exec.Command("bash", "-c", command)
        stdout, err := cmd.Output()
        if err != nil {
            return err.Error()
        }
        return string(stdout)
    } else {
        cmd := exec.Command(command, params...)
        stdout, err := cmd.Output()
        if err != nil {
            return err.Error()
        }
        return string(stdout)
    }
}