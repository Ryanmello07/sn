package miner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/docopt/docopt-go"
)

// provider setup walks a new device through sign in, registration, the
// payout address and the background service. Each step runs the same
// command a person would run by hand, as a child of this one, so setup
// never behaves differently from them. It is safe to run again: a step
// that is already done says so and moves on.
//
// --yes answers every question with its default and --auth-code-file
// supplies the sign-in, so a fleet can run it unattended.

// runSelf runs this binary with args. Tests replace it.
var runSelf = func(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	executable, err := cliExecutable()
	if err != nil {
		return err
	}
	command := exec.Command(executable, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

// startSelf starts this binary with args and returns it running. Tests replace it.
var startSelf = func(args []string, stdout io.Writer, stderr io.Writer) (*exec.Cmd, error) {
	executable, err := cliExecutable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(executable, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command, command.Start()
}

type setupRun struct {
	in     *os.File
	out    io.Writer
	errOut io.Writer
	yes    bool
	steps  int
	step   int
}

func (self *setupRun) heading(title string) {
	self.step++
	fmt.Fprintf(self.out, "\n%s  %s\n", cliBold(self.out, fmt.Sprintf("Step %d of %d", self.step, self.steps)), title)
}

func (self *setupRun) ok(text string) {
	fmt.Fprintf(self.out, "  %s %s\n", cliGreen(self.out, "✓"), text)
}
func (self *setupRun) warn(text string) {
	fmt.Fprintf(self.out, "  %s %s\n", cliYellow(self.out, "!"), text)
}
func (self *setupRun) note(text string) { fmt.Fprintf(self.out, "  %s\n", cliDim(self.out, text)) }

func (self *setupRun) ask(question string, yesByDefault bool) bool {
	if self.yes {
		return yesByDefault
	}
	return cliAsk(self.in, self.out, "  "+question, yesByDefault)
}

func setupCmd(opts docopt.Opts, stdout io.Writer, stderr io.Writer) int {
	system, _ := opts.Bool("--system")
	yes, _ := opts.Bool("--yes")
	noService, _ := opts.Bool("--no-service")
	codeFile, _ := opts.String("--auth-code-file")
	if system {
		if os.Geteuid() != 0 {
			writeCliError(stderr, "Setting up a system service needs root. Run it again with sudo:\n  sudo "+strings.Join(append([]string{"provider"}, os.Args[1:]...), " "))
			return 1
		}
		if os.Getenv("URNETWORK_STATE_DIR") == "" {
			// the service and every step below share this folder
			os.Setenv("URNETWORK_STATE_DIR", minerSystemStateDir)
		}
	}
	interactive := cliIsTerminal(os.Stdin) && cliIsTerminal(stdout)
	if !interactive && !yes {
		writeCliError(stderr, "provider setup asks questions, and this isn't a terminal.\n"+
			"  For an unattended setup run: provider setup --yes --auth-code-file=<path>")
		return 1
	}
	run := &setupRun{in: os.Stdin, out: stdout, errOut: stderr, yes: yes, steps: 4}
	fmt.Fprintf(stdout, "%s   %s\n", cliBold(stdout, "URnetwork miner setup"), cliDim(stdout, cliBareVersion()+" · "+runtime.GOOS+"/"+runtime.GOARCH+" · state in "+displayStateDir()))

	// 1. sign in
	run.heading("Sign in")
	signIn := readMinerSignIn()
	needSignIn := !signIn.signedIn
	if signIn.signedIn {
		name := ""
		if signIn.networkName != "" {
			name = fmt.Sprintf(" to network %q", signIn.networkName)
		}
		run.ok("Already signed in" + name)
		if codeFile != "" || !run.ask("Keep this sign-in?", true) {
			needSignIn = true
		}
	}
	if needSignIn {
		if !setupSignIn(run, codeFile) {
			return 1
		}
	}

	// 2. a miner identity for this device
	run.heading("Register this device")
	device := readMinerDevice()
	switch {
	case device.hasHistory && device.clientId != "":
		run.ok("Already registered, miner identity " + shortId(device.clientId))
	case device.hasHistory:
		run.ok("Already has a miner identity")
	default:
		fmt.Fprintln(stdout, "  This device is new, so it gets its own miner identity.")
		if !run.ask("Register it now?", true) {
			fmt.Fprintln(stdout, "  Not registered. Run provider setup again when you're ready.")
			return minerExitNeedsSetup
		}
		if !setupRegister(run) {
			return 1
		}
	}
	run.warn("Your identity lives in " + displayStateDir() + ". Back it up: losing it means starting over.")

	// 3. payout address
	run.heading("Payout address")
	setupPayout(run)

	// 4. keep it running
	run.heading("Keep it running")
	if noService {
		run.note("Skipped (--no-service). Start it by hand with: provider start")
	} else if manager, err := minerServiceFor(system); err != nil {
		run.warn(firstLine(err.Error()))
		run.note("Start it by hand with: provider start")
	} else if manager.installed() {
		run.ok("The background service is already installed")
		if !manager.active() {
			_ = manager.control("start")
		}
		reportServiceStart(stdout, 20*time.Second)
	} else if other := manager.foreign(); other != "" {
		run.ok("The URnetwork installer's " + other + " already runs the miner on this device")
	} else if run.ask("Start the miner now and on every boot?", true) {
		opts := docopt.Opts{"install": true, "--system": system}
		if code := serviceCmd(opts, stdout, stderr); code != 0 {
			return code
		}
	} else {
		run.note("Start it by hand with: provider start")
	}

	fmt.Fprintf(stdout, "\n%s Check on it any time: provider status\n", cliGreen(stdout, "All set."))
	return 0
}

func setupSignIn(run *setupRun, codeFile string) bool {
	if codeFile != "" {
		raw, err := os.ReadFile(codeFile)
		if err != nil {
			writeCliError(run.errOut, "Couldn't read the auth code file: "+err.Error())
			return false
		}
		var out bytes.Buffer
		// the code goes in on stdin, never on a command line others can see
		if err := runSelf([]string{"auth", "-f"}, bytes.NewReader(raw), &out, &out); err != nil {
			writeCliError(run.errOut, "Sign-in failed:\n"+indent(strings.TrimSpace(out.String())))
			return false
		}
	} else {
		if run.yes {
			writeCliError(run.errOut, "Signing in needs an auth code: pass --auth-code-file=<path>")
			return false
		}
		fmt.Fprintln(run.out, "  Get an auth code from your URnetwork app or account page, and paste it here.")
		if err := runSelf([]string{"auth", "-f"}, run.in, run.out, run.errOut); err != nil {
			return false
		}
	}
	signIn := readMinerSignIn()
	if !signIn.signedIn {
		writeCliError(run.errOut, "Sign-in didn't finish.")
		return false
	}
	if signIn.networkName != "" {
		run.ok(fmt.Sprintf("Signed in to network %q", signIn.networkName))
	} else {
		run.ok("Signed in")
	}
	return true
}

// setupRegister makes the first start that registers this device, and
// stops the miner again once the network has given it its identity.
func setupRegister(run *setupRun) bool {
	var log bytes.Buffer
	command, err := startSelf([]string{"provide", "--allow-client-registration"}, &log, &log)
	if err != nil {
		writeCliError(run.errOut, "Couldn't start the miner to register: "+err.Error())
		return false
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	fmt.Fprintln(run.out, "  "+cliDim(run.out, "… Registering (this takes a few seconds)"))
	deadline := time.After(90 * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-exited:
			if device := readMinerDevice(); device.clientId != "" {
				run.ok("Registered, miner identity " + shortId(device.clientId))
				return true
			}
			writeCliError(run.errOut, fmt.Sprintf("Registration didn't finish (%v). What the miner said:\n%s", err, indent(lastLines(log.String(), 8))))
			return false
		case <-deadline:
			_ = command.Process.Signal(syscall.SIGINT)
			<-exited
			writeCliError(run.errOut, "Registration didn't finish within 90 seconds. Check the network with: provider doctor")
			return false
		case <-ticker.C:
			if device := readMinerDevice(); device.clientId != "" {
				_ = command.Process.Signal(syscall.SIGINT)
				select {
				case <-exited:
				case <-time.After(20 * time.Second):
					_ = command.Process.Kill()
					<-exited
				}
				run.ok("Registered, miner identity " + shortId(device.clientId))
				return true
			}
		}
	}
}

var setupMessagePattern = regexp.MustCompile(`(?s)\n----- message -----\n(.*)\n----- end -----\n`)

// setupPayout sets the wallet rewards go to: the challenge, a signature
// made in the person's own wallet app, then wallet set.
func setupPayout(run *setupRun) {
	if run.yes {
		run.note("Skipped. Set it later with: provider wallet set <your 5… address>")
		return
	}
	run.note("Press Enter to skip; set it later with provider wallet set.")
	fmt.Fprint(run.out, "  Your Bittensor coldkey address (starts with 5): ")
	address, ok := cliReadLine(run.in)
	address = strings.TrimSpace(address)
	if !ok || address == "" {
		run.note("Skipped.")
		return
	}
	var challenge bytes.Buffer
	if err := runSelf([]string{"wallet", "challenge", address}, nil, &challenge, &challenge); err != nil {
		run.warn("Couldn't get a message to sign: " + firstLine(strings.TrimSpace(challenge.String())))
		run.note("Try again later with: provider wallet challenge " + address)
		return
	}
	match := setupMessagePattern.FindStringSubmatch(challenge.String())
	if match == nil {
		run.warn("The wallet challenge looked different than expected; showing it as is:")
		fmt.Fprintln(run.out, indent(challenge.String()))
		return
	}
	fmt.Fprintln(run.out, "  Sign this message in your wallet app (btcli, Polkadot.js or Talisman). It expires in 5 minutes:")
	fmt.Fprintf(run.out, "\n%s\n\n", indent(match[1]))
	fmt.Fprint(run.out, "  Signature (0x…), or Enter to skip: ")
	signature, ok := cliReadLine(run.in)
	signature = strings.TrimSpace(signature)
	if !ok || signature == "" {
		run.note("Skipped. Set it later with: provider wallet set " + address)
		return
	}
	var out bytes.Buffer
	if err := runSelf([]string{"wallet", "set", address, "--message=" + match[1], "--signature=" + signature}, nil, &out, &out); err != nil {
		run.warn("The payout address wasn't set: " + firstLine(strings.TrimSpace(out.String())))
		run.note("Try again with: provider wallet challenge " + address)
		return
	}
	run.ok("Payouts will go to " + shortId(address))
}

func indent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "    " + line
	}
	return strings.Join(lines, "\n")
}

func lastLines(text string, count int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if count < len(lines) {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}
