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
)

const SchemaVersion = "1"

var DefaultRequiredRoles = []string{"planner", "tester"}

type Manager struct {
	root string
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
}

func NewManager(root string) *Manager {
	if root == "" {
		root = ".harness"
	}
	return &Manager{root: root}
}

func (m *Manager) CurrentSprintNumber() (int, error) {
	dir := filepath.Join(m.root, "contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	re := regexp.MustCompile(`sprint-(\d+)\.md`)
	max := 0
	for _, entry := range entries {
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
	hash, contract, err := m.contractHash(sprintNumber)
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
	if errs := contract.Validate(); len(errs) > 0 {
		st.StructuralErrs = errs
		st.Reason = strings.Join(errs, "; ")
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
	for _, approval := range approvals {
		if approval.ContractHash != hash {
			continue
		}
		switch approval.Decision {
		case "approve":
			st.ApprovedRoles = append(st.ApprovedRoles, approval.Role)
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
		return st, nil
	}
	st.State = "proposed"
	st.Reason = "waiting for required approvals"
	return st, nil
}

func (m *Manager) Propose(sprintNumber int) (Status, error) {
	if sprintNumber == 0 {
		var err error
		sprintNumber, err = m.CurrentSprintNumber()
		if err != nil {
			return Status{}, err
		}
	}
	hash, contract, err := m.contractHash(sprintNumber)
	if err != nil {
		return Status{}, err
	}
	if errs := contract.Validate(); len(errs) > 0 {
		return Status{}, fmt.Errorf("contract not structurally valid: %s", strings.Join(errs, "; "))
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
	return m.refreshLockState(sprintNumber)
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

func (m *Manager) ContractPath(sprintNumber int) string {
	return filepath.Join(m.root, "contracts", fmt.Sprintf("sprint-%03d.md", sprintNumber))
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
	return m.Status(sprintNumber)
}

func (m *Manager) contractHash(sprintNumber int) (string, *planner.Contract, error) {
	path := m.ContractPath(sprintNumber)
	contract, err := planner.Parse(path)
	if err != nil {
		return "", nil, err
	}
	canonical := strings.TrimSpace(strings.ReplaceAll(contract.RawMarkdown, "\r\n", "\n")) + "\n"
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), contract, nil
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
