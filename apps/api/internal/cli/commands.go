package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const usage = `paddock — sandbox control

Usage:
  paddock <command> [args]

Commands:
  ls                       list sandboxes
  get <id>                 show sandbox detail
  create                   create a new sandbox
  rm <id>                  remove a sandbox
  start <id>               start a sandbox
  stop <id>                stop a sandbox
  exec <id> -- <cmd...>    run a command inside a sandbox
  logs <id> [--tail N]     show sandbox logs
  tui                      launch interactive dashboard
  help                     show this help

Flags:
  ls, get   --json         output raw JSON

Env:
  PADDOCK_API_URL          API base URL (default http://localhost:8000)
`

// Execute dispatches a subcommand. Returns a process exit code.
func Execute(c *Client, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	cmd, rest := args[0], args[1:]
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var err error
	switch cmd {
	case "ls", "list":
		err = cmdList(ctx, c, rest)
	case "get", "inspect":
		err = cmdGet(ctx, c, rest)
	case "create":
		err = cmdCreate(ctx, c)
	case "rm", "remove", "delete":
		err = cmdSimple(ctx, c, rest, "rm", c.Remove)
	case "start":
		err = cmdState(ctx, c, rest, "start")
	case "stop":
		err = cmdState(ctx, c, rest, "stop")
	case "exec":
		err = cmdExec(ctx, c, rest)
	case "logs", "log":
		err = cmdLogs(ctx, c, rest)
	case "tui":
		err = cmdTUI(c)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func cmdList(ctx context.Context, c *Client, args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "output raw JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	list, err := c.List(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(list)
	}
	if len(list) == 0 {
		fmt.Println("no sandboxes")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATE\tIMAGE\tCREATED")
	for _, s := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.State, s.Image, s.Created.Format("2006-01-02 15:04"))
	}
	return w.Flush()
}

func cmdGet(ctx context.Context, c *Client, args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "output raw JSON")
	id, err := parseIDFlags(fs, args)
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("usage: paddock get <id>")
	}

	s, err := c.Get(ctx, id)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(s)
	}

	lastExec := "—"
	if !s.LastExec.IsZero() {
		lastExec = s.LastExec.Format(time.RFC3339)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, r := range [][2]string{
		{"ID", s.ID}, {"Name", s.Name}, {"State", s.State}, {"Image", s.Image},
		{"Container", s.ContainerId}, {"Engine", s.Engine}, {"Network", s.NetworkId},
		{"Volume", s.VolumeName}, {"Created", s.Created.Format(time.RFC3339)}, {"LastExec", lastExec},
	} {
		fmt.Fprintf(w, "%s\t%s\n", r[0], r[1])
	}
	fmt.Fprintf(w, "Terminal\t%s\n", portURL("http://localhost:"+s.Ports.Terminal, s.Ports.Terminal))
	fmt.Fprintf(w, "VNC\t%s\n", portURL("http://localhost:"+s.Ports.VNC+"/vnc.html", s.Ports.VNC))
	fmt.Fprintf(w, "CDP\t%s\n", portURL("http://localhost:"+s.Ports.CDP+"/json/version", s.Ports.CDP))
	return w.Flush()
}

func cmdCreate(ctx context.Context, c *Client) error {
	s, err := c.Create(ctx)
	if err != nil {
		return err
	}
	fmt.Println(s.ID)
	return nil
}

// cmdSimple handles commands that take a single id and return only an error.
func cmdSimple(ctx context.Context, c *Client, args []string, name string, fn func(context.Context, string) error) error {
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("usage: paddock %s <id>", name)
	}
	if err := fn(ctx, args[0]); err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", name, args[0])
	return nil
}

func cmdState(ctx context.Context, c *Client, args []string, state string) error {
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("usage: paddock %s <id>", state)
	}
	if err := c.ChangeState(ctx, args[0], state); err != nil {
		return err
	}
	past := map[string]string{"start": "started", "stop": "stopped"}[state]
	fmt.Printf("%s: %s\n", past, args[0])
	return nil
}

func cmdExec(ctx context.Context, c *Client, args []string) error {
	// Accept: exec <id> -- cmd...   or   exec <id> cmd...
	if len(args) < 2 {
		return errors.New("usage: paddock exec <id> -- <cmd...>")
	}
	id := args[0]
	rest := args[1:]
	if rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return errors.New("usage: paddock exec <id> -- <cmd...>")
	}

	res, err := c.Exec(ctx, id, rest)
	if err != nil {
		return err
	}
	if res.Stdout != "" {
		fmt.Fprint(os.Stdout, res.Stdout)
		if res.Stdout[len(res.Stdout)-1] != '\n' {
			fmt.Fprintln(os.Stdout)
		}
	}
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
		if res.Stderr[len(res.Stderr)-1] != '\n' {
			fmt.Fprintln(os.Stderr)
		}
	}
	// Surface the remote exit code to the shell.
	if res.ExitCode != 0 {
		os.Exit(res.ExitCode)
	}
	return nil
}

func cmdLogs(ctx context.Context, c *Client, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	tail := fs.Int("tail", 0, "number of lines from the end (0 = all)")
	id, err := parseIDFlags(fs, args)
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("usage: paddock logs <id> [--tail N]")
	}

	res, err := c.Logs(ctx, id, *tail)
	if err != nil {
		return err
	}
	fmt.Print(res.Logs)
	if res.Logs != "" && res.Logs[len(res.Logs)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func cmdTUI(c *Client) error {
	p := tea.NewProgram(NewModel(c), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// parseIDFlags allows the positional <id> to appear before flags
// (e.g. "get <id> --json"). If the first arg is non-flag it is taken as the id
// and the remainder parsed as flags; otherwise flags are parsed and the id is
// read from the first non-flag arg.
func parseIDFlags(fs *flag.FlagSet, args []string) (string, error) {
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		id := args[0]
		return id, fs.Parse(args[1:])
	}
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	return fs.Arg(0), nil
}

func portURL(url, port string) string {
	if port == "" || port == "0" {
		return "— (not bound)"
	}
	return url
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
