package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:               "wtf",
	Short:             "WorkTreeForge — a fast git worktree workflow tool",
	Long:              cyanBold("WorkTreeForge (WTF)") + " streamlines git worktree operations, project setup, and forge integrations.",
	Version:           Version,
	SilenceErrors:     true,
	SilenceUsage:      true,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
}

func init() {
	cobra.AddTemplateFunc("heading", func(s string) string { return bold(s) })
	cobra.AddTemplateFunc("accent", func(s string) string { return cyan(s) })
	cobra.AddTemplateFunc("dimText", func(s string) string { return dim(s) })
	cobra.AddTemplateFunc("bold", func(s string) string { return bold(s) })

	rootCmd.PersistentFlags().BoolVarP(&jsonOutput, "json", "j", false, "Output in machine-readable JSON")

	rootCmd.SetUsageTemplate(usageTemplate)
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.SetVersionTemplate(`{{with .Name}}{{bold .}} {{end}}version {{accent .Version}}
`)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

const usageTemplate = `{{heading "Usage:"}}
  {{accent .UseLine}}{{if .HasAvailableSubCommands}}
  {{accent (printf "%s [command]" .CommandPath)}}{{end}}{{if gt (len .Aliases) 0}}

{{heading "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{heading "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{heading "Commands:"}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{accent (rpad .Name .NamePadding)}}  {{.Short}}{{end}}{{end}}{{else}}{{range .Groups}}

{{heading .Title}}{{range $cmds}}{{if (and (eq .GroupID .GroupID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{accent (rpad .Name .NamePadding)}}  {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{heading "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{accent (rpad .Name .NamePadding)}}  {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{heading "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{heading "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{heading "Topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{accent (rpad .Name .NamePadding)}}  {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{accent (printf "%s [command] --help" .CommandPath)}}" for more information about a command.{{end}}
`
