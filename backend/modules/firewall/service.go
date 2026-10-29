// Package firewall manages UFW. It never lets a request silently cut off the
// ports the admin depends on (SSH, the panel itself) or the admin's own IP.
package firewall

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rpanel/backend/system"
)

var (
	ErrBadInput = errors.New("invalid input")
	// ErrLockout is returned when an operation could cut off SSH, the panel or
	// the requester; the caller may repeat it with Confirm set.
	ErrLockout = errors.New("this change could lock you out")
	// ErrPanelRule is returned when someone tries to delete the rule that lets people reach the panel.
	ErrPanelRule = errors.New("this rule keeps the panel reachable and cannot be deleted")
	ErrStale     = errors.New("rules changed since you loaded the list; refresh and try again")

	portRe    = regexp.MustCompile(`^[0-9]{1,5}(:[0-9]{1,5})?$`)
	commentRe = regexp.MustCompile(`^[A-Za-z0-9 _.,-]{0,60}$`)
	ruleRe    = regexp.MustCompile(`^\[\s*([0-9]+)\]\s+(.+?)\s{2,}(ALLOW|DENY|REJECT|LIMIT)(?: (IN|OUT|FWD))?\s{2,}(.+?)\s*$`)
	defaultRe = regexp.MustCompile(`(?m)^Default:\s*(\w+) \(incoming\), (\w+) \(outgoing\)`)
	sshPortRe = regexp.MustCompile(`(?im)^\s*Port\s+([0-9]{1,5})\s*$`)
)

type Rule struct {
	Number    int    `json:"number"`
	To        string `json:"to"`
	Action    string `json:"action"`
	Direction string `json:"direction"`
	From      string `json:"from"`
	V6        bool   `json:"v6"`
	// Locked marks the inbound allow rule for the panel port; it cannot be deleted from the panel.
	Locked bool `json:"locked"`
}

type Status struct {
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
	Incoming  string `json:"default_incoming"`
	Outgoing  string `json:"default_outgoing"`
	Rules     []Rule `json:"rules"`
	Protected []int  `json:"protected_ports"`
	Error     string `json:"error,omitempty"`
}

type Service struct {
	run       system.Runner
	sshdConf  string
	extra     []int
	panelPort int
}

func NewService(run system.Runner, sshdConf string, panelPort string, extra []string) *Service {
	s := &Service{run: run, sshdConf: sshdConf}
	if p, err := strconv.Atoi(panelPort); err == nil {
		s.panelPort = p
	}
	for _, e := range extra {
		if p, err := strconv.Atoi(strings.TrimSpace(e)); err == nil && p > 0 && p < 65536 {
			s.extra = append(s.extra, p)
		}
	}
	return s
}

// ProtectedPorts are the TCP ports whose access must survive any change.
func (s *Service) ProtectedPorts() []int {
	seen := map[int]bool{}
	var out []int
	add := func(p int) {
		if p > 0 && p < 65536 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	ssh := []int{}
	if f, err := os.Open(s.sshdConf); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if m := sshPortRe.FindStringSubmatch(sc.Text()); m != nil {
				p, _ := strconv.Atoi(m[1])
				ssh = append(ssh, p)
			}
		}
		f.Close()
	}
	if len(ssh) == 0 {
		ssh = []int{22}
	}
	for _, p := range ssh {
		add(p)
	}
	add(s.panelPort)
	for _, p := range s.extra {
		add(p)
	}
	return out
}

// ParseRules parses `ufw status numbered` output.
func ParseRules(out string) []Rule {
	rules := []Rule{}
	for _, line := range strings.Split(out, "\n") {
		m := ruleRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		to, from := m[2], m[5]
		v6 := strings.Contains(to, "(v6)") || strings.Contains(from, "(v6)")
		dir := m[4]
		if dir == "" {
			dir = "IN"
		}
		rules = append(rules, Rule{Number: n, To: to, Action: m[3], Direction: dir, From: from, V6: v6})
	}
	return rules
}

func (s *Service) Status(ctx context.Context) Status {
	st := Status{Rules: []Rule{}, Protected: s.ProtectedPorts()}
	res, err := s.run.Run(ctx, 15*time.Second, "ufw", "status", "numbered")
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return st
		}
		st.Installed = true
		st.Error = strings.TrimSpace(res.Stderr + " " + err.Error())
		return st
	}
	st.Installed = true
	st.Active = strings.Contains(res.Stdout, "Status: active")
	st.Rules = ParseRules(res.Stdout)
	for i := range st.Rules {
		st.Rules[i].Locked = s.isPanelRule(st.Rules[i])
	}
	if v, err := s.run.Run(ctx, 15*time.Second, "ufw", "status", "verbose"); err == nil {
		if m := defaultRe.FindStringSubmatch(v.Stdout); m != nil {
			st.Incoming, st.Outgoing = m[1], m[2]
		}
	}
	return st
}

func (s *Service) ufw(ctx context.Context, args ...string) error {
	res, err := s.run.Run(ctx, 60*time.Second, "ufw", args...)
	if err != nil {
		msg := strings.TrimSpace(res.Stdout + " " + res.Stderr)
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// Install installs ufw via apt (without enabling it).
func (s *Service) Install(ctx context.Context) error {
	if res, err := s.run.Run(ctx, 5*time.Minute, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt-get update failed: %s", strings.TrimSpace(res.Stderr))
	}
	if res, err := s.run.Run(ctx, 10*time.Minute, "apt-get", "install", "-y", "ufw"); err != nil {
		return fmt.Errorf("install failed: %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

// ensureProtectedAllowed opens every protected port so enabling the firewall
// or tightening the default policy cannot cut off SSH or the panel.
func (s *Service) ensureProtectedAllowed(ctx context.Context) error {
	for _, p := range s.ProtectedPorts() {
		if err := s.ufw(ctx, "allow", fmt.Sprintf("%d/tcp", p)); err != nil {
			return fmt.Errorf("cannot allow protected port %d: %w", p, err)
		}
	}
	return nil
}

func (s *Service) Enable(ctx context.Context) error {
	if err := s.ensureProtectedAllowed(ctx); err != nil {
		return err
	}
	return s.ufw(ctx, "--force", "enable")
}

func (s *Service) Disable(ctx context.Context) error { return s.ufw(ctx, "disable") }

// SetDefaultIncoming sets the default policy for incoming traffic.
func (s *Service) SetDefaultIncoming(ctx context.Context, policy string) error {
	switch policy {
	case "allow", "deny", "reject":
	default:
		return fmt.Errorf("%w: policy", ErrBadInput)
	}
	if policy != "allow" {
		if err := s.ensureProtectedAllowed(ctx); err != nil {
			return err
		}
	}
	return s.ufw(ctx, "default", policy, "incoming")
}

type RuleInput struct {
	Action  string `json:"action"` // allow | deny | reject | limit
	Port    string `json:"port"`   // 80 or 8000:8100
	Proto   string `json:"proto"`  // tcp | udp | any
	Source  string `json:"source"` // any | IP | CIDR
	Comment string `json:"comment"`
	Confirm bool   `json:"confirm"` // acknowledge a lockout warning
}

func parsePorts(spec string) (lo, hi int, err error) {
	if !portRe.MatchString(spec) {
		return 0, 0, fmt.Errorf("%w: port must be a number 1-65535 or a range like 8000:8100", ErrBadInput)
	}
	parts := strings.SplitN(spec, ":", 2)
	lo, _ = strconv.Atoi(parts[0])
	hi = lo
	if len(parts) == 2 {
		hi, _ = strconv.Atoi(parts[1])
	}
	if lo < 1 || hi > 65535 || lo > hi {
		return 0, 0, fmt.Errorf("%w: port out of range", ErrBadInput)
	}
	return lo, hi, nil
}

// parseSource returns the normalized source and the network it covers (nil = anywhere).
func parseSource(src string) (string, *net.IPNet, error) {
	if src == "" || strings.EqualFold(src, "any") || strings.EqualFold(src, "anywhere") {
		return "any", nil, nil
	}
	if ip := net.ParseIP(src); ip != nil {
		bits := 128
		if ip.To4() != nil {
			bits = 32
		}
		return ip.String(), &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
	}
	if _, n, err := net.ParseCIDR(src); err == nil {
		return src, n, nil
	}
	return "", nil, fmt.Errorf("%w: source must be 'any', an IP address or a CIDR", ErrBadInput)
}

func (s *Service) protectedIn(lo, hi int) []int {
	var hit []int
	for _, p := range s.ProtectedPorts() {
		if p >= lo && p <= hi {
			hit = append(hit, p)
		}
	}
	return hit
}

// AddRule validates and adds an inbound rule. actorIP is the requester's address:
// blocking it, or blocking a protected port, needs Confirm.
func (s *Service) AddRule(ctx context.Context, in RuleInput, actorIP string) error {
	switch in.Action {
	case "allow", "deny", "reject", "limit":
	default:
		return fmt.Errorf("%w: action", ErrBadInput)
	}
	switch in.Proto {
	case "tcp", "udp", "any":
	case "":
		in.Proto = "tcp"
	default:
		return fmt.Errorf("%w: protocol", ErrBadInput)
	}
	lo, hi, err := parsePorts(in.Port)
	if err != nil {
		return err
	}
	if lo != hi && in.Proto == "any" {
		return fmt.Errorf("%w: port ranges need tcp or udp", ErrBadInput)
	}
	src, netw, err := parseSource(in.Source)
	if err != nil {
		return err
	}
	if !commentRe.MatchString(in.Comment) {
		return fmt.Errorf("%w: comment may contain letters, digits, spaces and . , _ - only (max 60)", ErrBadInput)
	}

	if in.Action != "allow" && in.Action != "limit" && !in.Confirm {
		if hit := s.protectedIn(lo, hi); len(hit) > 0 && netw == nil {
			return fmt.Errorf("%w: %s on protected port(s) %v from anywhere", ErrLockout, in.Action, hit)
		}
		if ip := net.ParseIP(actorIP); ip != nil && (netw == nil || netw.Contains(ip)) && len(s.protectedIn(lo, hi)) > 0 {
			return fmt.Errorf("%w: the rule matches your own address %s on a protected port", ErrLockout, actorIP)
		}
	}

	args := []string{in.Action, "in"}
	if in.Proto != "any" {
		args = append(args, "proto", in.Proto)
	}
	args = append(args, "from", src, "to", "any", "port", in.Port)
	if in.Comment != "" {
		args = append(args, "comment", in.Comment)
	}
	return s.ufw(ctx, args...)
}

// isPanelRule reports whether r is an inbound allow rule covering the panel's public port.
func (s *Service) isPanelRule(r Rule) bool {
	if s.panelPort == 0 || r.Action != "ALLOW" || r.Direction != "IN" {
		return false
	}
	for _, pr := range portsOf(r.To) {
		if s.panelPort >= pr[0] && s.panelPort <= pr[1] {
			return true
		}
	}
	return false
}

// portsOf extracts the numeric ports/ranges from a rule's "To" column ("22/tcp",
// "80,443/tcp", "8000:8100/tcp", "22/tcp (v6)"). Rules with app profiles yield none.
func portsOf(to string) [][2]int {
	to = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(to), "(v6)"))
	to = strings.TrimSpace(strings.SplitN(to, " ", 2)[0])
	to = strings.SplitN(to, "/", 2)[0]
	var out [][2]int
	for _, part := range strings.Split(to, ",") {
		if !portRe.MatchString(part) {
			continue
		}
		lo, hi, err := parsePorts(part)
		if err == nil {
			out = append(out, [2]int{lo, hi})
		}
	}
	return out
}

// DeleteRule removes rule number n. expectedTo/expectedAction must match the
// current rule n, which protects against deleting the wrong rule after the list
// shifted. Deleting an inbound ALLOW on a protected port needs Confirm.
func (s *Service) DeleteRule(ctx context.Context, n int, expectedTo, expectedAction string, confirm bool) error {
	if n < 1 {
		return fmt.Errorf("%w: rule number", ErrBadInput)
	}
	res, err := s.run.Run(ctx, 15*time.Second, "ufw", "status", "numbered")
	if err != nil {
		return errors.New(strings.TrimSpace(res.Stderr + " " + err.Error()))
	}
	var rule *Rule
	for _, r := range ParseRules(res.Stdout) {
		if r.Number == n {
			r := r
			rule = &r
			break
		}
	}
	if rule == nil || rule.To != expectedTo || rule.Action != expectedAction {
		return ErrStale
	}
	if s.isPanelRule(*rule) {
		return ErrPanelRule // no confirm override: without it the panel is unreachable
	}
	if rule.Action == "ALLOW" && rule.Direction == "IN" && !confirm {
		for _, pr := range portsOf(rule.To) {
			if hit := s.protectedIn(pr[0], pr[1]); len(hit) > 0 {
				return fmt.Errorf("%w: this rule allows protected port(s) %v", ErrLockout, hit)
			}
		}
	}
	return s.ufw(ctx, "--force", "delete", strconv.Itoa(n))
}
