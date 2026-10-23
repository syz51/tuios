package tuie2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuitest"
)

// Failure modes, recorded before implementation:
//   - A settled stty query misses the startup hold: the rc records stty before
//     the first prompt, independently of the renderer's eventual rectangle.
//   - zsh's initial PROMPT_SP wraps at an obsolete full-session width, before
//     OSC 133 A. Check fresh guest rows, without sending input or clearing.
//   - Suppressing PROMPT_SP or clearing the screen loses startup output with
//     no LF. A second fixture must retain that output in every fresh pane.
//   - Frame deductions, rail/dock reserves, gaps and nested splits differ.
//     Compare startup dimensions with the actual published content rectangle.
//   - Removing the startup hold crashes/deadlocks macOS bash 3.2. Exercise its
//     real prompt and subsequent input too; keep the existing winsize tests.
//
// Negative control: remove the sized send at addDaemonWindow's callsite.
func TestStartupPaneSize(t *testing.T) {
	for _, tc := range []struct {
		name, shell, appearance string
		shared, partial         bool
	}{
		{"shared", "/bin/zsh", "gap = 1\ndockbar_position = 'top'\n", true, false},
		{"framed-rail", "/bin/zsh", "gap = 3\n[appearance.sidebar]\nenabled = true\nposition = 'left'\n", false, false},
		{"partial", "/bin/zsh", "gap = 1\n", true, true},
		{"bash", "/bin/bash", "gap = 1\n", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := os.Stat(tc.shell); err != nil {
				t.Skip(err)
			}
			base := t.TempDir()
			var runtimeDir string
			// An explicitly supplied root keeps this focused run's sockets and
			// retained fixtures inside the parent's scratch directory on macOS.
			if root := os.Getenv("TUIOS_STARTUP_TEST_ROOT"); root != "" {
				var err error
				base, err = os.MkdirTemp(root, "startup-")
				if err != nil {
					t.Fatal(err)
				}
				runtimeDir = filepath.Join(root, "r")
			} else {
				runtimeDir = xdgDir(base, "XDG_RUNTIME_DIR")
			}
			env := append(os.Environ(), "TUIOS_SOCKET=", "TUIOS_SERVER=g", "TERM=xterm-256color", "COLORTERM=truecolor", "BASH_SILENCE_DEPRECATION_WARNING=1")
			for _, key := range xdgKeys {
				dir := filepath.Join(base, key)
				if key == "XDG_RUNTIME_DIR" {
					dir = runtimeDir
				}
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				env = append(env, key+"="+dir)
			}
			write := func(name, body string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(base, name), []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			rc := `stty size > "$STARTUP_ROOT/$TUIOS_WINDOW_ID.size"
`
			if tc.partial {
				rc += "printf 'STARTUP-PARTIAL'\n"
			}
			wrapper := "#!/bin/sh\nexec " + tc.shell
			if tc.shell == "/bin/zsh" {
				rc += "PROMPT_EOL_MARK=''\nPS1=$'%{\\e]133;A\\a%}➜ %{\\e]133;B\\a%}'\n"
				rc += "preexec() { printf '\\e]133;C\\a'; }\nprecmd() { printf '\\e]133;D;%d\\a' \"$?\"; }\n"
				write(".zshrc", rc, 0o600)
				wrapper += " -di\n"
			} else {
				rc += "PS1='BASH-READY> '\n"
				write("bashrc", rc, 0o600)
				wrapper += " --noprofile --rcfile \"$STARTUP_ROOT/bashrc\" -i\n"
			}
			write("shell", wrapper, 0o700)
			env = append(env, "SHELL="+filepath.Join(base, "shell"), "ZDOTDIR="+base, "STARTUP_ROOT="+base)
			cfg := filepath.Join(base, "XDG_CONFIG_HOME", "tuios")
			if err := os.MkdirAll(cfg, 0o700); err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("[startup]\ntiled = true\nopen_default_window = true\nstart_in_terminal_mode = false\n[appearance]\nshared_borders = %t\nwindow_title_position = 'hidden'\n%s", tc.shared, tc.appearance)
			if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			var commands strings.Builder
			cli := func(args ...string) string {
				t.Helper()
				cmd := exec.Command(tuiosBin, append([]string{"--server", "g"}, args...)...)
				cmd.Env, cmd.Dir = env, base
				out, err := cmd.CombinedOutput()
				fmt.Fprintf(&commands, "%q\n%s\nerror=%v\n", args, out, err)
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			t.Cleanup(func() {
				cmd := exec.Command(tuiosBin, "--server", "g", "kill-server")
				cmd.Env = env
				_ = cmd.Run()
				_ = os.WriteFile(filepath.Join(artifactDir(t), "commands.log"), []byte(commands.String()), 0o600)
			})
			term := tuitest.StartT(t, []string{tuiosBin, "--server", "g", "new", "startup", "--no-animations"}, tuitest.WithSize(120, 40), tuitest.WithEnv(env...), tuitest.WithDir(base))
			prompt := "➜"
			if tc.shell == "/bin/bash" {
				prompt = "BASH-READY>"
			}
			waitForAll(t, term, bootTimeout, "initial shell", prompt)
			check := func(id, label string) {
				t.Helper()
				if err := term.WaitFor(func(s tuitest.Screen) bool { return strings.Count(s.Text(), prompt) > 0 }, uiTimeout); err != nil {
					t.Fatal(err)
				}
				var result struct {
					Windows []layoutRect `json:"windows"`
				}
				var text string
				deadline := time.Now().Add(uiTimeout)
				for time.Now().Before(deadline) {
					text = cli("capture-pane", "-s", "startup", "-w", id)
					if strings.Contains(text, prompt) {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err := json.Unmarshal([]byte(cli("list-windows", "-s", "startup", "--json")), &result); err != nil {
					t.Fatal(err)
				}
				var cols, rows int
				for _, w := range result.Windows {
					if w.ID == id {
						cols, rows = w.Width, w.Height
					}
				}
				if !tc.shared {
					cols, rows = max(cols-2, 1), max(rows-2, 1)
				}
				initial, err := os.ReadFile(filepath.Join(base, id+".size"))
				var initialRows, initialCols int
				_, scanErr := fmt.Sscanf(string(initial), "%d %d", &initialRows, &initialCols)
				saveArtifact(t, term, artifactDir(t), label)
				savePNG(t, term.Screen(), hostPalette(t, ""), artifactDir(t), label)
				_ = os.WriteFile(filepath.Join(artifactDir(t), label+"-guest.txt"), []byte(text), 0o600)
				if err != nil || scanErr != nil || cols != initialCols || rows != initialRows {
					t.Errorf("ASSERTION: %s startup kernel PTY %dx%d (%v, %v), rendered content %dx%d", label, initialCols, initialRows, err, scanErr, cols, rows)
				}
				if !strings.Contains(text, prompt) || strings.HasPrefix(text, "\n") || (tc.partial && !strings.Contains(text, "STARTUP-PARTIAL")) {
					t.Errorf("ASSERTION: %s startup guest output: %q", label, text)
				}
			}
			var result struct {
				Windows []layoutRect `json:"windows"`
			}
			if err := json.Unmarshal([]byte(cli("list-windows", "-s", "startup", "--json")), &result); err != nil || len(result.Windows) != 1 {
				t.Fatalf("initial windows: %v %+v", err, result)
			}
			id := result.Windows[0].ID
			check(id, "initial")
			// Exercise the actual new-window key, both with automatic insertion
			// and with a preselection that addDaemonWindow must consume once.
			for _, side := range []string{"", "left"} {
				if side != "" {
					cli("run-command", "-s", "startup", "Preselect", side)
				}
				if err := term.SendKeys("n"); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(uiTimeout)
				var added string
				for time.Now().Before(deadline) {
					if err := json.Unmarshal([]byte(cli("list-windows", "-s", "startup", "--json")), &result); err != nil {
						t.Fatal(err)
					}
					if len(result.Windows) == 2 {
						for _, w := range result.Windows {
							if w.ID != id {
								added = w.ID
							}
						}
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if added == "" {
					t.Fatal("new-window key made no pane")
				}
				check(added, "ordinary-"+side)
				cli("run-command", "-s", "startup", "CloseWindow", added)
				deadline = time.Now().Add(uiTimeout)
				for time.Now().Before(deadline) {
					if err := json.Unmarshal([]byte(cli("list-windows", "-s", "startup", "--json")), &result); err != nil {
						t.Fatal(err)
					}
					if len(result.Windows) == 1 {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			for n, side := range []string{"right", "down", "right"} {
				var split struct {
					WindowID string `json:"window_id"`
				}
				if err := json.Unmarshal([]byte(cli("split-window", side, "-s", "startup", "-w", id, "--json")), &split); err != nil || split.WindowID == "" {
					t.Fatalf("split result: %v %+v", err, split)
				}
				id = split.WindowID
				check(id, fmt.Sprintf("split-%d-%s", n, side))
			}
			cli("send-text", "-s", "startup", "-w", id, "printf 'AFTER-%s' preserved\n")
			waitForAll(t, term, shellTimeout, "shell accepts input after startup", "AFTER-preserved")
			saveArtifact(t, term, artifactDir(t), "after-input")
			savePNG(t, term.Screen(), hostPalette(t, ""), artifactDir(t), "after-input")
		})
	}
}
