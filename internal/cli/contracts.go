package cli

import "github.com/spf13/cobra"

// ReportVersion is the version of the machine-readable CLI result contracts.
// Individual reports may evolve independently, but all current reports use
// version one.
const ReportVersion = 1

// Finding is the shared diagnostic shape used by doctor and resource reports.
// IDs are always durable WTF UUIDs; callers must not substitute paths or names.
type Finding struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	RepositoryID  string `json:"repository_id"`
	WorkspaceID   string `json:"workspace_id"`
	Message       string `json:"message"`
	RepairCommand string `json:"repair_command,omitempty"`
}

// Finding severities are deliberately separate from lifecycle or check status.
const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// capabilitiesReport advertises contracts, rather than the version of the
// installed binary. Resource and doctor entries are names reserved by the
// v0.11 contract; their lifecycle commands are intentionally not implemented
// by this change.
type capabilitiesReport struct {
	Version                int            `json:"version"`
	ResultSchemas          map[string]int `json:"result_schemas"`
	SupportedVCSBackends   []string       `json:"supported_vcs_backends"`
	SupportedResourceKinds []string       `json:"supported_resource_kinds"`
	SupportedDoctorChecks  []string       `json:"supported_doctor_checks"`
}

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Describe the machine-readable CLI contracts",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), currentCapabilities())
		}
		_, err := cmd.OutOrStdout().Write([]byte("wtf capabilities are available with --json\n"))
		return err
	},
}

func currentCapabilities() capabilitiesReport {
	return capabilitiesReport{
		Version: ReportVersion,
		ResultSchemas: map[string]int{
			"capabilities":      ReportVersion,
			"resources":         ReportVersion,
			"doctor":            ReportVersion,
			"workspace_current": ReportVersion,
			"workspace_inspect": ReportVersion,
			"workspace_list":    ReportVersion,
			"finding":           ReportVersion,
		},
		SupportedVCSBackends:   []string{"git", "jj"},
		SupportedResourceKinds: []string{"ports", "files"},
		SupportedDoctorChecks:  []string{"identity", "vcs_registration", "cleanup_failed", "git_shadow", "managed_files", "port_leases"},
	}
}
