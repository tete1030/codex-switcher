package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"codex-switcher/internal/app"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	svc := app.NewService()

	root := &cobra.Command{
		Use:          "codex-switcher",
		Short:        "Switch OpenAI Codex OAuth profiles across tools",
		SilenceUsage: true,
	}

	root.AddCommand(newInspectCommand(svc))
	root.AddCommand(newStatusCommand(svc))
	root.AddCommand(newCaptureCommand(svc))
	root.AddCommand(newSwitchCommand(svc))
	root.AddCommand(newUsageCommand(svc))
	root.AddCommand(newProfilesCommand(svc))

	return root
}

func newStatusCommand(svc *app.Service) *cobra.Command {
	var toolCSV string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show auth status, profiles, and pending-create state",
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := app.ParseTools(toolCSV)
			if err != nil {
				return app.WrapExit(app.ExitUserError, err)
			}
			results, err := svc.Status(tools)
			if err != nil {
				return app.WrapExit(app.ExitIOFailure, err)
			}
			if jsonOut {
				return printJSON(results)
			}
			for _, item := range results {
				fmt.Printf("%s\n", item.Tool)
				fmt.Printf("  active: %v\n", item.HasActive)
				fmt.Printf("  active_profile: %s\n", zeroDefault(item.ActiveProfile, "-"))
				fmt.Printf("  previous_profile: %s\n", zeroDefault(item.PreviousProfile, "-"))
				fmt.Printf("  pending_create: %s\n", zeroDefault(item.PendingCreateProfile, "-"))
				if item.PendingCreateSince != "" {
					fmt.Printf("  pending_since: %s\n", item.PendingCreateSince)
				}
				fmt.Printf("  profiles: %d\n", item.ProfileCount)
				fmt.Printf("  profile_names: %s\n", zeroDefault(strings.Join(item.Profiles, ","), "-"))
				if item.StoreMode != "" {
					fmt.Printf("  store_mode: %s\n", item.StoreMode)
				}
				if item.SwitchBlocked {
					fmt.Printf("  switch_blocked: true (%s)\n", item.SwitchBlockReason)
				}
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&toolCSV, "tools", "", "Comma-separated tools: codex,opencode,openclaw")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}

func newInspectCommand(svc *app.Service) *cobra.Command {
	var toolCSV string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect auth status and paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := app.ParseTools(toolCSV)
			if err != nil {
				return app.WrapExit(app.ExitUserError, err)
			}
			results, err := svc.Inspect(tools)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(results)
			}
			for _, item := range results {
				fmt.Printf("%s\n", item.Tool)
				fmt.Printf("  active: %v\n", item.HasActive)
				fmt.Printf("  capturable: %v\n", item.Capturable)
				if item.StoreMode != "" {
					fmt.Printf("  store_mode: %s\n", item.StoreMode)
				}
				if item.SwitchBlocked {
					fmt.Printf("  switch_blocked: true (%s)\n", item.SwitchBlockReason)
				}
				if item.AccountID != "" {
					fmt.Printf("  account_id: %s\n", item.AccountID)
				}
				fmt.Printf("  active_file: %s\n", item.Paths.ActivePath)
				fmt.Printf("  profiles_dir: %s\n", item.Paths.ProfileDir)
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&toolCSV, "tools", "", "Comma-separated tools: codex,opencode,openclaw")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}

func newCaptureCommand(svc *app.Service) *cobra.Command {
	var toolCSV string
	var force bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "capture <profile>",
		Short: "Capture current active credentials into a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := app.ParseTools(toolCSV)
			if err != nil {
				return app.WrapExit(app.ExitUserError, err)
			}
			results, err := svc.Capture(strings.TrimSpace(args[0]), tools, force)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(results)
			}
			for _, item := range results {
				status := "captured"
				if !item.HasActive {
					status = "skipped (no active credential)"
				}
				fmt.Printf("%s: %s\n", item.Tool, status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&toolCSV, "tools", "", "Comma-separated tools: codex,opencode,openclaw")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing profile file")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}

func newSwitchCommand(svc *app.Service) *cobra.Command {
	var toolCSV string
	var dryRun bool
	var createMissing bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "switch <profile>",
		Short: "Switch active credentials to a named profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := app.ParseTools(toolCSV)
			if err != nil {
				return app.WrapExit(app.ExitUserError, err)
			}
			results, err := svc.Switch(strings.TrimSpace(args[0]), tools, app.SwitchOptions{
				DryRun:        dryRun,
				CreateMissing: createMissing,
			})
			if err != nil {
				return err
			}
			partial := false
			for _, item := range results {
				if item.Status == "blocked" || item.Status == "skipped_missing" {
					partial = true
				}
			}
			if jsonOut {
				if err := printJSON(results); err != nil {
					return err
				}
				if partial {
					return app.WrapExit(app.ExitPartial, fmt.Errorf("switch completed with warnings"))
				}
				return nil
			}
			for _, item := range results {
				mode := item.Status
				if dryRun {
					mode = mode + ", dry-run"
				}
				switch item.Status {
				case "blocked", "skipped_missing":
					partial = true
					fmt.Printf("%s: %s (%s)\n", item.Tool, item.Status, item.Warning)
				case "prepared":
					fmt.Printf("%s: %s -> %s (%s, pending-create=true, snapshot=%s)\n", item.Tool, zeroDefault(item.FromProfile, "-"), item.ToProfile, mode, zeroDefault(item.SnapshotProfile, "-"))
				default:
					fmt.Printf("%s: %s -> %s (%s, snapshot=%s)\n", item.Tool, zeroDefault(item.FromProfile, "-"), item.ToProfile, mode, zeroDefault(item.SnapshotProfile, "-"))
				}
			}
			if partial {
				return app.WrapExit(app.ExitPartial, fmt.Errorf("switch completed with warnings"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&toolCSV, "tools", "", "Comma-separated tools: codex,opencode,openclaw")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show switch plan without writing files")
	cmd.Flags().BoolVar(&createMissing, "create", false, "Prepare missing profile by clearing active auth and marking pending-create")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}

func newUsageCommand(svc *app.Service) *cobra.Command {
	var profile string
	var allProfiles bool
	var toolsCSV string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Fetch usage for one or more profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			selectedTools := []app.ToolName{}
			if strings.TrimSpace(toolsCSV) != "" {
				parsed, err := app.ParseTools(toolsCSV)
				if err != nil {
					return app.WrapExit(app.ExitUserError, err)
				}
				selectedTools = parsed
			}

			results, err := svc.Usage(app.UsageOptions{
				Profile:     strings.TrimSpace(profile),
				AllProfiles: allProfiles,
				Tools:       selectedTools,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(results)
			}
			for _, item := range results {
				label := fmt.Sprintf("%s/%s", item.Tool, item.Profile)
				if item.Status != "ok" {
					fmt.Printf("%s: N/A", label)
					if item.AccountID != "" {
						fmt.Printf(" account_id=%s", item.AccountID)
					}
					fmt.Printf(" (%s)\n", item.Error)
					continue
				}
				fmt.Printf("%s: plan=%s", label, zeroDefault(item.Plan, "unknown"))
				if item.AccountID != "" {
					fmt.Printf(" account_id=%s", item.AccountID)
				}
				if item.CreditsBalance != nil {
					fmt.Printf(" credits=$%.2f", *item.CreditsBalance)
				}
				if item.Refreshed {
					fmt.Printf(" refreshed=true")
				}
				fmt.Println()
				for _, w := range item.Windows {
					fmt.Printf("  - %s: %.1f%%", w.Label, w.UsedPercent)
					if w.ResetAt > 0 {
						fmt.Printf(" reset=%s", formatResetForDisplay(w.ResetAtISO, w.ResetAt))
						fmt.Printf(" remaining=%.2fd", w.RemainingDays)
					}
					fmt.Println()
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Specific profile name")
	cmd.Flags().BoolVar(&allProfiles, "all-profiles", false, "Query all profiles (for selected tool(s), or all tools if none selected)")
	cmd.Flags().StringVar(&toolsCSV, "tools", "", "Comma-separated tools: codex,opencode,openclaw")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}

func newProfilesCommand(svc *app.Service) *cobra.Command {
	profiles := &cobra.Command{
		Use:   "profiles",
		Short: "Manage profile files",
	}
	profiles.AddCommand(newProfilesListCommand(svc))
	profiles.AddCommand(newProfilesDeleteCommand(svc))
	return profiles
}

func newProfilesListCommand(svc *app.Service) *cobra.Command {
	var tool string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List profiles for a tool",
		RunE: func(cmd *cobra.Command, args []string) error {
			target := app.ToolName(strings.ToLower(strings.TrimSpace(tool)))
			if target == "" {
				target = app.ToolCodex
			}
			if target != app.ToolCodex && target != app.ToolOpenCode && target != app.ToolOpenClaw {
				return app.WrapExit(app.ExitUserError, fmt.Errorf("invalid --tool %q", tool))
			}
			profiles, err := svc.ListProfiles(target)
			if err != nil {
				return app.WrapExit(app.ExitIOFailure, err)
			}
			if jsonOut {
				return printJSON(map[string]any{"tool": target, "profiles": profiles})
			}
			for _, name := range profiles {
				fmt.Println(name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&tool, "tool", "codex", "Target tool")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}

func newProfilesDeleteCommand(svc *app.Service) *cobra.Command {
	var toolCSV string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "delete <profile>",
		Aliases: []string{"remove", "rm"},
		Short:   "Delete profile files",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := app.ParseTools(toolCSV)
			if err != nil {
				return app.WrapExit(app.ExitUserError, err)
			}
			name := strings.TrimSpace(args[0])
			if err := svc.DeleteProfile(name, tools); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(map[string]any{"deleted": name, "tools": tools})
			}
			fmt.Printf("deleted profile %q for tools: %s\n", name, strings.Join(toStrings(tools), ","))
			return nil
		},
	}
	cmd.Flags().StringVar(&toolCSV, "tools", "", "Comma-separated tools: codex,opencode,openclaw")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func zeroDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func toStrings(tools []app.ToolName) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, string(tool))
	}
	return out
}

func formatResetForDisplay(resetISO string, resetMillis int64) string {
	if strings.TrimSpace(resetISO) != "" {
		return resetISO
	}
	if resetMillis <= 0 {
		return "-"
	}
	return time.UnixMilli(resetMillis).UTC().Format(time.RFC3339)
}
