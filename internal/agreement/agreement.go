// Package agreement implements the deterministic multi-agent contract gate.
package agreement

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/planner"
	"github.com/dancampari/harness/internal/planning"
	"github.com/dancampari/harness/internal/traceability"
)

const SchemaVersion = "1"

var DefaultRequiredRoles = []string{"planner", "tester"}

type Manager struct {
	root      string
	specsRoot string
}

type Lock struct {
	SchemaVersion string   `json:"schema_version"`
	SprintNumber  int      `json:"sprint_number"`
	State         string   `json:"state"`
	ContractHash  string   `json:"contract_hash"`
	RequiredRoles []string `json:"required_roles"`
	ApprovedRoles []string `json:"approved_roles"`
	RejectedRoles []string `json:"rejected_roles,omitempty"`
	UpdatedAt     string   `json:"updated_at"`
}

type Approval struct {
	SchemaVersion string `json:"schema_version"`
	SprintNumber  int    `json:"sprint_number"`
	Role          string `json:"role"`
	Decision      string `json:"decision"`
	ContractHash  string `json:"contract_hash"`
	Reason        string `json:"reason,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

type Status struct {
	SprintNumber   int
	State          string
	ContractHash   string
	RequiredRoles  []string
	ApprovedRoles  []string
	RejectedRoles  []string
	MissingRoles   []string
	Reason         string
	ContractPath   string
	LockPath       string
	StructuralOK   bool
	StructuralErrs []string
	AgreedAt       time.Time
	// Hashed lists which planning artifacts contributed to the current
	// contract hash. "contract" is always present when the contract file
	// exists; "design" and "tasks" are added when their files exist.
	Hashed []string
}

func NewManager(root string) *Manager {
	if root == "" {
		root = ".harness"
	}
	return &Manager{root: root, specsRoot: defaultSpecsRoot(root)}
}

// defaultSpecsRoot derives the .specs/ directory from the .harness/ root.
// Both live as siblings in the workspace; tests pass absolute temp paths,
// which still resolve correctly via filepath.Dir.
func defaultSpecsRoot(harnessRoot string) string {
	clean := filepath.Clean(harnessRoot)
	parent := filepath.Dir(clean)
	if parent == "" || parent == "." {
		return ".specs"
	}
	return filepath.Join(parent, ".specs")
}

func (m *Manager) Root() string {
	return m.root
}

func (m *Manager) SpecsRoot() string {
	return m.specsRoot
}

func featureSlug(sprintNumber int) string {
	return fmt.Sprintf("sprint-%03d", sprintNumber)
}

// featureDir returns the canonical .specs/features/<slug>/ directory.
func (m *Manager) featureDir(sprintNumber int) string {
	return filepath.Join(m.specsRoot, "features", featureSlug(sprintNumber))
}

// resolveArtifact returns the canonical .specs/ path when that file is
// already present, otherwise the legacy .harness/ path. This lets new
// projects write under .specs/ while existing projects continue to work
// until `harness upgrade` migrates them.
func (m *Manager) resolveArtifact(sprintNumber int, specsName, legacyDir string) string {
	specsPath := filepath.Join(m.featureDir(sprintNumber), specsName)
	if _, err := os.Stat(specsPath); err == nil {
		if specsName == "spec.md" {
			legacyPath := filepath.Join(m.root, legacyDir, fmt.Sprintf("sprint-%03d.md", sprintNumber))
			if hasTemplatePlaceholders(specsPath) {
				if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
					return legacyPath
				}
			}
		}
		return specsPath
	}
	return filepath.Join(m.root, legacyDir, fmt.Sprintf("sprint-%03d.md", sprintNumber))
}

func hasTemplatePlaceholders(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return len(planner.TemplatePlaceholderErrors(string(b))) > 0
}

func (m *Manager) CurrentSprintNumber() (int, error) {
	max := 0

	// Canonical .specs/features/sprint-NNN/spec.md
	featuresDir := filepath.Join(m.specsRoot, "features")
	if entries, err := os.ReadDir(featuresDir); err == nil {
		re := regexp.MustCompile(`^sprint-(\d+)$`)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			match := re.FindStringSubmatch(entry.Name())
			if match == nil {
				continue
			}
			spec := filepath.Join(featuresDir, entry.Name(), "spec.md")
			if _, err := os.Stat(spec); err != nil {
				continue
			}
			var n int
			fmt.Sscanf(match[1], "%d", &n)
			if n > max {
				max = n
			}
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}

	// Legacy .harness/contracts/sprint-NNN.md
	contractsDir := filepath.Join(m.root, "contracts")
	if entries, err := os.ReadDir(contractsDir); err == nil {
		re := regexp.MustCompile(`sprint-(\d+)\.md$`)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			match := re.FindStringSubmatch(entry.Name())
			if match == nil {
				continue
			}
			var n int
			fmt.Sscanf(match[1], "%d", &n)
			if n > max {
				max = n
			}
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}

	return max, nil
}

func (m *Manager) Status(sprintNumber int) (Status, error) {
	if sprintNumber == 0 {
		var err error
		sprintNumber, err = m.CurrentSprintNumber()
		if err != nil {
			return Status{}, err
		}
	}
	st := Status{
		SprintNumber:  sprintNumber,
		State:         "missing",
		RequiredRoles: append([]string(nil), DefaultRequiredRoles...),
		ContractPath:  m.ContractPath(sprintNumber),
		LockPath:      m.LockPath(sprintNumber),
	}
	if sprintNumber == 0 {
		st.Reason = "no sprint contract exists"
		st.MissingRoles = append([]string(nil), st.RequiredRoles...)
		return st, nil
	}
	hash, contract, hashed, err := m.contractHash(sprintNumber)
	if err != nil {
		if os.IsNotExist(err) {
			st.Reason = "contract file missing"
			st.MissingRoles = append([]string(nil), st.RequiredRoles...)
			return st, nil
		}
		return Status{}, err
	}
	st.State = "draft"
	st.ContractHash = hash
	st.Hashed = hashed
	allErrs := contract.Validate()
	mode := planning.ReadMode(m.root)
	allErrs = append(allErrs, planning.ContractPolicyErrorsWith(mode, contract, m.artifactPresence(sprintNumber))...)
	if len(allErrs) > 0 {
		st.StructuralErrs = allErrs
		st.Reason = strings.Join(allErrs, "; ")
		st.MissingRoles = append([]string(nil), st.RequiredRoles...)
		return st, nil
	}
	st.StructuralOK = true

	lock, lockErr := m.readLock(sprintNumber)
	if lockErr == nil && len(lock.RequiredRoles) > 0 {
		st.RequiredRoles = uniqueSorted(lock.RequiredRoles)
	}
	approvals, err := m.readApprovals(sprintNumber)
	if err != nil {
		return Status{}, err
	}
	approvalTimes := map[string]time.Time{}
	for _, approval := range approvals {
		if approval.ContractHash != hash {
			continue
		}
		switch approval.Decision {
		case "approve":
			st.ApprovedRoles = append(st.ApprovedRoles, approval.Role)
			if t, err := time.Parse(time.RFC3339, approval.UpdatedAt); err == nil {
				approvalTimes[approval.Role] = t
			}
		case "reject":
			st.RejectedRoles = append(st.RejectedRoles, approval.Role)
		}
	}
	st.ApprovedRoles = uniqueSorted(st.ApprovedRoles)
	st.RejectedRoles = uniqueSorted(st.RejectedRoles)
	st.MissingRoles = missingRoles(st.RequiredRoles, st.ApprovedRoles)

	if len(st.RejectedRoles) > 0 {
		st.State = "rejected"
		st.Reason = "current contract hash was rejected"
		return st, nil
	}
	if lockErr != nil {
		if errors.Is(lockErr, os.ErrNotExist) {
			st.State = "draft"
			st.Reason = "contract has not been proposed"
			return st, nil
		}
		return Status{}, lockErr
	}
	if lock.ContractHash != hash {
		st.State = "changed"
		st.Reason = "contract changed after proposal or approval"
		st.MissingRoles = append([]string(nil), st.RequiredRoles...)
		return st, nil
	}
	if len(st.MissingRoles) == 0 {
		st.State = "agreed"
		for _, role := range st.RequiredRoles {
			if t := approvalTimes[role]; t.After(st.AgreedAt) {
				st.AgreedAt = t
			}
		}
		return st, nil
	}
	st.State = "proposed"
	st.Reason = "waiting for required approvals"
	return st, nil
}

func (s Status) ReportIsCurrent(reportTime time.Time) bool {
	return strings.EqualFold(s.State, "agreed") &&
		!s.AgreedAt.IsZero() &&
		!reportTime.IsZero() &&
		!reportTime.Before(s.AgreedAt)
}

func (m *Manager) Propose(sprintNumber int) (Status, error) {
	if sprintNumber == 0 {
		var err error
		sprintNumber, err = m.CurrentSprintNumber()
		if err != nil {
			return Status{}, err
		}
	}
	hash, contract, _, err := m.contractHash(sprintNumber)
	if err != nil {
		return Status{}, err
	}
	if errs := contract.Validate(); len(errs) > 0 {
		return Status{}, fmt.Errorf("contract not structurally valid: %s", strings.Join(errs, "; "))
	}
	mode := planning.ReadMode(m.root)
	if errs := planning.ContractPolicyErrorsWith(mode, contract, m.artifactPresence(sprintNumber)); len(errs) > 0 {
		return Status{}, fmt.Errorf("contract violates %s policy: %s", mode, strings.Join(errs, "; "))
	}
	lock := Lock{
		SchemaVersion: SchemaVersion,
		SprintNumber:  sprintNumber,
		State:         "proposed",
		ContractHash:  hash,
		RequiredRoles: append([]string(nil), DefaultRequiredRoles...),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.writeLock(lock); err != nil {
		return Status{}, err
	}
	m.seedTraceability(sprintNumber, contract)
	return m.refreshLockState(sprintNumber)
}

// seedTraceability creates Pending entries in the traceability ledger
// for every REQ-NNN declared in the contract. Status transitions to
// In Design / In Tasks happen as the matching artifact appears; this
// seeding ensures the ledger is never empty after a first Propose.
func (m *Manager) seedTraceability(sprintNumber int, contract *planner.Contract) {
	if contract == nil || len(contract.Requirements) == 0 {
		return
	}
	reqs := make([]traceability.Requirement, 0, len(contract.Requirements))
	for _, r := range contract.Requirements {
		reqs = append(reqs, traceability.Requirement{ID: r.ID, Statement: r.Statement})
	}
	slug := featureSlug(sprintNumber)
	_ = traceability.BulkRegister(m.root, slug, reqs)
	presence := m.artifactPresence(sprintNumber)
	next := traceability.StatusPending
	if presence.HasDesign {
		next = traceability.StatusInDesign
	}
	if presence.HasTasks {
		next = traceability.StatusInTasks
	}
	if next == traceability.StatusPending {
		return
	}
	for _, r := range contract.Requirements {
		_, _, _ = traceability.Update(m.root, slug, r.ID, r.Statement, next)
	}
}

func (m *Manager) Approve(sprintNumber int, role string) (Status, error) {
	return m.recordDecision(sprintNumber, role, "approve", "")
}

func (m *Manager) Reject(sprintNumber int, role, reason string) (Status, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Status{}, errors.New("reject requires a reason")
	}
	return m.recordDecision(sprintNumber, role, "reject", reason)
}

func (m *Manager) EnsureAgreed(sprintNumber int) error {
	st, err := m.Status(sprintNumber)
	if err != nil {
		return err
	}
	if st.State == "agreed" {
		return nil
	}
	return fmt.Errorf("contract agreement required: sprint %03d is %s (%s); run `harness contract propose` then `harness contract approve --role planner` and `harness contract approve --role tester`, or pass --allow-unagreed for an explicit override",
		st.SprintNumber, strings.ToUpper(st.State), st.Reason)
}

// artifactPresence reports which optional planning files exist on disk
// for a sprint, so the planning policy can enforce the size-based gates
// (medium requires tasks, large requires both design and tasks).
func (m *Manager) artifactPresence(sprintNumber int) planning.ArtifactPresence {
	presence := planning.ArtifactPresence{}
	if _, err := os.Stat(m.DesignPath(sprintNumber)); err == nil {
		presence.HasDesign = true
	}
	tasksPath := m.TasksPath(sprintNumber)
	if _, err := os.Stat(tasksPath); err == nil {
		presence.HasTasks = true
		presence.TaskPlanPath = tasksPath
	}
	// TESTING.md lives in the codebase mapping tree, not under a
	// per-feature directory. Both layouts are tried so legacy projects
	// keep their .harness/context/TESTING.md until they migrate.
	for _, candidate := range []string{
		filepath.Join(m.specsRoot, "codebase", "TESTING.md"),
		filepath.Join(m.root, "context", "TESTING.md"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			presence.TestingMatrixPath = candidate
			break
		}
	}
	return presence
}

func (m *Manager) ContractPath(sprintNumber int) string {
	return m.resolveArtifact(sprintNumber, "spec.md", "contracts")
}

func (m *Manager) DesignPath(sprintNumber int) string {
	return m.resolveArtifact(sprintNumber, "design.md", "design")
}

func (m *Manager) TasksPath(sprintNumber int) string {
	return m.resolveArtifact(sprintNumber, "tasks.md", "tasks")
}

// CanonicalContractPath returns the .specs/ path regardless of whether
// the artifact has been migrated yet. New contracts (`harness feature new`)
// write here; migration moves legacy files to this path.
func (m *Manager) CanonicalContractPath(sprintNumber int) string {
	return filepath.Join(m.featureDir(sprintNumber), "spec.md")
}

func (m *Manager) CanonicalDesignPath(sprintNumber int) string {
	return filepath.Join(m.featureDir(sprintNumber), "design.md")
}

func (m *Manager) CanonicalTasksPath(sprintNumber int) string {
	return filepath.Join(m.featureDir(sprintNumber), "tasks.md")
}

// LegacyContractPath returns the pre-Phase-2 location. Migration code
// reads from here and writes to CanonicalContractPath.
func (m *Manager) LegacyContractPath(sprintNumber int) string {
	return filepath.Join(m.root, "contracts", fmt.Sprintf("sprint-%03d.md", sprintNumber))
}

func (m *Manager) LegacyDesignPath(sprintNumber int) string {
	return filepath.Join(m.root, "design", fmt.Sprintf("sprint-%03d.md", sprintNumber))
}

func (m *Manager) LegacyTasksPath(sprintNumber int) string {
	return filepath.Join(m.root, "tasks", fmt.Sprintf("sprint-%03d.md", sprintNumber))
}

func (m *Manager) LockPath(sprintNumber int) string {
	return filepath.Join(m.root, "contracts", fmt.Sprintf("sprint-%03d.lock.json", sprintNumber))
}

func (m *Manager) recordDecision(sprintNumber int, role, decision, reason string) (Status, error) {
	role = normalizeRole(role)
	if role == "" {
		return Status{}, errors.New("role is required")
	}
	if sprintNumber == 0 {
		var err error
		sprintNumber, err = m.CurrentSprintNumber()
		if err != nil {
			return Status{}, err
		}
	}
	st, err := m.Status(sprintNumber)
	if err != nil {
		return Status{}, err
	}
	if st.State == "missing" {
		return Status{}, errors.New("no sprint contract exists")
	}
	if st.State == "draft" || st.State == "changed" {
		return Status{}, fmt.Errorf("contract must be proposed before approval; current state is %s", st.State)
	}
	if st.State == "rejected" && decision == "approve" {
		return Status{}, errors.New("contract is rejected; edit the contract and propose a new hash")
	}
	if !contains(st.RequiredRoles, role) {
		return Status{}, fmt.Errorf("role %q is not required for this contract; required: %s", role, strings.Join(st.RequiredRoles, ", "))
	}
	approval := Approval{
		SchemaVersion: SchemaVersion,
		SprintNumber:  sprintNumber,
		Role:          role,
		Decision:      decision,
		ContractHash:  st.ContractHash,
		Reason:        reason,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.writeApproval(approval); err != nil {
		return Status{}, err
	}
	return m.refreshLockState(sprintNumber)
}

func (m *Manager) refreshLockState(sprintNumber int) (Status, error) {
	st, err := m.Status(sprintNumber)
	if err != nil {
		return Status{}, err
	}
	if st.State == "missing" || st.State == "draft" || st.State == "changed" {
		return st, nil
	}
	lock := Lock{
		SchemaVersion: SchemaVersion,
		SprintNumber:  sprintNumber,
		State:         st.State,
		ContractHash:  st.ContractHash,
		RequiredRoles: st.RequiredRoles,
		ApprovedRoles: st.ApprovedRoles,
		RejectedRoles: st.RejectedRoles,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.writeLock(lock); err != nil {
		return Status{}, err
	}
	final, err := m.Status(sprintNumber)
	if err != nil {
		return Status{}, err
	}
	if final.State == "agreed" {
		m.advanceTraceability(sprintNumber, traceability.StatusImplementing)
	}
	return final, nil
}

// advanceTraceability transitions every requirement of the given
// sprint to the supplied status. Best-effort — errors are swallowed so
// the ledger never blocks the agreement pipeline.
func (m *Manager) advanceTraceability(sprintNumber int, next traceability.Status) {
	_, contract, _, err := m.contractHash(sprintNumber)
	if err != nil || contract == nil {
		return
	}
	slug := featureSlug(sprintNumber)
	for _, r := range contract.Requirements {
		_, _, _ = traceability.Update(m.root, slug, r.ID, r.Statement, next)
	}
}

// AdvanceTraceabilityTo is the public hook that sprint.Consolidate uses
// to mark every requirement of the sprint Verified after a passing
// QA score. Wraps advanceTraceability so callers outside this package
// can drive the ledger without needing access to internals.
func (m *Manager) AdvanceTraceabilityTo(sprintNumber int, status traceability.Status) {
	m.advanceTraceability(sprintNumber, status)
}

// contractHash computes the hash that defines an agreed sprint state.
//
// Backwards compatibility: contracts without design/tasks artifacts hash
// the same value as v0.5.x — only the contract markdown is canonicalised
// and digested. When a design/tasks file is present, it is appended to
// the canonical envelope under a stable marker so any change there also
// invalidates the agreement. The third return value lists the artifacts
// that participated, so Status can surface them.
func (m *Manager) contractHash(sprintNumber int) (string, *planner.Contract, []string, error) {
	path := m.ContractPath(sprintNumber)
	contract, err := planner.Parse(path)
	if err != nil {
		return "", nil, nil, err
	}
	canonical := strings.TrimSpace(strings.ReplaceAll(contract.RawMarkdown, "\r\n", "\n")) + "\n"
	hashed := []string{"contract"}
	if data, err := os.ReadFile(m.DesignPath(sprintNumber)); err == nil {
		canonical += "\n---design---\n" + strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")) + "\n"
		hashed = append(hashed, "design")
	}
	if data, err := os.ReadFile(m.TasksPath(sprintNumber)); err == nil {
		canonical += "\n---tasks---\n" + strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")) + "\n"
		hashed = append(hashed, "tasks")
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), contract, hashed, nil
}

func (m *Manager) readLock(sprintNumber int) (Lock, error) {
	var lock Lock
	b, err := os.ReadFile(m.LockPath(sprintNumber))
	if err != nil {
		return Lock{}, err
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func (m *Manager) writeLock(lock Lock) error {
	if len(lock.RequiredRoles) == 0 {
		lock.RequiredRoles = append([]string(nil), DefaultRequiredRoles...)
	}
	lock.RequiredRoles = uniqueSorted(lock.RequiredRoles)
	lock.ApprovedRoles = uniqueSorted(lock.ApprovedRoles)
	lock.RejectedRoles = uniqueSorted(lock.RejectedRoles)
	b, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(m.LockPath(lock.SprintNumber)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.LockPath(lock.SprintNumber), b, 0o644)
}

func (m *Manager) readApprovals(sprintNumber int) ([]Approval, error) {
	dir := filepath.Join(m.root, "approvals", fmt.Sprintf("sprint-%03d", sprintNumber))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var approvals []Approval
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var approval Approval
		if json.Unmarshal(b, &approval) == nil {
			approval.Role = normalizeRole(approval.Role)
			approval.Decision = strings.ToLower(strings.TrimSpace(approval.Decision))
			approvals = append(approvals, approval)
		}
	}
	return approvals, nil
}

func (m *Manager) writeApproval(approval Approval) error {
	dir := filepath.Join(m.root, "approvals", fmt.Sprintf("sprint-%03d", approval.SprintNumber))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, approval.Role+".json")
	b, err := json.MarshalIndent(approval, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func missingRoles(required, approved []string) []string {
	approvedSet := map[string]bool{}
	for _, role := range approved {
		approvedSet[role] = true
	}
	var missing []string
	for _, role := range required {
		if !approvedSet[role] {
			missing = append(missing, role)
		}
	}
	return missing
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = normalizeRole(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
