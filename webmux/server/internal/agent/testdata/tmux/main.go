// This executable is an isolated tmux protocol fixture for both backends. It
// never contacts a tmux server or starts another process.
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) >= 3 && (args[0] == "-L" || args[0] == "-S") {
		switch args[2] {
		case "list-sessions":
			fmt.Print("alpha-task-a\t1\t0\t1700000000\t1700000000\nalpha-task-b\t2\t1\t1700000100\t1700000100\n")
			return
		case "display-message":
			fmt.Println(os.Getenv("WEBMUX_AGENT_FIXTURE_CWD"))
			return
		case "attach-session":
		default:
			os.Exit(2)
		}
	}
	fmt.Println("agent-fixture-ready")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Println("agent-fixture-reply:" + scanner.Text())
		if scanner.Text() == "exit" {
			return
		}
	}
}
