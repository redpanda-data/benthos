package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dir := fs.String("dir", "", "clean room base directory (from prepare)")
	mode := fs.String("mode", "both", "which clean room to run: predict-output, predict-mapping, or both")
	agentCmd := fs.String("agent", "claude", "agent command")
	model := fs.String("model", "", "model flag passed to agent (e.g. sonnet)")
	maxTurns := fs.Int("max-turns", 500, "max turns for the agent")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: specagent run [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "error: --dir is required")
		fs.Usage()
		os.Exit(1)
	}

	modes := resolveModes(*mode)
	for _, m := range modes {
		cleanRoom := filepath.Join(*dir, m)
		if _, err := os.Stat(cleanRoom); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "error: clean room %s does not exist (run prepare first)\n", cleanRoom)
			os.Exit(1)
		}

		prompt, err := os.ReadFile(filepath.Join(cleanRoom, "PROMPT.md"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading prompt: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("=== Running agent in %s ===\n", m)
		if err := runAgent(cleanRoom, string(prompt), *agentCmd, *model, *maxTurns); err != nil {
			fmt.Fprintf(os.Stderr, "error running agent in %s: %v\n", m, err)
			os.Exit(1)
		}
		fmt.Printf("=== Agent finished %s ===\n\n", m)
	}
}

func resolveModes(mode string) []string {
	switch strings.ToLower(mode) {
	case "predict-output":
		return []string{"predict_output"}
	case "predict-mapping":
		return []string{"predict_mapping"}
	case "both":
		return []string{"predict_output", "predict_mapping"}
	default:
		fmt.Fprintf(os.Stderr, "error: unknown mode %q (use predict-output, predict-mapping, or both)\n", mode)
		os.Exit(1)
		return nil
	}
}

func runAgent(cleanRoomDir, prompt, agentCmd, model string, maxTurns int) error {
	cmdArgs := []string{
		"-p", prompt,
		"--dangerously-skip-permissions",
		"--max-turns", fmt.Sprintf("%d", maxTurns),
		"--allowedTools", "Read,Write,Glob,Grep",
	}
	if model != "" {
		cmdArgs = append([]string{"--model", model}, cmdArgs...)
	}

	cmd := exec.Command(agentCmd, cmdArgs...)
	cmd.Dir = cleanRoomDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
