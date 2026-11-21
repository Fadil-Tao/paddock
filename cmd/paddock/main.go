// Command paddock manages local microVM sandboxes for AI agents.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Fadil-Tao/paddock/internal/proto"
	"github.com/Fadil-Tao/paddock/internal/sandbox"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: paddock <command> [args]

commands:
  create [--cpus N] [--mem MiB] [--disk SIZE]
                                  boot a new sandbox, print its id
  exec <id> -- <cmd> [args...]    run a command in a sandbox
  list                            list sandboxes
  destroy <id>...                 stop and delete sandboxes
  version                         print version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	m := &sandbox.Manager{Root: sandbox.DefaultRoot(), Agent: agentPath()}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "create":
		err = create(ctx, m, args)
	case "exec":
		var code int
		code, err = execCmd(ctx, m, args)
		if err == nil {
			os.Exit(code)
		}
	case "list", "ls":
		err = list(m)
	case "destroy", "rm":
		err = destroy(m, args)
	case "version", "--version":
		fmt.Println(version)
		return
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "paddock: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "paddock:", err)
		os.Exit(1)
	}
}

func create(ctx context.Context, m *sandbox.Manager, args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	cpus := fs.Int("cpus", sandbox.DefaultCPUs, "vCPU count")
	mem := fs.Int("mem", sandbox.DefaultMemMiB, "memory in MiB")
	disk := fs.String("disk", "10G", "root disk size, e.g. 512M, 20G (sparse)")
	_ = fs.Parse(args) // ExitOnError: never returns an error

	diskBytes, err := parseSize(*disk)
	if err != nil {
		return fmt.Errorf("--disk: %w", err)
	}
	start := time.Now()
	sb, err := m.Create(ctx, sandbox.Spec{CPUs: *cpus, MemMiB: *mem, DiskBytes: diskBytes})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "ready in %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Println(sb.ID)
	return nil
}

func execCmd(ctx context.Context, m *sandbox.Manager, args []string) (int, error) {
	if len(args) < 2 {
		return 0, errors.New("usage: paddock exec <id> -- <cmd> [args...]")
	}
	id, argv := args[0], args[1:]
	if argv[0] == "--" {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return 0, errors.New("no command given")
	}
	return m.Exec(ctx, id, proto.Request{Argv: argv}, os.Stdout, os.Stderr)
}

func list(m *sandbox.Manager) error {
	sbs, err := m.List()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tCPUS\tMEM\tDISK\tCREATED")
	for _, sb := range sbs {
		status := "running"
		if !sb.Running {
			status = "dead"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%dMiB\t%s\t%s ago\n", sb.ID, status, sb.CPUs, sb.MemMiB,
			formatSize(sb.DiskBytes), time.Since(sb.CreatedAt).Round(time.Second))
	}
	return w.Flush()
}

func destroy(m *sandbox.Manager, ids []string) error {
	if len(ids) == 0 {
		return errors.New("usage: paddock destroy <id> [id...]")
	}
	for _, id := range ids {
		if err := m.Destroy(id); err != nil {
			return err
		}
		fmt.Println(id)
	}
	return nil
}

// agentPath returns $PADDOCK_AGENT, else paddock-agent next to this binary.
func agentPath() string {
	if p := os.Getenv("PADDOCK_AGENT"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return "paddock-agent"
	}
	return filepath.Join(filepath.Dir(exe), "paddock-agent")
}

// parseSize parses sizes like "512M", "20G" or "1048576" (bytes). Suffixes
// are binary: K=2^10, M=2^20, G=2^30, T=2^40.
func parseSize(s string) (int64, error) {
	units := map[byte]int64{'K': 1 << 10, 'M': 1 << 20, 'G': 1 << 30, 'T': 1 << 40}
	s = strings.ToUpper(strings.TrimSuffix(strings.TrimSpace(s), "iB"))
	s = strings.TrimSuffix(s, "B")
	mult := int64(1)
	if n := len(s); n > 0 {
		if u, ok := units[s[n-1]]; ok {
			mult, s = u, s[:n-1]
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	if n > math.MaxInt64/mult {
		return 0, fmt.Errorf("size %q too large", s)
	}
	return n * mult, nil
}

func formatSize(b int64) string {
	switch {
	case b >= 1<<30 && b%(1<<30) == 0:
		return fmt.Sprintf("%dG", b>>30)
	case b >= 1<<20:
		return fmt.Sprintf("%dM", b>>20)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
